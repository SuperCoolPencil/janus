package ext4

import (
	"encoding/binary"
	"fmt"
)

// A directory in ext4 is a flat file that maps file names to inode numbers.
// Its data blocks contain a packed sequence of variable-length directory
// entries. There is no gap between entries and entries never span a block
// boundary — the last entry in each block has a rec_len that reaches exactly
// to the end of that block.
//
// There are two on-disk entry formats:
//   - ext4_dir_entry   (legacy, filetype feature NOT set)
//   - ext4_dir_entry_2 (default, filetype feature set)
//
// We always parse as ext4_dir_entry_2. The only difference is that the
// 2-byte name_len field of the legacy format becomes a 1-byte name_len and
// a 1-byte file_type in the modern format.
//
// # htree (hashed B-tree) directories
//
// When a directory grows large enough the kernel promotes it to an htree
// index (EXT4_INDEX_FL, I_flags bit 0x1000). Block 0 of the directory file
// then contains a dx_root structure:
//
//	[0..11]  . entry   (12 bytes, inode = self)
//	[12..23] .. entry  (12 bytes, inode = parent)
//	[24..]   dx_root info header + dx_entry[] index records
//
// The dx_root header uses a "fake" directory entry (rec_len spans the rest
// of the block) so that old linear-scan code skips it cleanly. All
// subsequent blocks are leaf blocks — plain DirEntry2 arrays identical to
// those in non-htree directories.
//
// Our read-only strategy: for an htree directory, extract only . and .. from
// block 0, then parse all remaining blocks linearly. We never follow the
// B-tree index — we don't need to for a correct, complete listing.
//
// See docs: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/directory.md

// DirEntry2 is the in-memory representation of a single parsed directory
// entry. It corresponds to struct ext4_dir_entry_2 on disk.
//
// On-disk layout (variable length, always a multiple of 4):
//
//	0x0  __le32  Inode       — inode number (0 = unused entry)
//	0x4  __le16  RecLen      — byte length of this whole record
//	0x6  __u8    NameLen     — byte length of the file name
//	0x7  __u8    FileType    — file type code (see DirFileType* constants)
//	0x8  char[]  Name        — file name (NOT null-terminated on disk)
type DirEntry2 struct {
	// Inode is the inode number this entry points to.
	// A value of 0 means the entry is unused and should be skipped.
	Inode uint32
	// RecLen is the total byte length of this directory entry record,
	// including the header and the name. It is always a multiple of 4.
	// The next entry starts at currentOffset + RecLen.
	RecLen uint16
	// NameLen is the byte length of Name. It does not include a null
	// terminator — the name is not null-terminated on disk.
	NameLen uint8
	// FileType is the type of the file this entry points to.
	// See the DirFileType* constants below.
	// This field is only valid when the "filetype" feature flag is set in
	// the superblock's S_feature_incompat field (INCOMPAT_FILETYPE = 0x2).
	FileType uint8
	// Name is the file name decoded from the raw bytes on disk.
	Name string
}

// File type codes stored in DirEntry2.FileType.
// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/directory.md#ftype
const (
	DirFileTypeUnknown  = 0x0 // Unknown file type
	DirFileTypeRegular  = 0x1 // Regular file
	DirFileTypeDir      = 0x2 // Directory
	DirFileTypeCharDev  = 0x3 // Character device
	DirFileTypeBlockDev = 0x4 // Block device
	DirFileTypeFIFO     = 0x5 // FIFO / named pipe
	DirFileTypeSocket   = 0x6 // Unix domain socket
	DirFileTypeSymlink  = 0x7 // Symbolic link
)

// DirFileTypeName maps a DirFileType* constant to a human-readable string,
// matching the single-character flags used by tools like `ls -l`.
var DirFileTypeName = map[uint8]string{
	DirFileTypeUnknown:  "?",
	DirFileTypeRegular:  "f",
	DirFileTypeDir:      "d",
	DirFileTypeCharDev:  "c",
	DirFileTypeBlockDev: "b",
	DirFileTypeFIFO:     "p",
	DirFileTypeSocket:   "s",
	DirFileTypeSymlink:  "l",
}

// dirEntryHeaderSize is the fixed-size portion of every DirEntry2 record
// (inode + rec_len + name_len + file_type = 4+2+1+1 = 8 bytes).
// The variable-length name field follows immediately after.
const dirEntryHeaderSize = 8

// ReadDir reads all directory entries from a directory inode.
//
// The process has three stages:
//  1. Walk the inode's extent tree to find each physical data block.
//     (See ext4/extent.go and docs/ext4/extent_allocation.md)
//  2. Read each physical block from the device.
//  3. Walk the packed DirEntry2 records inside each block using rec_len
//     as the stride. Skip entries with inode == 0 (unused slots).
//
// For htree directories (EXT4_INDEX_FL set), block 0 is the dx_root block.
// Only the leading . and .. entries are extracted from it; the rest of the
// block holds the B-tree index and is skipped. All subsequent blocks are
// normal leaf blocks and are parsed with parseDirBlock.
//
// The caller is responsible for ensuring that `inode` is a directory
// (i.e. inode.I_mode & S_IFDIR != 0).
func (fs *FileSystem) ReadDir(inode *Inode) ([]DirEntry2, error) {
	// Sanity-check: only directory inodes have DirEntry2 data blocks.
	// Calling this on a regular file or symlink would attempt to parse
	// arbitrary file data as directory records, producing nonsense or panics.
	if !inode.IsDir() {
		return nil, fmt.Errorf(
			"ReadDir called on a non-directory inode (I_mode=0x%04x)",
			inode.I_mode,
		)
	}

	// Detect htree index directories. When the EXT4_INDEX_FL flag is set,
	// block 0 holds a dx_root header after the . and .. entries. We must not
	// parse it as a normal directory block.
	isHtree := inode.I_flags&EXT4_INDEX_FL != 0

	// Resolve the extent tree rooted in the inode's I_block field.
	// This gives us a flat list of (logical_block → physical_block, len)
	// extents in logical order, covering the full file.
	extents, err := fs.ReadExtents(inode)
	if err != nil {
		return nil, fmt.Errorf("failed to read extent tree: %w", err)
	}

	var entries []DirEntry2

	// logicalBlock tracks the logical block index across all extents so we
	// can identify block 0 (the dx_root block) in htree directories.
	logicalBlock := uint64(0)

	// Iterate over every extent (each extent describes a contiguous run of
	// physical blocks). For a small root directory there will typically be
	// just one extent covering a single block.
	for _, ext := range extents {
		// Reconstruct the 48-bit physical start block from the two halves
		// stored in the on-disk extent structure.
		// physStart = (EE_start_hi << 32) | EE_start_lo
		physStart := (uint64(ext.EE_start_hi) << 32) | uint64(ext.EE_start_lo)

		// EE_len holds the number of blocks covered by this extent.
		// Iterate over each block in the run.
		for i := range uint64(ext.EE_len) {
			physBlock := physStart + i
			blockOffset := int64(physBlock * fs.BlockSize)

			// Read the full block into a buffer.
			buf := make([]byte, fs.BlockSize)
			_, err := fs.dev.ReadAt(buf, blockOffset)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to read directory block %d (offset %d): %w",
					physBlock, blockOffset, err,
				)
			}

			var (
				blockEntries []DirEntry2
				parseErr     error
			)
			if isHtree && logicalBlock == 0 {
				// Block 0 of an htree directory: extract only . and ..
				// The rest of the block is the dx_root index structure.
				blockEntries, parseErr = parseDirBlockHtreeRoot(buf)
			} else {
				// Normal leaf block: parse all DirEntry2 records linearly.
				blockEntries, parseErr = parseDirBlock(buf)
			}
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse directory block %d: %w", physBlock, parseErr)
			}
			entries = append(entries, blockEntries...)
			logicalBlock++
		}
	}

	return entries, nil
}

// ReadDirCached is like ReadDir but caches the result by the directory's
// inode number (using I_block as a proxy key). Subsequent calls for the
// same directory return the cached slice without touching the disk.
func (fs *FileSystem) ReadDirCached(inode *Inode) ([]DirEntry2, error) {
	key := inode.I_block
	if cached, ok := fs.dirCache.Load(key); ok {
		return cached.([]DirEntry2), nil
	}
	entries, err := fs.ReadDir(inode)
	if err != nil {
		return nil, err
	}
	fs.dirCache.Store(key, entries)
	return entries, nil
}

// Lookup searches dirInode for a directory entry whose name matches the given
// string, and returns the inode number of the matched entry.
// It returns ErrNotExist if the entry is not found.
func (fs *FileSystem) Lookup(dirInode *Inode, name string) (uint32, error) {
	entries, err := fs.ReadDirCached(dirInode)
	if err != nil {
		return 0, fmt.Errorf("Lookup(%q): failed to read directory: %w", name, err)
	}
	for _, e := range entries {
		if e.Name == name {
			return e.Inode, nil
		}
	}
	return 0, ErrNotExist
}

// parseDirBlockHtreeRoot extracts only the . and .. entries from block 0 of
// an htree (hashed B-tree) directory.
//
// Layout of block 0 in an htree directory:
//
//		[0..11]  . entry   (DirEntry2, inode = dir itself, rec_len = 12)
//		[12..23] .. entry  (DirEntry2, inode = parent dir, rec_len = 12)
//		[24..]   dx_root info header + dx_entry[] hash index records
//	            (disguised as a single DirEntry2 with inode=0 and
//	             rec_len spanning the rest of the block)
//
// We read exactly two entries from the start of the block and stop,
// discarding the rest (the B-tree index data we don't need to follow).
func parseDirBlockHtreeRoot(buf []byte) ([]DirEntry2, error) {
	var entries []DirEntry2
	offset := 0
	blockSize := len(buf)

	// The . and .. entries are always the first two entries. We read at most
	// two entries and stop before we hit the dx_root index data at offset 24.
	for range 2 {
		if offset+dirEntryHeaderSize > blockSize {
			break
		}

		inode := binary.LittleEndian.Uint32(buf[offset : offset+4])
		recLen := binary.LittleEndian.Uint16(buf[offset+4 : offset+6])
		nameLen := buf[offset+6]
		fileType := buf[offset+7]

		if recLen == 0 {
			return nil, fmt.Errorf("htree root: entry at offset %d has rec_len=0 (corrupt block)", offset)
		}

		if inode != 0 && nameLen > 0 {
			nameStart := offset + dirEntryHeaderSize
			nameEnd := nameStart + int(nameLen)
			if nameEnd > blockSize {
				return nil, fmt.Errorf(
					"htree root: entry at offset %d: name extends beyond block boundary (%d > %d)",
					offset, nameEnd, blockSize,
				)
			}
			entries = append(entries, DirEntry2{
				Inode:    inode,
				RecLen:   recLen,
				NameLen:  nameLen,
				FileType: fileType,
				Name:     string(buf[nameStart:nameEnd]),
			})
		}

		offset += int(recLen)
	}

	return entries, nil
}

// parseDirBlock walks the packed DirEntry2 records inside a single raw
// directory block. It uses rec_len as the stride so it handles both
// active entries and the padded-to-end-of-block tail correctly.
//
// Entries with inode == 0 are skipped — they represent unused or deleted
// directory slots. The last valid entry in a block always has a rec_len
// that takes it exactly to the end of the block.
func parseDirBlock(buf []byte) ([]DirEntry2, error) {
	var entries []DirEntry2
	offset := 0
	blockSize := len(buf)

	for offset < blockSize {
		// We need at least the fixed header to read rec_len and name_len.
		if offset+dirEntryHeaderSize > blockSize {
			break
		}

		// Read the fixed header fields individually using little-endian
		// byte order, consistent with how all ext4 structures are encoded.
		//
		// We do NOT use binary.Read into a struct here because the Name
		// field is variable-length and would require a separate read anyway.
		inode := binary.LittleEndian.Uint32(buf[offset : offset+4])
		recLen := binary.LittleEndian.Uint16(buf[offset+4 : offset+6])
		nameLen := buf[offset+6]
		fileType := buf[offset+7]

		// A rec_len of 0 would cause an infinite loop, and it is never
		// valid on a well-formed filesystem.
		if recLen == 0 {
			return nil, fmt.Errorf("directory entry at offset %d has rec_len=0 (corrupt block)", offset)
		}

		// Entries with inode == 0 are holes / deleted entries — skip them.
		if inode != 0 && nameLen > 0 {
			nameStart := offset + dirEntryHeaderSize
			nameEnd := nameStart + int(nameLen)
			if nameEnd > blockSize {
				return nil, fmt.Errorf(
					"directory entry at offset %d: name extends beyond block boundary (%d > %d)",
					offset, nameEnd, blockSize,
				)
			}
			name := string(buf[nameStart:nameEnd])

			entries = append(entries, DirEntry2{
				Inode:    inode,
				RecLen:   recLen,
				NameLen:  nameLen,
				FileType: fileType,
				Name:     name,
			})
		}

		offset += int(recLen)
	}

	return entries, nil
}
