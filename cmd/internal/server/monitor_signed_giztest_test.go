package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizedge"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/proto"
)

func TestMonitorSignedGiztest(t *testing.T) {
	runMonitorSignedGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--report", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}

// Only the persistent log backend is a boundary fixture. Signed authorization,
// Edge forwarding, SQLite History/assets, metrics and reverse device RPC are real.
func runMonitorSignedGiztest(t *testing.T, run func(context.Context, string, string) ([]byte, error)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	admin, edge, owner := testKeyPair(t, 141), testKeyPair(t, 142), testKeyPair(t, 143)
	srv, serverURL, _ := startPeerAccessServer(t, kv.NewMemory(nil), kv.NewMemory(nil), edge.Public, admin.Public, func(srv *CmdServer) {
		service, err := gizclaw.NewServerLogQueryService(monitorSignedLogBackend{owner: owner.Public.String()})
		if err != nil {
			t.Fatal(err)
		}
		srv.ServerLogQuery = service
	})
	for _, key := range []*giznet.KeyPair{admin, owner} {
		if _, err := srv.Manager().Peers.SavePeer(ctx, apitypes.Peer{PublicKey: key.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive, Device: apitypes.DeviceInfo{Name: new("Signed fixture")}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.Manager().PeerRoutes.Assign(ctx, owner.Public, nil); err != nil {
		t.Fatal(err)
	}
	profile := adminhttp.RuntimeProfileUpsert{Id: "monitor-signed", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{}, Mhs: &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "wifi.main", Hwd: "wifi"}, {Id: "modem.main", Hwd: "modem"}, {Id: "nfc.left", Hwd: "nfc"}}}}}}
	if response, err := srv.Manager().RuntimeProfiles.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &profile}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("create profile: %T", response)
	}
	if err := srv.Manager().RuntimeProfiles.BindOwnerProfile(ctx, owner.Public.String(), profile.Id); err != nil {
		t.Fatal(err)
	}
	if err := srv.Manager().PeerRun.SetDebugMode(ctx, owner.Public, "off"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Manager().PeerRun.PutStatus(ctx, owner.Public, apitypes.PeerStatus{BatteryPercent: new(50)}); err != nil {
		t.Fatal(err)
	}

	device := &gizcli.Client{KeyPair: owner, DialTransport: func(key *giznet.KeyPair, server giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, server, gizwebrtc.DialConfig{SignalingURL: serverURL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	if err := device.HandleDeviceControl(gizcli.DeviceControlHandlers{
		ReadMhsHwd: func(_ context.Context, request *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
			var value proto.Message
			switch request.Id {
			case "wifi.main":
				value = &rpcpb.WifiHwdReadResponse{Connected: new(true), Ssid: new("Signed Wi-Fi")}
			case "modem.main":
				value = &rpcpb.ModemHwdReadResponse{Enabled: new(false), Powered: new(false)}
			case "nfc.left":
				value = &rpcpb.NfcHwdReadResponse{Enabled: new(false), CardPresent: new(false)}
			default:
				return nil, gizcli.ErrDeviceResourceNotFound
			}
			data, err := proto.Marshal(value)
			return &rpcpb.ClientMhsV0ReadResponse{Payload: data}, err
		},
		SavedWifi: func(context.Context) ([]rpcapi.WifiSavedNetwork, error) {
			return []rpcapi.WifiSavedNetwork{{Ssid: "Signed Wi-Fi"}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_GET, func(context.Context, proto.Message) (proto.Message, error) {
		return &rpcpb.ClientUsageTimeGetResponse{State: &rpcpb.UsageTimeState{LimitSeconds: new(uint32(0)), UsedSeconds: new(uint32(0)), Exhausted: new(false)}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_MANAGEMENT_PASSWORD_GET, func(context.Context, proto.Message) (proto.Message, error) {
		return &rpcpb.ClientManagementPasswordGetResponse{IsSet: new(false)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := device.Dial(srv.PublicKey(), serverURL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = device.Serve() }()
	t.Cleanup(func() { _ = device.Close() })

	// The fixture's script workflow needs no external model and is never executed.
	flow := adminhttp.WorkflowUpsert{Id: "monitor-signed-flow", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &apitypes.EinoWorkflowSpec{}}}
	const graph = "{\"graph\":{\"name\":\"monitor-signed\",\"compile\":{\"node_trigger_mode\":\"any_predecessor\"},\"state\":{\"fields\":[{\"name\":\"answer\",\"type\":\"string\",\"merge\":\"replace\"}]},\"nodes\":[{\"id\":\"answer\",\"type\":\"script\",\"language\":\"starlark\",\"entrypoint\":\"run\",\"source\":\"def run(input):\\n    return {\\\"answer\\\": \\\"Unused.\\\"}\\n\",\"outputs\":{\"answer\":\"answer\"},\"limits\":{\"max_execution_steps\":1000,\"timeout\":\"1s\",\"max_input_bytes\":1024,\"max_output_bytes\":1024}}],\"edges\":[{\"from\":\"start\",\"to\":\"answer\"},{\"from\":\"answer\",\"to\":\"end\"}],\"branches\":[],\"outputs\":[{\"node\":\"answer\",\"field\":\"answer\",\"name\":\"answer\",\"mime_type\":\"text/plain\",\"primary\":true}]}}"
	if err := json.Unmarshal([]byte(graph), flow.Spec.Eino); err != nil {
		t.Fatal(err)
	}
	if response, err := srv.Manager().Workflows.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &flow}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.CreateWorkflow200JSONResponse); !ok {
		t.Fatalf("create workflow: %T", response)
	}
	workspaces, ok := srv.Manager().Workspaces.(*workspace.Server)
	if !ok {
		t.Fatal("fixture requires the production Workspace service")
	}
	item, err := workspaces.CreatePeerWorkspace(ownership.WithOwner(ctx, owner.Public.String()), workspace.PeerWorkspaceCreateRequest{Name: "monitor-signed", WorkflowID: flow.Id})
	if err != nil {
		t.Fatal(err)
	}
	audio, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "audio", "sfu-tone.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := workspaces.AppendWorkspaceHistoryByID(ctx, item.Id, workspace.AppendHistoryRequest{Type: "agent", Name: "signed-fixture", Text: "Retained signed history", Asset: &workspace.AppendHistoryAsset{MIMEType: "audio/ogg", Data: audio}})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Millisecond)
	if err := srv.MetricsStore.Append(ctx, []metrics.Sample{{Name: peertelemetry.MetricBatteryPercent, Labels: map[string]string{"peer_id": owner.Public.String()}, Timestamp: at, Value: 50}}); err != nil {
		t.Fatal(err)
	}
	ap := startPeerAccessEdge(t, ctx, edge, []gizedge.UpstreamConfig{{Endpoint: serverURL, PublicKey: srv.PublicKey()}})
	mint := func(key *giznet.KeyPair, target giznet.PublicKey, lifetime time.Duration) string {
		t.Helper()
		token, err := peerhttp.NewSignedBearerToken(key, target, lifetime)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	signed, renewed, foreign := mint(admin, owner.Public, 2*time.Minute), mint(admin, owner.Public, 3*time.Minute), mint(owner, admin.Public, time.Minute)
	for name, value := range map[string]string{
		"GIZCLAW_TEST_ENDPOINT": ap, "GIZCLAW_MONITOR_SIGNED_BEARER": signed, "GIZCLAW_MONITOR_RENEWED_BEARER": renewed, "GIZCLAW_MONITOR_FOREIGN_BEARER": foreign,
		"GIZCLAW_MONITOR_SIGNED_KEY": owner.Public.String(), "GIZCLAW_MONITOR_SIGNED_WORKSPACE": item.Id, "GIZCLAW_MONITOR_SIGNED_HISTORY": entry.ID,
		"GIZCLAW_MONITOR_START_MS": fmt.Sprint(at.Add(-time.Minute).UnixMilli()), "GIZCLAW_MONITOR_END_MS": fmt.Sprint(at.Add(time.Minute).UnixMilli()),
	} {
		t.Setenv(name, value)
	}
	file, err := filepath.Abs(filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "giztest", "server.device.monitor.signed.giztest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(t.TempDir(), "monitor-signed.report.json")
	output, runErr := run(ctx, file, reportPath)
	data, readErr := os.ReadFile(reportPath)
	if bytes.Contains(data, []byte(signed)) || bytes.Contains(data, []byte(renewed)) || bytes.Contains(data, []byte(foreign)) {
		t.Fatal("signed credential leaked into Giztest report")
	}
	if runErr != nil || readErr != nil {
		t.Fatalf("Monitor signed Giztest: %v, report: %v\n%s\n%s", runErr, readErr, output, data)
	}
	type step struct {
		Status string `json:"status"`
	}
	var report struct {
		Status string `json:"status"`
		Tasks  []struct {
			Steps   []step `json:"steps"`
			Cleanup []step `json:"cleanup"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 37 || len(report.Tasks[0].Cleanup) != 1 {
		t.Fatalf("all Monitor reads must execute: %s", data)
	}
	for _, result := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
		if result.Status != "passed" {
			t.Fatalf("Monitor step did not pass: %s", data)
		}
	}
	if mode, err := srv.Manager().PeerRun.GetDebugMode(ctx, owner.Public); err != nil || mode != "off" {
		t.Fatalf("Monitor changed debug mode: %q, %v", mode, err)
	}
	t.Log("Monitor signed Giztest: 37 steps and 1 cleanup passed with debug off")
}

type monitorSignedLogBackend struct{ owner string }

func (b monitorSignedLogBackend) Query(_ context.Context, query logstore.Query) (logstore.Page, error) {
	for _, matcher := range query.Matchers {
		if matcher.Name == "peer_public_key" && matcher.Value == b.owner {
			return logstore.Page{}, nil
		}
	}
	return logstore.Page{}, fmt.Errorf("Monitor logs lost the target owner filter")
}
