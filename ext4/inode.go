package ext4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// The inode stores all the metdata pertaining to the file
// To find the information associated with a file,
// one must traverse the directory files to find the directory entry
// associated with a file, then load the inode to find the metadata for that file.

// See docs: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/inodes.md

// ext4 reserves some inode for special features, as follows:

// Inode 0: doesn't exist
// Inode 1: list of defective blocks.
// Inode 2: Root directory
// Inode 3: User quota.
// Inode 4: Group quota.
// Inode 5: Boot loader.
// Inode 6: Undelete directory.
// Inode 7: Journal inode.
// Inode 8: Reserved group descriptors inode.
// Inode 9: The "exclude" inode, for snapshots (?)
// Inode 10: Replica inode, used for some non upstream feature
// Inode 11: Traditional first non-reserved inode. Usually used for lost+found

// The inode table is a linear array of struct ext4_inode.
// The table is sized to have enough blocks to store at least
// sb.s_inode_size * sb.s_inodes_per_group bytes.

type Inode struct {
	// 0x0: File mode. See i_mode flags.
	I_mode uint16
	// 0x2: Lower 16-bits of Owner UID.
	I_uid uint16
	// 0x4: Lower 32-bits of size in bytes.
	I_size_lo uint32
	// 0x8: Last access time, in seconds since the epoch.
	I_atime uint32
	// 0xC: Last inode change time, in seconds since the epoch.
	I_ctime uint32
	// 0x10: Last data modification time, in seconds since the epoch.
	I_mtime uint32
	// 0x14: Deletion time, in seconds since the epoch.
	I_dtime uint32
	// 0x18: Lower 16-bits of GID.
	I_gid uint16
	// 0x1A: Hard link count.
	I_links_count uint16
	// 0x1C: Lower 32-bits of "block" count.
	I_blocks_lo uint32
	// 0x20: Inode flags. See i_flags flags.
	I_flags uint32
	// 0x24: (Linux) Inode version. Upper 32 bits of extended attribute value
	// reference count if EA_INODE flag is set.
	L_i_version uint32
	// 0x28: Block map or extent tree (EXT4_N_BLOCKS=15 entries, 60 bytes).
	I_block [60]byte
	// 0x64: File version (for NFS).
	I_generation uint32
	// 0x68: Lower 32-bits of extended attribute block.
	I_file_acl_lo uint32
	// 0x6C: Upper 32-bits of file/directory size. (i_dir_acl in ext2/3.)
	I_size_high uint32
	// 0x70: (Obsolete) fragment address.
	I_obso_faddr uint32
	// 0x74: (Linux osd2) Upper 16-bits of the block count.
	L_i_blocks_high uint16
	// 0x76: (Linux osd2) Upper 16-bits of the extended attribute block.
	L_i_file_acl_high uint16
	// 0x78: (Linux osd2) Upper 16-bits of the Owner UID.
	L_i_uid_high uint16
	// 0x7A: (Linux osd2) Upper 16-bits of the GID.
	L_i_gid_high uint16
	// 0x7C: (Linux osd2) Lower 16-bits of the inode checksum.
	L_i_checksum_lo uint16
	// 0x7E: (Linux osd2) Unused.
	L_i_reserved uint16
	// 0x80: Size of this inode - 128 (size of extended inode fields beyond original ext2 inode).
	I_extra_isize uint16
	// 0x82: Upper 16-bits of the inode checksum.
	I_checksum_hi uint16
	// 0x84: Extra change time bits for sub-second precision.
	I_ctime_extra uint32
	// 0x88: Extra modification time bits for sub-second precision.
	I_mtime_extra uint32
	// 0x8C: Extra access time bits for sub-second precision.
	I_atime_extra uint32
	// 0x90: File creation time, in seconds since the epoch.
	I_crtime uint32
	// 0x94: Extra file creation time bits for sub-second precision.
	I_crtime_extra uint32
	// 0x98: Upper 32-bits for version number.
	I_version_hi uint32
	// 0x9C: Project ID.
	I_projid uint32
}

const (
	RootInodeNum = 2

	// File mode (i_mode field) type and permission flags.
	S_IXOTH  = 0x1    // Others may execute
	S_IWOTH  = 0x2    // Others may write
	S_IROTH  = 0x4    // Others may read
	S_IXGRP  = 0x8    // Group members may execute
	S_IWGRP  = 0x10   // Group members may write
	S_IRGRP  = 0x20   // Group members may read
	S_IXUSR  = 0x40   // Owner may execute
	S_IWUSR  = 0x80   // Owner may write
	S_IRUSR  = 0x100  // Owner may read
	S_ISVTX  = 0x200  // Sticky bit
	S_ISGID  = 0x400  // Set GID
	S_ISUID  = 0x800  // Set UID
	S_IFIFO  = 0x1000 // FIFO
	S_IFCHR  = 0x2000 // Character device
	S_IFDIR  = 0x4000 // Directory
	S_IFBLK  = 0x6000 // Block device
	S_IFREG  = 0x8000 // Regular file
	S_IFLNK  = 0xA000 // Symbolic link
	S_IFSOCK = 0xC000 // Socket

	// Inode flags (i_flags field).
	EXT4_SECRM_FL            = 0x1        // Secure deletion
	EXT4_UNRM_FL             = 0x2        // Undelete
	EXT4_COMPR_FL            = 0x4        // Compress file
	EXT4_SYNC_FL             = 0x8        // Synchronous writes
	EXT4_IMMUTABLE_FL        = 0x10       // Immutable file
	EXT4_APPEND_FL           = 0x20       // Append-only file
	EXT4_NODUMP_FL           = 0x40       // Do not dump file
	EXT4_NOATIME_FL          = 0x80       // Do not update access time
	EXT4_DIRTY_FL            = 0x100      // Dirty compressed file
	EXT4_COMPRBLK_FL         = 0x200      // Compressed clusters
	EXT4_NOCOMPR_FL          = 0x400      // Do not compress
	EXT4_ENCRYPT_FL          = 0x800      // Encrypted inode
	EXT4_INDEX_FL            = 0x1000     // Hashed directory indexes
	EXT4_IMAGIC_FL           = 0x2000     // AFS magic directory
	EXT4_JOURNAL_DATA_FL     = 0x4000     // Journal file data
	EXT4_NOTAIL_FL           = 0x8000     // Do not merge file tail
	EXT4_DIRSYNC_FL          = 0x10000    // Synchronous directory changes
	EXT4_TOPDIR_FL           = 0x20000    // Directory hierarchy top
	EXT4_HUGE_FILE_FL        = 0x40000    // Huge file
	EXT4_EXTENTS_FL          = 0x80000    // Inode uses extents
	EXT4_VERITY_FL           = 0x100000   // Verity protected
	EXT4_EA_INODE_FL         = 0x200000   // Large extended attribute in data blocks
	EXT4_EOFBLOCKS_FL        = 0x400000   // Blocks allocated past EOF (deprecated)
	EXT4_SNAPFILE_FL         = 0x01000000 // Snapshot file
	EXT4_SNAPFILE_DELETED_FL = 0x04000000 // Snapshot being deleted
	EXT4_SNAPFILE_SHRUNK_FL  = 0x08000000 // Snapshot shrink completed
	EXT4_INLINE_DATA_FL      = 0x10000000 // Inode has inline data
	EXT4_PROJINHERIT_FL      = 0x20000000 // Inherit project ID
	EXT4_CASEFOLD_FL         = 0x40000000 // Case-insensitive lookups
	EXT4_RESERVED_FL         = 0x80000000 // Reserved for ext4 library
)

// ReadInode locates and parses any inode on the filesystem by its number.
func (fs *FileSystem) ReadInode(inodeNum uint32) (*Inode, error) {
	if inodeNum < 1 {
		return nil, fmt.Errorf("invalid inode number: %d (inodes start at 1)", inodeNum)
	}

	// Calculate which block group contains this inode
	groupIndex := (inodeNum - 1) / fs.sb.S_inodes_per_group

	// Calculate the index of the inode INSIDE that block group's table
	localInodeIndex := (inodeNum - 1) % fs.sb.S_inodes_per_group

	// Ensure we don't go out of bounds
	if groupIndex >= fs.GroupCount {
		return nil, fmt.Errorf(
			"inode %d belongs to group %d, but we only have %d groups",
			inodeNum, groupIndex, fs.GroupCount,
		)
	}

	// Get the Block Group Descriptor for that group
	bgd := fs.Bgds[groupIndex]
	if bgd.BG_inode_table_lo == 0 {
		return nil, fmt.Errorf("inode table not found in group %d", groupIndex)
	}

	// Find the start block of the Inode Table
	tableBlock := uint64(bgd.BG_inode_table_lo)
	if fs.DescSize > 32 {
		tableBlock |= uint64(bgd.BG_inode_table_hi) << 32
	}

	// Calculate the exact byte offset on the disk
	// Offset = (Table Start Block * Block Size) + (Local Index * Inode Size)
	offset := (tableBlock * fs.BlockSize) + (uint64(localInodeIndex) * uint64(fs.InodeSize))

	if offset > math.MaxInt64 {
		return nil, fmt.Errorf("inode offset %d overflows int64", offset)
	}

	// Read and decode the Inode
	var inode Inode
	buf := make([]byte, fs.InodeSize)
	_, err := fs.dev.ReadAt(buf, int64(offset))
	if err != nil {
		return nil, fmt.Errorf("failed to read inode %d at offset %d: %w", inodeNum, offset, err)
	}

	err = binary.Read(bytes.NewReader(buf), binary.LittleEndian, &inode)
	if err != nil {
		return nil, fmt.Errorf("failed to decode inode %d: %w", inodeNum, err)
	}

	return &inode, nil
}

// ReadRootInode is just a convenient wrapper.
func (fs *FileSystem) ReadRootInode() (*Inode, error) {
	// In ext2/3/4, the root directory is ALWAYS Inode 2.
	// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/inodes.md
	return fs.ReadInode(RootInodeNum)
}

// ── Inode helper methods ──────────────────────────────────────────────────────
//
// The I_mode field encodes two orthogonal things in a single uint16:
//
//   Bits 15..12  file type   (4 bits, mask 0xF000)
//   Bits 11..0   permissions (12 bits, mask 0x0FFF)
//
// To check the file type you MUST mask with 0xF000 before comparing,
// otherwise the permission bits will corrupt the comparison.
//
//   Example: a regular file owned by root with mode 0644 has
//            I_mode = 0x8000 | 0x01A4 = 0x81A4
//            0x81A4 & 0xF000 == 0x8000 == S_IFREG  ✓
//
// All S_IF* constants are defined above alongside the Inode struct.
// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/inodes.md

// ifmt is the file-type mask for the I_mode field.
// Applying this mask isolates the four type bits and strips permissions.
const ifmt = 0xF000

// IsDir reports whether this inode represents a directory.
//
// Directories are the containers that map file names to inode numbers.
// Every path component except the last one must be a directory.
// The root inode (number 2) is always a directory.
//
// On-disk: I_mode & 0xF000 == S_IFDIR (0x4000)
func (i *Inode) IsDir() bool {
	return i.I_mode&ifmt == S_IFDIR
}

// IsRegular reports whether this inode represents a regular file.
//
// Regular files hold arbitrary user data. Their contents are reached by
// walking the extent tree (or legacy indirect block map) stored in I_block.
// ReadFile uses this to guard against being called on a directory or device.
//
// On-disk: I_mode & 0xF000 == S_IFREG (0x8000)
func (i *Inode) IsRegular() bool {
	return i.I_mode&ifmt == S_IFREG
}

// IsSymlink reports whether this inode represents a symbolic link.
//
// Symbolic links store a target path string rather than file data.
// Ext4 distinguishes between two storage strategies depending on length:
//   - Fast symlink: target ≤ 60 bytes → stored directly in I_block[0..59]
//     (EXT4_EXTENTS_FL is NOT set; I_size_lo holds the target length)
//   - Slow symlink: target > 60 bytes → stored in a regular data block
//     (EXT4_EXTENTS_FL IS set; read like a regular file)
//
// See docs/ext4/ifork.md for the inline data layout.
//
// On-disk: I_mode & 0xF000 == S_IFLNK (0xA000)
func (i *Inode) IsSymlink() bool {
	return i.I_mode&ifmt == S_IFLNK
}

// Size returns the complete size of the inode's data in bytes.
//
// The size is stored as a split 64-bit value across two fields:
//
//   I_size_lo   lower 32 bits  (always valid, present since ext2)
//   I_size_high upper 32 bits  (valid for regular files on rev_level ≥ 1 filesystems)
//
// IMPORTANT: For directories, I_size_high is historically named i_dir_acl
// and carries a completely different meaning (extended attribute block
// pointer in very old ext2). To avoid misinterpreting it as a size, we
// return only I_size_lo for directory inodes.
//
// See: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/inodes.md
func (i *Inode) Size() uint64 {
	if i.IsDir() {
		// Directory size is always < 4 GiB; I_size_high is repurposed
		// for i_dir_acl in the original ext2 format and must not be
		// treated as the upper half of the size.
		return uint64(i.I_size_lo)
	}
	return (uint64(i.I_size_high) << 32) | uint64(i.I_size_lo)
}

// UsesExtents reports whether this inode's data blocks are addressed via
// the ext4 extent tree (as opposed to the legacy ext2/ext3 indirect block
// scheme).
//
// All directories and regular files created by Linux kernel 2.6.23+ have
// this flag set. The legacy scheme exists for backwards compatibility with
// very old filesystems that were created before extents were introduced.
//
// When this flag is NOT set, the 60 bytes of I_block contain up to 15
// direct/indirect block pointers (EXT4_N_BLOCKS = 15) — a structure we
// do not yet support. Callers should check this flag and return a clear
// error rather than silently misparsing the data.
//
// On-disk: I_flags & EXT4_EXTENTS_FL (0x80000) != 0
//
// See docs/ext4/ifork.md for the fork (I_block) layout under both schemes.
func (i *Inode) UsesExtents() bool {
	return i.I_flags&EXT4_EXTENTS_FL != 0
}
