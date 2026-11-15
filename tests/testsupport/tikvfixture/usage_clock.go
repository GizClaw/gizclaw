package tikvfixture

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/tikv/client-go/v2/oracle"
)

// usageReadClock freezes cached timestamp issuance without rewinding an already
// selected snapshot. Fresh timestamps, future-TS validation and GC checks still
// use the SDK oracle.
type usageReadClock struct {
	oracle.Oracle
	regionalEnabled bool
	observed        *atomic.Uint64
	frozen          atomic.Uint64
	mu              sync.Mutex
	issued          uint64
	fallbacks       map[*usageReadFallback]struct{}
}

type usageReadFallback struct{ ctx context.Context }

// WrapReadClock installs the private fixture's optional cached-clock control.
// Install its result once, before exposing the real transactional client.
func (g *UsageGate) WrapReadClock(delegate oracle.Oracle, regionalEnabled bool) oracle.Oracle {
	clock := &usageReadClock{Oracle: delegate, regionalEnabled: regionalEnabled, observed: &g.regionalReadTS}
	g.readClock.Store(clock)
	return clock
}

func (o *usageReadClock) GetLowResolutionTimestamp(ctx context.Context, opt *oracle.Option) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if ts := o.frozen.Load(); ts != 0 {
		return ts, nil
	}
	ts, err := o.Oracle.GetLowResolutionTimestamp(ctx, opt)
	return o.publishCachedTimestamp(ctx, ts, err)
}

func (o *usageReadClock) GetLowResolutionTimestampAsync(ctx context.Context, opt *oracle.Option) oracle.Future {
	future := &usageReadFuture{clock: o, ctx: ctx, ts: o.frozen.Load()}
	if ctx.Err() == nil && future.ts == 0 {
		future.delegate = o.Oracle.GetLowResolutionTimestampAsync(ctx, opt)
	}
	return future
}

type usageReadFuture struct {
	clock    *usageReadClock
	ctx      context.Context
	delegate oracle.Future
	ts       uint64
}

func (f *usageReadFuture) Wait() (uint64, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	ts := f.ts
	var err error
	if f.delegate != nil {
		ts, err = f.delegate.Wait()
	}
	return f.clock.publishCachedTimestamp(f.ctx, ts, err)
}

func (o *usageReadClock) publishCachedTimestamp(ctx context.Context, ts uint64, err error) (uint64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// A delegate call or future may have started before the freeze was installed.
	// Publish its result under the same lock as the transition, including errors
	// that would otherwise escape into the production reader's fresh-TS fallback.
	if frozen := o.frozen.Load(); frozen != 0 {
		return frozen, nil
	}
	if err == nil && ts != 0 {
		o.issued = max(o.issued, ts)
		return ts, nil
	}
	// The production reader retries a missing cache through GetTimestampWithRetry.
	// Its read-view context is canceled only after that snapshot finishes, so
	// freeze cannot acknowledge while this untracked fresh selection can escape.
	// Do not intercept fresh oracle calls: writers and primary reads share them.
	fallback := &usageReadFallback{ctx: ctx}
	if o.fallbacks == nil {
		o.fallbacks = make(map[*usageReadFallback]struct{})
	}
	o.fallbacks[fallback] = struct{}{}
	context.AfterFunc(ctx, func() {
		o.mu.Lock()
		delete(o.fallbacks, fallback)
		o.mu.Unlock()
	})
	return ts, err
}

func (o *usageReadClock) setFrozen(ctx context.Context, frozen bool) (uint64, error) {
	if !frozen {
		o.mu.Lock()
		o.frozen.Store(0)
		o.mu.Unlock()
		return 0, nil
	}
	if !o.regionalEnabled {
		return 0, errors.New("regional read labels required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if ts := o.frozen.Load(); ts != 0 {
		return ts, nil
	}
	cached, err := o.Oracle.GetLowResolutionTimestamp(ctx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err != nil {
		return 0, err
	}
	if cached == 0 {
		return 0, errors.New("SDK cached timestamp is unavailable")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if ts := o.frozen.Load(); ts != 0 {
		return ts, nil
	}
	for fallback := range o.fallbacks {
		if fallback.ctx.Err() != nil {
			delete(o.fallbacks, fallback)
			continue
		}
		return 0, errors.New("cached timestamp fallback is still in flight; retry after the read finishes")
	}
	var observed uint64
	if o.observed != nil {
		observed = o.observed.Load()
	}
	if observed == 0 || observed > cached {
		return 0, errors.New("successful regional snapshot within cached PD time required")
	}
	// Native Learner success proves the regional path is usable. A newer cached
	// timestamp already issued to a snapshot must also be retained: its late
	// completion can advance the production root's monotonic read floor.
	ts := max(observed, o.issued)
	if ts > cached {
		return 0, errors.New("issued regional timestamp exceeds current SDK cache")
	}
	o.issued = ts
	o.frozen.Store(ts)
	return ts, nil
}
