package ext4

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Ext4 uses a B-tree of extents to map logical file blocks to physical disk
// blocks. The tree is stored directly in the inode's 60-byte I_block field
// when it fits (depth 0), or spills into additional "index" blocks on disk
// when the file is large or fragmented (depth > 0).
//
// An extent (struct ext4_extent) describes a run of *contiguous* physical
// blocks. A single extent can cover at most 32,768 blocks (~128 MiB with
// 4 KiB blocks).
//
// Tree structure
// ──────────────
//   I_block[0..59]  (60 bytes embedded in the inode)
//   ├── ExtentHeader  (12 bytes)
//   │     EH_magic      = 0xF30A
//   │     EH_entries    = number of valid entries that follow
//   │     EH_max        = max entries that fit in this node
//   │     EH_depth      = 0 at leaf nodes, >0 at interior nodes
//   │     EH_generation = snapshot generation (we ignore this)
//   │
//   ├── [EH_depth == 0] Extent  × EH_entries   (leaf node — physical blocks)
//   └── [EH_depth >  0] ExtentIdx × EH_entries  (interior node — child blocks)
//
// See docs: https://github.com/SuperCoolPencil/janus/blob/master/docs/ext4/extent_allocation.md

// ExtentHeader is the 12-byte header present at the start of every extent
// tree node — both the inline node in the inode and any on-disk index blocks.
//
// On-disk layout:
//
//	0x0  __le16  EH_magic      — must be 0xF30A (EXT4_EXT_MAGIC)
//	0x2  __le16  EH_entries    — number of valid entries following this header
//	0x4  __le16  EH_max        — maximum entries that fit in this node
//	0x6  __le16  EH_depth      — 0 = leaf (Extent entries), >0 = interior (ExtentIdx entries)
//	0x8  __le32  EH_generation — snapshot generation (unused here)
type ExtentHeader struct {
	EH_magic      uint16
	EH_entries    uint16
	EH_max        uint16
	EH_depth      uint16
	EH_generation uint32
}

// Extent is a leaf node entry. Each entry maps a contiguous run of logical
// file blocks to a contiguous run of physical disk blocks.
//
// On-disk layout (12 bytes):
//
//	0x0  __le32  EE_block     — first logical block number covered by this extent
//	0x4  __le16  EE_len       — number of blocks in the run (max 32768)
//	0x6  __le16  EE_start_hi  — high 16 bits of the starting physical block number
//	0x8  __le32  EE_start_lo  — low 32 bits of the starting physical block number
//
// The full 48-bit physical start is: (EE_start_hi << 32) | EE_start_lo
type Extent struct {
	EE_block    uint32
	EE_len      uint16
	EE_start_hi uint16
	EE_start_lo uint32
}

// ExtentIdx is an interior (index) node entry. It does not hold physical data
// blocks itself; instead it points to another extent tree node (either another
// interior node or a leaf node) stored in a separate physical block.
//
// On-disk layout (12 bytes):
//
//	0x0  __le32  EI_block   — first logical block number covered by the subtree
//	0x4  __le32  EI_leaf_lo — low 32 bits of the physical block holding the child node
//	0x8  __le16  EI_leaf_hi — high 16 bits of the physical block holding the child node
//	0xA  __u16   EI_unused
//
// The full 48-bit child block is: (EI_leaf_hi << 32) | EI_leaf_lo
type ExtentIdx struct {
	EI_block   uint32
	EI_leaf_lo uint32
	EI_leaf_hi uint16
	EI_unused  uint16
}

// EXT4_EXT_MAGIC is the magic number that must appear in every ExtentHeader.
// Its presence distinguishes an extent tree node from stale/uninitialized data.
const EXT4_EXT_MAGIC uint16 = 0xF30A

// extentHeaderSize and extentEntrySize are the byte sizes of the fixed
// structures. Each node begins with one ExtentHeader (12 bytes) followed by
// EH_entries entries of either Extent or ExtentIdx (12 bytes each).
const (
	extentHeaderSize = 12
	extentEntrySize  = 12
)

// ReadExtents walks the extent B-tree rooted in inode.I_block and returns
// all leaf Extent records in logical block order.
//
// For most small files and all small directories (including the root
// directory) the tree has depth 0 and fits entirely inside the inode — no
// additional disk reads are needed. For larger or more fragmented files the
// tree can be up to 5 levels deep; each interior node is stored in one full
// filesystem block.
//
// The inode MUST have the EXT4_EXTENTS_FL flag set (0x80000) — this is
// always true for directories and for files created by modern kernels.
// If the flag is absent the inode uses the legacy indirect block scheme,
// which we do not yet support.
func (fs *FileSystem) ReadExtents(inode *Inode) ([]Extent, error) {
	// Guard: the EXT4_EXTENTS_FL flag (0x80000) in I_flags tells us that
	// I_block holds an extent tree header. Without this flag, I_block holds
	// the legacy ext2/ext3 direct/indirect block pointer table instead.
	// Parsing one format as the other would produce completely wrong results.
	// We return a clear error so the caller knows exactly what happened.
	if !inode.UsesExtents() {
		return nil, fmt.Errorf(
			"inode uses legacy indirect block addressing (EXT4_EXTENTS_FL not set in I_flags=0x%08x); "+
				"indirect block scheme is not yet supported",
			inode.I_flags,
		)
	}

	// The 60-byte I_block field is the root of the extent tree.
	// Parse it as a byte slice so we can share parseExtentNode with the
	// recursive on-disk-block case.
	return fs.parseExtentNode(inode.I_block[:])
}

// parseExtentNode decodes one extent tree node from a raw byte slice.
// The slice must begin with an ExtentHeader; what follows depends on depth:
//   - depth == 0 → EH_entries × Extent  (leaf; we collect and return them)
//   - depth >  0 → EH_entries × ExtentIdx (interior; recurse into each child)
func (fs *FileSystem) parseExtentNode(data []byte) ([]Extent, error) {
	if len(data) < extentHeaderSize {
		return nil, fmt.Errorf("extent node too small: %d bytes (need at least %d)", len(data), extentHeaderSize)
	}

	var hdr ExtentHeader
	if err := binary.Read(bytes.NewReader(data[:extentHeaderSize]), binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("failed to decode extent header: %w", err)
	}

	if hdr.EH_magic != EXT4_EXT_MAGIC {
		return nil, fmt.Errorf("bad extent magic: expected 0x%04X, got 0x%04X", EXT4_EXT_MAGIC, hdr.EH_magic)
	}

	if hdr.EH_depth == 0 {
		// ── Leaf node ──────────────────────────────────────────────────────────
		// Each entry is an Extent (logical block → physical run mapping).
		return parseLeafEntries(data[extentHeaderSize:], hdr.EH_entries)
	}

	// ── Interior node ──────────────────────────────────────────────────────
	// Each entry is an ExtentIdx pointing to a child node in a disk block.
	// We collect all leaf extents from all subtrees.
	var all []Extent

	for i := range uint16(hdr.EH_entries) {
		entryOffset := extentHeaderSize + int(i)*extentEntrySize
		if entryOffset+extentEntrySize > len(data) {
			return nil, fmt.Errorf("extent index %d extends beyond node data", i)
		}

		var idx ExtentIdx
		if err := binary.Read(
			bytes.NewReader(data[entryOffset:entryOffset+extentEntrySize]),
			binary.LittleEndian, &idx,
		); err != nil {
			return nil, fmt.Errorf("failed to decode extent index %d: %w", i, err)
		}

		// Reconstruct the 48-bit physical block number of the child node.
		childBlock := (uint64(idx.EI_leaf_hi) << 32) | uint64(idx.EI_leaf_lo)
		childOffset := int64(childBlock * fs.BlockSize)

		// Read the full child node block from disk.
		childBuf := make([]byte, fs.BlockSize)
		if _, err := fs.dev.ReadAt(childBuf, childOffset); err != nil {
			return nil, fmt.Errorf(
				"failed to read extent index block %d (offset %d): %w",
				childBlock, childOffset, err,
			)
		}

		// Recurse — the child may itself be an interior node or a leaf.
		childExtents, err := fs.parseExtentNode(childBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to parse child extent node at block %d: %w", childBlock, err)
		}
		all = append(all, childExtents...)
	}

	return all, nil
}

// parseLeafEntries decodes EH_entries Extent records from the raw bytes that
// immediately follow the ExtentHeader in a leaf node.
func parseLeafEntries(data []byte, count uint16) ([]Extent, error) {
	extents := make([]Extent, 0, count)

	for i := range uint16(count) {
		offset := int(i) * extentEntrySize
		if offset+extentEntrySize > len(data) {
			return nil, fmt.Errorf("extent entry %d extends beyond leaf node data", i)
		}

		var ext Extent
		if err := binary.Read(
			bytes.NewReader(data[offset:offset+extentEntrySize]),
			binary.LittleEndian, &ext,
		); err != nil {
			return nil, fmt.Errorf("failed to decode extent entry %d: %w", i, err)
		}
		extents = append(extents, ext)
	}

	return extents, nil
}
