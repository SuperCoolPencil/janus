# 2. High Level Design - The Linux Kernel  documentation

[![Logo](https://docs.kernel.org/_static/logo.svg)](https://docs.kernel.org/index.md)

# [The Linux Kernel](https://docs.kernel.org/index.md)

7.1.0-rc4



# 2. High Level Design[¶](overview.md#high-level-design "Permalink to this heading")

An ext4 file system is split into a series of block groups. To reduce
performance difficulties due to fragmentation, the block allocator tries
very hard to keep each file’s blocks within the same group, thereby
reducing seek times. The size of a block group is specified in
`sb.s_blocks_per_group` blocks, though it can also calculated as 8 \*
`block_size_in_bytes`. With the default block size of 4KiB, each group
will contain 32,768 blocks, for a length of 128MiB. The number of block
groups is the size of the device divided by the size of a block group.

All fields in ext4 are written to disk in little-endian order. HOWEVER,
all fields in jbd2 (the journal) are written to disk in big-endian
order.

* [2.1. Blocks](blocks.md)
* [2.2. Block Groups](blockgroup.md)
  + [2.2.1. Layout](blockgroup.md#layout)
  + [2.2.2. Flexible Block Groups](blockgroup.md#flexible-block-groups)
  + [2.2.3. Meta Block Groups](blockgroup.md#meta-block-groups)
  + [2.2.4. Lazy Block Group Initialization](blockgroup.md#lazy-block-group-initialization)
* [2.3. Special inodes](special_inodes.md)
* [2.4. Block and Inode Allocation Policy](allocators.md)
* [2.5. Checksums](checksums.md)
* [2.6. Bigalloc](bigalloc.md)
* [2.7. Inline Data](inlinedata.md)
  + [2.7.1. Inline Directories](inlinedata.md#inline-directories)
* [2.8. Large Extended Attribute Values](eainode.md)
* [2.9. Verity files](verity.md)
* [2.10. Atomic Block Writes](atomic_writes.md)
  + [2.10.1. Introduction](atomic_writes.md#introduction)
  + [2.10.2. Requirements](atomic_writes.md#requirements)
  + [2.10.3. Multi-fsblock Implementation Details](atomic_writes.md#multi-fsblock-implementation-details)
  + [2.10.4. Handling Split Extents Across Leaf Blocks](atomic_writes.md#handling-split-extents-across-leaf-blocks)
  + [2.10.5. Handling Journal transactions](atomic_writes.md#handling-journal-transactions)
  + [2.10.6. How to](atomic_writes.md#how-to)
    - [2.10.6.1. Creating Filesystems with Atomic Write Support](atomic_writes.md#creating-filesystems-with-atomic-write-support)
    - [2.10.6.2. Application Interface](atomic_writes.md#application-interface)
  + [2.10.7. Hardware Support](atomic_writes.md#hardware-support)
  + [2.10.8. See Also](atomic_writes.md#see-also)
* [2.11. Extents and Extent Allocation](extent_allocation.md)

