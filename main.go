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

	fmt.Printf("Mounted ext4 filesystem with volume name: %s\n", volName)

	if sb.S_state != ext4.SUPERBLOCK_STATE_CLEAN {
		fmt.Printf("Warning: Filesystem is not clean! State: 0x%04x\n", sb.S_state)
		return
	}

	block_group_descr, err := fs.ReadGroupDescriptor(0)

	if err != nil {
		log.Fatalf("Failed to read group descriptor: %v", err)
	}

	fmt.Printf("First block group descriptor: %+v\n", block_group_descr)
}
