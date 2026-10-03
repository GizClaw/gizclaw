package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"sigs.k8s.io/yaml"
)

// TestSelfHostedMem0Giztest uses the real service, models and shared PostgreSQL
// from run_mem0_tests.sh. The ordinary Go suite has no paid-model dependency.
func TestSelfHostedMem0Giztest(t *testing.T) {
	endpoint := os.Getenv("GIZCLAW_TEST_MEM0_ENDPOINT")
	if endpoint == "" {
		t.Skip("run tests/gizclaw-e2e/run_mem0_tests.sh for real Mem0 Giztests")
	}
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("GIZCLAW_TEST_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cfg := validLayeredConfig(t.TempDir())
	cfg.PendingDeletion = PendingDeletionConfig{
		ScanInterval: "100ms", PageSize: 100, DispatchCapacity: 256, Workers: 4,
		LeaseDuration: "2m", AttemptTimeout: "90s", RetryInitial: "100ms", RetryMax: "2s", MaxAttempts: 10,
	}
	cfg.Storage["business-db"] = storage.PostgreSQLConfig{DSN: dsn}
	cfg.Storage["peer-runs-db"] = storage.PostgreSQLConfig{DSN: dsn}
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

	root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e")
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	load := func(path string) apitypes.Resource {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, "testdata", "resources", path))
		if err != nil {
			t.Fatal(err)
		}
		raw, err = yaml.YAMLToJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		var resource apitypes.Resource
		if err := json.Unmarshal(raw, &resource); err != nil {
			t.Fatal(err)
		}
		return resource
	}
	for _, path := range []string{"00-default.yaml", "14-scope-peer.yaml", "15-scope-workspace.yaml"} {
		resource := load("04-memory-layouts/" + path)
		layout, err := resource.AsMemoryLayoutResource()
		if err != nil {
			t.Fatal(err)
		}
		// Opposite Cloud scopes prove routing uses connection.type, even in
		// the Workspace/Peer deletion handlers and reload path.
		if path == "14-scope-peer.yaml" {
			layout.Spec.Mem0.Scope = new(apitypes.Mem0MemoryLayoutPolicyScopeWorkspace)
		} else {
			layout.Spec.Mem0.Scope = new(apitypes.Mem0MemoryLayoutPolicyScopePeer)
		}
		response, err := api.CreateMemoryLayoutWithResponse(ctx, adminhttp.MemoryLayoutUpsert{Id: layout.Metadata.Id, Spec: layout.Spec})
		if err != nil || response == nil || response.StatusCode() != http.StatusOK {
			t.Fatalf("create Layout: %v, %#v", err, response)
		}
	}
	memories := make(map[string]apitypes.RuntimeProfileMemoryBinding)
	for alias, layout := range map[string]string{
		"default-memory": "default-memory", "giztest-peer-memory-a": "giztest-scope-peer-memory",
		"giztest-peer-memory-b": "giztest-scope-peer-memory", "giztest-workspace-memory": "giztest-scope-workspace-memory",
	} {
		var connection apitypes.RuntimeProfileMemoryConnection
		if err := connection.FromRuntimeProfileMem0SelfHostedConnection(apitypes.RuntimeProfileMem0SelfHostedConnection{
			Type: apitypes.RuntimeProfileMem0SelfHostedConnectionTypeMem0SelfHosted, Endpoint: endpoint,
			ApiKey: new(os.Getenv("GIZCLAW_E2E_MEM0_API_KEY")),
		}); err != nil {
			t.Fatal(err)
		}
		memories[alias] = apitypes.RuntimeProfileMemoryBinding{Driver: apitypes.RuntimeProfileMemoryDriverMem0, LayoutId: layout, Connection: connection}
	}
	workflows := make(apitypes.RuntimeProfileWorkflows)
	for _, path := range []string{"42-flowcraft-memory-peer-a.yaml", "43-flowcraft-memory-peer-b.yaml", "44-flowcraft-memory-workspace.yaml", "48-mem0-extraction.yaml", "49-mem0-batch.yaml"} {
		resource := load("04-workflows/" + path)
		workflow, err := resource.AsWorkflowResource()
		if err != nil {
			t.Fatal(err)
		}
		// Scope probes write direct Facts and need no answer model. The raw
		// extraction document reaches the real Mem0 LLM and embedder.
		if workflow.Spec.Flowcraft != nil {
			for i := range workflow.Spec.Flowcraft.Graph.Nodes {
				node := &workflow.Spec.Flowcraft.Graph.Nodes[i]
				kind, err := node.Discriminator()
				if err != nil {
					t.Fatal(err)
				}
				if kind == "llm" {
					definition, err := node.AsFlowcraftLLMNode()
					if err != nil {
						t.Fatal(err)
					}
					if err := node.FromFlowcraftScriptNode(apitypes.FlowcraftScriptNode{
						Id: definition.Id, Publish: definition.Publish, Type: apitypes.FlowcraftScriptNodeTypeScript,
						Config: apitypes.FlowcraftScriptNodeConfig{Source: `host.emit("token", {content: "Memory observation accepted."});`},
					}); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		id := workflow.Metadata.Id
		if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: id, Spec: workflow.Spec}); err != nil {
			t.Fatal(err)
		}
		workflows[id] = apitypes.RuntimeProfileBinding{ResourceId: id, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: id}, "zh-CN": {DisplayName: id}}}
	}
	profile := adminhttp.RuntimeProfileUpsert{Id: "mem0-giztest", Spec: apitypes.RuntimeProfileSpec{
		Workflows: workflows, Resources: apitypes.RuntimeProfileResources{Memories: &memories},
	}}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "mem0-token", Token: "mem0-test-token", RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "mem0-test-token")
	for _, name := range []string{"flowcraft-memory-scope.peer-and-workspace", "mem0-self-hosted.extraction", "mem0-self-hosted.batch"} {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		report := filepath.Join(t.TempDir(), name+".json")
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", "--parallel", "1", "--output", report, filepath.Join(root, "giztest", name+".giztest.yaml")})
		if err := command.ExecuteContext(ctx); err != nil {
			body, _ := os.ReadFile(report)
			t.Fatalf("Giztest %s: %v\n%s\n%s", name, err, output.String(), body)
		}
		t.Log(output.String())
	}
	// Peer deletion acknowledges a durable retirement marker. Keep Server
	// running until its child Workspace and owner-binding cleanup completes.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var owners, workspaces int
		if err := server.WorkspaceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM runtime_profile_owners WHERE runtime_profile_id IS NOT NULL").Scan(&owners); err != nil {
			t.Fatal(err)
		}
		if err := server.WorkspaceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces").Scan(&workspaces); err != nil {
			t.Fatal(err)
		}
		if owners == 0 && workspaces == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Peer cleanup unfinished: owners=%d, workspaces=%d", owners, workspaces)
		case <-ticker.C:
		}
	}

}
