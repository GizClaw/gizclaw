package infraflow

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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

func TestBusyCollectorCannotBlockBusinessReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, err := NewCounters(infraapi.E2e, CaptureConfig{Node: "server", Routes: []CaptureRoute{{Route: Route{"server", "kv"}, Store: "records"}}})
		if err != nil {
			t.Fatal(err)
		}
		base := kv.NewMemory(nil)
		defer base.Close()
		key := kv.Key{"record"}
		if err := base.Set(t.Context(), key, []byte("unchanged")); err != nil {
			t.Fatal(err)
		}
		store := kv.ObserveReads(base, c.StoreObserver("records"))
		c.mu.Lock() // Simulate a stalled collector holding all aggregation state.
		done := make(chan struct{})
		var work sync.WaitGroup
		for range 8 {
			work.Go(func() {
				for range 10000 {
					value, err := store.Get(t.Context(), key)
					if err != nil || string(value) != "unchanged" {
						t.Error("observation altered the business result")
						return
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
			t.Fatal("collector lock delayed business")
		}
		if c.values[0].Count != 0 {
			t.Fatal("contended samples were not dropped")
		}
	})
}
