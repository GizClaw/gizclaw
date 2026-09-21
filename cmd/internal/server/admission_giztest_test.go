package server

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	var denied atomic.Int64
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != gizwebrtc.SignalingPath {
			srv.ServeHTTP(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, r)
		if recorder.Code == http.StatusForbidden && bytes.Contains(recorder.Body.Bytes(), []byte("peer_forbidden")) {
			denied.Add(1)
		}
		maps.Copy(w.Header(), recorder.Header())
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
	}))
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
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, adminhttp.RuntimeProfileUpsert{Id: "admission", Spec: apitypes.RuntimeProfileSpec{AppConfig: new(apitypes.RuntimeProfileAppConfig{"admission.marker": "accepted"})}}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []adminhttp.RegistrationTokenUpsert{
		{Id: "sdk", Token: "admission-sdk", RuntimeProfileId: "admission"},
		{Id: "one-slot", Token: "admission-one-slot", RuntimeProfileId: "admission", MaxActivations: new(int64(1))},
		{Id: "disabled", Token: "admission-disabled", RuntimeProfileId: "admission", Enabled: new(false)},
		{Id: "expired", Token: "admission-expired", RuntimeProfileId: "admission", ExpiresAt: new(time.Now().Add(-time.Hour))},
	} {
		if _, err := adminapi.CreateRegistrationToken(ctx, admin, body); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIZCLAW_TEST_ENDPOINT", httpServer.URL)
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "admission"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, file, token string
		accepted          bool
	}{
		{"valid and credential-free reconnect", "valid", "admission-one-slot", true},
		{"missing", "missing", "admission-one-slot", false},
		{"wrong", "wrong", "admission-one-slot", false},
		{"unknown type", "unknown-type", "admission-one-slot", false},
		{"legacy unnamespaced type", "legacy-type", "admission-one-slot", false},
		{"unknown version", "unknown-version", "admission-one-slot", false},
		{"disabled", "valid", "admission-disabled", false},
		{"expired", "valid", "admission-expired", false},
		{"exhausted new device", "valid", "admission-one-slot", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", test.token)
			reportPath := filepath.Join(t.TempDir(), "report.json")
			before := denied.Load()
			output, runErr := run(ctx, filepath.Join(root, test.file+".giztest.yaml"), reportPath)
			if test.accepted {
				if runErr != nil {
					t.Fatalf("runner: %v\n%s", runErr, output)
				}
			} else if runErr == nil || denied.Load() != before+1 {
				t.Fatalf("expected exactly one Server admission denial: err=%v denials=%d\n%s", runErr, denied.Load()-before, output)
			}
			data, err := os.ReadFile(reportPath)
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				Status string `json:"status"`
				Tasks  []struct {
					Status string `json:"status"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			want := "failed"
			if test.accepted {
				want = "passed"
			}
			if report.Status != want || len(report.Tasks) != 1 || report.Tasks[0].Status != want {
				t.Fatalf("unexpected report: %s", data)
			}
		})
	}
	if after != nil {
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "admission-sdk")
		after(ctx)
	}
	item, err := adminapi.GetRegistrationToken(ctx, admin, "one-slot")
	if err != nil || item.ActivationCount != 1 {
		t.Fatalf("activation count = %d, %v", item.ActivationCount, err)
	}
}
