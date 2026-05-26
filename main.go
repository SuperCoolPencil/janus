package main

// main.go is the entry point for the janus CLI. It dispatches between four
// operating modes based on command-line arguments:
//
//	janus                              — dev mode: open testfs.img directly
//	janus <device> [<partNum>]         — list partitions, or read one partition's root dir
//	janus devices                      — list all physical disks (Windows: \\.\PhysicalDriveN)
//	janus mount <letter> <disk> <part> — mount partition as a drive letter via WinFsp
//
// The dev / list / read modes are carryovers from the initial development phase
// and remain useful for debugging. The mount mode is the production use case.

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/supercoolpencil/janus/disk"
	"github.com/supercoolpencil/janus/ext4"
	"github.com/supercoolpencil/janus/mount"
	"github.com/winfsp/cgofuse/fuse"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

// run is the top-level dispatcher. It reads os.Args and calls the appropriate
// mode function. Using a separate run() function (rather than putting
// everything in main) allows us to return errors cleanly without calling
// os.Exit directly, keeping defer statements effective.
func run() error {
	// Dispatch on the first argument (or lack thereof).
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "devices":
			// List all available physical disks on this machine.
			// On Windows: probes \\.\PhysicalDrive0 … PhysicalDrive15.
			// On Linux: reads /sys/block/.
			return runDevices()

		case "mount":
			// Mount an ext4 partition as a drive letter via WinFsp / cgofuse.
			// Usage: janus mount <letter> <diskNum> <partNum>
			// Example: janus mount G: 0 1
			return runMount()
		}
	}

	// Legacy modes (retained for development and debugging).
	switch len(os.Args) {
	case 1:
		// No arguments: open testfs.img directly as a raw ext4 image.
		return runImageMode()
	case 2:
		// One argument: treat it as a device path and list its partitions.
		return runListPartitions(os.Args[1])
	case 3:
		// Two arguments: device path + partition number → read that partition.
		n, err := strconv.Atoi(os.Args[2])
		if err != nil {
			return fmt.Errorf("partition number must be an integer, got %q", os.Args[2])
		}
		return runReadPartition(os.Args[1], n)
	default:
		return fmt.Errorf(
			"usage:\n" +
				"  janus                                  open testfs.img (dev mode)\n" +
				"  janus devices                          list physical disks\n" +
				"  janus <device>                         list partitions on device\n" +
				"  janus <device> <partNum>               read root dir of partition\n" +
				"  janus mount <letter> <disk> <partNum>  mount partition as drive letter\n",
		)
	}
}

// Mode: devices

// runDevices lists all physical disks, probes their partition tables, and prints a summary.
func runDevices() error {
	disks, err := disk.EnumerateDisks()
	if err != nil {
		return fmt.Errorf("failed to enumerate disks: %w", err)
	}

	if len(disks) == 0 {
		fmt.Println("No physical disks found.")
		fmt.Println("On Windows, ensure you are running as Administrator.")
		return nil
	}

	fmt.Printf("Found %d disk(s):\n\n", len(disks))

	for di, diskPath := range disks {
		fmt.Printf("Disk %d: %s\n", di, diskPath)

		f, err := os.Open(diskPath)
		if err != nil {
			fmt.Printf("  (could not open: %v)\n\n", err)
			continue
		}

		scheme, partitions, err := disk.ProbePartitions(f)
		f.Close()
		if err != nil {
			fmt.Printf("  (could not read partition table: %v)\n\n", err)
			continue
		}

		fmt.Printf("  Partition table: %s\n", scheme)
		fmt.Printf("  %-4s  %-24s  %-18s  %s\n", "#", "Name", "Type", "Size")
		fmt.Printf("  %-4s  %-24s  %-18s  %s\n",
			"----", "------------------------", "------------------", "--------")

		for _, p := range partitions {
			sizeMiB := p.ByteSize() / (1024 * 1024)
			fmt.Printf("  %-4d  %-24s  %-18s  %d MiB\n",
				p.Number, truncate(p.Name, 24), p.Type, sizeMiB)
		}
		fmt.Println()
	}

	fmt.Println("To mount a partition:")
	fmt.Println("  janus mount <letter> <diskNum> <partNum>")
	fmt.Println("  Example: janus mount G: 0 1")
	return nil
}

// Mode: mount

// runMount mounts an ext4 partition as a drive letter using cgofuse and WinFsp.
func runMount() error {
	// Validate argument count: we need exactly 3 args after "mount".
	// os.Args = ["janus", "mount", letter, diskNum, partNum]
	if len(os.Args) != 5 {
		return fmt.Errorf(
			"usage: janus mount <letter> <diskNum> <partNum>\n" +
				"  letter   drive letter, e.g. G: or G\n" +
				"  diskNum  disk index from `janus devices` (0-based)\n" +
				"  partNum  partition number from `janus devices` (1-based)\n",
		)
	}

	// Parse arguments.

	mountPoint := os.Args[2]
	// Normalise "G" → "G:" so WinFsp gets a valid drive-letter mount point.
	// On Linux, mountPoint is a directory path and this is a no-op.
	if len(mountPoint) == 1 && mountPoint[0] >= 'A' && mountPoint[0] <= 'Z' ||
		len(mountPoint) == 1 && mountPoint[0] >= 'a' && mountPoint[0] <= 'z' {
		mountPoint = strings.ToUpper(mountPoint) + ":"
	}

	diskNum, err := strconv.Atoi(os.Args[3])
	if err != nil {
		return fmt.Errorf("diskNum must be an integer, got %q", os.Args[3])
	}

	partNum, err := strconv.Atoi(os.Args[4])
	if err != nil {
		return fmt.Errorf("partNum must be an integer, got %q", os.Args[4])
	}

	// Open the physical disk.
	diskPath := fmt.Sprintf(`\\.\PhysicalDrive%d`, diskNum)
	f, err := os.Open(diskPath)
	if err != nil {
		return fmt.Errorf(
			"failed to open disk %q: %w\n"+
				"Ensure janus is running as Administrator.",
			diskPath, err,
		)
	}
	defer f.Close()

	fmt.Printf("Opened disk: %s\n", diskPath)

	// Probe the partition table.
	scheme, partitions, err := disk.ProbePartitions(f)
	if err != nil {
		return fmt.Errorf("failed to read partition table on %q: %w", diskPath, err)
	}
	fmt.Printf("Partition table: %s\n", scheme)

	// Locate the target partition.

	var target *disk.Partition
	for i := range partitions {
		if partitions[i].Number == partNum {
			target = &partitions[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf(
			"partition %d not found on %s (run `janus devices` to see available partitions)",
			partNum, diskPath,
		)
	}

	if target.Type != disk.TypeLinuxData {
		return fmt.Errorf(
			"partition %d is %q — janus only supports Linux ext4 partitions",
			partNum, target.Type,
		)
	}

	fmt.Printf("Partition %d: %s, offset %d bytes, size %d MiB\n",
		target.Number, target.Name,
		target.StartOffset(), target.ByteSize()/(1024*1024),
	)

	// Set up the ext4 filesystem.
	pr := disk.NewPartitionReader(f, target)

	fs, err := ext4.NewFileSystem(pr)
	if err != nil {
		return fmt.Errorf("failed to initialise ext4 filesystem: %w", err)
	}

	// Read ext4 superblock.
	sb, err := fs.ReadSuperBlock()
	if err != nil {
		return fmt.Errorf("failed to read ext4 superblock: %w", err)
	}

	volName := string(bytes.Trim(sb.S_volume_name[:], "\x00"))
	if volName == "" {
		// Use a plain fallback — angle brackets like <unnamed> are treated as
		// argument delimiters by WinFsp's option parser and will break the
		// volname= option, causing the drive letter to silently not appear.
		volName = "ext4"
	}
	// Strip any characters that WinFsp's FUSE option parser cannot handle
	// inside an option value: angle brackets, quotes, spaces, commas.
	volName = sanitizeVolName(volName)
	fmt.Printf("ext4 volume: %q (sanitized), block size: %d bytes\n", volName, fs.BlockSize)

	// Only mount clean filesystems. Journal replay is not yet supported.
	if sb.S_state != ext4.SUPERBLOCK_STATE_CLEAN {
		return fmt.Errorf(
			"filesystem is not clean (state=0x%04x) — unmount it cleanly on Linux before mounting with janus",
			sb.S_state,
		)
	}

	// Read block group descriptors.
	if err := fs.ReadGroupDescriptors(); err != nil {
		return fmt.Errorf("failed to read block group descriptors: %w", err)
	}

	// Create and start the FUSE filesystem.
	janusFS := mount.NewJanusFS(fs)
	host := fuse.NewFileSystemHost(janusFS)

	// Mount options: volname sets the drive label in Explorer.
	mountArgs := []string{
		"-o", fmt.Sprintf("volname=%s", volName),
	}
	fmt.Printf("Mount args: %v\n", mountArgs)

	fmt.Printf("\nMounting at %s … (press Ctrl+C or right-click → Eject to unmount)\n\n", mountPoint)

	// host.Mount blocks until the filesystem is unmounted. This is by design —
	// janus must remain alive to service VFS requests from the kernel.
	ok := host.Mount(mountPoint, mountArgs)
	if !ok {
		return fmt.Errorf("mount failed — ensure WinFsp is installed (https://winfsp.dev) and janus is running as Administrator")
	}

	fmt.Println("Unmounted successfully.")
	return nil
}

// Mode: raw image

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

// Mode: list partitions

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

// Mode: read partition

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

// Shared ext4 reading logic

// readExt4 is the common path shared by the dev and partition-read modes. It
// takes any io.ReaderAt and runs the full ext4 read sequence:
// superblock → group descriptors → root inode → root directory listing.
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
	if sb.S_state != ext4.SUPERBLOCK_STATE_CLEAN {
		fmt.Printf("Warning: filesystem is not clean (state=0x%04x) — aborting\n", sb.S_state)
		return nil
	}

	// Read all block group descriptors.
	if err := fs.ReadGroupDescriptors(); err != nil {
		return fmt.Errorf("failed to read group descriptors: %w", err)
	}

	// Read inode 2, which is always the root directory in ext4.
	rootInode, err := fs.ReadRootInode()
	if err != nil {
		return fmt.Errorf("failed to read root inode: %w", err)
	}

	// Walk the extent tree and parse DirEntry2 records.
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

// sanitizeVolName replaces characters that WinFsp's FUSE option parser cannot
// handle inside a volname= value. The problematic characters are:
//
//	< >  — treated as XML/argument delimiters
//	"    — closes the quoted string in WinFsp's option tokenizer
//	,    — separates comma-delimited option values
//	' '  — separates option tokens
//
// We replace any of these with an underscore so the volname= option is always
// a single, unambiguous token that WinFsp can parse safely.
func sanitizeVolName(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		switch r {
		case '<', '>', '"', ',', ' ', '\t', '\n', '\r':
			runes[i] = '_'
		}
	}
	result := string(runes)
	if result == "" {
		return "ext4"
	}
	return result
}
