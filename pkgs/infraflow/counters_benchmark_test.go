package infraflow

import (
	"testing"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
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
