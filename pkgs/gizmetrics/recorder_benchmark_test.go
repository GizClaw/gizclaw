package gizmetrics

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkCounterReporting(b *testing.B) {
	labels := [3][]Label{
		{{Name: "provider", Value: "vendor"}, {Name: "model", Value: "model"}, {Name: "modality", Value: "text"}, {Name: "token_type", Value: "input"}},
		{{Name: "provider", Value: "vendor"}, {Name: "model", Value: "model"}, {Name: "modality", Value: "text"}, {Name: "token_type", Value: "cached_input"}},
		{{Name: "provider", Value: "vendor"}, {Name: "model", Value: "model"}, {Name: "modality", Value: "text"}, {Name: "token_type", Value: "output"}},
	}
	deltas := []CounterDelta{{Value: 3, Labels: labels[0]}, {Value: 2, Labels: labels[1]}, {Value: 1, Labels: labels[2]}}
	for _, batch := range []bool{false, true} {
		b.Run(fmt.Sprintf("batch=%t", batch), func(b *testing.B) {
			r := newStoreRecorder(nil, recorderConfig{maxSeries: DefaultMaxSeries,
				labels: []Label{{Name: "node_id", Value: "node"}, {Name: "node_type", Value: "server"}}})
			ctx := context.Background()
			r.addPeriodicCounters(ctx, "tokens_total", deltas)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if batch {
						r.addPeriodicCounters(ctx, "tokens_total", deltas)
					} else {
						for _, delta := range deltas {
							r.addCounter(ctx, "tokens_total", delta.Value, delta.Labels)
						}
					}
				}
			})
		})
	}
}

func BenchmarkIdleSnapshot(b *testing.B) {
	for _, periodic := range []bool{false, true} {
		for _, size := range []int{100, 1_000, 10_000} {
			b.Run(fmt.Sprintf("periodic=%t/series=%d", periodic, size), func(b *testing.B) {
				r := newStoreRecorder(nil, recorderConfig{maxSeries: DefaultMaxSeries})
				for i := range size {
					labels := []Label{{Name: "model", Value: fmt.Sprint(i)}}
					if periodic {
						r.addPeriodicCounters(context.Background(), "tokens_total", []CounterDelta{{Value: 1, Labels: labels}})
					} else {
						r.addCounter(context.Background(), "requests_total", 1, labels)
					}
				}
				r.markClean(r.snapshot())
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					pending := r.snapshot()
					if periodic && len(pending) != size || !periodic && len(pending) != 0 {
						b.Fatal("unexpected idle series count")
					}
				}
			})
		}
	}
}
