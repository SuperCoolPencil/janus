# ext4 Data Structures and Algorithms — The Linux Kernel  documentation

[![Logo](https://docs.kernel.org/_static/logo.svg)](https://docs.kernel.org/index.md)

# [The Linux Kernel](https://docs.kernel.org/index.md)

7.1.0-rc4



# ext4 Data Structures and Algorithms[¶](index.md#ext4-data-structures-and-algorithms "Permalink to this heading")

* [1. About this Book](about.md)
  + [1.1. License](about.md#license)
  + [1.2. Terminology](about.md#terminology)
  + [1.3. Other References](about.md#other-references)
* [2. High Level Design](overview.md)
  + [2.1. Blocks](blocks.md)
  + [2.2. Block Groups](blockgroup.md)
  + [2.3. Special inodes](special_inodes.md)
  + [2.4. Block and Inode Allocation Policy](allocators.md)
  + [2.5. Checksums](checksums.md)
  + [2.6. Bigalloc](bigalloc.md)
  + [2.7. Inline Data](inlinedata.md)
  + [2.8. Large Extended Attribute Values](eainode.md)
  + [2.9. Verity files](verity.md)
  + [2.10. Atomic Block Writes](atomic_writes.md)
* [3. Global Structures](globals.md)
  + [3.1. Super Block](super.md)
  + [3.2. Block Group Descriptors](group_descr.md)
  + [3.3. Block and inode Bitmaps](bitmaps.md)
  + [3.4. Inode Table](inode_table.md)
  + [3.5. Multiple Mount Protection](mmp.md)
  + [3.6. Journal (jbd2)](journal.md)
  + [3.7. Orphan file](orphan.md)
* [4. Dynamic Structures](dynamic.md)
  + [4.1. Index Nodes](inodes.md)
  + [4.2. The Contents of inode.i\_block](ifork.md)
  + [4.3. Directory Entries](directory.md)
  + [4.4. Extended Attributes](attributes.md)
