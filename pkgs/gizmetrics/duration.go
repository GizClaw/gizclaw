package gizmetrics

import (
	"context"
	"errors"
	"time"
)

// ObserveDuration records a latency in seconds using shared speech/model buckets.
// Negative durations are unknown timing origins and are not samples.
func ObserveDuration(ctx context.Context, name string, elapsed time.Duration, labels ...Label) {
	if elapsed < 0 {
		return
	}
	ObserveHistogram(ctx, name, elapsed.Seconds(), []float64{.025, .05, .1, .25, .5, 1, 2, 5, 10, 30, 60, 120}, labels...)
}

// Result classifies completion without putting error messages in metric labels.
func Result(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}
