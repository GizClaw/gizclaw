package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/gizedge"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/proto"
)

// Ordinary AP HTTP reuses existing route resolution and resource handlers.
// The fixture has two authoritative Servers and no client WebRTC gateway.
func TestSignedBearerThroughAP(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	admin, edge, roleAdmin := testKeyPair(t, 101), testKeyPair(t, 102), testKeyPair(t, 103)
	client, target, missing := testKeyPair(t, 104), testKeyPair(t, 105), testKeyPair(t, 106)
	peers, keys := kv.NewMemory(nil), kv.NewMemory(nil)
	a, aURL, _ := startPeerAccessServer(t, peers, keys, edge.Public, admin.Public)
	b, bURL, stopB := startPeerAccessServer(t, peers, keys, edge.Public, admin.Public)
	for _, record := range []apitypes.Peer{
		{PublicKey: admin.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive},
		{PublicKey: roleAdmin.Public.String(), Role: apitypes.PeerRoleAdmin, Status: apitypes.PeerRegistrationStatusActive},
		{PublicKey: client.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive, Device: apitypes.DeviceInfo{Name: new("peer-a"), Hardware: &apitypes.HardwareInfo{Model: new("H106"), HardwareRevision: new("r1")}, Identifiers: &apitypes.DeviceIdentifiers{Sn: new("signed-device"), Imeis: &[]apitypes.PeerIMEI{{Tac: "49015420", Serial: "3237518"}}}}},
		{PublicKey: target.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive, Device: apitypes.DeviceInfo{Name: new("peer-b"), Hardware: &apitypes.HardwareInfo{Model: new("H106"), HardwareRevision: new("r1")}, Identifiers: &apitypes.DeviceIdentifiers{Sn: new("signed-device"), Imeis: &[]apitypes.PeerIMEI{{Tac: "49015420", Serial: "3237518"}}}}},
		{PublicKey: missing.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive},
	} {
		if _, err := a.Manager().Peers.SavePeer(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Manager().PeerRoutes.Assign(ctx, client.Public, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Manager().PeerRoutes.Assign(ctx, target.Public, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Manager().RuntimeProfiles.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{Id: "signed-http", Spec: apitypes.RuntimeProfileSpec{}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Manager().RuntimeProfiles.BindOwnerProfile(ctx, client.Public.String(), "signed-http"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	if err := b.Manager().PeerRun.RecordLastSeen(ctx, target.Public, at); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Manager().PeerRun.PutStatus(ctx, target.Public, apitypes.PeerStatus{FirmwareVersion: new("0.33.fixture"), TelemetryObservedAt: &apitypes.PeerStatusTelemetryObservedAt{FirmwareVersion: &at}}); err != nil {
		t.Fatal(err)
	}
	for _, server := range []*CmdServer{a, b} {
		value := `{"nested":[null,false,0]}`
		if server == a {
			value = `"wrong-server"`
		}
		svc := &peertelemetry.Service{Status: peertelemetry.StatusSync{Store: server.Manager().PeerRun}}
		if err := svc.Report(ctx, target.Public, &telemetrypb.TelemetryFrame{ObservedAtUnixMs: at.UnixMilli(), Observations: []*telemetrypb.Observation{{Body: &telemetrypb.Observation_Kv{Kv: &telemetrypb.KVObservation{Key: "diagnostic", JsonValue: &value}}}, {Body: &telemetrypb.Observation_Kv{Kv: &telemetrypb.KVObservation{Key: "online", JsonValue: new("true")}}}}}); err != nil {
			t.Fatal(err)
		}
	}
	// A poisoned copy on the other Server must never be returned through AP.
	if _, err := a.Manager().PeerRun.PutStatus(ctx, target.Public, apitypes.PeerStatus{FirmwareVersion: new("wrong-server")}); err != nil {
		t.Fatal(err)
	}
	fields := []struct {
		field, metric string
		value         float64
	}{
		{"battery.percent", peertelemetry.MetricBatteryPercent, 72},
		{"battery.charging", peertelemetry.MetricBatteryCharging, 1},
		{"network.connected", peertelemetry.MetricNetworkConnected, 1},
		{"network.rssi_dbm", peertelemetry.MetricNetworkRSSIDbm, -61},
		{"network.signal_level", peertelemetry.MetricNetworkSignal, 4},
		{"system.uptime_seconds", peertelemetry.MetricSystemUptime, 3600},
	}
	for _, field := range fields {
		if err := b.MetricsStore.Append(ctx, []metrics.Sample{
			{Name: field.metric, Labels: map[string]string{"peer_id": target.Public.String()}, Timestamp: at.Add(-time.Second), Value: field.value - 1},
			{Name: field.metric, Labels: map[string]string{"peer_id": target.Public.String()}, Timestamp: at, Value: field.value},
		}); err != nil {
			t.Fatal(err)
		}
	}
	ap := startPeerAccessEdge(t, ctx, edge, []gizedge.UpstreamConfig{{Endpoint: aURL, PublicKey: a.PublicKey()}, {Endpoint: bURL, PublicKey: b.PublicKey()}})
	token := func(key *giznet.KeyPair, owner giznet.PublicKey) string {
		t.Helper()
		value, err := peerhttp.NewSignedBearerToken(key, owner, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	adminToken, roleToken, selfToken := token(admin, target.Public), token(roleAdmin, target.Public), token(client, client.Public)
	request := func(bearer, method, path, body string, want int) string {
		t.Helper()
		status, data := peerAccessRequest(t, ctx, ap, bearer, method, path, body)
		if status != want {
			t.Fatalf("%s %s: HTTP %d, want %d: %s", method, path, status, want, data)
		}
		return string(data)
	}
	if !strings.Contains(request(adminToken, "GET", "/device", "", 200), "peer-b") {
		t.Fatal("Admin read the wrong target")
	}
	request(roleToken, "GET", "/device", "", 200)
	if !strings.Contains(request(selfToken, "GET", "/device", "", 200), "peer-a") {
		t.Fatal("Client read the wrong owner")
	}
	info := decodePeerAccess[apitypes.DeviceInfo](t, request(adminToken, "GET", "/device", "", 200))
	if info.Hardware == nil || info.Hardware.Model == nil || *info.Hardware.Model != "H106" || info.Hardware.HardwareRevision == nil || *info.Hardware.HardwareRevision != "r1" || info.Identifiers == nil || info.Identifiers.Sn == nil || *info.Identifiers.Sn != "signed-device" || info.Identifiers.Imeis == nil || len(*info.Identifiers.Imeis) != 1 || (*info.Identifiers.Imeis)[0].Tac != "49015420" || (*info.Identifiers.Imeis)[0].Serial != "3237518" {
		t.Fatal("device identity metadata lost")
	}
	for _, path := range []string{"/peers/@findBySn/signed-device", "/peers/@findByImei/49015420/3237518"} {
		found := decodePeerAccess[map[string][]string](t, request("", "GET", path, "", 200))["public_keys"]
		if len(found) != 2 || !slices.Contains(found, client.Public.String()) || !slices.Contains(found, target.Public.String()) {
			t.Fatal("directory discarded a matching identity")
		}
	}
	runtime := decodePeerAccess[apitypes.Runtime](t, request(adminToken, "GET", "/device/runtime", "", 200))
	if runtime.Online || !runtime.LastSeenAt.Equal(at) || runtime.DebugMode == nil || *runtime.DebugMode != "off" {
		t.Fatal("offline evidence requires debug or lost last activity")
	}
	status := decodePeerAccess[apitypes.PeerStatus](t, request(adminToken, "GET", "/device/status", "", 200))
	if status.FirmwareVersion == nil || *status.FirmwareVersion != "0.33.fixture" || status.TelemetryObservedAt == nil || status.TelemetryObservedAt.FirmwareVersion == nil || !status.TelemetryObservedAt.FirmwareVersion.Equal(at) {
		t.Fatal("firmware observation came from the wrong Server or lost its time")
	}
	if runtime.ReportedKv == nil || status.ReportedKv == nil {
		t.Fatal("offline runtime/status omitted KV")
	}
	runtimeKV, _ := json.Marshal(runtime.ReportedKv)
	statusKV, _ := json.Marshal(status.ReportedKv)
	if string(runtimeKV) != string(statusKV) || !strings.Contains(string(statusKV), `"value":{"nested":[null,false,0]}`) || !(*status.ReportedKv)["diagnostic"].ObservedAt.Equal(at) {
		t.Fatal("signed API lost KV type, owner or observation time")
	}
	request(token(client, target.Public), "GET", "/device/status", "", 403)
	request(token(client, target.Public), "GET", "/device/runtime", "", 403)
	for _, field := range fields {
		latest := decodePeerAccess[apitypes.PeerTelemetryLatestResponse](t, request(adminToken, "GET", "/device/telemetry/"+field.field+"/latest", "", 200))
		if len(latest.Values) != 1 || latest.Values[0].Value != field.value || latest.Values[0].ObservedAtUnixMs != at.UnixMilli() {
			t.Fatalf("latest %s lost its value or observation time", field.field)
		}
		path := fmt.Sprintf("/device/telemetry?field=%s&start_time_ms=%d&end_time_ms=%d&limit=1", field.field, at.Add(-time.Second).UnixMilli(), at.Add(time.Second).UnixMilli())
		points := decodePeerAccess[apitypes.PeerTelemetryRangeResponse](t, request(adminToken, "GET", path, "", 200))
		if len(points.Points) != 1 {
			t.Fatalf("range %s did not honor its point limit", field.field)
		}
	}
	emptyStatus := request(selfToken, "GET", "/device/status", "", 200)
	emptyLatest := decodePeerAccess[apitypes.PeerTelemetryLatestResponse](t, request(selfToken, "GET", "/device/telemetry/battery.percent/latest", "", 200))
	rangePath := fmt.Sprintf("/device/telemetry?field=battery.percent&start_time_ms=%d&end_time_ms=%d&limit=1", at.Add(-time.Second).UnixMilli(), at.Add(time.Second).UnixMilli())
	emptyRange := decodePeerAccess[apitypes.PeerTelemetryRangeResponse](t, request(selfToken, "GET", rangePath, "", 200))
	if strings.TrimSpace(emptyStatus) != "{}" || emptyLatest.Values == nil || len(emptyLatest.Values) != 0 || emptyRange.Points == nil || len(emptyRange.Points) != 0 {
		t.Fatal("missing data fabricated a reading")
	}
	request(adminToken, "GET", strings.Replace(rangePath, "limit=1", "limit=1001", 1), "", 400)
	after := decodePeerAccess[apitypes.Runtime](t, request(adminToken, "GET", "/device/runtime", "", 200))
	if !after.LastSeenAt.Equal(at) || after.DebugMode == nil || *after.DebugMode != "off" {
		t.Fatal("reads refreshed activity or changed debug mode")
	}
	device := &gizcli.Client{KeyPair: client, Device: info, DialTransport: func(key *giznet.KeyPair, serverKey giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, serverKey, gizwebrtc.DialConfig{SignalingURL: aURL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	limit := uint32(0)
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_SET, func(_ context.Context, input proto.Message) (proto.Message, error) {
		limit = min(input.(*rpcpb.ClientUsageTimeSetRequest).GetLimitSeconds(), 3600)
		return &rpcpb.ClientUsageTimeSetResponse{Applied: &rpcpb.UsageTimeState{LimitSeconds: &limit, UsedSeconds: new(uint32(0)), Exhausted: new(false)}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := device.Dial(a.PublicKey(), aURL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = device.Serve() }()
	t.Cleanup(func() { _ = device.Close() })
	if online := decodePeerAccess[apitypes.Runtime](t, request(token(admin, client.Public), "GET", "/device/runtime", "", 200)); !online.Online {
		t.Fatal("connected device reported offline")
	}
	applied := request(token(admin, client.Public), "POST", "/device/tool/v0/invoke", `{"tool":"usage.time.set","args":{"limit_seconds":9000}}`, 200)
	if limit != 3600 || !strings.Contains(applied, `"limit_seconds":3600`) {
		t.Fatal("management invoke lost actual clamped result")
	}
	request(token(admin, client.Public), "POST", "/device/tool/v0/invoke", `{"tool":"ai.safety.get","args":{}}`, 501)
	request(adminToken, "POST", "/contacts", `{"name":"signed-contact","display_name":"Signed"}`, 201)
	if !strings.Contains(request(adminToken, "GET", "/contacts", "", 200), "signed-contact") {
		t.Fatal("existing read/write handlers disagree on owner")
	}
	request(adminToken, "DELETE", "/contacts/signed-contact", "", 204)
	request(adminToken, "POST", "/device/tool/v0/invoke", `{"tool":"device.find","args":{}}`, 409)
	request(token(client, target.Public), "GET", "/contacts", "", 403)
	request(token(client, target.Public), "POST", "/contacts", `{"name":"forbidden","display_name":"Forbidden"}`, 403)
	request(adminToken, "GET", "/api-keys", "", 403)
	request(token(admin, missing.Public), "GET", "/device", "", 403)
	request("gizclaw_sig_v1_invalid.invalid", "GET", "/device", "", 401)
	request("", "GET", "/device", "", 401)
	legacy, err := apikey.NewServer(keys).Create(ctx, client.Public.String(), "legacy", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request(legacy.Secret, "GET", "/device?public_key="+target.Public.String(), "", 200), "peer-a") {
		t.Fatal("API key escaped its original owner")
	}
	record, err := a.Manager().Peers.LoadPeer(ctx, roleAdmin.Public)
	if err != nil {
		t.Fatal(err)
	}
	record.Role = apitypes.PeerRoleClient
	if _, err := a.Manager().Peers.SavePeer(ctx, record); err != nil {
		t.Fatal(err)
	}
	request(roleToken, "GET", "/device", "", 403) // same token, revoked live role
	record.Role = apitypes.PeerRoleAdmin
	if _, err := a.Manager().Peers.SavePeer(ctx, record); err != nil {
		t.Fatal(err)
	}
	record, err = a.Manager().Peers.LoadPeer(ctx, admin.Public)
	if err != nil {
		t.Fatal(err)
	}
	if record.Role != apitypes.PeerRoleClient {
		t.Fatal("host grant rewrote the stored role")
	}
	record.Status = apitypes.PeerRegistrationStatusBlocked
	if _, err := a.Manager().Peers.SavePeer(ctx, record); err != nil {
		t.Fatal(err)
	}
	request(adminToken, "GET", "/device", "", 403) // configured Admin grant cannot bypass blocking
	stopB()
	request(roleToken, "GET", "/device", "", 503)

}

func startPeerAccessServer(t *testing.T, peers, keys kv.Store, edge, admin giznet.PublicKey) (*CmdServer, string, func()) {
	t.Helper()
	cfg := validLayeredConfig(t.TempDir())
	cfg.EdgeNodes = []giznet.PublicKey{edge}
	cfg.AdminPublicKey = admin
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv.PeerStore, srv.APIKeyStore, srv.MetricsStore = peers, keys, metrics.NewMemoryStore()
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
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	stop := sync.OnceFunc(func() {
		httpServer.Close()
		srv.Manager().DetachPeerConnections(edge)()
		if err := srv.Close(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Server did not stop")
		}
	})
	t.Cleanup(stop)
	return srv, httpServer.URL, stop
}

func startPeerAccessEdge(t *testing.T, ctx context.Context, key *giznet.KeyPair, upstreams []gizedge.UpstreamConfig) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	_ = probe.Close()
	var config strings.Builder
	fmt.Fprintf(&config, "identity:\n  private-key: %s\nwebrtc:\n  listen: %s\n  endpoint: %s\nhttp:\n  listeners:\n    - listen: %s\nupstreams:\n", key.Private.String(), address, address, address)
	for _, entry := range upstreams {
		fmt.Fprintf(&config, "  - endpoint: %s\n    public-key: %s\n", entry.Endpoint, entry.PublicKey.String())
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(config.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	edgeCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- gizedge.ServeContext(edgeCtx, root) }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Edge did not stop")
		}
	})
	endpoint := "http://" + address
	ready, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ready, "GET", endpoint+"/server-info", nil)
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == 200 {
				return endpoint
			}
		}
		select {
		case <-ready.Done():
			t.Fatalf("Edge not ready: %v", ready.Err())
		case <-ticker.C:
		}
	}
}

func peerAccessRequest(t *testing.T, ctx context.Context, endpoint, token, method, path, body string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, endpoint+"/gizclaw/v1"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func decodePeerAccess[T any](t *testing.T, body string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
