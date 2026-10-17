package genx

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func BenchmarkUsageMetrics(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		b.Run(fmt.Sprintf("enabled=%t", enabled), func(b *testing.B) {
			if enabled {
				stop, err := gizmetrics.InstallStore(storemetrics.NewMemoryStore(), gizmetrics.WithFlushInterval(time.Hour),
					gizmetrics.WithLabels(gizmetrics.Label{Name: "node_id", Value: "node"}, gizmetrics.Label{Name: "node_type", Value: "server"}))
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() {
					if err := stop(context.Background()); err != nil {
						b.Error(err)
					}
				})
			}
			ctx := WithUsageRecorder(context.Background(), func(UsageRecord) {})
			record := UsageRecord{Provider: "vendor", Model: "configured-model", Modality: UsageModalityText,
				Unit: UsageUnitToken, Input: 1_000, CachedInput: 500, Output: 200}
			RecordUsage(ctx, record)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					RecordUsage(ctx, record)
				}
			})
		})
	}
}
