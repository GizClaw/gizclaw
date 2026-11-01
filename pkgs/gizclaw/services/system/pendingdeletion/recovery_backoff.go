package pendingdeletion

import (
	"container/list"
	"sync"
	"time"
)

const (
	failedRecoveryCacheCapacity = 4096
	failedRecoveryRetryInitial  = 30 * time.Second
	failedRecoveryRetryMax      = 30 * time.Minute
)

type failedRecoveryKey struct {
	source     string
	deletionID string
}

type failedRecoveryProbe struct {
	key      failedRecoveryKey
	observed Task
	element  *list.Element
	next     time.Time
	failures int
	inFlight bool
	warned   bool
	lastCode string
}

type failedRecoveryBackoff struct {
	mu       sync.Mutex
	entries  map[failedRecoveryKey]*failedRecoveryProbe
	order    list.List
	capacity int
}

// begin reserves one observation without holding a lock across the domain
// probe. A changed failed observation bypasses an older observation's delay.
func (b *failedRecoveryBackoff) begin(task Task, now time.Time) *failedRecoveryProbe {
	key := failedRecoveryKey{source: task.Source, deletionID: task.Record.DeletionID}
	b.mu.Lock()
	defer b.mu.Unlock()
	if previous := b.entries[key]; previous != nil {
		if previous.inFlight {
			return nil
		}
		if sameFailedObservation(previous.observed, task) {
			b.order.MoveToFront(previous.element)
			if now.Before(previous.next) {
				return nil
			}
			previous.inFlight = true
			return previous
		}
		b.remove(previous)
	}
	capacity := b.capacity
	if capacity <= 0 {
		capacity = failedRecoveryCacheCapacity
	}
	if len(b.entries) >= capacity {
		var evicted *failedRecoveryProbe
		for element := b.order.Back(); element != nil; element = element.Prev() {
			if candidate := element.Value.(*failedRecoveryProbe); !candidate.inFlight {
				evicted = candidate
				break
			}
		}
		if evicted == nil {
			return nil
		}
		b.remove(evicted)
	}
	if b.entries == nil {
		b.entries = make(map[failedRecoveryKey]*failedRecoveryProbe)
	}
	observed := task
	// sameFailedObservation needs the deletion identity, not the descriptor
	// or resource snapshot. Do not retain those payloads in the bounded cache.
	observed.Record = Record{DeletionID: task.Record.DeletionID}
	probe := &failedRecoveryProbe{key: key, observed: observed, inFlight: true}
	probe.element = b.order.PushFront(probe)
	b.entries[key] = probe
	return probe
}

func (b *failedRecoveryBackoff) shouldWarn(probe *failedRecoveryProbe, code string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries[probe.key] != probe || probe.warned && probe.lastCode == code {
		return false
	}
	probe.warned, probe.lastCode = true, code
	return true
}

func (b *failedRecoveryBackoff) end(probe *failedRecoveryProbe, now time.Time, retained bool) {
	// The reservation owns failures until end publishes inFlight=false.
	// Generate jitter before taking the shared cache lock.
	var delay time.Duration
	if retained {
		delay = retryDelay(failedRecoveryRetryInitial, failedRecoveryRetryMax, probe.failures+1)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries[probe.key] != probe {
		return
	}
	if !retained {
		b.remove(probe)
		return
	}
	probe.inFlight = false
	probe.failures = min(probe.failures+1, 31)
	probe.next = now.Add(delay)
}

// remove is called only while mu is held.
func (b *failedRecoveryBackoff) remove(probe *failedRecoveryProbe) {
	delete(b.entries, probe.key)
	b.order.Remove(probe.element)
}
