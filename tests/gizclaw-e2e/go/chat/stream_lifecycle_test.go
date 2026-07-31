//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestPeerStreamWorkspaceReloadContinuity(t *testing.T) {
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}
	h := clitest.NewSetupHarness(t, "go-chat-stream-lifecycle")
	configPath := filepath.Join(
		h.RepoRoot,
		"tests",
		"gizclaw-e2e",
		"testdata",
		"workspaces",
		"doubao-realtime.json",
	)
	cfg, err := loadConfig(configPath, clientContextConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rounds = 1
	cfg.Ensure = ptr(false)
	cfg.Workflow.Parameters.Input = string(rpcapi.WorkspaceInputModePushToTalk)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, serveDone, err := dialClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = client.Close()
			<-serveDone
		}
	}()
	registrationToken := createChatRegistrationToken(t, workspaceCasePushToTalkRoundtrip)
	if _, err := client.Register(ctx, "stream-lifecycle.register", registrationToken); err != nil {
		t.Fatalf("register chat client: %v", err)
	}
	workspaceSuffix := fmt.Sprintf("%x", time.Now().UnixNano())
	firstWorkspace := "go-stream-lifecycle-a-" + workspaceSuffix
	alternateWorkspace := "go-stream-lifecycle-b-" + workspaceSuffix

	original, err := client.GetServerRunWorkspace(ctx, "stream-lifecycle.original")
	if err != nil {
		t.Fatalf("capture original run Workspace: %v", err)
	}
	restored := false
	defer func() {
		if !restored {
			if err := restoreRunWorkspace(client, original.WorkspaceName); err != nil {
				t.Errorf("restore original Workspace after failure: %v", err)
				cleanupCtx, cleanupCancel := context.WithTimeout(
					context.Background(),
					10*time.Second,
				)
				if _, stopErr := client.StopServerRun(
					cleanupCtx,
					"stream-lifecycle.cleanup-stop",
				); stopErr != nil {
					t.Errorf("stop active Workspace before cleanup: %v", stopErr)
				}
				cleanupCancel()
			}
			deleteLifecycleWorkspaces(t, client, firstWorkspace, alternateWorkspace)
		}
	}()
	firstConfig := cfg
	firstConfig.Workspace = firstWorkspace
	if _, err := ensureWorkspace(ctx, client, firstConfig); err != nil {
		t.Fatalf("create first lifecycle Workspace: %v", err)
	}
	alternateConfig := cfg
	alternateConfig.Workspace = alternateWorkspace
	if _, err := ensureWorkspace(ctx, client, alternateConfig); err != nil {
		t.Fatalf("create alternate lifecycle Workspace: %v", err)
	}

	openaiHTTPClient := client.HTTPClient(gizcli.ServicePeerOpenAI)
	openaiHTTPClient.Timeout = cfg.timeout
	openaiClient := openai.NewClient(
		option.WithAPIKey("gizclaw-peer"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(openaiHTTPClient),
	)
	driver := &personaDriver{
		cfg:           cfg,
		client:        openaiClient,
		runtimeClient: client,
		newTransport: func() (*chatTransport, error) {
			return newChatTransport(client)
		},
	}
	defer driver.close()
	peerConn := client.PeerConn()
	if peerConn == nil {
		t.Fatal("PeerConn is nil after Dial")
	}
	requireDirectPacketTelemetry(t, ctx, client)
	if client.PeerConn() != peerConn {
		t.Fatal("Direct Packet telemetry changed the PeerConn identity")
	}

	steps := []struct {
		name      string
		workspace string
		set       bool
	}{
		{name: "set-first", workspace: firstWorkspace, set: true},
		{name: "reload-active", workspace: firstWorkspace},
		{name: "set-alternate", workspace: alternateWorkspace, set: true},
	}
	var stableTransport *chatTransport
	for index, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			driver.cfg.Workspace = step.workspace
			driver.reloadAgent = func(ctx context.Context) error {
				if step.set {
					return selectAndReloadAgent(ctx, client, driver.cfg)
				}
				return reloadActiveWorkspace(ctx, client, step.workspace)
			}
			stats, err := driver.runPushToTalkRoundtripWithMode(
				ctx,
				lightweightBehaviorMode("peer-stream-workspace-lifecycle"),
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(stats) != 1 {
				t.Fatalf("round count = %d, want 1", len(stats))
			}
			stat := stats[0]
			if stat.EventCount == 0 || stat.InputOpusPackets == 0 || stat.DownlinkPackets == 0 {
				t.Fatalf(
					"round transport evidence events=%d input_opus=%d downlink_opus=%d",
					stat.EventCount,
					stat.InputOpusPackets,
					stat.DownlinkPackets,
				)
			}
			if client.PeerConn() != peerConn {
				t.Fatal("Workspace replacement changed the PeerConn identity")
			}
			if index == 0 {
				stableTransport = driver.transport
			} else if driver.transport != stableTransport {
				t.Fatal("Workspace replacement changed the logical Peer stream")
			}
		})
	}

	restoreErr := restoreRunWorkspace(client, original.WorkspaceName)
	if restoreErr != nil {
		t.Fatalf("restore original Workspace: %v", restoreErr)
	}
	deleteLifecycleWorkspaces(t, client, firstWorkspace, alternateWorkspace)
	restored = true
	driver.close()
	if err := client.Close(); err != nil {
		t.Fatalf("close chat client: %v", err)
	}
	<-serveDone
	closed = true
	requirePeerOffline(t, h, client.KeyPair.Public.String())
}

func deleteLifecycleWorkspaces(
	t *testing.T,
	client *gizcli.Client,
	workspaces ...string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, workspace := range workspaces {
		if _, err := client.DeleteWorkspace(
			ctx,
			"stream-lifecycle.cleanup."+workspace,
			rpcapi.WorkspaceDeleteRequest{Name: workspace},
		); err != nil && !isRPCNotFound(err) {
			t.Errorf("delete lifecycle Workspace %q: %v", workspace, err)
		}
	}
}

func reloadActiveWorkspace(
	ctx context.Context,
	client *gizcli.Client,
	workspace string,
) error {
	reloaded, err := client.ReloadServerRunWorkspace(ctx, "stream-lifecycle.reload-active")
	if err != nil {
		return err
	}
	if reloaded.WorkspaceName != workspace {
		return fmt.Errorf("reloaded Workspace = %q, want %q", reloaded.WorkspaceName, workspace)
	}
	return nil
}

func restoreRunWorkspace(client *gizcli.Client, workspace string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if strings.TrimSpace(workspace) == "" {
		if _, err := client.StopServerRun(ctx, "stream-lifecycle.restore-stop"); err != nil {
			return fmt.Errorf("stop active Workspace: %w", err)
		}
		return nil
	}
	cfg := config{Workspace: workspace}
	if err := selectAndReloadAgent(ctx, client, cfg); err != nil {
		return fmt.Errorf("select and reload %q: %w", workspace, err)
	}
	return nil
}

func requireDirectPacketTelemetry(
	t *testing.T,
	ctx context.Context,
	client *gizcli.Client,
) {
	t.Helper()
	const batteryPercent = 66
	if err := client.SendBatteryTelemetry(batteryPercent, true); err != nil {
		t.Fatalf("send Direct Packet telemetry: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, err := client.GetServerStatus(ctx, "stream-lifecycle.telemetry")
		if err != nil {
			t.Fatalf("read telemetry through server.status.get: %v", err)
		}
		if status.BatteryPercent != nil &&
			*status.BatteryPercent == batteryPercent &&
			status.Charging != nil &&
			*status.Charging {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server.status.get did not reflect Direct Packet telemetry: %#v", status)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func requirePeerOffline(t *testing.T, h *clitest.Harness, publicKey string) {
	t.Helper()
	identitiesHome := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_IDENTITIES_HOME"))
	if identitiesHome == "" {
		identitiesHome = filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities")
	}
	adminName := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_IDENTITY"))
	if adminName == "" {
		adminName = "admin"
	}
	h.SetContextDirAlias("stream-lifecycle-admin", filepath.Join(identitiesHome, adminName))
	admin := h.ConnectClientFromContext("stream-lifecycle-admin")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client for offline assertion: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		response, err := api.GetPeerRuntimeWithResponse(ctx, publicKey)
		cancel()
		if err == nil && response.JSON200 != nil && !response.JSON200.Online {
			return
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("query Peer runtime after close: %v", err)
			}
			t.Fatalf("Peer %s remained online after client close: status=%d body=%s", publicKey, response.StatusCode(), response.Body)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
