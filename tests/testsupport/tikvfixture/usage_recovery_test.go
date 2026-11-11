package tikvfixture

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUsageAutomaticRecoveryPauseDrainsBeforeBlockingNewCallbacks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	gate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- RunUsageAutomaticRecovery(ctx, func(ctx context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	gate.recoveryMu.Lock()
	changed := gate.recoveryChanged
	gate.recoveryMu.Unlock()
	paused := make(chan error, 1)
	go func() {
		active, err := gate.pauseAutomaticRecovery(ctx, true)
		if err == nil && active != 0 {
			err = errors.New("pause returned before actual pass drained")
		}
		paused <- err
	}()
	select {
	case <-changed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	secondEntered := make(chan struct{})
	second := make(chan error, 1)
	actualError := errors.New("actual reconcile result")
	go func() {
		second <- RunUsageAutomaticRecovery(ctx, func(context.Context) error { close(secondEntered); return actualError })
	}()
	select {
	case <-paused:
		t.Fatal("pause acknowledged while first domain callback remained active")
	default:
	}
	close(release)
	select {
	case err := <-first:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-paused:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-secondEntered:
		t.Fatal("paused automatic worker entered domain callback")
	default:
	}
	if _, err := gate.pauseAutomaticRecovery(ctx, false); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-second:
		if err != actualError {
			t.Fatalf("actual callback result changed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestUsageAutomaticRecoveryPauseHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	gate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	if _, err := gate.pauseAutomaticRecovery(ctx, true); err != nil {
		t.Fatal(err)
	}
	request, stop := context.WithCancel(ctx)
	stop()
	called := false
	err = RunUsageAutomaticRecovery(request, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("paused cancelled pass entered domain: called=%t err=%v", called, err)
	}
}
