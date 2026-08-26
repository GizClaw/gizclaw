package buffer

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrIteratorDone is returned when iteration is complete.
var ErrIteratorDone = errors.New("iterator done")

// Buffer is a thread-safe growable buffer.
type Buffer[T any] struct {
	writeNotify chan struct{}

	mu         sync.Mutex
	closeWrite bool
	closeErr   error
	buf        []T
	ids        []uint64
	nextID     uint64
}

// N creates a new Buffer with the specified initial capacity.
func N[T any](n int) *Buffer[T] {
	return &Buffer[T]{
		writeNotify: make(chan struct{}, 1),
		buf:         make([]T, 0, n),
	}
}

func (b *Buffer[T]) Write(p []T) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closeErr != nil {
		return 0, fmt.Errorf("buffer: write to closed buffer: %w", b.closeErr)
	}
	if b.closeWrite {
		return 0, fmt.Errorf("buffer: write to closed buffer: %w", io.ErrClosedPipe)
	}
	select {
	case b.writeNotify <- struct{}{}:
	default:
	}

	b.buf = append(b.buf, p...)
	for range p {
		b.nextID++
		b.ids = append(b.ids, b.nextID)
	}
	return len(p), nil
}

func (b *Buffer[T]) Discard(n int) (err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closeErr != nil {
		return fmt.Errorf("buffer: skip from closed buffer: %w", b.closeErr)
	}
	if n > len(b.buf) {
		b.buf = b.buf[:0]
		b.ids = b.ids[:0]
		return nil
	}
	b.buf = b.buf[n:]
	b.ids = b.ids[n:]
	return nil
}

func (b *Buffer[T]) Read(p []T) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closeErr != nil {
		return 0, fmt.Errorf("buffer: read from closed buffer: %w", b.closeErr)
	}

	for len(b.buf) == 0 {
		if b.closeWrite {
			return 0, io.EOF
		}
		b.mu.Unlock()
		<-b.writeNotify
		b.mu.Lock()
		if b.closeErr != nil {
			return 0, fmt.Errorf("buffer: read from closed buffer: %w", b.closeErr)
		}
	}
	n = copy(p, b.buf)
	b.buf = b.buf[n:]
	b.ids = b.ids[n:]
	return n, nil
}

func (b *Buffer[T]) closeWithErrorLocked(err error) error {
	if b.closeErr != nil {
		return nil
	}
	b.closeErr = err
	b.buf = nil
	b.ids = nil
	if !b.closeWrite {
		b.closeWrite = true
		close(b.writeNotify)
	}
	return nil
}

func (b *Buffer[T]) CloseWithError(err error) error {
	if err == nil {
		err = io.ErrClosedPipe
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeWithErrorLocked(err)
}

func (b *Buffer[T]) Error() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeErr
}

func (b *Buffer[T]) Close() error {
	return b.CloseWithError(io.ErrClosedPipe)
}

func (b *Buffer[T]) CloseWrite() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closeWrite {
		return nil
	}
	b.closeWrite = true
	close(b.writeNotify)
	return nil
}

func (b *Buffer[T]) Next() (t T, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closeErr != nil {
		err = fmt.Errorf("buffer: read from closed buffer: %w", b.closeErr)
		return
	}
	for len(b.buf) == 0 {
		if b.closeWrite {
			err = ErrIteratorDone
			return
		}
		b.mu.Unlock()
		<-b.writeNotify
		b.mu.Lock()
		if b.closeErr != nil {
			err = fmt.Errorf("buffer: read from closed buffer: %w", b.closeErr)
			return
		}
	}
	t = b.buf[0]
	b.buf = b.buf[1:]
	b.ids = b.ids[1:]
	return
}

func (b *Buffer[T]) Add(t T) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closeErr != nil {
		return fmt.Errorf("buffer: write to closed buffer: %w", b.closeErr)
	}
	if b.closeWrite {
		return fmt.Errorf("buffer: write to closed buffer: %w", io.ErrClosedPipe)
	}
	b.buf = append(b.buf, t)
	b.nextID++
	b.ids = append(b.ids, b.nextID)
	select {
	case b.writeNotify <- struct{}{}:
	default:
	}
	return nil
}

func (b *Buffer[T]) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = b.buf[:0]
	b.ids = b.ids[:0]
}

// RemoveIf removes buffered values that match predicate while preserving the order of the rest.
func (b *Buffer[T]) RemoveIf(predicate func(T) bool) int {
	if predicate == nil {
		return 0
	}
	b.mu.Lock()
	values := append([]T(nil), b.buf...)
	ids := append([]uint64(nil), b.ids...)
	b.mu.Unlock()

	removeIDs := make(map[uint64]struct{})
	for i, value := range values {
		if predicate(value) {
			removeIDs[ids[i]] = struct{}{}
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.buf[:0]
	keptIDs := b.ids[:0]
	removed := 0
	for i, value := range b.buf {
		if _, remove := removeIDs[b.ids[i]]; remove {
			removed++
			continue
		}
		kept = append(kept, value)
		keptIDs = append(keptIDs, b.ids[i])
	}
	var zero T
	for i := len(kept); i < len(b.buf); i++ {
		b.buf[i] = zero
	}
	b.buf = kept
	clear(b.ids[len(keptIDs):])
	b.ids = keptIDs
	return removed
}

func (b *Buffer[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

func (b *Buffer[T]) Bytes() []T {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf
}
