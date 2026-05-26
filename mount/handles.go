package mount

import (
	"sync"
	"sync/atomic"
)

// HandleTable is a thread-safe map from a uint64 FUSE handle to an arbitrary value.
type HandleTable[V any] struct {
	// mu is the map holding active handle mappings.
	mu sync.Map

	// next is the next handle ID to allocate.
	next atomic.Uint64
}

// Store saves a value associated with a newly allocated handle and returns
// the handle. The handle is unique for the lifetime of this HandleTable.
//
// Callers should use the returned handle as the `fh` argument in cgofuse
// OpenOut so that subsequent Read/Release calls receive it back.
func (t *HandleTable[V]) Store(v V) uint64 {
	// Atomically increment handle ID.
	h := t.next.Add(1)
	t.mu.Store(h, v)
	return h
}

// Load retrieves the value associated with handle h.
//
// Returns (value, true) if the handle is active, or (zero, false) if the
// handle is unknown (e.g. it was never allocated, or Release has already
// been called for it).
//
// Returns (value, true) if found, or (zero, false) if not.
func (t *HandleTable[V]) Load(h uint64) (V, bool) {
	raw, ok := t.mu.Load(h)
	if !ok {
		var zero V
		return zero, false
	}
	return raw.(V), true
}

// Delete removes the handle from the table, releasing any reference to the
// stored value.
//
// Delete removes the handle from the table.
func (t *HandleTable[V]) Delete(h uint64) {
	t.mu.Delete(h)
}
