# 3. Global Structures — The Linux Kernel  documentation

[![Logo](https://docs.kernel.org/_static/logo.svg)](https://docs.kernel.org/index.md)

# [The Linux Kernel](https://docs.kernel.org/index.md)

7.1.0-rc4



# 3. Global Structures[¶](globals.md#global-structures "Permalink to this heading")

The filesystem is sharded into a number of block groups, each of which
have static metadata at fixed locations.

* [3.1. Super Block](super.md)
* [3.2. Block Group Descriptors](group_descr.md)
* [3.3. Block and inode Bitmaps](bitmaps.md)
* [3.4. Inode Table](inode_table.md)
* [3.5. Multiple Mount Protection](mmp.md)
* [3.6. Journal (jbd2)](journal.md)
  + [3.6.1. Layout](journal.md#layout)
  + [3.6.2. External Journal](journal.md#external-journal)
  + [3.6.3. Block Header](journal.md#block-header)
  + [3.6.4. Super Block](journal.md#super-block)
  + [3.6.5. Descriptor Block](journal.md#descriptor-block)
  + [3.6.6. Data Block](journal.md#data-block)
  + [3.6.7. Revocation Block](journal.md#revocation-block)
  + [3.6.8. Commit Block](journal.md#commit-block)
  + [3.6.9. Fast commits](journal.md#fast-commits)
  + [3.6.10. Fast Commit Replay Idempotence](journal.md#fast-commit-replay-idempotence)
  + [3.6.11. Journal Checkpoint](journal.md#journal-checkpoint)
* [3.7. Orphan file](orphan.md)

|
& [Alabaster 0.7.16](https://alabaster.readthedocs.io)
|
