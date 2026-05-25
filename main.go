package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/supercoolpencil/janus/disk"
	"github.com/supercoolpencil/janus/ext4"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

// run dispatches between two modes based on the command-line arguments:
//
//	janus
//	    Open "testfs.img" directly as a raw ext4 image (development mode).
//	    Useful when working on the parser without needing a real disk.
//
//	janus <device>
//	    Open a physical disk or image, probe its partition table (GPT or MBR),
//	    and list all discovered partitions.
//	    Examples:
//	      janus /dev/sda                (Linux — needs root or disk group)
//	      janus \\.\PhysicalDrive0      (Windows — needs elevation)
//	      janus disk.img                (any image that has a partition table)
//
//	janus <device> <partition-number>
//	    Open partition N from the given device and read its root directory
//	    as an ext4 filesystem. The partition number matches the listing
//	    printed by the two-argument form above (1-based).
func run() error {
	switch len(os.Args) {
	case 1:
		return runImageMode()
	case 2:
		return runListPartitions(os.Args[1])
	case 3:
		n, err := strconv.Atoi(os.Args[2])
		if err != nil {
			return fmt.Errorf("partition number must be an integer, got %q", os.Args[2])
		}
		return runReadPartition(os.Args[1], n)
	default:
		return fmt.Errorf("usage: janus [<device> [<partition-number>]]")
	}
}

// ── Mode 1: raw image (no args) ───────────────────────────────────────────────

// runImageMode opens "testfs.img" directly as a bare ext4 filesystem image.
// This is the development mode used while building the parser — the image
// has no partition table; it IS the filesystem.
func runImageMode() error {
	file, err := os.Open("testfs.img")
	if err != nil {
		return fmt.Errorf("failed to open testfs.img: %w\n(Did you create it and unmount it in Linux?)", err)
	}
	defer file.Close()

	fmt.Println("Mode: raw image (testfs.img)")
	return readExt4(file)
}

// ── Mode 2: list partitions ───────────────────────────────────────────────────

// runListPartitions opens a physical disk or image, probes its partition table,
// and prints a summary table of all discovered partitions.
//
// On Linux, device paths look like:
//
//	/dev/sda, /dev/sdb, /dev/nvme0n1, /dev/mmcblk0
//
// On Windows (run as Administrator), device paths look like:
//
//	\\.\PhysicalDrive0, \\.\PhysicalDrive1
//
// The function only reads from the device — it never writes.
func runListPartitions(devicePath string) error {
	f, err := os.Open(devicePath)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w\n(On Linux, try running with sudo. On Windows, run as Administrator.)", devicePath, err)
	}
	defer f.Close()

	scheme, partitions, err := disk.ProbePartitions(f)
	if err != nil {
		return fmt.Errorf("failed to probe partition table on %q: %w", devicePath, err)
	}

	fmt.Printf("Device: %s\n", devicePath)
	fmt.Printf("Partition table: %s\n\n", scheme)
	fmt.Printf("  %-4s  %-24s  %-18s  %s\n", "#", "Name", "Type", "Size")
	fmt.Printf("  %-4s  %-24s  %-18s  %s\n", "----", "------------------------", "------------------", "--------")

	for _, p := range partitions {
		sizeMiB := p.ByteSize() / (1024 * 1024)
		fmt.Printf("  %-4d  %-24s  %-18s  %d MiB\n",
			p.Number, truncate(p.Name, 24), p.Type, sizeMiB,
		)
	}

	fmt.Printf("\nTo read a partition: janus %s <partition-number>\n", devicePath)
	return nil
}

// ── Mode 3: read a specific partition ─────────────────────────────────────────

// runReadPartition opens partition number `partNum` from the given device,
// validates it is a Linux filesystem partition, and reads its ext4 root
// directory listing.
//
// The partition is opened via a PartitionReader, which confines all reads to
// the byte range of that partition. The ext4 package receives an io.ReaderAt
// whose offset 0 corresponds to the partition's first byte — it has no
// knowledge of the physical disk layout or other partitions.
func runReadPartition(devicePath string, partNum int) error {
	f, err := os.Open(devicePath)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", devicePath, err)
	}
	defer f.Close()

	_, partitions, err := disk.ProbePartitions(f)
	if err != nil {
		return fmt.Errorf("failed to probe partition table: %w", err)
	}

	// Find the partition with the matching number.
	var target *disk.Partition
	for i := range partitions {
		if partitions[i].Number == partNum {
			target = &partitions[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("partition %d not found on %q (run `janus %s` to list available partitions)",
			partNum, devicePath, devicePath)
	}

	if target.Type != disk.TypeLinuxData {
		return fmt.Errorf(
			"partition %d is %q, not a Linux filesystem — janus only supports ext4",
			partNum, target.Type,
		)
	}

	fmt.Printf("Opening partition %d (%s) from %s\n", target.Number, target.Name, devicePath)
	fmt.Printf("  Offset: %d bytes, Size: %d MiB\n\n", target.StartOffset(), target.ByteSize()/(1024*1024))

	// PartitionReader translates every ReadAt(buf, off) to
	// disk.ReadAt(buf, partitionStart + off), so the ext4 parser
	// sees the partition as if it were the entire device.
	pr := disk.NewPartitionReader(f, target)
	return readExt4(pr)
}

// ── Shared ext4 reading logic ─────────────────────────────────────────────────

// readExt4 is the common path shared by all three modes. It takes any
// io.ReaderAt (a raw image file, a PartitionReader, or anything else) and
// runs the full ext4 read sequence: superblock → group descriptors →
// root inode → root directory listing.
func readExt4(dev interface {
	ReadAt([]byte, int64) (int, error)
}) error {
	fs, err := ext4.NewFileSystem(dev)
	if err != nil {
		return fmt.Errorf("failed to initialise filesystem: %w", err)
	}

	// Read and decode the Superblock.
	// The superblock is always located at byte offset 1024 from the start
	// of the filesystem (partition or image) regardless of block size.
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/super.md
	sb, err := fs.ReadSuperBlock()
	if err != nil {
		return fmt.Errorf("failed to read superblock: %w", err)
	}

	volName := string(bytes.Trim(sb.S_volume_name[:], "\x00"))
	if volName == "" {
		volName = "<unnamed>"
	}

	fmt.Printf("Mounted ext4 filesystem: %s\n", volName)
	fmt.Printf("  Block size:    %d bytes\n", fs.BlockSize)
	fmt.Printf("  Inodes:        %d\n", sb.S_inodes_count)
	fmt.Printf("  Block groups:  %d\n", sb.BlockGroupCount())
	fmt.Printf("  Descriptor sz: %d bytes\n", fs.DescSize)

	// Only support clean filesystems for now.
	// A dirty filesystem (e.g. one that was not properly unmounted) may have
	// in-flight journal transactions that we would misread as committed data.
	if sb.S_state != ext4.SUPERBLOCK_STATE_CLEAN {
		fmt.Printf("Warning: filesystem is not clean (state=0x%04x) — aborting\n", sb.S_state)
		return nil
	}

	// Read all block group descriptors.
	// These map each block group to its inode table, bitmaps, etc.
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/group_descr.md
	if err := fs.ReadGroupDescriptors(); err != nil {
		return fmt.Errorf("failed to read group descriptors: %w", err)
	}

	// Read inode 2, which is always the root directory in ext4.
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/inodes.md
	rootInode, err := fs.ReadRootInode()
	if err != nil {
		return fmt.Errorf("failed to read root inode: %w", err)
	}

	// Walk the extent tree embedded in the root inode's I_block field and
	// parse the packed DirEntry2 records in each data block.
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/directory.md
	entries, err := fs.ReadDir(rootInode)
	if err != nil {
		return fmt.Errorf("failed to read root directory: %w", err)
	}

	fmt.Printf("\nRoot directory listing (%d entries):\n", len(entries))
	fmt.Printf("  %-6s  %-8s  %s\n", "type", "inode", "name")
	fmt.Printf("  %-6s  %-8s  %s\n", "------", "--------", "----")
	for _, e := range entries {
		typeName := ext4.DirFileTypeName[e.FileType]
		fmt.Printf("  %-6s  %-8d  %s\n", typeName, e.Inode, e.Name)
	}

	return nil
}

// truncate shortens s to at most n runes, appending "…" if it was cut.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
