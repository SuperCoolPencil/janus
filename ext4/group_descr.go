package ext4

// Group descriptors if present are the second structure in the block group.
// group descriptor records the location of both bitmaps and the inode table
// within a block group, the only data structures with fixed locations are the
// superblock and the group descriptor table.
