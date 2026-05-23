package ext4

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// The superblock records various information about the enclosing filesystem,
// such as block counts, inode counts, supported features, maintenance information, and more.
// https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/super.md#super-block
type SuperBlock struct {
	// 0x00: Total inode count.
	S_inodes_count uint32
	// 0x04: Total block count.
	S_blocks_count_lo uint32
	// 0x08: This number of blocks can only be allocated by the super-user.
	S_r_blocks_count_lo uint32
	// 0x0C: Free block count.
	S_free_blocks_count_lo uint32
	// 0x10: Free inode count.
	S_free_inodes_count uint32
	// 0x14: First data block.
	S_first_data_block uint32
	// 0x18: Block size is 2^(10 + S_log_block_size).
	S_log_block_size uint32
	// 0x1C: Cluster size if bigalloc enabled.
	S_log_cluster_size uint32
	// 0x20: Blocks per group.
	S_blocks_per_group uint32
	// 0x24: Clusters per group.
	S_clusters_per_group uint32
	// 0x28: Inodes per group.
	S_inodes_per_group uint32
	// 0x2C: Mount time, seconds since epoch.
	S_mtime uint32
	// 0x30: Write time, seconds since epoch.
	S_wtime uint32
	// 0x34: Number of mounts since the last fsck.
	S_mnt_count uint16
	// 0x36: Number of mounts beyond which a fsck is needed.
	S_max_mnt_count uint16
	// 0x38: Magic signature (0xEF53).
	S_magic uint16
	// 0x3A: File system state.
	S_state uint16
	// 0x3C: Behaviour when detecting errors.
	S_errors uint16
	// 0x3E: Minor revision level.
	S_minor_rev_level uint16
	// 0x40: Time of last check, seconds since epoch.
	S_lastcheck uint32
	// 0x44: Maximum time between checks, in seconds.
	S_checkinterval uint32
	// 0x48: Creator OS.
	S_creator_os uint32
	// 0x4C: Revision level.
	S_rev_level uint32
	// 0x50: Default uid for reserved blocks.
	S_def_resuid uint16
	// 0x52: Default gid for reserved blocks.
	S_def_resgid uint16
	// 0x54: First non-reserved inode.
	S_first_ino uint32
	// 0x58: Size of inode structure, in bytes.
	S_inode_size uint16
	// 0x5A: Block group # of this superblock.
	S_block_group_nr uint16
	// 0x5C: Compatible feature set flags.
	S_feature_compat uint32
	// 0x60: Incompatible feature set.
	S_feature_incompat uint32
	// 0x64: Readonly-compatible feature set.
	S_feature_ro_compat uint32
	// 0x68: 128-bit UUID for volume.
	S_uuid [16]byte
	// 0x78: Volume label.
	S_volume_name [16]byte
	// 0x88: Directory where filesystem was last mounted.
	S_last_mounted [64]byte
	// 0xC8: For compression (not used in e2fsprogs/Linux).
	S_algorithm_usage_bitmap uint32
	// 0xCC: Number of blocks to try to preallocate for files.
	S_prealloc_blocks uint8
	// 0xCD: Number of blocks to preallocate for directories.
	S_prealloc_dir_blocks uint8
	// 0xCE: Number of reserved GDT entries for future filesystem expansion.
	S_reserved_gdt_blocks uint16
	// 0xD0: UUID of journal superblock.
	S_journal_uuid [16]byte
	// 0xE0: Inode number of journal file.
	S_journal_inum uint32
	// 0xE4: Device number of journal file.
	S_journal_dev uint32
	// 0xE8: Start of list of orphaned inodes to delete.
	S_last_orphan uint32
	// 0xEC: HTREE hash seed.
	S_hash_seed [4]uint32
	// 0xFC: Default hash algorithm to use for directory hashes.
	S_def_hash_version uint8
	// 0xFD: If 0 or EXT3_JNL_BACKUP_BLOCKS (1) thenS_jnl_blocks contains duplicate copy.
	S_jnl_backup_type uint8
	// 0xFE: Size of group descriptors, in bytes.
	S_desc_size uint16
	// 0x100: Default mount options.
	S_default_mount_opts uint32
	// 0x104: First metablock block group.
	S_first_meta_bg uint32
	// 0x108: When the filesystem was created, seconds since epoch.
	S_mkfs_time uint32
	// 0x10C: Backup copy of the journal inode's i_block[] array.
	S_jnl_blocks [17]uint32
	// 0x150: High 32-bits of the block count.
	S_blocks_count_hi uint32
	// 0x154: High 32-bits of the reserved block count.
	S_r_blocks_count_hi uint32
	// 0x158: High 32-bits of the free block count.
	S_free_blocks_count_hi uint32
	// 0x15C: All inodes have at least # bytes.
	S_min_extra_isize uint16
	// 0x15E: New inodes should reserve # bytes.
	S_want_extra_isize uint16
	// 0x160: Miscellaneous flags.
	S_flags uint32
	// 0x164: RAID stride.
	S_raid_stride uint16
	// 0x166: Seconds to wait in multi-mount prevention checking.
	S_mmp_interval uint16
	// 0x168: Block # for multi-mount protection data.
	S_mmp_block uint64
	// 0x170: RAID stripe width.
	S_raid_stripe_width uint32
	// 0x174: Size of a flexible block group is 2^s_log_groups_per_flex.
	S_log_groups_per_flex uint8
	// 0x175: Metadata checksum algorithm type.
	S_checksum_type uint8
	// 0x176: Versioning level for encryption.
	S_encryption_level uint8
	// 0x177: Padding to next 32bits.
	S_reserved_pad uint8
	// 0x178: Number of KiB written to this filesystem over its lifetime.
	S_kbytes_written uint64
	// 0x180: Inode number of active snapshot.
	S_snapshot_inum uint32
	// 0x184: Sequential ID of active snapshot.
	S_snapshot_id uint32
	// 0x188: Number of blocks reserved for active snapshot’s future use.
	S_snapshot_r_blocks_count uint64
	// 0x190: Inode number of the head of the on-disk snapshot list.
	S_snapshot_list uint32
	// 0x194: Number of errors seen.
	S_error_count uint32
	// 0x198: First time an error happened, seconds since epoch.
	S_first_error_time uint32
	// 0x19C: Inode involved in first error.
	S_first_error_ino uint32
	// 0x1A0: Number of block involved of first error.
	S_first_error_block uint64
	// 0x1A8: Name of function where the error happened.
	S_first_error_func [32]byte
	// 0x1C8: Line number where error happened.
	S_first_error_line uint32
	// 0x1CC: Time of most recent error, seconds since epoch.
	S_last_error_time uint32
	// 0x1D0: Inode involved in most recent error.
	S_last_error_ino uint32
	// 0x1D4: Line number where most recent error happened.
	S_last_error_line uint32
	// 0x1D8: Number of block involved in most recent error.
	S_last_error_block uint64
	// 0x1E0: Name of function where the most recent error happened.
	S_last_error_func [32]byte
	// 0x200: ASCIIZ string of mount options.
	S_mount_opts [64]byte
	// 0x240: Inode number of user quota file.
	S_usr_quota_inum uint32
	// 0x244: Inode number of group quota file.
	S_grp_quota_inum uint32
	// 0x248: Overhead blocks/clusters in fs.
	S_overhead_blocks uint32
	// 0x24C: Block groups containing superblock backups.
	S_backup_bgs [2]uint32
	// 0x254: Encryption algorithms in use.
	S_encrypt_algos [4]uint8
	// 0x258: Salt for the string2key algorithm for encryption.
	S_encrypt_pw_salt [16]byte
	// 0x268: Inode number of lost+found.
	S_lpf_ino uint32
	// 0x26C: Inode that tracks project quotas.
	S_prj_quota_inum uint32
	// 0x270: Checksum seed used for metadata_csum calculations.
	S_checksum_seed uint32
	// 0x274: Upper 8 bits of the S_wtime field.
	S_wtime_hi uint8
	// 0x275: Upper 8 bits of the S_mtime field.
	S_mtime_hi uint8
	// 0x276: Upper 8 bits of the S_mkfs_time field.
	S_mkfs_time_hi uint8
	// 0x277: Upper 8 bits of the S_lastcheck field.
	S_lastcheck_hi uint8
	// 0x278: Upper 8 bits of th eS_first_error_time field.
	S_first_error_time_hi uint8
	// 0x279: Upper 8 bits of the S_last_error_time field.
	S_last_error_time_hi uint8
	// 0x27A: First error errcode.
	S_first_error_errcode uint8
	// 0x27B: Last error errcode.
	S_last_error_errcode uint8
	// 0x27C: Filename charset encoding.
	S_encoding uint16
	// 0x27E: Filename charset encoding flags.
	S_encoding_flags uint16
	// 0x280: Orphan file inode number.
	S_orphan_file_inum uint32
	// 0x284: Padding to the end of the block.
	S_reserved [94]uint32
	// 0x3FC: Superblock checksum.
	S_checksum uint32
}

const (

	// The superblock is located at an offset of 1024 bytes from the start of the device.
	SUPERBLOCK_OFFSET = 1024
	// The magic number that identifies an ext4 filesystem.
	MAGIC_NUMBER = 0xEF53
)

// ReadSuperBlock reads and decodes the superblock from the underlying device/file.
func (fs *FileSystem) ReadSuperBlock() (*SuperBlock, error) {
	var sb SuperBlock
	buf := make([]byte, 1024)

	_, err := fs.dev.ReadAt(buf, SUPERBLOCK_OFFSET)
	if err != nil {
		return nil, fmt.Errorf("failed to read superblock from device: %w", err)
	}

	err = decodeSuperBlock(buf, &sb)
	if err != nil {
		return nil, err
	}

	fs.sb = &sb
	return &sb, nil
}

func decodeSuperBlock(data []byte, sb *SuperBlock) error {

	// Wrap the raw bytes in an io.Reader
	reader := bytes.NewReader(data)

	// Ext4 stores all integers in Little Endian format.
	// binary.Read uses reflection to instantly map the bytes
	// directly into aligned struct fields.
	err := binary.Read(reader, binary.LittleEndian, sb)
	if err != nil {
		return fmt.Errorf("failed to parse superblock binary data: %w", err)
	}

	// Always validate the magic number immediately after parsing
	if sb.S_magic != MAGIC_NUMBER {
		return fmt.Errorf("invalid ext4 magic number: expected 0x%X, got 0x%X", MAGIC_NUMBER, sb.S_magic)
	}

	return nil
}

// BlockSize calculates the actual block size in bytes.
// (Block size is 2^(10 + S_log_block_size))
func (sb *SuperBlock) BlockSize() uint64 {
	return 1024 << sb.S_log_block_size
}

// InodeSize returns the size of an inode.
// In dynamic revisions (>= 1), this is stored in the superblock.
// In the original revision (0), it is hardcoded to 128.
func (sb *SuperBlock) InodeSize() uint16 {
	if sb.S_rev_level > 0 {
		return sb.S_inode_size
	}
	return 128
}
