# Understanding Ext4 Disk Layout, Part 2

> **Source:** [Oracle Linux Blog - Understanding Ext4 Disk Layout, Part 2](https://blogs.oracle.com/linux/post/understanding-ext4-disk-layout-part-2)
> **Author:** Srivathsa Dara · August 8, 2023

This document is the second in a series covering Ext4 internals. Part 1 covered the Superblock, GDT, Bitmaps, and Inode Table. Here we focus on the **on-disk structures** of the extent tree, hash tree, and their associated data structures.

---

## `i_block`

The `i_block` field, present in `struct ext4_inode`, occupies **60 bytes** (`EXT4_N_BLOCKS == 15`). It serves as a container for block-related information associated with the inode.

```c
struct ext4_inode {
    __le16  i_mode;         /* File mode */
    __le16  i_uid;          /* Low 16 bits of Owner Uid */
    __le32  i_size_lo;      /* Size in bytes */
    __le32  i_atime;        /* Access time */
    __le32  i_ctime;        /* Inode Change time */
    __le32  i_mtime;        /* Modification time */
    __le32  i_dtime;        /* Deletion Time */
    __le16  i_gid;          /* Low 16 bits of Group Id */
    __le16  i_links_count;  /* Links count */
    __le32  i_blocks_lo;    /* Blocks count */
    __le32  i_flags;        /* File flags */
    union {
        struct { __le32 l_i_version; } linux1;
        struct { __u32  h_i_translator; } hurd1;
        struct { __u32  m_i_reserved1; } masix1;
    } osd1;                         /* OS dependent 1 */
    __le32  i_block[EXT4_N_BLOCKS]; /* Pointers to blocks */
    __le32  i_generation;   /* File version (for NFS) */
    __le32  i_file_acl_lo;  /* File ACL */
    __le32  i_size_high;
    __le32  i_obso_faddr;   /* Obsoleted fragment address */
    union {
        struct {
            __le16  l_i_blocks_high;
            __le16  l_i_file_acl_high;
            __le16  l_i_uid_high;
            __le16  l_i_gid_high;
            __le16  l_i_checksum_lo;  /* crc32c(uuid+inum+inode) LE */
            __le16  l_i_reserved;
        } linux2;
        struct {
            __le16  h_i_reserved1;
            __u16   h_i_mode_high;
            __u16   h_i_uid_high;
            __u16   h_i_gid_high;
            __u32   h_i_author;
        } hurd2;
        struct {
            __le16  h_i_reserved1;
            __le16  m_i_file_acl_high;
            __u32   m_i_reserved2[2];
        } masix2;
    } osd2;                         /* OS dependent 2 */
    __le16  i_extra_isize;
    __le16  i_checksum_hi;  /* crc32c(uuid+inum+inode) BE */
    __le32  i_ctime_extra;  /* extra Change time      (nsec <<2 | epoch) */
    __le32  i_mtime_extra;  /* extra Modification time(nsec <<2 | epoch) */
    __le32  i_atime_extra;  /* extra Access time      (nsec <<2 | epoch) */
    __le32  i_crtime;       /* File Creation time */
    __le32  i_crtime_extra; /* extra File Creation time (nsec <<2 | epoch) */
    __le32  i_version_hi;   /* high 32 bits for 64-bit version */
    __le32  i_projid;       /* Project ID */
};
```

---

## Extent Tree

An **extent** refers to a group of physically contiguous blocks. This grouping reduces the need for direct block mapping between logical and physical blocks, which reduces the amount of metadata to be maintained, resulting in improved performance and efficiency.

An **extent tree** is a data structure that maintains the extents associated with an inode. It provides faster traversal and retrieval of data.

The `i_block` field stores the following structures, each of which is **12 bytes**:
- `struct ext4_extent_header`
- `struct ext4_extent_idx`
- `struct ext4_extent`

Within the 60 bytes of `i_block`:
- The first **12 bytes** are allocated for the extent header.
- The remaining **48 bytes** hold either `ext4_extent_idx` or `ext4_extent` structures (maximum of 4).

### `ext4_extent_header`

```c
struct ext4_extent_header {
    __le16  eh_magic;       /* probably will support different formats */
    __le16  eh_entries;     /* number of valid entries */
    __le16  eh_max;         /* capacity of store in entries */
    __le16  eh_depth;       /* has tree real underlying blocks? */
    __le32  eh_generation;  /* generation of the tree */
};
```

### `ext4_extent_idx`

```c
struct ext4_extent_idx {
    __le32  ei_block;    /* index covers logical blocks from 'block' */
    __le32  ei_leaf_lo;  /* pointer to the physical block of the next level */
    __le16  ei_leaf_hi;  /* high 16 bits of physical block */
    __u16   ei_unused;
};
```

### `ext4_extent`

```c
struct ext4_extent {
    __le32  ee_block;     /* first logical block extent covers */
    __le16  ee_len;       /* number of blocks covered by extent */
    __le16  ee_start_hi;  /* high 16 bits of physical block */
    __le32  ee_start_lo;  /* low 32 bits of physical block */
};
```

### Purpose of Each Structure

| Structure | Role |
|---|---|
| `ext4_extent_header` | Provides metadata about the extent tree: magic number, depth, and number of valid entries. When `depth == 0`, entries are `ext4_extent`s; otherwise they are `ext4_extent_idx`s. Occupies the first 12 bytes in all blocks of the extent tree. |
| `ext4_extent_idx` | An intermediate index node. Holds references to further levels of the tree. Present in all non-leaf blocks. |
| `ext4_extent` | A leaf node. Describes the starting logical block and length of a physically contiguous range of data blocks. Exclusively present in leaf blocks. |

> **Note:** Ext4 uses **little-endian** notation. The least significant byte is stored first. For example, if `0x1234` is written on disk, its value in memory is `0x3412`.

### Worked Example (depth = 1)

The hexdump of an inode shows the first 12 bytes as the extent header with `eh_depth = 1`. This means the header is followed by `ext4_extent_idx` structures (not leaf extents).

The `ext4_extent_idx` in the example points to physical block `0x85ca`. Examining that block:

- The first 12 bytes are again an extent header (`eh_depth = 0`, `eh_max = 340`).
  - `eh_max = 340` because: `(4096 - 12 [header] - 4 [ext4_extent_tail]) / 12 [bytes per extent] = 340`.
- Since `depth == 0`, the subsequent entries are `ext4_extent` structures.
- `eh_entries = 182` - so 182 extents follow.

**First three extents from the example:**

| # | Logical Block (`ee_block`) | Length (`ee_len`) | First Physical Block |
|---|---|---|---|
| 1 | 0 | 140 | 3,576,372 |
| 2 | 140 | 160 | 36,139,252 |
| 3 | 300 | 158 | 3,700,229 |

The `debugfs` `ex` command can be used to view the same information:

```
debugfs> ex <inode>
Level Entries   Logical              Physical  Length  Flags
 0/ 1   1/  1  0 - 139      3576372 - 3576511   140
 0/ 1   2/  1  140 - 299   36139252 - 36139411  160
 0/ 1   3/  1  300 - 457    3700229 - 3700386   158
 ...
```

### Depth and Indirection

| Depth | Structure within `i_block` | Indirection |
|---|---|---|
| 0 | `ext4_extent_header` + up to 4 × `ext4_extent` | None - extents point directly to data blocks |
| 1 | `ext4_extent_header` + up to 4 × `ext4_extent_idx` | One intermediate block between `i_block` and data |
| 2 | `ext4_extent_header` + up to 4 × `ext4_extent_idx` | Two intermediate blocks |

When a 5th extent is needed in a depth-0 tree, the depth is promoted to 1 to accommodate it.

---

## Hash Tree (HTree)

A **directory** stores dirents (directory entries) of all its files in its data blocks. For directories with many files, a linear search becomes inefficient. Ext4 implements a **hash tree** (HTree) to address this.

### Overview

- **Unindexed mode:** Dirents are stored sequentially in a single data block. Efficient for small directories.
- **Indexed mode (HTree):** Once dirents overflow a single block and the hash tree feature is enabled, the directory is converted to an indexed tree.

The hash tree consists of:
- **Root node** - holds tree metadata (depth, hash algorithm).
- **Intermediate nodes** - act as branches, directing search based on hash values.
- **Leaf nodes** - hold the actual `ext4_dir_entry_2` structures.

### `ext4_dir_entry_2`

```c
struct ext4_dir_entry_2 {
    __le32  inode;                  /* Inode number */
    __le16  rec_len;                /* Directory entry length */
    __u8    name_len;               /* Name length */
    __u8    file_type;
    char    name[EXT4_NAME_LEN];    /* File name (EXT4_NAME_LEN == 255) */
};
```

### `dx_root`

The first data block of an indexed directory stores the hash tree root:

```c
struct dx_root {
    struct fake_dirent dot;
    char dot_name[4];
    struct fake_dirent dotdot;
    char dotdot_name[4];
    struct dx_root_info {
        __le32  reserved_zero;
        u8      hash_version;
        u8      info_length;    /* 8 */
        u8      indirect_levels;
        u8      unused_flags;
    } info;
    struct dx_entry entries[0];
};
```

### `fake_dirent`

```c
struct fake_dirent {
    __le32  inode;
    __le16  rec_len;
    u8      name_len;
    u8      file_type;
};
```

`fake_dirent` acts as the dirent for `.` and `..` inside `dx_root`. It omits the `name` field since the name is inlined in `dx_root` (`dot_name`, `dotdot_name`).

### `dx_entry`

```c
struct dx_entry {
    __le32  hash;   /* hash value (0 for the first entry) */
    __le32  block;  /* logical block number */
};
```

Each `dx_entry` maps a hash value to a logical block. That block stores dirents whose hash values fall in the range `[hash(this entry), hash(next entry))`.

### Worked Example

Directory inode `131007` contains 2500 files. Its extent tree has depth 0 with a single extent starting at block 532,576 (30 contiguous blocks).

The first 36 bytes of block 532,576 are the `dx_root`, followed by 28 `dx_entry` structures. `eh_entries = 29` (28 `dx_entry`s + the root itself).

**First three `dx_entry` values:**

| Hash Value | Logical Block |
|---|---|
| 0 (Zero Hash) | `0x01` |
| `0x082e0162` | `0x12` |
| `0x103050d2` | `0x09` |
| `0x17358dcc` | `0x18` |

**Example lookup:** To insert a dirent with hash `0x092e0111`:
- `0x082e0162 < 0x092e0111 < 0x103050d2` → insert into logical block `0x12`.

**Example search:** To find a dirent with hash between `0` and `0x082e0162` → search block `0x01`.

### Hash Tree Depth > 0

When `dx_root.info.indirect_levels > 0`, there are intermediate blocks between the root and the leaf blocks. These intermediate blocks contain only `dx_entry` structures. During a search, only **one block per level** is examined until the leaf is reached; a linear search is then performed within that leaf block.

### Supported Hash Algorithms

| Value | Hash Algorithm |
|---|---|
| `0x0` | Legacy |
| `0x1` | Half MD4 |
| `0x2` | Tea |
| `0x3` | Legacy, unsigned |
| `0x4` | Half MD4, unsigned |
| `0x5` | Tea, unsigned |
| `0x6` | SipHash |

### File Type Values in `ext4_dir_entry_2`

| Value | File Type |
|---|---|
| `0x0` | Unknown |
| `0x1` | Regular file |
| `0x2` | Directory |
| `0x3` | Character device file |
| `0x4` | Block device file |
| `0x5` | FIFO |
| `0x6` | Socket |
| `0x7` | Symbolic link |

### Dirent Structure (leaf block)

Each dirent in a leaf block has:

1. **4 bytes** - Inode number
2. **2 bytes** - `rec_len` (record length; the last dirent's `rec_len` spans to the end of the block)
3. **1 byte** - `name_len`
4. **1 byte** - `file_type`
5. **`name_len` bytes** - file name

---

## Hash Tree Growth

### Conversion from Unindexed to Indexed

Initially a directory is unindexed (linear list of dirents in a single block). When a new dirent cannot fit:

1. Two new blocks are created.
2. One becomes the root block (initialized with `dx_root`).
3. The existing dirents are sorted by hash and split 50/50: the first half stays in the original block, the second half moves to the new block.
4. Two `dx_entry` structures are added to `dx_root` - one with hash `0` (pointing to the first block) and one with the hash of the first dirent in the second block.

### Growth After Conversion

If a leaf block becomes full:
1. A new leaf block is created.
2. Dirents are redistributed equally between the two blocks based on hash values.
3. Dirents with identical hash values always stay together (collision-safe redistribution).

**Example:**

Before adding 20 new files (depth = 0, 3 leaf blocks):

```
dx_root
├── [hash=0]       → block 1
├── [hash=0xAAA]   → block 2
└── [hash=0xBBB]   → block 3
```

After (block 1 was full, new block 4 inserted between block 1 and block 2):

```
dx_root
├── [hash=0]       → block 1
├── [hash=0x555]   → block 4  (new)
├── [hash=0xAAA]   → block 2
└── [hash=0xBBB]   → block 3
```

---

## Hash Collisions

If multiple dirents share the same hash value, they are stored together in the same block. When the target block is located, a **linear search** is performed within that block to find the specific dirent.

When a collision forces a split of a completely full block:
- A new block is created and dirents are redistributed by hash.
- **Dirents with the same hash always remain together** in the same block, even after redistribution.

---

## Summary

| Feature | Purpose |
|---|---|
| **Extent tree** | Maps logical blocks to physically contiguous ranges (extents), reducing metadata overhead and improving performance. |
| **Hash tree (HTree)** | Provides O(depth) directory lookups instead of O(n) linear scans, crucial for directories with many files. |

Both structures share the same general pattern: a header at the root of `i_block`, index nodes for intermediate levels, and leaf nodes holding the actual data (extents or dirents).

---

## References

- Kernel code `fs/ext4/` - 5.4.17-2136.310.7.1.el8uek.x86_64
- <https://www.kernel.org/doc/html/latest/filesystems/ext4/>
- <https://www.sans.org/blog/understanding-ext4-part-3-extent-trees/>
- <https://www.sans.org/blog/understanding-ext4-part-6-directories/>
