// Package keyedlock provides context-aware, reference-counted locks for
// resource identities.
package keyedlock

import (
	"context"
	"sync"
)

// Locker serializes callers that acquire the same key while allowing distinct
// keys to make progress independently. Idle key entries are removed.
type Locker[K comparable] struct {
	mu      sync.Mutex
	entries map[K]*entry
}

type entry struct {
	token chan struct{}
	refs  int
}

// Acquire waits for exclusive ownership of key. The returned release function
// is idempotent.
func (locker *Locker[K]) Acquire(ctx context.Context, key K) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	locker.mu.Lock()
	if locker.entries == nil {
		locker.entries = make(map[K]*entry)
	}
	item := locker.entries[key]
	if item == nil {
		item = &entry{token: make(chan struct{}, 1)}
		locker.entries[key] = item
	}
	item.refs++
	locker.mu.Unlock()

	select {
	case item.token <- struct{}{}:
		// Cancellation may race with the previous holder returning the token.
		// A select can choose the token even when ctx.Done is also ready, so
		// recheck after acquisition before publishing ownership to the caller.
		if err := ctx.Err(); err != nil {
			<-item.token
			locker.releaseReference(key, item)
			return nil, err
		}
		var once sync.Once
		return func() {
			once.Do(func() {
				<-item.token
				locker.releaseReference(key, item)
			})
		}, nil
	case <-ctx.Done():
		locker.releaseReference(key, item)
		return nil, ctx.Err()
	}
}

func (locker *Locker[K]) releaseReference(key K, item *entry) {
	locker.mu.Lock()
	item.refs--
	if item.refs == 0 && locker.entries[key] == item {
		delete(locker.entries, key)
	}
	locker.mu.Unlock()
}

// ActiveKeys reports the number of keys with a holder or waiter.
func (locker *Locker[K]) ActiveKeys() int {
	locker.mu.Lock()
	defer locker.mu.Unlock()
	return len(locker.entries)
}
