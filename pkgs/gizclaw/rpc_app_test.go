package gizclaw

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/app"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
	"github.com/google/jsonschema-go/jsonschema"
)

type appCountingConn struct {
	giznet.Conn
	calls atomic.Int32
}

func (c *appCountingConn) Dial(uint64) (net.Conn, error) {
	c.calls.Add(1)
	return nil, errors.New("device unavailable")
}

func TestPeerAppCachedProjectionAndInvocation(t *testing.T) {
	tools := toolkittest.New(t)
	apps := &app.Server{DB: tools.DB}
	if err := apps.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	value := apitypes.App{Id: "clock", AppName: "clock", Runtime: "runtime.lua.gizos", Sha256: "hash", Methods: []apitypes.AppMethod{{Name: "read", Mode: "call", InputSchema: jsonschema.Schema{Type: "object"}}}}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tools.DB.Exec(`INSERT INTO apps (id,app_name,data) VALUES (?,?,?)`, value.Id, value.AppName, string(raw)); err != nil {
		t.Fatal(err)
	}
	peer := &PeerConn{}
	conn := &appCountingConn{}
	client := peerAppClient{conn: conn, apps: apps, status: &peer.appStatus}
	ctx, err := agenthost.WithToolExecution(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx = agenthost.WithAppExecution(ctx, &map[string]apitypes.RuntimeProfileBinding{"clock": {ResourceId: "clock"}}, client)
	invoker := &agenthost.ToolkitInvoker{Builder: &toolkit.Builder{Tools: tools}}
	for _, state := range []struct {
		runtime, hash string
		ready         bool
		want          int
	}{
		{value.Runtime, value.Sha256, false, 0},
		{"other", value.Sha256, true, 0},
		{value.Runtime, "old", true, 0},
		{value.Runtime, value.Sha256, true, 1},
	} {
		peer.publishAppStatus(state.runtime, map[string]string{"clock": state.hash}, state.ready)
		definitions, err := invoker.ResolveTools(ctx)
		if err != nil || len(definitions) != state.want {
			t.Fatalf("projection: %v %v", definitions, err)
		}
	}
	if conn.calls.Load() != 0 {
		t.Fatal("projection contacted device")
	}
	// Only the method invocation may dial; a preceding list would add another dial.
	if _, err := invoker.InvokeTool(ctx, "clock__read", nil); err != nil {
		t.Fatal(err)
	}
	if conn.calls.Load() != 1 {
		t.Fatalf("invocation dialed %d times", conn.calls.Load())
	}
	peer.appStatus.Store(nil)
	if defs, err := invoker.ResolveTools(ctx); err != nil || len(defs) != 0 {
		t.Fatalf("unreconciled: %v %v", defs, err)
	}
	if conn.calls.Load() != 1 {
		t.Fatal("unreconciled projection contacted device")
	}

	// HTTP execution with App bindings must never contact the device.
	_, err = tools.CreateTool(t.Context(), toolkit.Tool{ID: "weather", InvokeName: "weather", Type: toolkit.ToolTypeHTTPRequest, Enabled: true,
		InputSchema: jsonschema.Schema{Type: "object"}, HTTP: toolkit.HTTPRequest{URL: "https://example.com/weather", Method: http.MethodGet, Auth: toolkit.HTTPAuth{Method: "none"}, Timeout: time.Second, MaxResponseBytes: 1024, SuccessStatusCodes: []int{200}}})
	if err != nil {
		t.Fatal(err)
	}
	httpCtx, err := agenthost.WithToolExecution(t.Context(), &map[string]apitypes.RuntimeProfileBinding{"weather": {ResourceId: "weather"}})
	if err != nil {
		t.Fatal(err)
	}
	invoker.HTTP = giztools.HTTPExecutor{Transport: appHTTPTransport{}}
	for _, state := range []struct {
		client agenthost.AppClient
		ready  bool
	}{{nil, false}, {client, false}, {client, true}} {
		peer.publishAppStatus(value.Runtime, map[string]string{"clock": value.Sha256}, state.ready)
		scope := agenthost.WithAppExecution(httpCtx, &map[string]apitypes.RuntimeProfileBinding{"clock": {ResourceId: "clock"}}, state.client)
		result, err := invoker.InvokeTool(scope, "weather", nil)
		if err != nil || string(result) != `{"ok":true}` {
			t.Fatalf("HTTP: %s %v", result, err)
		}
	}
	if conn.calls.Load() != 1 {
		t.Fatal("HTTP invocation contacted device")
	}
}

func TestPeerAppStatusConcurrentSnapshots(t *testing.T) {
	peer := &PeerConn{}
	installed := map[string]string{"clock": "first"}
	peer.publishAppStatus("runtime", installed, true)
	installed["clock"] = "second"
	if peer.appStatus.Load().installed["clock"] != "first" {
		t.Fatal("snapshot aliases writer map")
	}
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for range 1000 {
				peer.publishAppStatus("runtime", installed, true)
				snapshot := peer.appStatus.Load()
				if snapshot.runtime != "runtime" || !snapshot.ready || snapshot.installed["clock"] == "" {
					t.Error("inconsistent snapshot")
				}
			}
		})
	}
	group.Wait()
}

type appHTTPTransport struct{}

func (appHTTPTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
}
