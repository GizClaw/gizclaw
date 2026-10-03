package server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/admissiontest"
)

func TestAdmissionGiztestGo(t *testing.T) {
	runAdmissionGiztests(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--output", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	}, nil)
}

// Negative documents intentionally fail during client setup. Only a structured
// peer_forbidden response from this Server counts as an expected rejection.
func runAdmissionGiztests(t *testing.T, run func(context.Context, string, string) ([]byte, error), after func(context.Context)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cfg := validLayeredConfig(t.TempDir())
	cfg.PeerAdmission = "registration-token"
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminPublicKey = key.Public
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
	// Provision the administrative identity as an existing Peer. Admin service
	// authorization intentionally does not bypass the selected admission policy.
	if _, err := srv.Manager().Peers.EnsureConnectedPeer(ctx, key.Public); err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()
	t.Cleanup(func() {
		_ = srv.Close()
		select {
		case err := <-served:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Server did not stop")
		}
	})
	admin := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = admin.Serve() }()

	root, err := filepath.Abs(filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "admission"))
	if err != nil {
		t.Fatal(err)
	}
	admissiontest.Run(t, admissiontest.Host{
		Endpoint: httpServer.URL, Documents: root, Run: run,
		PublicTransport: admin.HTTPClient(gizcli.ServicePeerHTTP).Transport,
		CreateProfile: func(ctx context.Context, body adminhttp.RuntimeProfileUpsert) error {
			_, err := adminapi.CreateRuntimeProfile(ctx, admin, body)
			return err
		},
		CreateToken: func(ctx context.Context, body adminhttp.RegistrationTokenUpsert) error {
			_, err := adminapi.CreateRegistrationToken(ctx, admin, body)
			return err
		},
		GetToken: func(ctx context.Context, id string) (apitypes.RegistrationToken, error) {
			return adminapi.GetRegistrationToken(ctx, admin, id)
		},
		Block: func(ctx context.Context, key string) error { _, err := adminapi.BlockPeer(ctx, admin, key); return err },
		Approve: func(ctx context.Context, key string) error {
			_, err := adminapi.ApprovePeer(ctx, admin, key, apitypes.PeerRoleClient)
			return err
		},
		GetPeer: func(ctx context.Context, key string) (apitypes.Registration, error) {
			result, err := adminapi.GetPeer(ctx, admin, key)
			if err != nil {
				return apitypes.Registration{}, err
			}
			return result.AsExternalRef0Registration()
		},
		Runtime: func(ctx context.Context, key string) (apitypes.Runtime, error) {
			return adminapi.GetPeerRuntime(ctx, admin, key)
		},
	})
	if after != nil {
		if _, err := adminapi.CreateRuntimeProfile(ctx, admin, adminhttp.RuntimeProfileUpsert{Id: "sdk", Spec: apitypes.RuntimeProfileSpec{AppConfig: new(apitypes.RuntimeProfileAppConfig{"admission.marker": "accepted"})}}); err != nil {
			t.Fatal(err)
		}
		if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "sdk", Token: "admission-sdk", RuntimeProfileId: "sdk"}); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIZCLAW_TEST_ENDPOINT", httpServer.URL)
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "admission-sdk")
		after(ctx)
	}
}
