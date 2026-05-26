package ext4

import (
	"fmt"
)

// ReadSymlink returns the target path string of a symbolic link inode.
func (fs *FileSystem) ReadSymlink(inode *Inode) (string, error) {
	// Guard: caller must pass a symlink inode. Calling this on a regular file
	// or directory would read and return directory/file data as a path string,
	// producing complete nonsense.
	if !inode.IsSymlink() {
		return "", fmt.Errorf(
			"ReadSymlink: inode has I_mode=0x%04x - expected S_IFLNK (0xA000)",
			inode.I_mode,
		)
	}

	size := inode.Size()
	if size == 0 {
		// A zero-length symlink target is degenerate but not outright corrupt.
		// Return an empty string and let the caller decide.
		return "", nil
	}

	// Fast symlink path (target <= 60 bytes, stored inline in I_block).
	if !inode.UsesExtents() && size <= 60 {
		// I_block is a [60]byte. Slice it to the exact target length and
		// convert to a Go string. No null terminator is present on disk.
		return string(inode.I_block[:size]), nil
	}

	// Slow symlink path (stored in external data blocks).
	extents, err := fs.ReadExtents(inode)
	if err != nil {
		return "", fmt.Errorf("ReadSymlink: failed to walk extent tree: %w", err)
	}

	// Allocate a buffer exactly the size of the target string.
	buf := make([]byte, size)
	written := uint64(0)

	for _, ext := range extents {
		if written >= size {
			break
		}

		// Uninitialized extents inside a symlink are degenerate but we handle
		// them gracefully by treating them as zero bytes (which produce an
		// invalid path and will fail at resolution time).
		if ext.EE_len&0x8000 != 0 {
			blockCount := uint64(ext.EE_len & 0x7FFF)
			written += blockCount * fs.BlockSize
			continue
		}

		physStart := (uint64(ext.EE_start_hi) << 32) | uint64(ext.EE_start_lo)
		blockCount := uint64(ext.EE_len)

		for i := uint64(0); i < blockCount; i++ {
			if written >= size {
				break
			}
			physBlock := physStart + i
			blockBuf := make([]byte, fs.BlockSize)
			if _, err := fs.dev.ReadAt(blockBuf, int64(physBlock*fs.BlockSize)); err != nil {
				return "", fmt.Errorf("ReadSymlink: failed to read block %d: %w", physBlock, err)
			}
			remaining := size - written
			toCopy := fs.BlockSize
			if toCopy > remaining {
				toCopy = remaining
			}
			copy(buf[written:written+toCopy], blockBuf[:toCopy])
			written += toCopy
		}
	}

	return string(buf), nil
}
