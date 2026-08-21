package giztest

import (
	"context"
	"fmt"
	"sync"
)

type taskBarrier struct {
	total, arrived int
	done           chan struct{}
	completed      bool
	err            error
	mu             sync.Mutex
}

func newTaskBarrier(total int) *taskBarrier {
	return &taskBarrier{total: total, done: make(chan struct{})}
}
func (b *taskBarrier) Wait(ctx context.Context) error {
	b.mu.Lock()
	if b.completed {
		err := b.err
		b.mu.Unlock()
		if err != nil {
			return err
		}
		return fmt.Errorf("barrier received too many participants")
	}
	b.arrived++
	if b.arrived == b.total {
		b.completed = true
		close(b.done)
	}
	done := b.done
	b.mu.Unlock()
	select {
	case <-done:
		b.mu.Lock()
		err := b.err
		b.mu.Unlock()
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (b *taskBarrier) Abort(err error) {
	if b == nil || err == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.completed {
		return
	}
	b.err = fmt.Errorf("barrier aborted before all participants arrived: %w", err)
	b.completed = true
	close(b.done)
}
