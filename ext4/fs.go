package ext4

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// ErrNotExist is the sentinel error returned by Lookup and Walk when a path
// component or directory entry cannot be found on the filesystem.
//
// Callers should test for it with errors.Is rather than string comparison:
//
//	inode, err := fs.Walk("/etc/fstab")
//	if errors.Is(err, ErrNotExist) {
//	    // map to -fuse.ENOENT, os.ErrNotExist, etc.
//	}
var ErrNotExist = errors.New("no such file or directory")

// FileSystem is the central object for interacting with a mounted ext4
// partition. It holds the parsed superblock, all block group descriptors,
// and a handle to the underlying storage device.
//
// Lifecycle:
//  1. Call NewFileSystem(device) to create a zero-valued FileSystem.
//  2. Call ReadSuperBlock() — this populates BlockSize, InodeSize,
//     GroupCount, DescSize and caches the superblock internally.
//  3. Call ReadGroupDescriptors() — this populates Bgds.
//  4. All other methods (ReadInode, ReadDir, Walk, …) are now usable.
//
// The FileSystem only ever reads from `dev`. It never writes.
type FileSystem struct {
	// dev is the raw storage device or image file. Every read in the ext4
	// package ultimately goes through this single io.ReaderAt. Using an
	// interface (rather than *os.File) allows unit tests to pass a bytes.Reader
	// or any in-memory buffer without opening a real disk.
	dev io.ReaderAt

	// sb is the parsed SuperBlock, cached after the first ReadSuperBlock call.
	// It is nil until ReadSuperBlock succeeds. Most derived values (block size,
	// inode size, …) are extracted from sb during ReadSuperBlock and stored as
	// plain fields below for fast access without nil checks.
	sb *SuperBlock

	// Bgds is the ordered slice of all BlockGroupDescriptors on the filesystem.
	// Index 0 is group 0, index 1 is group 1, etc. Each descriptor locates the
	// inode table, inode/block bitmaps, and free counts for its group.
	//
	// Populated by ReadGroupDescriptors(). Must be populated before any
	// ReadInode call, because inode lookup must find the correct group's table.
	//
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/group_descr.md
	Bgds []GroupDescriptor

	// BlockSize is the filesystem's block size in bytes, always a power of two.
	// Derived from the superblock field S_log_block_size: blockSize = 1024 << S_log_block_size.
	// Typical values: 1024, 2048, 4096 (4096 is by far the most common on modern systems).
	// Used whenever a physical block number needs to be converted to a byte offset:
	//   byteOffset = blockNumber * BlockSize
	//
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/super.md
	BlockSize uint64

	// InodeSize is the on-disk size of each inode record in bytes.
	// For original-format (rev_level == 0) filesystems this is fixed at 128.
	// For dynamic-format filesystems (rev_level >= 1) it is stored in S_inode_size
	// and is always at least 128; modern kernels default to 256.
	//
	// Used in ReadInode to correctly stride through the inode table:
	//   inodeOffset = tableStart + localIndex * InodeSize
	//
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/inodes.md
	InodeSize uint16

	// GroupCount is the total number of block groups on the filesystem.
	// Derived from the superblock: ceil(S_blocks_count_lo / S_blocks_per_group).
	// We use it to bounds-check inode lookup: a valid inode's group index must
	// be in [0, GroupCount).
	GroupCount uint32

	// DescSize is the on-disk size of each BlockGroupDescriptor in bytes.
	// Either 32 (legacy) or 64 (with the 64BIT feature). Stored in S_desc_size.
	// Used when reading the descriptor array to correctly stride through it.
	//
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/group_descr.md
	DescSize uint16

	// dirCache caches directory entry listings keyed by inode's I_block.
	// This avoids re-reading the same directory blocks from disk on every
	// Lookup or Getattr call — Explorer calls Getattr on every file in a
	// directory, and each one triggers a full ReadDir without this cache.
	dirCache sync.Map
}

// NewFileSystem creates a FileSystem backed by the given io.ReaderAt.
//
// The device is not read during construction. Call ReadSuperBlock() and then
// ReadGroupDescriptors() to initialise the filesystem before using any other
// methods.
//
// `device` is typically one of:
//   - *os.File opened on a raw ext4 image (testfs.img)
//   - *disk.PartitionReader wrapping a physical disk file — it translates
//     every ReadAt offset so that offset 0 maps to the partition start byte
//   - *bytes.Reader for unit tests
func NewFileSystem(device io.ReaderAt) (*FileSystem, error) {
	return &FileSystem{dev: device}, nil
}

// Superblock returns the cached SuperBlock parsed by ReadSuperBlock.
// Returns an error if ReadSuperBlock has not been called yet.
func (fs *FileSystem) Superblock() (*SuperBlock, error) {
	if fs.sb == nil {
		return nil, fmt.Errorf("superblock not read yet — call ReadSuperBlock first")
	}
	return fs.sb, nil
}

// Walk resolves an absolute POSIX path string to the Inode it names.
// It follows symlinks encountered in intermediate path components.
func (fs *FileSystem) Walk(path string) (*Inode, error) {
	return fs.walk(path, 0)
}

// maxSymlinks is the maximum number of symbolic links that Walk will follow
// before giving up.
const maxSymlinks = 40

// walk is the recursive implementation of Walk.
func (fs *FileSystem) walk(path string, symDepth int) (*Inode, error) {
	if symDepth > maxSymlinks {
		return nil, fmt.Errorf("too many levels of symbolic links")
	}

	// Inode 2 is always the root directory in ext4.
	cur, err := fs.ReadInode(RootInodeNum)
	if err != nil {
		return nil, fmt.Errorf("walk: failed to read root inode: %w", err)
	}

	// Split the path and iterate over each component.
	components := strings.Split(path, "/")

	for _, component := range components {
		// Skip empty components (e.g. leading slash or consecutive slashes).
		if component == "" {
			continue
		}

		// Every intermediate component must be a directory.
		if !cur.IsDir() {
			return nil, fmt.Errorf("walk: %q is not a directory", component)
		}

		// Look up the component name in the current directory.
		childInodeNum, err := fs.Lookup(cur, component)
		if err != nil {
			return nil, err
		}

		// Load the child inode's metadata.
		child, err := fs.ReadInode(childInodeNum)
		if err != nil {
			return nil, fmt.Errorf("walk: failed to read inode %d for %q: %w", childInodeNum, component, err)
		}

		// Follow symlinks in intermediate path components.
		if child.IsSymlink() {
			target, err := fs.ReadSymlink(child)
			if err != nil {
				return nil, fmt.Errorf("walk: failed to read symlink %q: %w", component, err)
			}
			// Recurse to resolve the symlink target.
			resolved, err := fs.walk(target, symDepth+1)
			if err != nil {
				return nil, fmt.Errorf("walk: symlink %q → %q: %w", component, target, err)
			}
			child = resolved
		}

		cur = child
	}

	return cur, nil
}
