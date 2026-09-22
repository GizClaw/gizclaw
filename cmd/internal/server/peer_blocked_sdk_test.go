package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// This SDK E2E uses real WebRTC and Admin HTTP in normal Go CI. The empty
// selection deliberately exercises the default open admission configuration.
func TestPeerBlockedSDKWebRTC(t *testing.T) {
	for _, admission := range []string{"", "registration-token"} {
		name := admission
		if name == "" {
			name = "default-open"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			cfg := validLayeredConfig(t.TempDir())
			cfg.PeerAdmission = admission
			adminKey, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			deviceKey, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			cfg.AdminPublicKey = adminKey.Public
			srv, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			httpServer := httptest.NewServer(srv)
			t.Cleanup(httpServer.Close)
			srv.PublicEndpoint = strings.TrimPrefix(httpServer.URL, "http://")
			srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
				listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: opts.SecurityPolicy, PeerEventHandler: opts.PeerEventHandler}).Listen(opts.KeyPair)
				if err == nil {
					srv.WebRTCSignalingHandler = listener.SignalingHandler()
				}
				return listener, err
			}}
			if err := srv.Listen(); err != nil {
				_ = srv.Close()
				t.Fatal(err)
			}
			if admission != "" {
				for _, key := range []giznet.PublicKey{adminKey.Public, deviceKey.Public} {
					if _, err := srv.Manager().Peers.EnsureConnectedPeer(ctx, key); err != nil {
						t.Fatal(err)
					}
				}
			}
			serverDone := make(chan error, 1)
			go func() { serverDone <- srv.Serve() }()
			t.Cleanup(func() {
				_ = srv.Close()
				select {
				case err := <-serverDone:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					t.Error("Server did not stop")
				}
			})
			newClient := func(key *giznet.KeyPair) *gizcli.Client {
				client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
					return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
				}}
				t.Cleanup(func() { _ = client.Close() })
				return client
			}
			connect := func(client *gizcli.Client) chan error {
				t.Helper()
				if err := client.Dial(srv.PublicKey(), httpServer.URL); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- client.Serve() }()
				t.Cleanup(func() {
					_ = client.Close()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("client did not stop")
					}
				})
				if _, err := client.Ping(ctx, "before-block"); err != nil {
					t.Fatal(err)
				}
				return done
			}
			admin := newClient(adminKey)
			connect(admin)
			device := newClient(deviceKey)
			deviceDone := connect(device)
			// Continuously open real RPC service streams while Admin block races
			// with the connected device; transport closure must stop this work.
			firstStream := make(chan error, 1)
			streamsDone := make(chan struct{})
			go func() {
				defer close(streamsDone)
				_, err := device.Ping(ctx, "stream-before-block")
				firstStream <- err
				for err == nil {
					_, err = device.Ping(ctx, "stream-racing-block")
				}
			}()
			t.Cleanup(func() {
				_ = device.Close()
				select {
				case <-streamsDone:
				case <-time.After(5 * time.Second):
					t.Error("concurrent RPC worker did not stop")
				}
			})
			if err := <-firstStream; err != nil {
				t.Fatal(err)
			}
			blocked, err := adminapi.BlockPeer(ctx, admin, deviceKey.Public.String())
			if err != nil || blocked.Status != apitypes.PeerRegistrationStatusBlocked {
				t.Fatalf("Admin block = %+v, %v", blocked, err)
			}
			select {
			case err := <-deviceDone:
				// Preserve the completion for the common lifecycle cleanup.
				deviceDone <- err
			case <-time.After(5 * time.Second):
				t.Fatal("blocked online device was not disconnected")
			}
			if _, online := srv.Manager().Peer(deviceKey.Public); online {
				t.Fatal("blocked device remains online")
			}
			select {
			case <-streamsDone:
			case <-time.After(5 * time.Second):
				t.Fatal("blocked device continued opening RPC streams")
			}
			attempt := newClient(deviceKey)
			err = attempt.Dial(srv.PublicKey(), httpServer.URL)
			if admission == "registration-token" && (err == nil || !strings.Contains(err.Error(), "peer_forbidden")) {
				t.Fatalf("handshake no longer rejects blocked Peer: %v", err)
			}
			if err == nil {
				go func() { _ = attempt.Serve() }()
				pingCtx, stopPing := context.WithTimeout(ctx, 3*time.Second)
				_, pingErr := attempt.Ping(pingCtx, "blocked-reconnect")
				stopPing()
				if pingErr == nil {
					t.Fatal("blocked reconnect served RPC")
				}
			}
			_ = attempt.Close()
			if _, online := srv.Manager().Peer(deviceKey.Public); online {
				t.Fatal("blocked reconnect became online")
			}
			if _, err := adminapi.ApprovePeer(ctx, admin, deviceKey.Public.String(), apitypes.PeerRoleClient); err != nil {
				t.Fatal(err)
			}
			connect(newClient(deviceKey))
		})
	}
}
