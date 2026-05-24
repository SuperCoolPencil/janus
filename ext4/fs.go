package ext4

import (
	"fmt"
	"io"
)

// FileSystem represents an active, parsed ext4 filesystem.
type FileSystem struct {
	// The underlying device/file.
	dev io.ReaderAt

	// The parsed Superblock.
	sb *SuperBlock

	// A slice of all Block Group Descriptors.
	bgd []GroupDescriptor

	BlockSize  uint64
	InodeSize  uint16
	GroupCount uint32
	DescSize   uint16
}

func NewFileSystem(device io.ReaderAt) (*FileSystem, error) {
	return &FileSystem{dev: device}, nil
}

func (fs *FileSystem) Superblock() (*SuperBlock, error) {
	if fs.sb == nil {
		return nil, fmt.Errorf("superblock not read yet")
	}
	return fs.sb, nil
}
