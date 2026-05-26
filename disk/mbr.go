package disk

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// MBR - Master Boot Record
//
// The MBR occupies the first 512 bytes of the disk and has three parts:
//
//	Offset 0x000 (446 bytes): Bootstrap code (bootloader stage 1)
//	Offset 0x1BE (64 bytes):  Partition table - exactly 4 entries × 16 bytes
//	Offset 0x1FE (2 bytes):   Boot signature - must be 0x55 0xAA
//
// MBR limitations:
//   - At most 4 primary partitions. A workaround exists ("Extended" partitions,
//     type 0x05/0x0F) that acts as a container for a chain of "logical"
//     partitions, but parsing that chain is complex and rarely needed for
//     a Linux reader. We skip logical partitions for now.
//   - 32-bit LBA: maximum addressable disk size is 2 TiB (2^32 × 512 bytes).
//
// On a GPT disk, LBA 0 contains a "Protective MBR" with one type-0xEE entry
// spanning the full disk. parseGPT is always tried first; parseMBR will
// return an error if it encounters a protective MBR (0xEE entry with no
// real Linux partitions), preventing a false positive.
//
// Reference: https://en.wikipedia.org/wiki/Master_boot_record

// mbrOffset is the byte offset of the MBR partition table within the disk.
// The four 16-byte entries start here.
const mbrTableOffset = 0x1BE

// mbrSignatureOffset is where the 2-byte boot signature lives.
const mbrSignatureOffset = 0x1FE

// mbrSignature is the magic value that must appear at offset 0x1FE.
var mbrSignature = [2]byte{0x55, 0xAA}

// mbrEntry is the on-disk layout of one MBR partition table entry (16 bytes).
// All multi-byte fields are little-endian.
//
// Note: The three CHS (Cylinder-Head-Sector) fields are obsolete on any disk
// larger than ~8 GiB and are ignored here. LBA addressing is always used.
type mbrEntry struct {
	// 0x00: Status byte. 0x80 = active/bootable, 0x00 = inactive.
	//       Any other value is technically invalid but we accept it.
	Status uint8
	// 0x01: CHS address of the first sector (3 bytes, obsolete - ignored).
	CHSFirst [3]byte
	// 0x04: Partition type code. See mbrType* constants below.
	Type uint8
	// 0x05: CHS address of the last sector (3 bytes, obsolete - ignored).
	CHSLast [3]byte
	// 0x08: LBA of the first sector of this partition.
	StartLBA uint32
	// 0x0C: Total number of sectors in this partition.
	//       EndLBA (inclusive) = StartLBA + SectorCount - 1
	SectorCount uint32
}

// MBR partition type codes (the single byte at offset 0x04 in each entry).
// This is a small subset of the full list. A complete reference:
// https://en.wikipedia.org/wiki/Partition_type
const (
	mbrTypeEmpty         = 0x00 // Unused entry
	mbrTypeLinux         = 0x83 // Linux native filesystem (ext2/3/4, xfs, etc.)
	mbrTypeLinuxSwap     = 0x82 // Linux swap
	mbrTypeLinuxLVM      = 0x8E // Linux LVM physical volume
	mbrTypeEFI           = 0xEF // EFI System Partition
	mbrTypeGPTProtective = 0xEE // GPT Protective MBR entry - disk is actually GPT
	mbrTypeExtended      = 0x05 // Extended partition (CHS addressing, not supported)
	mbrTypeExtendedLBA   = 0x0F // Extended partition (LBA addressing, not supported)
	mbrTypeFAT32LBA      = 0x0C // FAT32 with LBA addressing (often Windows)
	mbrTypeNTFS          = 0x07 // NTFS / exFAT / HPFS
)

// parseMBR reads the 4-entry primary partition table from the first 512 bytes
// of disk and returns all non-empty, non-extended entries.
//
// Returns an error if:
//   - The 0x55AA boot signature is absent (not an MBR disk).
//   - The disk appears to be GPT (a type-0xEE protective entry is found and
//     no real Linux partitions are present alongside it).
func parseMBR(disk io.ReaderAt) ([]Partition, error) {
	// Read the full first sector (512 bytes).
	sector := make([]byte, 512)
	if _, err := disk.ReadAt(sector, 0); err != nil {
		return nil, fmt.Errorf("mbr: failed to read first sector: %w", err)
	}

	// Validate the boot signature. Its absence means this is not an MBR disk.
	sig := [2]byte{sector[mbrSignatureOffset], sector[mbrSignatureOffset+1]}
	if sig != mbrSignature {
		return nil, fmt.Errorf("mbr: invalid boot signature 0x%02X%02X (expected 0x55AA)", sig[0], sig[1])
	}

	// Parse the four 16-byte partition table entries.
	var partitions []Partition
	hasGPTProtective := false

	for i := range 4 {
		offset := mbrTableOffset + i*16
		var e mbrEntry
		if err := binary.Read(bytes.NewReader(sector[offset:offset+16]), binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("mbr: failed to decode entry %d: %w", i, err)
		}

		if e.Type == mbrTypeEmpty || e.SectorCount == 0 {
			continue // unused slot
		}

		if e.Type == mbrTypeGPTProtective {
			// A type-0xEE entry means this is a GPT disk. The caller
			// (ProbePartitions) should have found the GPT already; if we
			// reach this code, the GPT header was corrupt or missing.
			hasGPTProtective = true
			continue
		}

		if e.Type == mbrTypeExtended || e.Type == mbrTypeExtendedLBA {
			// Extended partition containers are not yet supported.
			// Logical partitions inside them are skipped silently.
			continue
		}

		endLBA := uint64(e.StartLBA) + uint64(e.SectorCount) - 1
		partitions = append(partitions, Partition{
			Number:      i + 1,
			Type:        classifyMBRType(e.Type),
			Name:        fmt.Sprintf("Partition %d", i+1),
			Scheme:      SchemeMBR,
			StartLBA:    uint64(e.StartLBA),
			EndLBA:      endLBA,
			SectorSize:  DefaultSectorSize,
			RawTypeByte: e.Type,
		})
	}

	// If the only significant content was a GPT protective entry and we found
	// no real partitions, report it clearly rather than returning an empty list.
	if len(partitions) == 0 && hasGPTProtective {
		return nil, fmt.Errorf("mbr: disk has GPT protective MBR; GPT header may be corrupt")
	}

	return partitions, nil
}

// classifyMBRType maps a 1-byte MBR type code to our PartitionType enum.
func classifyMBRType(t uint8) PartitionType {
	switch t {
	case mbrTypeLinux:
		return TypeLinuxData
	case mbrTypeLinuxSwap:
		return TypeLinuxSwap
	case mbrTypeLinuxLVM:
		return TypeLinuxLVM
	case mbrTypeEFI:
		return TypeEFISystem
	case mbrTypeFAT32LBA, mbrTypeNTFS:
		return TypeMicrosoftData
	default:
		return TypeUnknown
	}
}
