package ext4

import (
	"fmt"
)

// ReadFile reads the complete data contents of a regular file inode into a byte slice.
func (fs *FileSystem) ReadFile(inode *Inode) ([]byte, error) {
	// Guard: only call ReadFile on regular files. Directories are also stored
	// as "files" of DirEntry2 records internally, but callers should use
	// ReadDir for those. Symlinks short-circuit through ReadSymlink.
	if !inode.IsRegular() {
		return nil, fmt.Errorf(
			"ReadFile: inode has I_mode=0x%04x — only regular files (S_IFREG) are supported",
			inode.I_mode,
		)
	}

	// Guard: we only support the extent tree (EXT4_EXTENTS_FL). Legacy indirect
	// blocks are not yet implemented. See the doc comment above for context.
	if !inode.UsesExtents() {
		return nil, fmt.Errorf(
			"ReadFile: inode uses legacy indirect block addressing (EXT4_EXTENTS_FL not set, I_flags=0x%08x); "+
				"indirect block scheme is not yet supported",
			inode.I_flags,
		)
	}

	// Walk the extent tree to get the ordered list of physical block runs.
	extents, err := fs.ReadExtents(inode)
	if err != nil {
		return nil, fmt.Errorf("ReadFile: failed to walk extent tree: %w", err)
	}

	// fileSize is the authoritative byte count for this file. We use it to:
	//   (a) pre-allocate the output buffer to the right size
	//   (b) trim the final slice in case the last block is partially filled
	fileSize := inode.Size()
	if fileSize == 0 {
		return []byte{}, nil
	}

	// Pre-allocate the output buffer. For large files (hundreds of MiB) this
	// is one large allocation rather than many small append growths. We fill
	// it sequentially by copying each block's data into the appropriate window.
	out := make([]byte, fileSize)
	written := uint64(0) // tracks how many bytes have been filled so far

	for _, ext := range extents {
		if written >= fileSize {
			// We have already filled all bytes the inode claims to have.
			// This is unexpected (extent tree lists more data than inode.Size)
			// but harmless — stop processing.
			break
		}

		// Check for uninitialized extent flag (high bit of EE_len).
		uninitialized := ext.EE_len&0x8000 != 0
		blockCount := uint64(ext.EE_len & 0x7FFF)

		if uninitialized {
			// Zero-fill the region. The output buffer is already zeroed
			// by make(), so we just advance the write cursor.
			zeroes := blockCount * fs.BlockSize
			if written+zeroes > fileSize {
				zeroes = fileSize - written
			}
			written += zeroes
			continue
		}

		// Reconstruct the 48-bit physical start block.
		physStart := (uint64(ext.EE_start_hi) << 32) | uint64(ext.EE_start_lo)

		// Iterate over each block in this contiguous run.
		for i := uint64(0); i < blockCount; i++ {
			if written >= fileSize {
				break
			}

			physBlock := physStart + i
			blockOffset := int64(physBlock * fs.BlockSize)

			// Read one full block from the device.
			blockBuf := make([]byte, fs.BlockSize)
			if _, err := fs.dev.ReadAt(blockBuf, blockOffset); err != nil {
				return nil, fmt.Errorf(
					"ReadFile: failed to read block %d at byte offset %d: %w",
					physBlock, blockOffset, err,
				)
			}

			// Copy as many bytes as remain in the file (the last block may be
			// only partially filled with real data — the rest is padding).
			remaining := fileSize - written
			toCopy := fs.BlockSize
			if toCopy > remaining {
				toCopy = remaining
			}
			copy(out[written:written+toCopy], blockBuf[:toCopy])
			written += toCopy
		}
	}

	return out, nil
}
