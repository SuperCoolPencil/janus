package ext4

import (
	"fmt"
	"io"
)

type FileSystem struct {
	// The underlying device or file representing the filesystem.
	dev io.ReaderAt
	sb  *SuperBlock
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
