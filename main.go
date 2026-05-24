package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/supercoolpencil/janus/ext4"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	// Open the virtual ext4 disk image
	// We use os.O_RDONLY because a read-only driver is our first major milestone
	file, err := os.OpenFile("testfs.img", os.O_RDONLY, 0o666)
	if err != nil {
		return fmt.Errorf("failed to open testfs.img: %w\n(Did you create it and unmount it in Linux?)", err)
	}
	defer file.Close()

	fs, err := ext4.NewFileSystem(file)
	if err != nil {
		return fmt.Errorf("failed to initialize filesystem: %w", err)
	}

	// Read and decode the Superblock
	sb, err := fs.ReadSuperBlock()
	if err != nil {
		return fmt.Errorf("failed to mount filesystem: %w", err)
	}

	// Clean up C-style strings for Go printing
	// Volume name is 16 bytes padded with null characters (\x00)
	volName := string(bytes.Trim(sb.S_volume_name[:], "\x00"))
	if volName == "" {
		volName = "<unnamed>"
	}

	fmt.Printf("Mounted ext4 filesystem with volume name: %s\n", volName)
	fmt.Printf("Superblock Details:\n")
	fmt.Printf("  Block size: %d bytes (log_block_size: %d)\n", fs.BlockSize, sb.S_log_block_size)
	fmt.Printf("  Inodes count: %d\n", sb.S_inodes_count)
	fmt.Printf("  Blocks count (lo): %d\n", fs.GroupCount)
	fmt.Printf("  Inodes per group: %d\n", sb.S_inodes_per_group)
	fmt.Printf("  Blocks per group: %d\n", sb.S_blocks_per_group)
	fmt.Printf("  Descriptor size: %d\n", fs.DescSize)
	fmt.Printf("  Calculated Block Group Count: %d\n", sb.BlockGroupCount())

	// Only support clean filesystems for now
	if sb.S_state != ext4.SUPERBLOCK_STATE_CLEAN {
		fmt.Printf("Warning: Filesystem is not clean! State: 0x%04x\n", sb.S_state)
		return nil
	}

	// Read block group 0's descriptor
	err = fs.ReadGroupDescriptors()
	if err != nil {
		return fmt.Errorf("failed to read group descriptor: %w", err)
	}

	// Read Root Inode
	rootInode, err := fs.ReadRootInode()
	if err != nil {
		return fmt.Errorf("failed to read root inode: %w", err)
	}
	fmt.Printf("Root inode: %+v\n", rootInode)

	return nil
}
