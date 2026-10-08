package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

// TestFirmwareMetadataGiztest runs the committed scenario through real WebRTC
// against a provider-free Server, seeded through production manifest and Admin APIs.
func TestFirmwareMetadataGiztest(t *testing.T) {
	runFirmwareMetadataGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--output", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}

func runFirmwareMetadataGiztest(t *testing.T, run func(context.Context, string, string) ([]byte, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	cfg := validLayeredConfig(t.TempDir())
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
	srv.PublicEndpoint = strings.TrimPrefix(httpServer.URL, "http://")
	srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
		listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: opts.SecurityPolicy, PeerEventHandler: opts.PeerEventHandler}).Listen(opts.KeyPair)
		if err == nil {
			srv.WebRTCSignalingHandler = listener.SignalingHandler()
		}
		return listener, err
	}}
	if err := srv.Listen(); err != nil {
		httpServer.Close()
		_ = srv.Close()
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()
	admin := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	stop := sync.OnceFunc(func() {
		_ = admin.Close()
		httpServer.Close()
		if err := srv.Close(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-served:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Server did not stop")
		}
	})
	t.Cleanup(stop)
	if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = admin.Serve() }()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(root, "tests", "gizclaw-e2e", "testdata", "resources", "06-firmwares", "00-devkit-main.yaml")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := adminresource.DecodeManifest(adminresource.FormatYAML, data)
	if err != nil {
		t.Fatal(err)
	}
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminresource.NewClient(api, nil).ApplyResource(ctx, resource); err != nil {
		t.Fatal(err)
	}
	profileID := "firmware-metadata-giztest"
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, adminhttp.RuntimeProfileUpsert{Id: profileID, Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{
		Id: "firmware-metadata-token", Token: "local-firmware-metadata-test-token", RuntimeProfileId: profileID, FirmwareId: new("devkit-firmware-main"),
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_ENDPOINT", httpServer.URL)
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-firmware-metadata-test-token")
	file := filepath.Join(root, "tests", "gizclaw-e2e", "giztest", "server.firmware.metadata.get.giztest.yaml")
	reportPath := filepath.Join(t.TempDir(), "firmware-metadata.report.json")
	output, runErr := run(ctx, file, reportPath)
	reportData, readErr := os.ReadFile(reportPath)
	if runErr != nil || readErr != nil {
		t.Fatalf("Firmware Metadata Giztest: %v, report: %v\n%s\n%s", runErr, readErr, output, reportData)
	}
	var report struct {
		Status string `json:"status"`
		Tasks  []struct {
			Steps []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"steps"`
			Cleanup []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"cleanup"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 16 || len(report.Tasks[0].Cleanup) != 1 {
		t.Fatalf("Giztest must execute all steps: %s", reportData)
	}
	for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
		if step.Status != "passed" {
			t.Fatalf("step %s: %s", step.ID, step.Status)
		}
	}
	t.Logf("Firmware Metadata Giztest: 16 steps and 1 cleanup passed\n%s", output)
}
