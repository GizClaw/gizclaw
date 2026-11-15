package gizedge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

func TestGatewayAdmissionRedirectBeforeBusinessStreams(t *testing.T) {
	for _, outcome := range []string{"home", "loop", "unconfigured"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			serverA, serverB, edgeKey, clientKey := testKeyPair(t, 0xc1), testKeyPair(t, 0xc2), testKeyPair(t, 0xc3), testKeyPair(t, 0xc4)
			var attemptsA, attemptsB, business atomic.Int32
			listenerA, urlA := redirectTestListener(t, serverA)
			listenerB, urlB := redirectTestListener(t, serverB)
			cfg := Config{KeyPair: edgeKey, WebRTC: WebRTCConfig{Listen: "127.0.0.1:0", Endpoint: "localhost:0"},
				Upstreams: []UpstreamConfig{{Endpoint: urlA, PublicKey: serverA.Public}, {Endpoint: urlB, PublicKey: serverB.Public}}, Gateway: defaultGatewayConfig()}
			cfg.Gateway.Enabled = true
			cfg.Gateway.MaxUpstreams = 1
			gateway, err := newGateway(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = gateway.Close() })
			// Every lookup remains missing, including after the redirect. Only the
			// returned owner may select B; a fresh lookup would choose A again.
			var lookups atomic.Int32
			gateway.resolvePeerRoute = func(context.Context, giznet.PublicKey) (*rpcpb.PeerAssignment, error) {
				lookups.Add(1)
				return &rpcpb.PeerAssignment{ServerPublicKey: serverA.Public.String()}, errRouteAssignmentNotFound
			}
			serveRedirectTestUpstream(t, ctx, listenerA, false, func(context.Context, giztunnel.SessionDeclaration) error {
				attemptsA.Add(1)
				owner := serverB.Public
				if outcome == "unconfigured" {
					owner = giznet.PublicKey{99}
				}
				return &giztunnel.SessionRedirectError{ServerPublicKey: owner}
			}, &business)
			serveRedirectTestUpstream(t, ctx, listenerB, true, func(context.Context, giztunnel.SessionDeclaration) error {
				attemptsB.Add(1)
				if outcome == "loop" {
					return &giztunnel.SessionRedirectError{ServerPublicKey: serverA.Public}
				}
				return nil
			}, &business)
			edgeHTTP := httptest.NewServer(edgeIngressHandler(http.NotFoundHandler(), gateway))
			t.Cleanup(edgeHTTP.Close)
			listener, client, err := gizwebrtc.Dial(ctx, clientKey, edgeKey.Public, gizwebrtc.DialConfig{SignalingURL: edgeHTTP.URL + gizwebrtc.SignalingPath, SecurityPolicy: gatewayAllowAllPolicy{}})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			defer client.Close()
			stream, err := client.DialContext(ctx, gizclaw.ServicePeerRPC)
			if err == nil {
				defer stream.Close()
				_ = stream.SetDeadline(time.Now().Add(5 * time.Second))
				_, err = stream.Write([]byte("ping"))
				if err == nil {
					var response [4]byte
					_, err = io.ReadFull(stream, response[:])
					if err == nil && string(response[:]) != "pong" {
						t.Fatalf("business response=%q", response)
					}
				}
			}
			if outcome == "home" {
				if err != nil || business.Load() != 1 || attemptsA.Load() != 1 || attemptsB.Load() != 1 {
					t.Fatalf("redirect failed: err=%v business=%d attempts=%d/%d", err, business.Load(), attemptsA.Load(), attemptsB.Load())
				}
			} else if err == nil || business.Load() != 0 || attemptsA.Load()+attemptsB.Load() > gatewaySessionMaxAttempts {
				t.Fatalf("redirect did not fail closed: err=%v business=%d attempts=%d/%d", err, business.Load(), attemptsA.Load(), attemptsB.Load())
			}
			wantB := int32(1)
			if outcome == "unconfigured" {
				wantB = 0
			}
			if attemptsA.Load() != 1 || attemptsB.Load() != wantB {
				t.Fatalf("unexpected admission attempts A=%d B=%d, want 1/%d", attemptsA.Load(), attemptsB.Load(), wantB)
			}
			if lookups.Load() != 1 {
				t.Fatalf("redirect repeated route lookup: %d", lookups.Load())
			}
		})
	}
}

func redirectTestListener(t *testing.T, key *giznet.KeyPair) (*gizwebrtc.Listener, string) {
	t.Helper()
	listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: gatewayAllowAllPolicy{}}).Listen(key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := httptest.NewServer(listener.SignalingHandler())
	t.Cleanup(server.Close)
	return listener, server.URL
}

func serveRedirectTestUpstream(t *testing.T, ctx context.Context, listener *gizwebrtc.Listener, waitForProbe bool, admit func(context.Context, giztunnel.SessionDeclaration) error, business *atomic.Int32) {
	t.Helper()
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("redirect test upstream did not stop")
		}
	})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		probeSeen, tunnelReady := make(chan struct{}), make(chan struct{})
		var firstProbe atomic.Bool
		control := conn.ListenService(gizclaw.ServiceEdgeHTTP)
		httpDone := make(chan struct{})
		go func() {
			defer close(httpDone)
			_ = http.Serve(control, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if firstProbe.CompareAndSwap(false, true) {
					close(probeSeen)
				}
				select {
				case <-tunnelReady:
					w.WriteHeader(http.StatusOK)
				case <-ctx.Done():
				}
			}))
		}()
		defer func() { _ = control.Close(); <-httpDone }()
		if waitForProbe {
			// Delay the target's tunnel registration until the control probe is
			// in flight, reproducing a transport-ready but not-yet-admitted pool.
			select {
			case <-probeSeen:
			case <-ctx.Done():
				return
			}
		}
		router, err := giztunnel.NewRouter(conn.(giznet.ChannelConn), giztunnel.Config{AcceptSessions: true, AdmitSession: admit, AllowRemoteService: func(giznet.PublicKey, uint64) bool { return true }})
		if err != nil {
			t.Error(err)
			return
		}
		defer router.Close()
		close(tunnelReady)
		logical, _, err := router.Accept(ctx)
		if err != nil {
			return
		}
		defer logical.Close()
		service := logical.ListenService(gizclaw.ServicePeerRPC)
		defer service.Close()
		stream, err := service.Accept()
		if err != nil {
			return
		}
		defer stream.Close()
		var request [4]byte
		if _, err := io.ReadFull(stream, request[:]); err != nil {
			return
		}
		business.Add(1)
		if string(request[:]) != "ping" {
			t.Error(fmt.Errorf("unexpected business request: %q", request))
			return
		}
		_, _ = stream.Write([]byte("pong"))
		<-ctx.Done()
	}()
}
