package keyedlock

import (
	"context"
	"errors"
	"testing"
)

func TestLockerSeparatesKeysAndReclaimsEntries(t *testing.T) {
	var locker Locker[string]
	releaseA, err := locker.Acquire(t.Context(), "a")
	if err != nil {
		t.Fatal(err)
	}
	releaseB, err := locker.Acquire(t.Context(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if got := locker.ActiveKeys(); got != 2 {
		t.Fatalf("ActiveKeys() = %d, want 2", got)
	}
	releaseA()
	releaseA()
	releaseB()
	if got := locker.ActiveKeys(); got != 0 {
		t.Fatalf("ActiveKeys() after release = %d, want 0", got)
	}
}

func TestLockerSameKeyWaitHonorsCancellation(t *testing.T) {
	var locker Locker[string]
	release, err := locker.Acquire(t.Context(), "same")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := locker.Acquire(ctx, "same"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() error = %v, want context.Canceled", err)
	}
	if got := locker.ActiveKeys(); got != 1 {
		t.Fatalf("ActiveKeys() with holder = %d, want 1", got)
	}
	release()
	if got := locker.ActiveKeys(); got != 0 {
		t.Fatalf("ActiveKeys() after release = %d, want 0", got)
	}
}
