package giznet

import "sync"

// WriteBudget bounds outstanding bytes shared across a group of reliable
// channels. It only keeps the account; each transport owns how a writer waits
// for capacity and when its reservation drains.
type WriteBudget struct {
	limit uint64
	mu    sync.Mutex
	used  uint64
	wake  chan struct{}
}

// NewWriteBudget constructs a shared outstanding-byte budget.
func NewWriteBudget(limit uint64) *WriteBudget {
	return &WriteBudget{limit: limit, wake: make(chan struct{})}
}

// Limit reports the configured byte limit.
func (b *WriteBudget) Limit() uint64 {
	if b == nil {
		return 0
	}
	return b.limit
}

// Used reports the currently reserved byte count.
func (b *WriteBudget) Used() uint64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

// TryAcquire reserves size bytes when they fit. When they do not, it returns
// a channel that is closed by the next Release so the caller can retry. A nil
// budget or zero size always succeeds; a size above the limit can never fit
// and returns ErrPacketTooLarge.
func (b *WriteBudget) TryAcquire(size uint64) (acquired bool, wake <-chan struct{}, err error) {
	if b == nil || size == 0 {
		return true, nil, nil
	}
	if size > b.limit {
		return false, nil, ErrPacketTooLarge
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used <= b.limit-size {
		b.used += size
		return true, nil, nil
	}
	return false, b.wake, nil
}

// Release returns size reserved bytes and wakes every waiting writer.
func (b *WriteBudget) Release(size uint64) {
	if b == nil || size == 0 {
		return
	}
	b.mu.Lock()
	if size > b.used {
		size = b.used
	}
	b.used -= size
	close(b.wake)
	b.wake = make(chan struct{})
	b.mu.Unlock()
}
