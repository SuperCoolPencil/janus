// Package disk - aligned I/O wrapper for Windows raw disk handles.
//
// Windows requires that all reads from a raw physical disk handle
// (\\.\PhysicalDriveN) satisfy two constraints:
//
//  1. The byte offset must be a multiple of the sector size.
//  2. The buffer length must be a multiple of the sector size.
//
// If either rule is violated the kernel returns ERROR_INVALID_PARAMETER
// ("The parameter is incorrect.").
//
// AlignedReaderAt wraps any io.ReaderAt and transparently rounds every
// ReadAt request outward to sector boundaries, reading whole sectors into
// a temporary buffer and copying only the requested slice back to the
// caller.  Code above this layer (the ext4 parser, PartitionReader, …)
// can continue issuing arbitrarily-sized, arbitrarily-aligned reads.

package disk

import (
	"fmt"
	"io"
)

// AlignedReaderAt wraps an io.ReaderAt so that every physical read is
// aligned to sectorSize boundaries.  The caller sees the same logical
// byte stream - alignment is handled internally.
type AlignedReaderAt struct {
	inner      io.ReaderAt
	sectorSize int64
}

// NewAlignedReaderAt returns an AlignedReaderAt that rounds all reads on
// inner to multiples of sectorSize.
//
// sectorSize must be a positive power of two (typically 512 or 4096).
func NewAlignedReaderAt(inner io.ReaderAt, sectorSize int64) (*AlignedReaderAt, error) {
	if sectorSize <= 0 || sectorSize&(sectorSize-1) != 0 {
		return nil, fmt.Errorf("disk: sector size %d is not a positive power of two", sectorSize)
	}
	return &AlignedReaderAt{
		inner:      inner,
		sectorSize: sectorSize,
	}, nil
}

// ReadAt implements io.ReaderAt.
//
// The caller's (buf, off) pair is translated to a sector-aligned read:
//
//	alignedStart = off rounded down to the nearest sector boundary
//	alignedEnd   = (off + len(buf)) rounded up  to the nearest sector boundary
//
// A temporary buffer spanning [alignedStart, alignedEnd) is read from
// the underlying device, and the requested slice is copied into buf.
func (a *AlignedReaderAt) ReadAt(buf []byte, off int64) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}

	ss := a.sectorSize

	// Round the start offset DOWN to the nearest sector boundary.
	alignedStart := (off / ss) * ss

	// Round the end offset UP to the next sector boundary.
	end := off + int64(len(buf))
	alignedEnd := ((end + ss - 1) / ss) * ss

	alignedLen := alignedEnd - alignedStart

	// Fast path: if the request is already perfectly aligned, read
	// directly into the caller's buffer to avoid the extra copy.
	if alignedStart == off && alignedLen == int64(len(buf)) {
		return a.inner.ReadAt(buf, off)
	}

	// Slow path: read whole sectors into a temporary buffer.
	tmp := make([]byte, alignedLen)
	n, err := a.inner.ReadAt(tmp, alignedStart)

	// Calculate how far into `tmp` the caller's data starts.
	skip := int(off - alignedStart)

	// How many useful bytes we can copy back.
	usable := n - skip
	if usable < 0 {
		usable = 0
	}
	if usable > len(buf) {
		usable = len(buf)
	}

	copied := copy(buf[:usable], tmp[skip:skip+usable])

	// Translate the error: if we filled the caller's buffer completely,
	// suppress any io.EOF from the aligned over-read.
	if copied == len(buf) {
		return copied, nil
	}

	// If the underlying read returned an error (including EOF), and we
	// could not fill the caller's buffer, propagate it.
	if err != nil {
		return copied, err
	}

	// Partial read with no error from inner - shouldn't normally happen
	// with disk devices, but handle it gracefully.
	return copied, io.EOF
}
