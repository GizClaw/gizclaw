package tikvfixture

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tikv/client-go/v2/oracle"
)

type usageClockOracle struct {
	oracle.Oracle
	cached      atomic.Uint64
	freshCalls  atomic.Int64
	validated   atomic.Uint64
	cacheErr    error
	validateErr error
}

func (o *usageClockOracle) GetLowResolutionTimestamp(context.Context, *oracle.Option) (uint64, error) {
	return o.cached.Load(), o.cacheErr
}

func (o *usageClockOracle) GetTimestamp(context.Context, *oracle.Option) (uint64, error) {
	o.freshCalls.Add(1)
	return o.cached.Load() + 1, nil
}

func (o *usageClockOracle) ValidateReadTS(_ context.Context, ts uint64, _ bool, _ *oracle.Option) error {
	o.validated.Store(ts)
	return o.validateErr
}

func TestUsageReadClockRetainsCacheAndPreservesFreshOracle(t *testing.T) {
	ctx := t.Context()
	gate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gate.Close() })
	validationErr := errors.New("original SDK validation")
	delegate := &usageClockOracle{validateErr: validationErr}
	cached := oracle.GoTimeToTS(time.Now())
	delegate.cached.Store(cached)
	clock := gate.WrapReadClock(delegate, 1501*time.Millisecond)
	opt := &oracle.Option{TxnScope: oracle.GlobalTxnScope}
	if ts, err := clock.GetLowResolutionTimestamp(ctx, opt); err != nil || ts != cached {
		t.Fatalf("unfrozen cache = %d, %v", ts, err)
	}
	var state struct {
		Frozen      bool   `json:"frozen"`
		ReadVersion uint64 `json:"read_version"`
	}
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/read-clock", map[string]bool{"frozen": true}, &state); err != nil {
		t.Fatal(err)
	}
	want := oracle.GoTimeToTS(oracle.GetTimeFromTS(cached).Add(-1501 * time.Millisecond))
	if !state.Frozen || state.ReadVersion != want {
		t.Fatalf("captured real timestamp = %+v, want %d", state, want)
	}
	next := cached + 1<<18
	delegate.cached.Store(next)
	if ts, err := clock.GetLowResolutionTimestamp(ctx, opt); err != nil || ts != cached {
		t.Fatalf("retained cache = %d, %v", ts, err)
	}
	if ts, err := clock.GetTimestamp(ctx, opt); err != nil || ts != next+1 || delegate.freshCalls.Load() != 1 {
		t.Fatalf("fresh writer oracle = %d, %v", ts, err)
	}
	if err := clock.ValidateReadTS(ctx, want, true, opt); !errors.Is(err, validationErr) || delegate.validated.Load() != want {
		t.Fatalf("original SDK validation was bypassed: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := clock.GetLowResolutionTimestamp(cancelled, opt); !errors.Is(err, context.Canceled) {
		t.Fatalf("frozen read ignored cancellation: %v", err)
	}
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/reset", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if ts, err := clock.GetLowResolutionTimestamp(ctx, opt); err != nil || ts != next {
		t.Fatalf("reset did not restore live SDK cache: %d, %v", ts, err)
	}
}

func TestUsageReadClockDoesNotFreezeFailedCache(t *testing.T) {
	failure := errors.New("cache unavailable")
	delegate := &usageClockOracle{cacheErr: failure}
	clock := &usageReadClock{Oracle: delegate, delay: time.Second}
	if _, err := clock.setFrozen(t.Context(), true); !errors.Is(err, failure) || clock.frozen.Load() != 0 {
		t.Fatalf("failed cache was replaced: %v", err)
	}
}
