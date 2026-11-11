package tikvfixture

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/tikv/client-go/v2/oracle"
)

// usageReadClock retains an actual cached timestamp for historical snapshots.
// Fresh timestamps, future-TS validation and GC checks still use the SDK oracle.
type usageReadClock struct {
	oracle.Oracle
	delay  time.Duration
	frozen atomic.Uint64
}

// WrapReadClock installs the private fixture's optional cached-clock control.
// Install its result once, before exposing the real transactional client.
func (g *UsageGate) WrapReadClock(delegate oracle.Oracle, delay time.Duration) oracle.Oracle {
	clock := &usageReadClock{Oracle: delegate, delay: delay}
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
	return o.Oracle.GetLowResolutionTimestamp(ctx, opt)
}

func (o *usageReadClock) setFrozen(ctx context.Context, frozen bool) (uint64, error) {
	if !frozen {
		o.frozen.Store(0)
		return 0, nil
	}
	if o.delay <= 0 {
		return 0, errors.New("historical read configuration required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	ts, err := o.Oracle.GetLowResolutionTimestamp(ctx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
	if err != nil {
		return 0, err
	}
	if ts == 0 {
		return 0, errors.New("SDK cached timestamp is unavailable")
	}
	o.frozen.Store(ts)
	return oracle.GoTimeToTS(oracle.GetTimeFromTS(ts).Add(-o.delay)), nil
}
