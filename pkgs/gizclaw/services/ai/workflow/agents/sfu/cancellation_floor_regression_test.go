package sfu

import (
	"context"
	"testing"
	"time"
)

func TestCanceledSessionCannotGrantPendingFloor(t *testing.T) {
	h := newHarness(t, Config{FloorIdle: 5 * time.Second, TalkHangover: 5 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	defer output.Close()
	s := h.session(testPeer)

	// Hold cleanup behind the same state lock used by track detach. Cancellation
	// has occurred, but finish has not yet set its local finished flag.
	s.mu.Lock()
	s.utterances[testRemote] = &remoteUtterance{id: "pending", order: 1}
	cancel()
	granted := s.acquireFloorLocked()
	var holder string
	if s.floor != nil {
		holder = s.floor.identity
	}
	s.mu.Unlock()
	if granted || holder != "" {
		t.Fatalf("canceled session granted a new floor to %q", holder)
	}
}
