package iconasset

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRecordLockCancellationSharesOwnerCoordination(t *testing.T) {
	var locker Locker
	release := sync.OnceFunc(locker.LockOwner("peer"))
	t.Cleanup(release)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		unlock, err := locker.LockRecordContext(ctx, "peer")
		if unlock != nil {
			unlock()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("record wait bypassed the owner lock: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("record wait ignored cancellation")
	}
	other, err := locker.LockRecordContext(t.Context(), "other")
	if err != nil {
		t.Fatal(err)
	}
	other()
	if got := locker.locks.ActiveKeys(); got != 3 {
		t.Fatalf("canceled waiter retained extra keys: %d", got)
	}
	release()
	if got := locker.locks.ActiveKeys(); got != 0 {
		t.Fatalf("idle lock keys retained: %d", got)
	}
}

func TestRecordLockContextExcludesBlockingRecordAPI(t *testing.T) {
	var locker Locker
	unlock, err := locker.LockRecordContext(t.Context(), "peer")
	if err != nil {
		t.Fatal(err)
	}
	release := sync.OnceFunc(unlock)
	t.Cleanup(release)
	started, done := make(chan struct{}), make(chan struct{})
	go func() {
		close(started)
		unlock := locker.LockRecord("peer")
		unlock()
		close(done)
	}()
	<-started
	select {
	case <-done:
		t.Fatal("blocking record API bypassed the context-aware holder")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocking record API did not resume")
	}
	if got := locker.locks.ActiveKeys(); got != 0 {
		t.Fatalf("released record key retained: %d", got)
	}
}
