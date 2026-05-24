package ext4

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

	// File mode (i_mode field) type and permission flags
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

	// Inode flags (i_flags field)
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
