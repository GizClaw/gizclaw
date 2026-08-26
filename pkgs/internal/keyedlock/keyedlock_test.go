package keyedlock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func BenchmarkLockerResourceContention(b *testing.B) {
	const (
		width = 8
		delay = time.Millisecond
	)
	type acquireFunc func(context.Context, int) (func(), error)
	var global sync.Mutex
	var keyed Locker[int]
	benchmarks := []struct {
		name    string
		acquire acquireFunc
		sameKey bool
	}{
		{
			name: "global-distinct-8",
			acquire: func(context.Context, int) (func(), error) {
				global.Lock()
				return global.Unlock, nil
			},
		},
		{name: "keyed-same-8", acquire: keyed.Acquire, sameKey: true},
		{name: "keyed-distinct-8", acquire: keyed.Acquire},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(delay), "critical-delay-ns")
			for b.Loop() {
				if err := runLockBatch(b.Context(), width, delay, benchmark.sameKey, benchmark.acquire); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*width), "ns/resource")
		})
	}
}

func runLockBatch(
	ctx context.Context,
	width int,
	delay time.Duration,
	sameKey bool,
	acquire func(context.Context, int) (func(), error),
) error {
	errs := make([]error, width)
	var wait sync.WaitGroup
	for index := range width {
		wait.Go(func() {
			key := index
			if sameKey {
				key = 0
			}
			release, err := acquire(ctx, key)
			if err != nil {
				errs[index] = err
				return
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				err = ctx.Err()
			}
			timer.Stop()
			release()
			errs[index] = err
		})
	}
	wait.Wait()
	return errors.Join(errs...)
}

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

func TestLockerCancellationWinsReleaseRace(t *testing.T) {
	for range 100 {
		var locker Locker[string]
		release, err := locker.Acquire(t.Context(), "same")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		start := make(chan struct{})
		go func() {
			<-start
			acquiredRelease, err := locker.Acquire(ctx, "same")
			if acquiredRelease != nil {
				acquiredRelease()
			}
			result <- err
		}()
		cancel()
		release()
		close(start)
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("Acquire() error = %v, want context.Canceled", err)
		}
		if got := locker.ActiveKeys(); got != 0 {
			t.Fatalf("ActiveKeys() after canceled release race = %d, want 0", got)
		}
	}
}
