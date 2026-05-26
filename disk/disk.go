// Package disk provides partition-table parsing and raw-disk I/O for ext4
// filesystems stored on physical devices.
//
// The central problem it solves: Windows holds an exclusive lock on every
// mounted *volume* (e.g. \\.\D:), but allows unrestricted ReadAt access to
// the underlying *physical disk* (\\.\PhysicalDrive0) - even the boot disk,
// even while Windows is running. By opening the physical disk and locating
// the ext4 partition's byte offset ourselves, we sidestep the locking
// entirely. This is the same strategy used by TestDisk, DiskGenius, and
// every other raw-disk recovery tool.
//
// Usage:
//
//	f, _    := os.Open(`\\.\PhysicalDrive0`)      // Windows (needs elevation)
//	f, _    := os.Open("/dev/sda")                 // Linux   (needs root or disk group)
//	scheme, parts, _ := disk.ProbePartitions(f)
//	pr := disk.NewPartitionReader(f, &parts[N])
//	fs, _  := ext4.NewFileSystem(pr)               // ext4 package is unaware of partitions
package disk

import (
	"fmt"
	"io"
)

// PartitionScheme identifies which partition table format was found.
//
// GPT  (GUID Partition Table) is the modern standard, defined in the UEFI
//
//	specification. It supports 64-bit LBAs (disks up to 9.4 ZiB) and up
//	to 128 partitions by default. Every GPT disk also contains a
//	"Protective MBR" at LBA 0 to stop legacy MBR-only tools from
//	treating the disk as unpartitioned.
//	Spec: https://uefi.org/specs/UEFI/2.10/05_GUID_Partition_Table_Format.html
//
// MBR  (Master Boot Record) is the legacy format from the IBM PC era. It
//
//	lives in the first 512 bytes of the disk and supports at most four
//	primary partitions and disks up to 2 TiB (32-bit LBA).
//	Ref: https://en.wikipedia.org/wiki/Master_boot_record
type PartitionScheme int

const (
	SchemeUnknown PartitionScheme = iota
	SchemeGPT
	SchemeMBR
)

func (s PartitionScheme) String() string {
	switch s {
	case SchemeGPT:
		return "GPT"
	case SchemeMBR:
		return "MBR"
	default:
		return "Unknown"
	}
}

// PartitionType normalises the OS-specific type encoding (a 16-byte GUID in
// GPT, a 1-byte code in MBR) into a single enum so callers don't need to
// handle both representations.
type PartitionType int

const (
	TypeUnknown       PartitionType = iota
	TypeLinuxData                   // ext2/3/4, xfs, btrfs - the type we care about
	TypeLinuxSwap                   // swap space
	TypeLinuxLVM                    // LVM physical volume
	TypeEFISystem                   // EFI System Partition (ESP), usually FAT32
	TypeMicrosoftData               // NTFS or FAT32 Windows data partitions
	TypeBIOSBoot                    // BIOS boot partition (GRUB stage 2)
)

func (t PartitionType) String() string {
	switch t {
	case TypeLinuxData:
		return "Linux filesystem"
	case TypeLinuxSwap:
		return "Linux swap"
	case TypeLinuxLVM:
		return "Linux LVM"
	case TypeEFISystem:
		return "EFI System"
	case TypeMicrosoftData:
		return "Microsoft basic data"
	case TypeBIOSBoot:
		return "BIOS boot"
	default:
		return "Unknown"
	}
}

// DefaultSectorSize is the logical sector size assumed when the actual size
// cannot be queried from the device. The vast majority of disks - including
// "Advanced Format" drives operating in 512e (emulation) mode - present a
// 512-byte logical sector size even if physical sectors are 4096 bytes.
//
// True 4Kn drives (rare, mostly enterprise) use 4096-byte sectors and
// require the sector size to be queried via an ioctl:
//   - Linux:   BLKSSZGET ioctl on the block device fd
//   - Windows: IOCTL_STORAGE_QUERY_PROPERTY (StorageAccessAlignmentProperty)
const DefaultSectorSize = 512

// Partition describes one entry from a disk's partition table.
type Partition struct {
	// Number is the 1-based index of this partition in the table.
	Number int

	// Type is the normalised partition type.
	Type PartitionType

	// Name is the human-readable label. GPT stores this as a UTF-16LE
	// null-terminated string in the entry (36 UTF-16 code units max).
	// MBR has no name field; we generate "Partition N" for those.
	Name string

	// Scheme is the partition table format this entry came from.
	Scheme PartitionScheme

	// StartLBA is the first logical block address (sector) of this partition.
	// Multiply by SectorSize to get the byte offset from the disk start.
	StartLBA uint64

	// EndLBA is the last logical block address (inclusive). This matches
	// the GPT on-disk representation. The partition spans sectors
	// [StartLBA, EndLBA] inclusive, so its sector count is EndLBA-StartLBA+1.
	EndLBA uint64

	// SectorSize is the logical sector size in bytes used to convert LBAs
	// to byte offsets. See DefaultSectorSize.
	SectorSize uint64

	// RawTypeGUID is the 16-byte type GUID as it appears on disk (mixed-
	// endian). Only populated for GPT partitions; zero for MBR.
	RawTypeGUID [16]byte

	// RawTypeByte is the 1-byte MBR type code. Only populated for MBR
	// partitions; zero for GPT.
	RawTypeByte uint8
}

// StartOffset returns the byte offset from the beginning of the disk where
// this partition's first sector is located.
func (p *Partition) StartOffset() int64 {
	return int64(p.StartLBA * p.SectorSize)
}

// ByteSize returns the total size of this partition in bytes.
// (EndLBA is inclusive, hence the +1.)
func (p *Partition) ByteSize() int64 {
	return int64((p.EndLBA - p.StartLBA + 1) * p.SectorSize)
}

// PartitionReader implements io.ReaderAt over a single partition on a raw disk.
//
// It wraps an underlying io.ReaderAt (the whole physical disk) and shifts
// every read by the partition's byte offset so that offset 0 corresponds to
// the first byte of the partition - exactly what ext4.NewFileSystem expects.
//
// Reads that would extend past the partition boundary are clamped and an
// error is returned, preventing the ext4 parser from accidentally reading
// into adjacent partitions.
type PartitionReader struct {
	disk        io.ReaderAt
	startOffset int64 // byte position on disk where this partition begins
	size        int64 // total byte length of this partition
}

// NewPartitionReader wraps disk so that reads are confined to the byte range
// occupied by p.
//
// The underlying disk handle is wrapped in an AlignedReaderAt so that every
// physical read is rounded to sector boundaries.  This is required on Windows
// where raw-disk handles reject reads whose offset or length is not a multiple
// of the sector size (ERROR_INVALID_PARAMETER).
func NewPartitionReader(disk io.ReaderAt, p *Partition) *PartitionReader {
	sectorSize := int64(p.SectorSize)
	if sectorSize == 0 {
		sectorSize = DefaultSectorSize
	}

	aligned, err := NewAlignedReaderAt(disk, sectorSize)
	if err != nil {
		// SectorSize should always be a valid power of two coming from the
		// partition table parser, so this should never happen in practice.
		// Fall back to the raw handle if it does.
		return &PartitionReader{
			disk:        disk,
			startOffset: p.StartOffset(),
			size:        p.ByteSize(),
		}
	}

	return &PartitionReader{
		disk:        aligned,
		startOffset: p.StartOffset(),
		size:        p.ByteSize(),
	}
}

// ReadAt implements io.ReaderAt.
//
// off is the byte offset within the partition (relative to the partition
// start, NOT the physical disk start). We translate it to an absolute disk
// position by adding startOffset.
func (pr *PartitionReader) ReadAt(buf []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("disk: negative read offset %d", off)
	}
	if off >= pr.size {
		return 0, fmt.Errorf(
			"disk: read offset %d is beyond partition end (%d bytes)",
			off, pr.size,
		)
	}
	// Clamp the read to the partition boundary so we don't spill into the
	// next partition if the caller requests more bytes than remain.
	maxRead := pr.size - off
	if int64(len(buf)) > maxRead {
		buf = buf[:maxRead]
	}
	return pr.disk.ReadAt(buf, pr.startOffset+off)
}

// ProbePartitions reads the partition table from disk and returns all
// discovered partitions.
//
// Detection order:
//  1. GPT - look for the "EFI PART" signature at byte offset 512 (LBA 1).
//  2. MBR - fall back if GPT is absent; check for 0x55AA at offset 0x1FE.
//  3. Error - if neither format is found.
//
// This function only calls ReadAt and never writes to the device. It works
// on mounted disks and - on Windows, when running elevated - on the boot
// disk via \\.\PhysicalDriveN handles.
func ProbePartitions(disk io.ReaderAt) (PartitionScheme, []Partition, error) {
	// Try GPT first; it is the only correct choice on modern UEFI systems.
	// A GPT disk also has a Protective MBR, so we must check GPT before MBR
	// to avoid misidentifying it as a single-partition MBR disk.
	parts, err := parseGPT(disk)
	if err == nil {
		return SchemeGPT, parts, nil
	}

	// GPT absent or corrupt; try legacy MBR.
	parts, err = parseMBR(disk)
	if err == nil {
		return SchemeMBR, parts, nil
	}

	return SchemeUnknown, nil, fmt.Errorf("disk: no recognisable partition table (tried GPT and MBR)")
}
