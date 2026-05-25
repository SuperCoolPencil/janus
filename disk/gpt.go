package disk

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"
)

// GPT — GUID Partition Table
//
// A GPT-partitioned disk has the following layout (each LBA is one sector,
// typically 512 bytes):
//
//	LBA 0:      Protective MBR  — a fake MBR with one type-0xEE entry
//	                              spanning the whole disk. Prevents old
//	                              MBR-only tools from treating the disk
//	                              as unpartitioned and overwriting it.
//	LBA 1:      Primary GPT Header
//	LBA 2-33:   Partition Entry Array (up to 128 entries × 128 bytes)
//	LBA 34+:    Usable partition space
//	LBA N-33:   Backup Partition Entry Array (mirror of LBA 2-33)
//	LBA N:      Backup GPT Header (mirror of LBA 1)
//
// We read only the primary copies (LBA 1 and LBA 2+). Recovery from a
// damaged primary using the backup copies is out of scope for a reader.
//
// UEFI Specification §5.3:
// https://uefi.org/specs/UEFI/2.10/05_GUID_Partition_Table_Format.html

// gptHeaderOffset is the byte offset of the primary GPT header on disk.
// It is located at LBA 1, i.e. immediately after the 512-byte Protective MBR.
const gptHeaderOffset = 512

// gptSignature is the 8-byte magic that must appear at the start of every
// valid GPT header: the ASCII string "EFI PART".
var gptSignature = [8]byte{'E', 'F', 'I', ' ', 'P', 'A', 'R', 'T'}

// gptHeader is the on-disk layout of the GPT header (92 bytes of meaningful
// data, typically padded to a full 512-byte sector). All integer fields are
// stored in little-endian byte order.
//
// UEFI Spec §5.3.2, Table 5.5
type gptHeader struct {
	// 0x00: Must equal gptSignature ("EFI PART")
	Signature [8]byte
	// 0x08: Header format revision. Must be 0x00010000 (version 1.0).
	Revision uint32
	// 0x0C: Size in bytes of this header (usually 92).
	HeaderSize uint32
	// 0x10: CRC32 of this header, with this field set to zero during computation.
	HeaderCRC32 uint32
	// 0x14: Must be zero.
	Reserved uint32
	// 0x18: LBA containing this header (should be 1 for the primary).
	MyLBA uint64
	// 0x20: LBA of the backup GPT header (the last logical block on disk).
	AlternateLBA uint64
	// 0x28: First LBA that may be used by a partition.
	FirstUsableLBA uint64
	// 0x30: Last LBA that may be used by a partition.
	LastUsableLBA uint64
	// 0x38: 128-bit disk GUID, stored in mixed-endian (see GUID encoding note).
	DiskGUID [16]byte
	// 0x48: LBA of the start of the partition entry array (usually 2).
	PartitionEntryLBA uint64
	// 0x50: Total number of partition entries allocated in the array (usually 128).
	NumPartitionEntries uint32
	// 0x54: Size in bytes of each partition entry (usually 128).
	PartitionEntrySize uint32
	// 0x58: CRC32 of the partition entry array.
	PartitionArrayCRC32 uint32
}

// gptEntry is the on-disk layout of one partition table entry.
// The default size is 128 bytes; PartitionEntrySize in the header may be
// larger (never smaller) to accommodate vendor extensions.
//
// UEFI Spec §5.3.3, Table 5.6
type gptEntry struct {
	// 0x00: Partition Type GUID. All-zero means this slot is unused.
	//       See the gptTypeGUID* constants below for known types.
	TypeGUID [16]byte
	// 0x10: Unique GUID assigned to this specific partition instance.
	UniqueGUID [16]byte
	// 0x20: First LBA of this partition (inclusive).
	StartLBA uint64
	// 0x28: Last LBA of this partition (inclusive).
	EndLBA uint64
	// 0x30: Attribute flags.
	//   Bit 0: Required partition (must not be deleted).
	//   Bit 2: No auto-mount (Windows).
	//   Bits 48-63: Type-specific attributes.
	Attributes uint64
	// 0x38: UTF-16LE partition name, null-terminated, padded to 72 bytes
	//       (36 UTF-16 code units). Decoded to a Go string by decodeGPTName.
	Name [72]byte
}

// GPT Partition Type GUIDs as they appear on disk (mixed-endian byte order).
//
// GUID encoding: GPT GUIDs are NOT stored as a flat 16-byte big-endian value.
// The first three components are little-endian integers; the last two are
// stored as a big-endian byte string. For example, the Linux filesystem GUID:
//
//	String form:  0FC63DAF-8483-4772-8E79-3D69D8477DE4
//	On-disk bytes: AF 3D C6 0F | 83 84 | 72 47 | 8E 79 3D 69 D8 47 7D E4
//	               ^-le uint32-  -u16-  -u16-   ^---big-endian bytes------
//
// This mixed-endian layout is why we compare raw byte arrays rather than
// parsing them into a structured GUID type.
//
// Full list of type GUIDs: https://en.wikipedia.org/wiki/GUID_Partition_Table#Partition_type_GUIDs
var (
	// gptTypeLinuxData is the type GUID for Linux native filesystem partitions
	// (ext2, ext3, ext4, xfs, btrfs, and others).
	// String: 0FC63DAF-8483-4772-8E79-3D69D8477DE4
	gptTypeLinuxData = [16]byte{
		0xAF, 0x3D, 0xC6, 0x0F, 0x83, 0x84, 0x72, 0x47,
		0x8E, 0x79, 0x3D, 0x69, 0xD8, 0x47, 0x7D, 0xE4,
	}

	// gptTypeLinuxSwap — String: 0657FD6D-A4AB-43C4-84E5-0933C84B4F4F
	gptTypeLinuxSwap = [16]byte{
		0x6D, 0xFD, 0x57, 0x06, 0xAB, 0xA4, 0xC4, 0x43,
		0x84, 0xE5, 0x09, 0x33, 0xC8, 0x4B, 0x4F, 0x4F,
	}

	// gptTypeLinuxLVM — String: E6D6D379-F507-44C2-A23C-238F2A3DF928
	gptTypeLinuxLVM = [16]byte{
		0x79, 0xD3, 0xD6, 0xE6, 0x07, 0xF5, 0xC2, 0x44,
		0xA2, 0x3C, 0x23, 0x8F, 0x2A, 0x3D, 0xF9, 0x28,
	}

	// gptTypeEFI — String: C12A7328-F81F-11D2-BA4B-00A0C93EC93B
	gptTypeEFI = [16]byte{
		0x28, 0x73, 0x2A, 0xC1, 0x1F, 0xF8, 0xD2, 0x11,
		0xBA, 0x4B, 0x00, 0xA0, 0xC9, 0x3E, 0xC9, 0x3B,
	}

	// gptTypeMicrosoftData — String: EBD0A0A2-B9E5-4433-87C0-68B6B72699C7
	gptTypeMicrosoftData = [16]byte{
		0xA2, 0xA0, 0xD0, 0xEB, 0xE5, 0xB9, 0x33, 0x44,
		0x87, 0xC0, 0x68, 0xB6, 0xB7, 0x26, 0x99, 0xC7,
	}

	// gptTypeBIOSBoot — String: 21686148-6449-6E6F-744E-656564454649
	gptTypeBIOSBoot = [16]byte{
		0x48, 0x61, 0x68, 0x21, 0x49, 0x64, 0x6F, 0x6E,
		0x74, 0x4E, 0x65, 0x65, 0x64, 0x45, 0x46, 0x49,
	}

	// gptTypeUnused is all zeros — an entry with this GUID is an empty slot.
	gptTypeUnused [16]byte
)

// parseGPT reads the primary GPT header and partition entry array from disk
// and returns all non-empty partition entries.
//
// It returns an error (without modifying anything) if the GPT signature is
// absent, so ProbePartitions can safely fall back to MBR detection.
func parseGPT(disk io.ReaderAt) ([]Partition, error) {
	// ── Step 1: Read and validate the GPT header ──────────────────────────
	var hdr gptHeader
	hdrBuf := make([]byte, 512)
	if _, err := disk.ReadAt(hdrBuf, gptHeaderOffset); err != nil {
		return nil, fmt.Errorf("gpt: failed to read header: %w", err)
	}

	if err := binary.Read(bytes.NewReader(hdrBuf), binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("gpt: failed to decode header: %w", err)
	}

	// The signature check is the primary way we distinguish GPT from MBR.
	if hdr.Signature != gptSignature {
		return nil, fmt.Errorf("gpt: invalid signature (not a GPT disk)")
	}

	// ── Step 2: Read the partition entry array ────────────────────────────
	// The array starts at PartitionEntryLBA (byte offset = LBA × 512).
	// Each entry is PartitionEntrySize bytes (usually 128).
	arrayOffset := int64(hdr.PartitionEntryLBA) * DefaultSectorSize
	entrySize := int(hdr.PartitionEntrySize)
	totalArrayBytes := int(hdr.NumPartitionEntries) * entrySize

	arrayBuf := make([]byte, totalArrayBytes)
	if _, err := disk.ReadAt(arrayBuf, arrayOffset); err != nil {
		return nil, fmt.Errorf("gpt: failed to read partition array: %w", err)
	}

	// ── Step 3: Parse each entry ──────────────────────────────────────────
	var partitions []Partition
	for i := range int(hdr.NumPartitionEntries) {
		entryBuf := arrayBuf[i*entrySize : i*entrySize+entrySize]

		var e gptEntry
		// We only decode the first 128 bytes (the standard entry size).
		// Vendor-extended entries may be larger; extra bytes are ignored.
		if err := binary.Read(bytes.NewReader(entryBuf[:128]), binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("gpt: failed to decode entry %d: %w", i, err)
		}

		// An all-zero TypeGUID means this slot is unused. Skip it.
		if e.TypeGUID == gptTypeUnused {
			continue
		}

		partitions = append(partitions, Partition{
			Number:      i + 1,
			Type:        classifyGPTType(e.TypeGUID),
			Name:        decodeGPTName(e.Name[:]),
			Scheme:      SchemeGPT,
			StartLBA:    e.StartLBA,
			EndLBA:      e.EndLBA,
			SectorSize:  DefaultSectorSize,
			RawTypeGUID: e.TypeGUID,
		})
	}

	return partitions, nil
}

// classifyGPTType maps a raw 16-byte GUID to our PartitionType enum.
func classifyGPTType(guid [16]byte) PartitionType {
	switch guid {
	case gptTypeLinuxData:
		return TypeLinuxData
	case gptTypeLinuxSwap:
		return TypeLinuxSwap
	case gptTypeLinuxLVM:
		return TypeLinuxLVM
	case gptTypeEFI:
		return TypeEFISystem
	case gptTypeMicrosoftData:
		return TypeMicrosoftData
	case gptTypeBIOSBoot:
		return TypeBIOSBoot
	default:
		return TypeUnknown
	}
}

// decodeGPTName converts the on-disk UTF-16LE partition name to a Go string.
// The name field is 72 bytes (36 UTF-16 code units), null-terminated.
func decodeGPTName(raw []byte) string {
	// Convert the raw bytes into a slice of uint16 code units.
	u16 := make([]uint16, len(raw)/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(raw[i*2 : i*2+2])
	}
	// Trim at the first null terminator.
	for i, c := range u16 {
		if c == 0 {
			u16 = u16[:i]
			break
		}
	}
	return string(utf16.Decode(u16))
}
