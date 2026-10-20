package infraflow

import (
	"testing"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func BenchmarkExhaustedSamplingGate(b *testing.B) {
	observer := captureFixture(b, infraapi.E2e).EdgeObserver("edge-key")
	for range MaxSamplesPerWindow {
		observer.Begin()
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			observer.Begin()
		}
	})
}

func BenchmarkBusinessKVReadSampling(b *testing.B) {
	c, err := NewCounters(infraapi.E2e, CaptureConfig{Node: "server", Routes: []CaptureRoute{{Route: Route{"server", "kv"}, Store: "records"}}})
	if err != nil {
		b.Fatal(err)
	}
	observer := c.StoreObserver("records")
	for range MaxSamplesPerWindow {
		observer.Begin()
	}
	base := kv.NewMemory(nil)
	defer base.Close()
	key := kv.Key{"record"}
	ctx := b.Context()
	if err := base.Set(ctx, key, make([]byte, 128)); err != nil {
		b.Fatal(err)
	}
	sampled := kv.ObserveReads(base, observer)
	for _, scenario := range []struct {
		name  string
		store kv.Store
	}{{"disabled", base}, {"enabled", sampled}} {
		b.Run(scenario.name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					value, err := scenario.store.Get(ctx, key)
					if err != nil || len(value) != 128 {
						b.Error("business result changed")
						return
					}
				}
			})
		})
	}
}
