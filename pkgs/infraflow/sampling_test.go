package infraflow

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
)

func TestSamplingBudgetNeverRetriesOrRefillsFromTraffic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := captureFixture(t, infraapi.E2e)
		observer := c.EdgeObserver("edge-key")
		var selected int
		for range 100000 {
			if started, ok := observer.Begin(); ok {
				selected++
				observer.End(started, 1)
			}
		}
		if selected != MaxSamplesPerWindow {
			t.Fatalf("unbounded sampling: %d", selected)
		}
		for range 100 {
			c.Snapshot()
		}
		if _, ok := observer.Begin(); ok {
			t.Fatal("repeated readers replenished the same window")
		}
		time.Sleep(10 * Window)
		if _, ok := observer.Begin(); ok {
			t.Fatal("business path rotated or replenished its own budget")
		}
		c.Snapshot()
		if _, ok := observer.Begin(); !ok {
			t.Fatal("background snapshot did not replenish next window")
		}
	})
}

func TestBusyCollectorCannotBlockPeerObservations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := captureFixture(t, infraapi.E2e)
		observer := c.EdgeObserver("edge-key")
		c.mu.Lock() // Simulate a stalled collector holding all aggregation state.
		done := make(chan struct{})
		var work sync.WaitGroup
		for range 8 {
			work.Go(func() {
				for range 10000 {
					if started, selected := observer.Begin(); selected {
						observer.End(started, 1)
					}
				}
			})
		}
		go func() { work.Wait(); close(done) }()
		select {
		case <-done:
			c.mu.Unlock()
		case <-time.After(time.Second):
			c.mu.Unlock()
			<-done
			t.Fatal("collector lock delayed peer observations")
		}
		if c.values[0].Count != 0 {
			t.Fatal("contended samples were not dropped")
		}
	})
}
