package giztest

import (
	"context"
	"testing"
	"time"
)

func TestBarrierReleasesExactGroup(t *testing.T) {
	b := newTaskBarrier(2)
	done := make(chan error, 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { done <- b.Wait(ctx) }()
	go func() { done <- b.Wait(ctx) }()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestBarrierAbortReleasesWaiters(t *testing.T) {
	b := newTaskBarrier(2)
	done := make(chan error, 1)
	go func() { done <- b.Wait(context.Background()) }()
	b.Abort(context.Canceled)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("aborted barrier returned nil")
		}
	case <-time.After(time.Second):
		t.Fatal("aborted barrier did not release waiter")
	}
}
