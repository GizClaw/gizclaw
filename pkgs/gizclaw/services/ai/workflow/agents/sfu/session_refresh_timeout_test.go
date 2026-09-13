package sfu

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
)

type stalledRefreshResolver struct{ entered chan time.Duration }

func (r stalledRefreshResolver) ResolveSFUWorkspaceBinding(ctx context.Context, _, _ string) (socialutil.SFUWorkspaceBinding, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		r.entered <- 0
	} else {
		r.entered <- time.Until(deadline)
	}
	<-ctx.Done()
	return socialutil.SFUWorkspaceBinding{}, ctx.Err()
}

func TestBackgroundRefreshTimeoutRevokesForwarding(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(context.Canceled)
	resolver := stalledRefreshResolver{entered: make(chan time.Duration, 1)}
	s := &session{ctx: ctx, cancel: cancel, agent: &Agent{bindings: resolver, workspaceID: "workspace"}, peer: "peer", config: Config{RecheckInterval: 20 * time.Millisecond}, forwarding: true, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	done := make(chan struct{})
	go func() { defer close(done); s.recheck() }()
	t.Cleanup(func() { cancel(context.Canceled); <-done })
	select {
	case remaining := <-resolver.entered:
		if remaining <= 0 || remaining > 20*time.Millisecond {
			t.Fatalf("refresh deadline=%s", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stalled refresh did not revoke")
	}
	if !errors.Is(context.Cause(ctx), ErrRevoked) {
		t.Fatalf("cause=%v", context.Cause(ctx))
	}
	s.mu.Lock()
	forwarding := s.forwarding
	s.mu.Unlock()
	if forwarding {
		t.Fatal("expired membership still forwards audio")
	}
}
