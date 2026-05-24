# 2.9. Verity files — The Linux Kernel  documentation

[![Logo](https://docs.kernel.org/_static/logo.svg)](https://docs.kernel.org/index.md)

# [The Linux Kernel](https://docs.kernel.org/index.md)

7.1.0-rc4



# 2.9. Verity files[¶](verity.md#verity-files "Permalink to this heading")

ext4 supports fs-verity, which is a filesystem feature that provides
Merkle tree based hashing for individual readonly files. Most of
fs-verity is common to all filesystems that support it; see
[Documentation/filesystems/fsverity.rst](https://docs.kernel.org/filesystems/fsverity.md#fsverity) for the
fs-verity documentation. However, the on-disk layout of the verity
metadata is filesystem-specific. On ext4, the verity metadata is
stored after the end of the file data itself, in the following format:

* Zero-padding to the next 65536-byte boundary. This padding need not
  actually be allocated on-disk, i.e. it may be a hole.
* The Merkle tree, as documented in
  [Documentation/filesystems/fsverity.rst](https://docs.kernel.org/filesystems/fsverity.md#fsverity-merkle-tree), with the tree levels stored in order from
  root to leaf, and the tree blocks within each level stored in their
  natural order.
* Zero-padding to the next filesystem block boundary.
* The verity descriptor, as documented in
  [Documentation/filesystems/fsverity.rst](https://docs.kernel.org/filesystems/fsverity.md#fsverity-descriptor),
  with optionally appended signature blob.
* Zero-padding to the next offset that is 4 bytes before a filesystem
  block boundary.
* The size of the verity descriptor in bytes, as a 4-byte little
  endian integer.

Verity inodes have EXT4\_VERITY\_FL set, and they must use extents, i.e.
EXT4\_EXTENTS\_FL must be set and EXT4\_INLINE\_DATA\_FL must be clear.
They can have EXT4\_ENCRYPT\_FL set, in which case the verity metadata
is encrypted as well as the data itself.

Verity files cannot have blocks allocated past the end of the verity
metadata.

Verity and DAX are not compatible and attempts to set both of these flags
on a file will fail.
