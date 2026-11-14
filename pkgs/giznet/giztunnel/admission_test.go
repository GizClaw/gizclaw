package giztunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type admissionResult struct {
	conn *Conn
	decl SessionDeclaration
	err  error
}

func acceptAdmission(router *Router, ctx context.Context) <-chan admissionResult {
	done := make(chan admissionResult, 1)
	go func() {
		conn, decl, err := router.Accept(ctx)
		done <- admissionResult{conn, decl, err}
	}()
	return done
}

func dialAdmission(router *Router, ctx context.Context, decl SessionDeclaration) <-chan admissionResult {
	done := make(chan admissionResult, 1)
	go func() {
		conn, err := router.Dial(ctx, decl)
		done <- admissionResult{conn: conn, err: err}
	}()
	return done
}

func awaitAdmission(t *testing.T, done <-chan admissionResult) admissionResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("admission did not finish")
		return admissionResult{}
	}
}

func routerAdmissionsIdle(router *Router) bool {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.pending) == 0 && len(router.sessions) == 0 && router.pendingAdmissions == 0 && router.activeChannels == 0
}

func TestAdmissionRefusalKeepsAcceptWaitingAndReleasesCapacity(t *testing.T) {
	target := newMemDeclaration(t).ClientPublicKey
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"redirect", fmt.Errorf("route lookup: %w", &SessionRedirectError{ServerPublicKey: target})},
		{"reject", errors.New("private business reason must not cross the tunnel")},
		{"invalid redirect", &SessionRedirectError{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := make(chan SessionDeclaration, 1)
			release := make(chan struct{})
			var calls, serviceCalls atomic.Int32
			var router *Router
			pair := newMemRouters(t, Config{MaxPendingSessions: 1, MaxChannels: 3}, Config{
				MaxPendingSessions: 1, MaxChannels: 3,
				AdmitSession: func(ctx context.Context, decl SessionDeclaration) error {
					// This method takes Router.mu; successful progress proves the
					// caller does not hold that mutex during application admission.
					_ = router.ActiveChannels()
					if calls.Add(1) > 1 {
						return nil
					}
					started <- decl
					select {
					case <-release:
						return tc.err
					case <-ctx.Done():
						return ctx.Err()
					}
				},
				AllowRemoteService: func(giznet.PublicKey, uint64) bool {
					serviceCalls.Add(1)
					return true
				},
			})
			router = pair.serverRouter
			pair.edgePhysical.readChunk = 1
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			accepted := acceptAdmission(router, ctx)
			decl := newMemDeclaration(t)
			dialed := dialAdmission(pair.edgeRouter, ctx, decl)
			select {
			case got := <-started:
				if got != decl {
					t.Fatalf("callback declaration = %+v, want %+v", got, decl)
				}
			case <-ctx.Done():
				t.Fatal("callback blocked, possibly under Router.mu")
			}

			name, err := serviceLabel(decl.SessionID, 7, 1)
			if err != nil {
				t.Fatal(err)
			}
			stream, err := pair.edgePhysical.OpenChannel(ctx, name, giznet.ChannelReliable)
			if err != nil {
				t.Fatal(err)
			}
			_ = stream.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := stream.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
				t.Fatalf("service during admission = %v, want closed", err)
			}
			_ = stream.Close()
			if serviceCalls.Load() != 0 {
				t.Fatal("business service policy ran before admission")
			}
			// A callback occupies the pending quota until its result is sent.
			if conn, err := pair.edgeRouter.Dial(ctx, newMemDeclaration(t)); conn != nil || !errors.Is(err, ErrBufferLimit) {
				t.Fatalf("concurrent Dial = %v, %v; want bounded admission", conn, err)
			}
			close(release)
			result := awaitAdmission(t, dialed)
			if result.conn != nil {
				t.Fatal("refused session returned a connection")
			}
			var redirect *SessionRedirectError
			if tc.name == "redirect" {
				if !errors.As(result.err, &redirect) || redirect.ServerPublicKey != target {
					t.Fatalf("Dial = %v, want redirect to %s", result.err, target)
				}
			} else if !errors.Is(result.err, ErrSessionRejected) || result.err.Error() != "giztunnel: session rejected: session admission denied" || strings.Contains(result.err.Error(), tc.err.Error()) {
				t.Fatalf("Dial = %v, want stable generic rejection", result.err)
			}
			waitFor(t, "refused admission cleanup", func() bool {
				return routerAdmissionsIdle(pair.edgeRouter) && routerAdmissionsIdle(router)
			})
			select {
			case result := <-accepted:
				t.Fatalf("Accept returned for refused admission: %+v", result)
			default:
			}

			next := newMemDeclaration(t)
			edge, err := pair.edgeRouter.Dial(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			result = awaitAdmission(t, accepted)
			if result.err != nil || result.decl != next || result.conn == nil {
				t.Fatalf("later Accept = %+v", result)
			}
			listener := result.conn.ListenService(7)
			service, err := edge.Dial(7)
			if err != nil {
				t.Fatal(err)
			}
			remote, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			_ = service.Close()
			_ = remote.Close()
			_ = edge.Close()
			_ = result.conn.Close()
		})
	}
}

func TestAdmissionContextCancellation(t *testing.T) {
	for _, cause := range []string{"accept cancel", "router close", "handshake timeout"} {
		t.Run(cause, func(t *testing.T) {
			started := make(chan context.Context, 1)
			finished := make(chan error, 1)
			pair := newMemRouters(t, Config{}, Config{
				HandshakeTimeout: 150 * time.Millisecond,
				AdmitSession: func(ctx context.Context, _ SessionDeclaration) error {
					started <- ctx
					<-ctx.Done()
					finished <- ctx.Err()
					return nil // Cancellation must override even an accepting result.
				},
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			accepted := acceptAdmission(pair.serverRouter, ctx)
			dialed := dialAdmission(pair.edgeRouter, t.Context(), newMemDeclaration(t))
			select {
			case callbackCtx := <-started:
				if _, ok := callbackCtx.Deadline(); !ok {
					t.Fatal("admission context has no handshake deadline")
				}
			case <-time.After(time.Second):
				t.Fatal("callback did not start")
			}
			switch cause {
			case "accept cancel":
				cancel()
			case "router close":
				_ = pair.serverRouter.Close()
			}
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("callback context was not canceled")
				}
			case <-time.After(time.Second):
				t.Fatal("callback did not observe cancellation")
			}
			if result := awaitAdmission(t, dialed); result.conn != nil || result.err == nil {
				t.Fatalf("canceled Dial = %+v", result)
			}
			if cause == "handshake timeout" {
				cancel() // Accept should still be waiting for another declaration.
			}
			result := awaitAdmission(t, accepted)
			if result.conn != nil || result.err == nil {
				t.Fatalf("canceled Accept = %+v", result)
			}
			waitFor(t, "canceled admission cleanup", func() bool {
				return routerAdmissionsIdle(pair.edgeRouter) && routerAdmissionsIdle(pair.serverRouter)
			})
		})
	}
}

func TestDialCancellationClosesPendingChannels(t *testing.T) {
	pair := newMemRouters(t, Config{}, Config{HandshakeTimeout: 100 * time.Millisecond})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dialed := dialAdmission(pair.edgeRouter, ctx, newMemDeclaration(t))
	waitFor(t, "pending native channels", func() bool {
		return pair.edgeRouter.ActiveChannels() == 2 && pair.serverRouter.ActiveChannels() == 2
	})
	cancel()
	result := awaitAdmission(t, dialed)
	if result.conn != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("canceled Dial = %+v", result)
	}
	waitFor(t, "pending channel release", func() bool {
		return routerAdmissionsIdle(pair.edgeRouter) && routerAdmissionsIdle(pair.serverRouter)
	})
}

func TestAdmissionCallbacksRemainBoundedAfterTimeout(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	pair := newMemRouters(t, Config{}, Config{
		MaxPendingSessions: 1, HandshakeTimeout: 100 * time.Millisecond,
		AdmitSession: func(context.Context, SessionDeclaration) error {
			calls.Add(1)
			started <- struct{}{}
			<-release // Simulate a callback that does not honor cancellation.
			return nil
		},
	})
	releaseCallback := sync.OnceFunc(func() { close(release) })
	defer releaseCallback()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	firstAccept := acceptAdmission(pair.serverRouter, ctx)
	firstDial := dialAdmission(pair.edgeRouter, ctx, newMemDeclaration(t))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first callback did not start")
	}
	if result := awaitAdmission(t, firstDial); result.conn != nil || result.err == nil {
		t.Fatalf("expired Dial = %+v", result)
	}
	waitFor(t, "first timed out admission release", func() bool {
		return routerAdmissionsIdle(pair.serverRouter)
	})
	secondAccept := acceptAdmission(pair.serverRouter, ctx)
	secondDial := dialAdmission(pair.edgeRouter, ctx, newMemDeclaration(t))
	if result := awaitAdmission(t, secondDial); result.conn != nil || result.err == nil {
		t.Fatalf("quota-blocked Dial = %+v", result)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("callback calls = %d, want bounded at 1", got)
	}
	cancel()
	// The second Accept is cancelable while waiting for callback capacity.
	if result := awaitAdmission(t, secondAccept); result.conn != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("second Accept = %+v", result)
	}
	releaseCallback()
	if result := awaitAdmission(t, firstAccept); result.conn != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("first Accept = %+v", result)
	}
	waitFor(t, "quota-blocked admission cleanup", func() bool {
		return routerAdmissionsIdle(pair.edgeRouter) && routerAdmissionsIdle(pair.serverRouter)
	})
}

func TestAdmissionQueueStaysBoundedAfterTimeout(t *testing.T) {
	pair := newMemRouters(t, Config{}, Config{
		MaxPendingSessions: 1, HandshakeTimeout: 100 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for range 2 {
		if conn, err := pair.edgeRouter.Dial(ctx, newMemDeclaration(t)); conn != nil || err == nil {
			t.Fatalf("Dial without Accept = %v, %v", conn, err)
		}
		waitFor(t, "expired queued admission cleanup", func() bool {
			return routerAdmissionsIdle(pair.edgeRouter) && routerAdmissionsIdle(pair.serverRouter)
		})
	}
	if got := len(pair.serverRouter.acceptCh); got != 1 {
		t.Fatalf("queued admissions = %d, want bounded at 1", got)
	}
	pair.dial(t) // An expired queue entry must not prevent a later acceptance.
}

func TestAdmissionWriteFailureReleasesChannels(t *testing.T) {
	var physical *memChannelConn
	pair := newMemRouters(t, Config{}, Config{
		AdmitSession: func(context.Context, SessionDeclaration) error {
			// Closing the native peer makes the result write fail after admission.
			_ = physical.Close()
			return nil
		},
	})
	physical = pair.serverPhysical
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	accepted := acceptAdmission(pair.serverRouter, ctx)
	dialed := dialAdmission(pair.edgeRouter, ctx, newMemDeclaration(t))
	if result := awaitAdmission(t, accepted); result.conn != nil || result.err == nil {
		t.Fatalf("Accept after write failure = %+v", result)
	}
	if result := awaitAdmission(t, dialed); result.conn != nil || result.err == nil {
		t.Fatalf("Dial after write failure = %+v", result)
	}
	waitFor(t, "failed result write cleanup", func() bool {
		return routerAdmissionsIdle(pair.edgeRouter) && routerAdmissionsIdle(pair.serverRouter)
	})
}
