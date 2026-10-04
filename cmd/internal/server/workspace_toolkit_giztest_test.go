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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/google/jsonschema-go/jsonschema"
)

// TestRuntimeProfileAndWorkspaceToolkitGiztest runs committed Giztest documents
// through real WebRTC Peer RPC and SQL storage without an external provider.
func TestRuntimeProfileAndWorkspaceToolkitGiztest(t *testing.T) {
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

	// Keep these Tools disabled: Workspace selection resolves bindings whether
	// or not a Tool is enabled.
	tools := &toolkit.Server{DB: server.ToolDB}
	for _, entry := range []struct{ id, name string }{{"giztest-toolkit-echo", "giztest_echo"}, {"giztest-toolkit-other", "giztest_other"}} {
		if _, err := tools.CreateTool(ctx, toolkit.Tool{ID: entry.id, InvokeName: entry.name, Type: toolkit.ToolTypeHTTPRequest, Enabled: false, InputSchema: jsonschema.Schema{Type: "object"}, HTTP: &toolkit.HTTPRequest{URL: "https://giztest.invalid/" + entry.name, Method: "GET", Auth: toolkit.HTTPAuth{Method: "none"}, Timeout: time.Second, MaxResponseBytes: 1024}}); err != nil {
			t.Fatal(err)
		}
	}
	// No Agent runs in this document. A valid passthrough Workflow lets the real
	// Workspace service validate creation without any model or provider setup.
	graph := apitypes.EinoWorkflowSpec{}
	if err := json.Unmarshal([]byte(`{"graph":{"name":"toolkit","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":8},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    return {\"text\":input[\"text\"]}\n","inputs":{"text":{"from":"input.text"}},"outputs":{"text":"answer-text"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"initialize-conversation","to":"answer"},{"from":"answer","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}`), &graph); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "toolkit-workflow", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &graph}}); err != nil {
		t.Fatal(err)
	}
	bindings := map[string]apitypes.RuntimeProfileBinding{"giztest-echo": {ResourceId: "giztest-toolkit-echo", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Echo"}, "zh-CN": {DisplayName: "Echo"}}}, "giztest-other": {ResourceId: "giztest-toolkit-other", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Other"}, "zh-CN": {DisplayName: "Other"}}}}
	profile := adminhttp.RuntimeProfileUpsert{Id: "workspace-toolkit", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{
			"eino-chat-assistant":  {ResourceId: "toolkit-workflow", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Toolkit Chat"}, "zh-CN": {DisplayName: "Toolkit Chat"}}, Tags: &[]string{"assistants", "6-8", "catalog"}},
			"eino-voice-assistant": {ResourceId: "toolkit-workflow", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Toolkit Voice"}, "zh-CN": {DisplayName: "Toolkit Voice"}}, Tags: &[]string{"assistants", "9-12", "catalog"}},
		},
		Resources: apitypes.RuntimeProfileResources{Tools: &bindings},
	}}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "toolkit-token", Token: "local-toolkit-test-token", RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-toolkit-test-token")
	run := func(path string) {
		t.Helper()
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		reportPath := filepath.Join(t.TempDir(), filepath.Base(path)+".report.json")
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", "--parallel", "1", "--output", reportPath, path})
		if err := command.ExecuteContext(ctx); err != nil {
			report, _ := os.ReadFile(reportPath)
			t.Fatalf("Giztest %s: %v\n%s\n%s", path, err, output.String(), report)
		}
		t.Log(output.String())
	}
	root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e")
	run(filepath.Join(root, "giztest", "server.workspace.toolkit.roundtrip.giztest.yaml"))
	run(filepath.Join(root, "giztest", "server.runtime_profile.tags.giztest.yaml"))
	chat := profile.Spec.Workflows["eino-chat-assistant"]
	chat.Tags = &[]string{"assistants", "10-12", "catalog"}
	profile.Spec.Workflows["eino-chat-assistant"] = chat
	if _, err := adminapi.PutRuntimeProfile(ctx, admin, profile.Id, profile); err != nil {
		t.Fatal(err)
	}
	run(filepath.Join(root, "testdata", "runtime-profile", "updated.giztest.yaml"))
}
