package ext4

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Group descriptors if present are the second structure in the block group.
// group descriptor records the location of both bitmaps and the inode table
// within a block group, the only data structures with fixed locations are the
// superblock and the group descriptor table.

// See docs: github.com/SuperCoolPencil/janus/blob/master/docs/ext4/group_descr.md

// GroupDescriptor maps the on-disk group descriptor structure used by ext4.
// Field names follow the on-disk names and are annotated with their offset.
type GroupDescriptor struct {
	// 0x00: Lower 32-bits of location of block bitmap.
	BG_block_bitmap_lo uint32
	// 0x04: Lower 32-bits of location of inode bitmap.
	BG_inode_bitmap_lo uint32
	// 0x08: Lower 32-bits of location of inode table.
	BG_inode_table_lo uint32
	// 0x0C: Lower 16-bits of free block count.
	BG_free_blocks_count_lo uint16
	// 0x0E: Lower 16-bits of free inode count.
	BG_free_inodes_count_lo uint16
	// 0x10: Lower 16-bits of directory count.
	BG_used_dirs_count_lo uint16
	// 0x12: Block group flags.
	BG_flags uint16
	// 0x14: Lower 32-bits of location of snapshot exclusion bitmap.
	BG_exclude_bitmap_lo uint32
	// 0x18: Lower 16-bits of the block bitmap checksum.
	BG_block_bitmap_csum_lo uint16
	// 0x1A: Lower 16-bits of the inode bitmap checksum.
	BG_inode_bitmap_csum_lo uint16
	// 0x1C: Lower 16-bits of unused inode count.
	BG_itable_unused_lo uint16
	// 0x1E: Group descriptor checksum (crc16 or crc32c low 16 bits depending on features).
	BG_checksum uint16
	// The following fields exist only if the 64bit feature is enabled and
	// the superblock's S_desc_size > 32. They provide the upper halves for
	// 64-bit fields.
	// 0x20: Upper 32-bits of location of block bitmap.
	BG_block_bitmap_hi uint32
	// 0x24: Upper 32-bits of location of inode bitmap.
	BG_inode_bitmap_hi uint32
	// 0x28: Upper 32-bits of location of inode table.
	BG_inode_table_hi uint32
	// 0x2C: Upper 16-bits of free block count.
	BG_free_blocks_count_hi uint16
	// 0x2E: Upper 16-bits of free inode count.
	BG_free_inodes_count_hi uint16
	// 0x30: Upper 16-bits of directory count.
	BG_used_dirs_count_hi uint16
	// 0x32: Upper 16-bits of unused inode count.
	BG_itable_unused_hi uint16
	// 0x34: Upper 32-bits of location of snapshot exclusion bitmap.
	BG_exclude_bitmap_hi uint32
	// 0x38: Upper 16-bits of the block bitmap checksum.
	BG_block_bitmap_csum_hi uint16
	// 0x3A: Upper 16-bits of the inode bitmap checksum.
	BG_inode_bitmap_csum_hi uint16
	// 0x3C: Padding/reserved to 64 bytes.
	BG_reserved uint32
}

const (
	// block group flags
	BG_INODE_UNINIT = 0x0001
	BG_BLOCK_UNINIT = 0x0002
	BG_INODE_ZEROED = 0x0004
)

func (fs *FileSystem) ReadGroupDescriptor(groupNum uint32) (*GroupDescriptor, error) {

	sb := fs.sb
	if sb == nil {
		return nil, fmt.Errorf("failed to read superblock")
	}

	if groupNum >= sb.BlockGroupCount() {
		return nil, fmt.Errorf("group number %d out of range (total groups: %d)", groupNum, sb.BlockGroupCount())
	}

	descSize := sb.GroupDescriptorSize()

	// The Group Descriptor Table starts in the block following the superblock.
	// - If block size is 1024 bytes, the superblock is in block 1 (offset 1024),
	//   and the GDT starts in block 2 (offset 2048).
	// - If block size is larger (e.g. 4096), the superblock is in block 0 (offset 1024),
	//   and the GDT starts in block 1 (offset block_size).
	var descTableStart uint64
	if sb.BlockSize() == 1024 {
		descTableStart = 2048
	} else {
		descTableStart = sb.BlockSize()
	}
	descOffset := descTableStart + uint64(groupNum)*uint64(descSize)

	buf := make([]byte, descSize)
	_, err := fs.dev.ReadAt(buf, int64(descOffset))
	if err != nil {
		return nil, fmt.Errorf("failed to read group descriptor: %v", err)
	}

	var gd GroupDescriptor
	err = binary.Read(bytes.NewReader(buf), binary.LittleEndian, &gd)
	if err != nil {
		return nil, fmt.Errorf("failed to decode group descriptor: %v", err)
	}

	return &gd, nil
}

func (fs *FileSystem) ReadGroupDescriptors() error {

	sb := fs.sb
	if sb == nil {
		return fmt.Errorf("superblock not read yet!")
	}

	for i := range sb.BlockGroupCount() {
		gd, err := fs.ReadGroupDescriptor(i)
		if err != nil {
			return err
		}
		fs.Bgds = append(fs.Bgds, *gd)
	}

	return nil
}
