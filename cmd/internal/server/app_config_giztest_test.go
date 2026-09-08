package server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"reflect"
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
)

// This runs the committed Giztests against a real Server and WebRTC driver.
// Admin HTTP seeds and updates the same on-disk database the Peer RPCs read.
func TestRuntimeProfileAppConfigGiztest(t *testing.T) {
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
	start := func() {
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
	var node apitypes.FlowcraftNode
	if err := node.FromFlowcraftPassthroughNode(apitypes.FlowcraftPassthroughNode{Id: "passthrough", Type: apitypes.FlowcraftPassthroughNodeTypePassthrough, Publish: new(true)}); err != nil {
		t.Fatal(err)
	}
	_, err = adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "pet-care", Spec: apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverPet,
		Pet: &apitypes.PetWorkflowSpec{Driver: apitypes.ReusableWorkflowDriverFlowcraft, Flowcraft: &apitypes.FlowcraftWorkflowSpec{
			Graph: apitypes.FlowcraftGraph{Name: "app-config-pet", Entry: "passthrough", Nodes: []apitypes.FlowcraftNode{node}, Edges: new([]apitypes.FlowcraftEdge{{From: "passthrough", To: "__end__"}})},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	config := apitypes.RuntimeProfileAppConfig{
		"ui.theme": "dark", "feature.flags": "beta-voice,beta-pet",
		"app.entrypoints": "{\"home\": \"/tab/home\", \"settings\": \"/tab/settings\"}\n",
	}
	request := adminhttp.RuntimeProfileUpsert{Id: "app-config-giztest", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{System: apitypes.RuntimeProfileSystemWorkflows{Pet: "pet-care"}, Collections: apitypes.RuntimeProfileWorkflowCollections{}},
		AppConfig: &config,
	}}
	created, err := adminapi.CreateRuntimeProfile(ctx, admin, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "app-config-token", Token: "app-config-test-token", RuntimeProfileId: request.Id}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "app-config-test-token")
	root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e")
	run := func(files ...string) {
		t.Helper()
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs(append([]string{"run", "--parallel", "1"}, files...))
		if err := command.ExecuteContext(ctx); err != nil {
			t.Fatalf("Giztest: %v\n%s", err, output.String())
		}
		t.Log(output.String())
	}
	checkAdmin := func(want apitypes.RuntimeProfile) {
		t.Helper()
		got, err := adminapi.GetRuntimeProfile(ctx, admin, request.Id)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Admin read = %#v, %v; want %#v", got, err, want)
		}
	}
	run(filepath.Join(root, "giztest", "server.app_config.get.giztest.yaml"), filepath.Join(root, "giztest", "server.app_config.list.giztest.yaml"))
	checkAdmin(created)
	request.Spec.AppConfig = new(apitypes.RuntimeProfileAppConfig{"ui.theme": "light"})
	updated, err := adminapi.PutRuntimeProfile(ctx, admin, request.Id, request)
	if err != nil || updated.Revision == created.Revision {
		t.Fatalf("update revision = %q, error = %v", updated.Revision, err)
	}
	checkAdmin(updated)
	updatedCase := filepath.Join(root, "testdata", "app-config", "updated.giztest.yaml")
	run(updatedCase)
	stop()
	start()
	checkAdmin(updated)
	run(updatedCase)
	for _, empty := range []*apitypes.RuntimeProfileAppConfig{new(apitypes.RuntimeProfileAppConfig{}), nil} {
		request.Spec.AppConfig = empty
		cleared, err := adminapi.PutRuntimeProfile(ctx, admin, request.Id, request)
		if err != nil {
			t.Fatal(err)
		}
		checkAdmin(cleared)
		run(filepath.Join(root, "testdata", "app-config", "empty.giztest.yaml"))
	}
}
