package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/google/jsonschema-go/jsonschema"
)

// TestWorkspaceToolkitGiztest executes the committed policy document against
// real WebRTC Peer RPC and SQL storage, without an external endpoint or model.
func TestWorkspaceToolkitGiztest(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	cfg := validLayeredConfig(t.TempDir())
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminPublicKey = key.Public
	var admin *gizcli.Client
	var stop func()
	var server *CmdServer
	start := func() {
		srv, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		server = srv
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
		admin = &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
		}}
		client := admin
		stop = sync.OnceFunc(func() {
			_ = client.Close()
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
		if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
			stop()
			t.Fatal(err)
		}
		go func() { _ = client.Serve() }()
		t.Setenv("GIZCLAW_TEST_ENDPOINT", httpServer.URL)
	}
	start()
	t.Cleanup(func() { stop() })

	tools := &toolkit.Server{DB: server.ToolDB}
	for _, entry := range []struct{ id, name string }{{"giztest-client-echo", "giztest_echo"}, {"giztest-client-other", "giztest_other"}} {
		if _, err := tools.CreateTool(ctx, toolkit.Tool{ID: entry.id, InvokeName: entry.name, Type: toolkit.ToolTypeHTTPRequest, Enabled: true, InputSchema: jsonschema.Schema{Type: "object"}, HTTP: &toolkit.HTTPRequest{URL: "https://example.com/tool", Method: "GET", Auth: toolkit.HTTPAuth{Method: "none"}, Timeout: time.Second, MaxResponseBytes: 1024}}); err != nil {
			t.Fatal(err)
		}
	}
	// No Agent runs in this document. A valid passthrough Workflow lets the real
	// Workspace service validate creation without any model or provider setup.
	graph := apitypes.FlowcraftWorkflowSpec{}
	if err := json.Unmarshal([]byte(`{"graph":{"name":"toolkit","entry":"answer","nodes":[{"id":"answer","type":"passthrough","publish":true}],"edges":[{"from":"answer","to":"__end__"}]}}`), &graph); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "toolkit-workflow", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &graph}}); err != nil {
		t.Fatal(err)
	}
	bindings := map[string]apitypes.RuntimeProfileBinding{"giztest-echo": {ResourceId: "giztest-client-echo", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Echo"}, "zh-CN": {DisplayName: "Echo"}}}, "giztest-other": {ResourceId: "giztest-client-other", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Other"}, "zh-CN": {DisplayName: "Other"}}}}
	profile := adminhttp.RuntimeProfileUpsert{Id: "workspace-toolkit", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{"assistants": {"flowcraft-chat-assistant": {ResourceId: "toolkit-workflow", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Toolkit"}, "zh-CN": {DisplayName: "Toolkit"}}}}}},
		Resources: apitypes.RuntimeProfileResources{Tools: &bindings},
	}}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "toolkit-token", Token: "local-toolkit-test-token", RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-toolkit-test-token")
	command := giztestcmd.NewCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"run", "--parallel", "1", filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "giztest", "server.workspace.toolkit.roundtrip.giztest.yaml")})
	if err := command.ExecuteContext(ctx); err != nil {
		t.Fatalf("Giztest: %v\n%s", err, output.String())
	}
	t.Log(output.String())
}
