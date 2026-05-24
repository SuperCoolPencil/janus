# Extents and Extent Allocation in Ext4

## Introduction

In a filesystem, a file might seem like it is stored continuously on disk, but that’s not the case. The contents of a file are logically continuous but may not be physically contiguous and can be scattered across different locations on the disk. This poses a challenge for the filesystem when managing the creation, growth, and deletion of files. In this document, we’ll explore how ext4 addresses this challenge using extents to map logical blocks to physical blocks. We’ll also look at the different steps involved in extent allocation in an ext4 filesystem, and finally, we’ll discuss the significance of delayed allocation.

By the end of this document, you’ll understand how filesystems manage space while keeping files logically continuous, even when their data is scattered across the disk.

Let’s get started with an example.

### Example

Let’s assume, on a fresh filesystem three files (say, `file1`, `file2`, and `file3`) are created one after another, each occupying 16 KiB.

On disk, they might look like this:

![Unfragmented Files](https://blogs.oracle.com/linux/wp-content/uploads/sites/49/2025/10/Unfragmented-files.png)

As of now, the contents of each file are stored continuously on-disk.

Now, let’s say the user adds more data to `file2`. To accommodate this new data, a new on-disk block is needed. However, as seen in the image, `file2` is surrounded by `file1` on one side and `file3` on the other. So, where can the new data be placed?

One option is to move `file3` by one on-disk block, thus creating space for `file2` to use. This may seem like a good idea, but it is very inefficient. In a filesystem with hundreds or thousands of files, this approach would involve moving numerous files just to make space for a single physical block.

The approach that filesystems use is to allocate the next available physical block on the disk and update the [extent map](https://ext4.wiki.kernel.org/index.php/Ext4_Disk_Layout#Extent_Tree) (if the filesystem uses extents) or the [block addressing map](https://ext4.wiki.kernel.org/index.php/Ext4_Disk_Layout#Direct.2FIndirect_Block_Addressing) (if the filesystem uses direct/indirect addressing). The filesystem logically maps this newly allocated physical block as the 5th logical block of `file2`.

The disk looks something like this after allocation:

![Fragmented Files](https://blogs.oracle.com/linux/wp-content/uploads/sites/49/2025/10/Fragmented-files.png)

The contents of `file2` are not physically continuous, but they are continuous logically.

The animation below shows how files are allocated on-disk space over time. As the files grow, available blocks are allocated.

![File Space Allocation Animation](https://blogs.oracle.com/linux/wp-content/uploads/sites/49/2025/10/allocation.gif)

### Extent-Based Mapping vs. Direct/Indirect Block Addressing

In indirect/direct block addressing, logical and physical blocks are mapped one-to-one, whereas in extent-based mapping, a range of logical blocks are mapped to a range of physical blocks using a single extent structure.

Here’s how the extents for `file1`, `file2`, and `file3` look:

```
File 1:
Extent    Logical Blocks    Physical Blocks    Length
  0            0 - 3           100 - 103         4

File 2:
Extent    Logical Blocks    Physical Blocks    Length
  0            0 - 3           104 - 107         4
  1              4                112            1

File 3:
Extent    Logical Blocks    Physical Blocks    Length
  0            0 - 3           108 - 111         4
```

Extent structure in an ext4 filesystem:

```c
struct ext4_extent {
    __le32  ee_block;       /* First logical block that the extent covers */
    __le16  ee_len;         /* Number of blocks covered by this extent */
    __le16  ee_start_hi;    /* High 16 bits of the starting physical block */
    __le32  ee_start_lo;    /* Low 32 bits of the starting physical block */
};
```

For example, if an extent has the following values:

```c
struct ext4_extent {
    __le32  ee_block = 5;
    __le16  ee_len = 10;
    __le16  ee_start_hi = 0;
    __le32  ee_start_lo = 100;
};
```

That means the extent maps logical blocks 5 to 15 of the file to physical blocks 100 to 110 on disk.

For more information about extents, refer to the "Extent Tree" section of the [Understanding Ext4 Disk Layout, Part 2](https://blogs.oracle.com/linux/post/understanding-ext4-disk-layout-part-2) blog, where a detailed explanation of extents is provided.

Now we know what an extent is and its purpose. Let’s dive into the extent allocation algorithm in ext4. We’ll cover how an extent is allocated, where the filesystem starts searching when a request is made, and what happens if there isn’t enough space to fulfill the request.

---

## Extent Allocation

[Ext4](https://en.wikipedia.org/wiki/Ext4) follows a simple algorithm to allocate extents.

When a request to allocate physical blocks for an [inode](https://en.wikipedia.org/wiki/Inode) is made, the process of extent allocation starts. The first step in this process is finding the **hint**.

What is a hint? A hint is a physical block number calculated by the filesystem. The search for free space starts from the hint block. The method for finding the value of the hint block is different for a new inode and an existing inode. The calculation is done as follows:

### Hint For Newly Created Inodes

The hint for a newly created inode comes from the block group in which the inode is located. The exact block number of the hint block is calculated as follows:

```
hint = (first block of the inode's block group) + [(current->pid)%16 * (number of blocks present in the inode's block group)/16]
```

#### Example
Let’s assume in a 1GiB filesystem, an inode present in the fourth block group is initialized for use, and let’s assume the PID of the current process is 1863.

* The number of blocks in the fourth block group of a 1GiB filesystem is 32,768.
* The first block number of the fourth block group is 131,072.

Therefore, the hint can be calculated as follows:

```
hint = 131,072 + [(1863)%16 * (32,768)/16]
     = 131,072 + [7 * 2,048]
     = 145,408
```

So, the hint for this inode will be the physical block 145,408. As mentioned earlier, this block comes from the same block group in which the inode is present, which is the fourth block group in this case.

### Hint For Existing Inodes

The hint for an existing inode will be the physical block adjacent to the last extent of the inode.

#### Example
Consider the extent map of an inode:

```
Level Entries       Logical            Physical Length Flags
 0/ 0   1/  2     0 -  2047  2916352 -  2918399   2048
 0/ 0   2/  2  2048 -  2559  2946560 -  2947071    512
```

Here, the last extent of the inode spans from physical block 2946560 to 2947071. The adjacent physical block to the last allocated extent is 2947072. Therefore, the hint for this inode will be 2947072.

---

## Block Search and Allocation

The filesystem uses the hint to determine where to start looking for free blocks. The ext4 filesystem maintains block bitmaps and buddy bitmaps for each block group. The block bitmap tracks which blocks are in use and which are free. The buddy bitmap tracks the buddy information, such as the number of buddies available of different orders.

You can see the buddy information of the filesystem in the file `/proc/fs/ext4/<device>/mb_groups`.

Below is a snippet showing buddy info from `mb_groups` on an ext4 filesystem:

![Buddy Info mb_groups](https://blogs.oracle.com/linux/wp-content/uploads/sites/49/2025/10/mb_groups-buddy_info.png)

The `mb_groups` file displays the buddy information for each block group within the filesystem. Each column represents the number of free buddies available of sizes 4 KiB, 8 KiB, 16 KiB (Order of 2).

In the highlighted first block group, we observe that it contains 4 4 KiB buddies ($2^0 \times 4\text{ KiB}$ (block size)), 5 8 KiB buddies ($2^1 \times 4\text{ KiB}$), and so on.

The filesystem first loads the block bitmap and buddy bitmap of the hint block’s block group and checks if the hint block is in use or not.

### If the Hint Block is Free
* **Case 1**: There is enough contiguous free space starting from the hint block to satisfy the requested size. In this case, these blocks will be allocated and are marked as used in both the buddy bitmap and the block bitmap. The allocation ends here.
* **Case 2**: There isn’t enough contiguous space, meaning the hint block is available but there isn’t enough continuous space starting from the hint block to satisfy the requested size. In this case, if the flag `EXT4_MB_HINT_MERGE` is set, the physical blocks available so far are used. If the flag `EXT4_MB_HINT_MERGE` is not set, these blocks are ignored, and the search continues.

### If the Hint Block is Not Free (or Case 2 above when EXT4_MB_HINT_MERGE is not set)
In these cases, the filesystem will search through every block group to find free space. The search starts from the block group of the hint block and progresses through each block group in the filesystem.

#### Example
Let’s assume the filesystem to be 1 GiB in size (it has 8 block groups in total). If the block group of the hint block is 4, the search progresses in this order: `4 -> 5 -> 6 -> 7 -> 0 -> 1 -> 2 -> 3`.

For each block group, the buddy bitmaps are loaded, and a check is done to find the best possible contiguous buddies. The best possible contiguous buddies are those that are contiguous and whose sum of sizes is closest to the requested size. If an exact match is found, it’s used. If not, the buddies that are closest to the requested size will be chosen. The sum of the sizes of these buddies may be lower or higher.

* If their sum is more than the requested size, the buddies will be split into lower-order buddies to match the requested size.
* If their sum is lower than the requested size, then the buddies found so far will be used and added to the extent tree. For the remaining blocks, the process for extent allocation goes back to step 1 (Calculation of hint), and this process continues until the requested size is found. If there isn’t enough space, then `ENOSPC` will be returned.

The maximum extent size supported by ext4 is 128 MiB, which is the size of a block group. If a request is made for 150 MiB, even if there is 150 MiB of contiguous space available, the allocation will be split into two steps. First, 128 MiB will be allocated (as this is the maximum extent size in ext4), and for the remaining 22 MiB, the algorithm will loop back and try to allocate it.

---

## Delayed Allocation

Ext4 supports delayed allocation, which is also the default allocation algorithm. In delayed allocation, on-disk blocks are not allocated immediately after a write operation. Instead, the buffer heads are marked with the `BH_Delay` flag and the physical blocks are allocated later during the write-back process.

The reason is that delaying the allocation could result in the accumulation of more data, which causes the allocation of more contiguous physical blocks. As a result, the number of seek operations is reduced, fragmentation is minimized, and the overall performance of the filesystem is improved. It also has an advantage with short-lived files (e.g., temp files), as the files might be deleted before allocation even happens.

To disable delayed allocation, the filesystem has to be mounted with the `nodelalloc` option.

### Filesystem without Delayed Allocation
```bash
[bash]# mount -o nodelalloc /dev/sdc1 /media/
[bash]#
[bash]# cd /media
[bash]# date; dd if=/dev/random of=test bs=4k count=10
Tue Aug 27 13:22:25 GMT 2024     <=================
10+0 records in
10+0 records out
40960 bytes (41 kB, 40 KiB) copied, 0.000281584 s, 145 MB/s
[bash]# date; debugfs -R "ex test" /dev/sdc1
Tue Aug 27 13:22:27 GMT 2024     <=================
debugfs 1.45.6 (20-Mar-2020)
Level Entries       Logical          Physical Length Flags
 0/ 0   1/  1     0 -     9   34304 -   34313     10
[bash]#
```
To mount a filesystem without delayed allocation use (`-o nodelalloc`).

A file named `test` was created, and the `debugfs ex` command was used to see its allocated extents. We see that physical blocks were allocated immediately (see the time, there was only a 2-second gap).

### Filesystem with Delayed Allocation
```bash
[bash]# mount /dev/sdc1 /media
[bash]#
[bash]# cd /media
[bash]#
[bash]# date; dd if=/dev/random of=test2 bs=4k count=10
Tue Aug 27 13:25:23 GMT 2024     <=================
10+0 records in
10+0 records out
40960 bytes (41 kB, 40 KiB) copied, 0.00021046 s, 195 MB/s
[bash]#
[bash]#
[bash]# date; debugfs -R "ex test2" /dev/sdc1
Tue Aug 27 13:25:26 GMT 2024     <=================
debugfs 1.45.6 (20-Mar-2020)
Level Entries       Logical          Physical Length Flags
[bash]#
[bash]#
[bash]# date; debugfs -R "ex test2" /dev/sdc1
Tue Aug 27 13:26:32 GMT 2024     <=================
debugfs 1.45.6 (20-Mar-2020)
Level Entries       Logical          Physical Length Flags
 0/ 0   1/  1     0 -     9   33795 -   33804     10
[bash]#
```
A file named `test2` was created on a filesystem with delayed allocation enabled. When the `debugfs ex` command was run immediately afterward (just a 3-second gap), it did not output any extents. That is because, with delayed allocation, the physical blocks are not allocated immediately. However, after some time (~1 min), we can see from the `debugfs ex` command that the physical blocks were allocated.

---

## Summary

In this document, we explored how filesystems use logical-to-physical mapping to present data continuously to the user. We covered the different steps of the extent allocation process, which involve finding hints and searching for free space. At the end, we also discussed the importance of delayed allocation in improving performance and reducing fragmentation.

## References

* [Ext4 Wiki: Delayed Allocation](https://ext4.wiki.kernel.org/index.php/DelayedAllocation)
* [Understanding Ext4 Disk Layout, Part 2](https://blogs.oracle.com/linux/post/understanding-ext4-disk-layout-part-2)
* [Linux Source Code](https://github.com/torvalds/linux/tree/master/fs/ext4)
