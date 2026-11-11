package tikvfixture

import (
	"context"
	"errors"
)

// RunUsageAutomaticRecovery coordinates only the overlaid periodic worker.
// It enters before the actual reconcile callback acquires domain locks. Manual
// service calls remain unchanged and available while automatic passes pause.
func RunUsageAutomaticRecovery(ctx context.Context, reconcile func(context.Context) error) error {
	gate := usageTransport.Load()
	if gate == nil {
		return reconcile(ctx)
	}
	for {
		gate.recoveryMu.Lock()
		if !gate.recoveryPaused {
			gate.recoveryActive++
			gate.recoveryMu.Unlock()
			defer func() {
				gate.recoveryMu.Lock()
				gate.recoveryActive--
				gate.notifyRecoveryLocked()
				gate.recoveryMu.Unlock()
			}()
			return reconcile(ctx)
		}
		changed := gate.recoveryChanged
		gate.recoveryMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-gate.closing:
			return errors.New("usage automatic recovery control closed")
		case <-changed:
		}
	}
}

func (g *UsageGate) notifyRecoveryLocked() {
	close(g.recoveryChanged)
	g.recoveryChanged = make(chan struct{})
}

func (g *UsageGate) pauseAutomaticRecovery(ctx context.Context, paused bool) (int, error) {
	g.recoveryMu.Lock()
	g.recoveryPaused = paused
	g.notifyRecoveryLocked()
	for paused && g.recoveryActive != 0 {
		changed := g.recoveryChanged
		g.recoveryMu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-g.closing:
			return 0, errors.New("usage automatic recovery control closed")
		case <-changed:
		}
		g.recoveryMu.Lock()
	}
	active := g.recoveryActive
	g.recoveryMu.Unlock()
	return active, nil
}
