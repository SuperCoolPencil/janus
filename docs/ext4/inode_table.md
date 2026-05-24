# 3.4. Inode Table — The Linux Kernel  documentation

[![Logo](https://docs.kernel.org/_static/logo.svg)](https://docs.kernel.org/index.md)

# [The Linux Kernel](https://docs.kernel.org/index.md)

7.1.0-rc4



# 3.4. Inode Table[¶](inode_table.md#inode-table "Permalink to this heading")

Inode tables are statically allocated at mkfs time. Each block group
descriptor points to the start of the table, and the superblock records
the number of inodes per group. See [inode documentation](inodes.md)
for more information on inode table layout.
