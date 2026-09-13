package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/runtimeprofiletest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"io"
	"net"
	"net/http"
	"slices"
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
	value := apitypes.App{Id: "clock", AppName: "clock", Runtime: "runtime.lua.gizos", Sha256: "hash", Requires: &[]string{"litelink.notify"}, Methods: []apitypes.AppMethod{{Name: "read", Mode: "call", InputSchema: jsonschema.Schema{Type: "object"}}}}
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
		capabilities  []string
		ready         bool
		want          int
	}{
		{value.Runtime, value.Sha256, []string{"litelink.notify"}, false, 0},
		{"other", value.Sha256, []string{"litelink.notify"}, true, 0},
		{value.Runtime, "old", []string{"litelink.notify"}, true, 0},
		{value.Runtime, value.Sha256, nil, true, 0},
		{value.Runtime, value.Sha256, []string{"litelink.other"}, true, 0},
		{value.Runtime, value.Sha256, []string{"litelink.notify", "host.extra"}, true, 1},
	} {
		peer.publishAppStatus(state.runtime, map[string]string{"clock": state.hash}, state.capabilities, state.ready)
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
		peer.publishAppStatus(value.Runtime, map[string]string{"clock": value.Sha256}, nil, state.ready)
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
	capabilities := []string{"host.first"}
	peer.publishAppStatus("runtime", installed, capabilities, true)
	installed["clock"] = "second"
	capabilities[0] = "host.second"
	if peer.appStatus.Load().capabilities[0] != "host.first" {
		t.Fatal("snapshot aliases capabilities")
	}
	if peer.appStatus.Load().installed["clock"] != "first" {
		t.Fatal("snapshot aliases writer map")
	}
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for range 1000 {
				peer.publishAppStatus("runtime", installed, capabilities, true)
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

func TestAppMatchesPeer(t *testing.T) {
	for _, tc := range []struct {
		name         string
		requires     *[]string
		runtime      string
		capabilities []string
		want         bool
	}{
		{"legacy", nil, "runtime.lua.gizos", nil, true},
		{"empty", &[]string{}, "runtime.lua.gizos", nil, true},
		{"runtime mismatch", nil, "other", nil, false},
		{"subset", &[]string{"host.one", "host.two"}, "runtime.lua.gizos", []string{"host.extra", "host.two", "host.one"}, true},
		{"partial", &[]string{"host.one", "host.two"}, "runtime.lua.gizos", []string{"host.one"}, false},
		{"exact", &[]string{"host.one"}, "runtime.lua.gizos", []string{"host.One", "host.one.extra"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := apitypes.App{Runtime: "runtime.lua.gizos", Requires: tc.requires}
			if got := appMatchesPeer(value, tc.runtime, tc.capabilities); got != tc.want {
				t.Fatalf("match=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestPeerAppReconcileCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name         string
		runtime      string
		capabilities []string
		installed    bool
		wantInstall  bool
		wantVisible  bool
	}{
		{"missing", "runtime.lua.gizos", nil, false, false, false},
		{"missing already installed", "runtime.lua.gizos", nil, true, false, false},
		{"matching", "runtime.lua.gizos", []string{"host.notify"}, false, true, true},
		{"matching already installed", "runtime.lua.gizos", []string{"host.notify"}, true, false, true},
		{"runtime mismatch", "other", []string{"host.notify"}, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profiles := runtimeprofiletest.New(t)
			apps := &app.Server{DB: profiles.DB}
			if err := apps.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			value := apitypes.App{Id: "clock", AppName: "clock", Runtime: "runtime.lua.gizos", Sha256: "hash", Requires: &[]string{"host.notify"},
				Package: apitypes.FirmwarePackage{Url: "https://example.com/clock.tar.zlib", Sha256: "hash", Size: 42}}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := profiles.DB.Exec(`INSERT INTO apps (id,app_name,data) VALUES (?,?,?)`, value.Id, value.AppName, string(raw)); err != nil {
				t.Fatal(err)
			}
			response, err := profiles.CreateRuntimeProfile(t.Context(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
				Id: "profile", Spec: apitypes.RuntimeProfileSpec{Workflows: testRuntimeProfileWorkflows(), Resources: apitypes.RuntimeProfileResources{
					Apps: &map[string]apitypes.RuntimeProfileBinding{"clock": {ResourceId: "clock", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Clock"}, "zh-CN": {DisplayName: "时钟"}}}},
				}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
				t.Fatalf("profile: %#v", response)
			}
			var installs atomic.Int32
			conn := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
				switch req.Method {
				case rpcapi.RPCMethodClientAppList:
					status := &rpcpb.ClientAppListResponse{Runtime: tc.runtime, Capabilities: tc.capabilities}
					if tc.installed {
						status.Apps = []*rpcpb.InstalledApp{{AppName: value.AppName, Sha256: value.Sha256}}
					}
					return newRPCResultResponse(req.Id, status, (*rpcapi.RPCPayload).FromClientAppListResponse)
				case rpcapi.RPCMethodClientAppInstall:
					installs.Add(1)
					params, err := req.Params.AsClientAppInstallRequest()
					if err != nil {
						return nil, err
					}
					if params.AppName != value.AppName || params.Sha256 != value.Sha256 || params.Url != value.Package.Url || params.Size != value.Package.Size {
						return nil, fmt.Errorf("unexpected install: %v", params)
					}
					return newRPCResultResponse(req.Id, &rpcpb.ClientAppInstallResponse{}, (*rpcapi.RPCPayload).FromClientAppInstallResponse)
				default:
					return nil, fmt.Errorf("unexpected method: %v", req.Method)
				}
			})
			conn.publicKey = giznet.PublicKey{1}
			if err := profiles.BindOwnerProfile(t.Context(), conn.PublicKey().String(), "profile"); err != nil {
				t.Fatal(err)
			}
			peer := &PeerConn{Conn: conn, Service: &PeerService{manager: &Manager{RuntimeProfiles: profiles, Apps: apps}}}
			peer.registration.Store(&runtimeprofile.Registration{})
			if err := peer.reconcileApps(); err != nil {
				t.Fatal(err)
			}
			if got := installs.Load(); (got == 1) != tc.wantInstall || got > 1 {
				t.Fatalf("installs: %d", got)
			}
			snapshot := peer.appStatus.Load()
			if snapshot == nil || !snapshot.ready || !slices.Equal(snapshot.capabilities, tc.capabilities) {
				t.Fatalf("snapshot: %#v", snapshot)
			}
			client := peerAppClient{apps: apps, status: &peer.appStatus}
			visible, err := client.ResolveApps(t.Context(), []string{"clock"})
			if err != nil || (len(visible) == 1) != tc.wantVisible {
				t.Fatalf("visible: %v, %v", visible, err)
			}
		})
	}
}
