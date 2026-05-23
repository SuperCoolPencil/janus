package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/supercoolpencil/janus/ext4"
)

func main() {
	// 1. Open the virtual ext4 disk image
	// We use os.O_RDONLY because a read-only driver is our first major milestone
	file, err := os.OpenFile("testfs.img", os.O_RDONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open testfs.img: %v\n(Did you create it and unmount it in Linux?)", err)
	}
	defer file.Close()

	// 2. Initialize our FileSystem state object
	// (Assuming you have NewFileSystem defined in your ext4 package)
	fs, err := ext4.NewFileSystem(file)
	if err != nil {
		log.Fatalf("Failed to initialize filesystem: %v", err)
	}

	// 3. Read and decode the Superblock
	sb, err := fs.ReadSuperBlock()
	if err != nil {
		log.Fatalf("Failed to mount filesystem: %v", err)
	}

	// 4. Clean up C-style strings for Go printing
	// Volume name is 16 bytes padded with null characters (\x00)
	volName := string(bytes.Trim(sb.S_volume_name[:], "\x00"))
	if volName == "" {
		volName = "<unnamed>"
	}

	// 5. Print the "Geometry" of the filesystem
	fmt.Println("========================================")
	fmt.Println("         EXT4 MOUNT SUCCESSFUL          ")
	fmt.Println("========================================")
	fmt.Printf("Volume Name       : %s\n", volName)
	fmt.Printf("Magic Signature   : 0x%X\n", sb.S_magic)
	fmt.Println("------------------------------------------")
	fmt.Printf("Mount State       : 0x%X\n", sb.S_state)
	fmt.Printf("Error Behavior    : 0x%X\n", sb.S_errors)
	fmt.Printf("Creator OS        : 0x%X\n", sb.S_creator_os)
	fmt.Printf("Revision Level    : 0x%X\n", sb.S_rev_level)
	fmt.Println("----------------------------------------")
	fmt.Printf("Block Size        : %d bytes\n", sb.BlockSize())
	fmt.Printf("Inode Size        : %d bytes\n", sb.InodeSize())
	fmt.Printf("Total Blocks      : %d\n", sb.S_blocks_count_lo)
	fmt.Printf("Total Inodes      : %d\n", sb.S_inodes_count)
	fmt.Printf("Blocks Per Group  : %d\n", sb.S_blocks_per_group)
	fmt.Printf("Inodes Per Group  : %d\n", sb.S_inodes_per_group)
	fmt.Println("----------------------------------------")
	fmt.Printf("Block Group Count : %d\n", sb.BlockGroupCount())
	fmt.Println("========================================")
}
