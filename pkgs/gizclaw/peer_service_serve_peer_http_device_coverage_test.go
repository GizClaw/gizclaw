package gizclaw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

// deviceExtensionRoutes lists every extension route with a body that passes
// request validation, so authentication and ingress tests cover all of them.
var deviceExtensionRoutes = []struct{ method, path, body string }{
	{http.MethodGet, "/gizclaw/v1/device", ""},
	{http.MethodGet, "/gizclaw/v1/device/runtime", ""},
	{http.MethodGet, "/gizclaw/v1/device/status", ""},
	{http.MethodGet, "/gizclaw/v1/device/telemetry/latest", ""},
	{http.MethodGet, "/gizclaw/v1/device/telemetry?field=battery.percent&start_time_ms=1&end_time_ms=2", ""},
	{http.MethodGet, "/gizclaw/v1/device/telemetry/aggregate?field=battery.percent&start_time_ms=1&end_time_ms=2&bucket_ms=1&aggregate=avg", ""},
	{http.MethodPut, "/gizclaw/v1/device/volume", `{"level":1,"muted":false}`},
	{http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"chime"}`},
	{http.MethodPost, "/gizclaw/v1/device/actions/reboot", ""},
	{http.MethodGet, "/gizclaw/v1/device/wifi", ""},
	{http.MethodGet, "/gizclaw/v1/device/wifi/saved", ""},
	{http.MethodDelete, "/gizclaw/v1/device/wifi/saved/home", ""},
	{http.MethodGet, "/gizclaw/v1/contacts", ""},
	{http.MethodPost, "/gizclaw/v1/contacts", `{"name":"x","display_name":"X"}`},
	{http.MethodGet, "/gizclaw/v1/contacts/x", ""},
	{http.MethodPut, "/gizclaw/v1/contacts/x", `{"display_name":"Y"}`},
	{http.MethodDelete, "/gizclaw/v1/contacts/x", ""},
}

func (f *deviceHTTPFixture) doWithSecret(t *testing.T, secret, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func TestDeviceExtensionRoutesRequireAPIKey(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	for _, route := range deviceExtensionRoutes {
		for _, secret := range []string{"", "gizclaw_sk_v1_" + strings.Repeat("A", 43), "not-a-key"} {
			response := f.doWithSecret(t, secret, route.method, route.path, route.body)
			if response.Code != http.StatusUnauthorized || errorCode(t, response) != "INVALID_API_KEY" {
				t.Fatalf("%s %s secret=%q status = %d body=%s", route.method, route.path, secret, response.Code, response.Body.String())
			}
		}
	}
}

func TestDeviceExtensionFailsAfterKeyRevokeAndOwnerRetirement(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	ctx := context.Background()
	keys := f.apiKeys
	manager, err := keys.Create(ctx, f.owner.String(), "manager", true)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := keys.Create(ctx, f.owner.String(), "phone-2", false)
	if err != nil {
		t.Fatal(err)
	}
	// manage_api_keys grants nothing extra on the device surface.
	for _, secret := range []string{manager.Secret, ordinary.Secret} {
		if response := f.doWithSecret(t, secret, http.MethodGet, "/gizclaw/v1/device", ""); response.Code != http.StatusOK {
			t.Fatalf("GET device status = %d body=%s", response.Code, response.Body.String())
		}
	}
	principal, err := keys.Authenticate(ctx, manager.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := keys.Revoke(ctx, principal, ordinary.Key.Name); err != nil {
		t.Fatal(err)
	}
	for _, route := range deviceExtensionRoutes {
		if response := f.doWithSecret(t, ordinary.Secret, route.method, route.path, route.body); response.Code != http.StatusUnauthorized {
			t.Fatalf("revoked key %s %s status = %d", route.method, route.path, response.Code)
		}
	}

	// A blocked owner is no longer an active Client: every route answers 403.
	if _, err := f.peers.SavePeer(ctx, apitypes.Peer{
		PublicKey: f.owner.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusBlocked,
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range deviceExtensionRoutes {
		response := f.doWithSecret(t, manager.Secret, route.method, route.path, route.body)
		if response.Code != http.StatusForbidden || errorCode(t, response) != "API_KEY_OWNER_UNAVAILABLE" {
			t.Fatalf("blocked owner %s %s status = %d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
	}
}

func TestDeviceControlUTF8Bounds(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var lastSound, lastSSID string
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientDeviceSoundPlay:
			params, err := req.Params.AsClientDeviceSoundPlayRequest()
			if err != nil {
				return nil, err
			}
			lastSound = params.Sound
			return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSoundPlayResponse{}, (*rpcapi.RPCPayload).FromClientDeviceSoundPlayResponse)
		case rpcapi.RPCMethodClientWifiSavedForget:
			params, err := req.Params.AsClientWifiSavedForgetRequest()
			if err != nil {
				return nil, err
			}
			lastSSID = params.Ssid
			return newRPCResultResponse(req.Id, rpcapi.ClientWifiSavedForgetResponse{}, (*rpcapi.RPCPayload).FromClientWifiSavedForgetResponse)
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound}.RPCResponse(), nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)

	exact := strings.Repeat("a", 29) + "中" // 29 + 3 = 32 bytes
	over := strings.Repeat("a", 30) + "中"  // 33 bytes, 31 runes
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"`+exact+`"}`); response.Code != http.StatusNoContent {
		t.Fatalf("32-byte multibyte sound status = %d body=%s", response.Code, response.Body.String())
	}
	if lastSound != exact {
		t.Fatalf("device received sound %q", lastSound)
	}
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"`+over+`"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("33-byte multibyte sound status = %d", response.Code)
	}
	// JSON decoding already normalizes invalid bytes in a body; the raw path
	// parameter is the only way invalid UTF-8 reaches the handler.
	if response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/wifi/saved/%FF%FE", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid UTF-8 ssid status = %d body=%s", response.Code, response.Body.String())
	}

	if response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/wifi/saved/"+exact, ""); response.Code != http.StatusNoContent {
		t.Fatalf("32-byte multibyte ssid status = %d body=%s", response.Code, response.Body.String())
	}
	if lastSSID != exact {
		t.Fatalf("device received ssid %q", lastSSID)
	}
	if response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/wifi/saved/"+over, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("33-byte multibyte ssid status = %d", response.Code)
	}
	// Percent-encoded spaces round-trip and surrounding whitespace is trimmed.
	if response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/wifi/saved/%20home%20net%20", ""); response.Code != http.StatusNoContent {
		t.Fatalf("encoded ssid status = %d body=%s", response.Code, response.Body.String())
	}
	if lastSSID != "home net" {
		t.Fatalf("device received trimmed ssid %q", lastSSID)
	}
	if device.calls.Load() != 3 {
		t.Fatalf("device calls = %d; rejected inputs must not reach the device", device.calls.Load())
	}
}

func TestDeviceControlWriteBackRacesWithTelemetry(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	base := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	var clock atomicTime
	clock.set(base)
	f.control.now = clock.now
	var volume atomicInt
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		params, err := req.Params.AsClientDeviceVolumeSetRequest()
		if err != nil {
			return nil, err
		}
		level := int(params.Level)
		volume.set(level)
		return deviceStatusResult(req.Id, rpcapi.PeerStatus{Volume: &level, Muted: &params.Muted})
	})
	f.manager.SetPeerUp(f.owner, device)
	telemetry := peerConnTelemetryStatusSync{
		mu:   f.manager.telemetryStatusLock(f.owner),
		next: peertelemetry.StatusSync{Store: f.manager.PeerRun},
	}

	const rounds = 40
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range rounds {
			level := 10 + i
			body := `{"level":` + strings.TrimSpace(itoa(int64(level))) + `,"muted":false}`
			if response := f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", body); response.Code != http.StatusOK {
				t.Errorf("PUT volume %d status = %d body=%s", level, response.Code, response.Body.String())
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := range rounds {
			at := base.Add(time.Duration(i+1) * time.Second)
			percent := 50 + i
			if err := telemetry.SyncTelemetryStatus(context.Background(), f.owner, peertelemetry.StatusPatch{
				ReportedAt: at, BatteryPercent: &percent, BatteryPercentAt: at,
			}); err != nil {
				t.Errorf("telemetry sync %d: %v", i, err)
				return
			}
		}
	}()
	wg.Wait()

	stored, err := f.manager.PeerRun.GetStatus(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	// Control owns volume, telemetry owns battery; neither write may clobber the other.
	if stored.Volume == nil || *stored.Volume != 10+rounds-1 || stored.BatteryPercent == nil || *stored.BatteryPercent != 50+rounds-1 {
		t.Fatalf("stored status after concurrent writes = %+v", stored)
	}
	if stored.ReportedAt == nil || stored.ReportedAt.Before(base.Add(rounds*time.Second)) {
		t.Fatalf("reported_at = %v, want newest telemetry time", stored.ReportedAt)
	}
}

func TestDeviceControlStatusWriteBackUsesServerClockWithoutDeviceTime(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	fixed := time.Date(2026, 9, 3, 9, 15, 0, 0, time.UTC)
	f.control.now = func() time.Time { return fixed }
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return deviceStatusResult(req.Id, rpcapi.PeerStatus{Volume: new(7)})
	})
	f.manager.SetPeerUp(f.owner, device)
	response := f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", `{"level":7,"muted":false}`)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT volume status = %d body=%s", response.Code, response.Body.String())
	}
	result := decodeJSON[peerhttp.DeviceControlStatus](t, response)
	if result.Status.ReportedAt == nil || !result.Status.ReportedAt.Equal(fixed) {
		t.Fatalf("reported_at = %v, want server clock %v", result.Status.ReportedAt, fixed)
	}
	if result.Status.Muted != nil {
		t.Fatalf("muted = %v, want absent when the device omits it", *result.Status.Muted)
	}
}

func TestDeviceControlHalfDeadDeviceTimesOut(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	f.control.timeout = 150 * time.Millisecond
	release := make(chan struct{})
	defer close(release)
	// The device accepts the stream but never answers and ignores ctx.
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		<-release
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError}.RPCResponse(), nil
	})
	f.manager.SetPeerUp(f.owner, device)
	started := time.Now()
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", "")
	if response.Code != http.StatusGatewayTimeout || errorCode(t, response) != deviceTimeoutCode {
		t.Fatalf("half-dead device status = %d body=%s", response.Code, response.Body.String())
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
	// The owner lock is released after the timeout: the next command is not stuck.
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", ""); response.Code != http.StatusGatewayTimeout {
		t.Fatalf("second command status = %d", response.Code)
	}
}

func TestDeviceControlEmptySavedListAndRebootBody(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientWifiSavedList:
			return newRPCResultResponse(req.Id, rpcapi.ClientWifiSavedListResponse{}, (*rpcapi.RPCPayload).FromClientWifiSavedListResponse)
		case rpcapi.RPCMethodClientWifiStatusGet:
			return newRPCResultResponse(req.Id, rpcapi.ClientWifiStatusGetResponse{}, (*rpcapi.RPCPayload).FromClientWifiStatusGetResponse)
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound}.RPCResponse(), nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi/saved", "")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"networks":[]}` {
		t.Fatalf("empty saved list = %d %s", response.Code, response.Body.String())
	}
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/wifi", "")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"connected":false}` {
		t.Fatalf("disconnected wifi status = %d %s", response.Code, response.Body.String())
	}
	// A malformed reboot body is rejected before any RPC.
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", `{"delay_ms":"soon"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("malformed reboot body status = %d", response.Code)
	}
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", `{"delay_ms":-1}`); response.Code != http.StatusBadRequest {
		t.Fatalf("negative delay status = %d", response.Code)
	}
	if device.calls.Load() != 2 {
		t.Fatalf("device calls = %d", device.calls.Load())
	}
}

func TestDeviceHTTPContactValidationAndPagination(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	for _, body := range []string{`{"name":" mom ","display_name":"Mom"}`, `{"name":"","display_name":"Mom"}`, `{"display_name":"Mom"}`, `not json`} {
		if response := f.do(t, http.MethodPost, "/gizclaw/v1/contacts", body); response.Code != http.StatusBadRequest {
			t.Fatalf("create %q status = %d body=%s", body, response.Code, response.Body.String())
		}
	}
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/contacts", `{"name":"mom","display_name":"Mom"}`); response.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", response.Code, response.Body.String())
	}
	// Clearing both fields is rejected; the contact keeps its display name.
	if response := f.do(t, http.MethodPut, "/gizclaw/v1/contacts/mom", `{"display_name":"","phone_number":""}`); response.Code != http.StatusBadRequest {
		t.Fatalf("empty put status = %d body=%s", response.Code, response.Body.String())
	}
	response := f.do(t, http.MethodGet, "/gizclaw/v1/contacts/mom", "")
	if got := decodeJSON[peerhttp.Contact](t, response); got.DisplayName == nil || *got.DisplayName != "Mom" {
		t.Fatalf("contact after rejected put = %+v", got)
	}
	for _, query := range []string{"limit=201", "limit=-1", "cursor=%20"} {
		if response := f.do(t, http.MethodGet, "/gizclaw/v1/contacts?"+query, ""); response.Code != http.StatusBadRequest {
			t.Fatalf("list ?%s status = %d body=%s", query, response.Code, response.Body.String())
		}
	}
	// An unknown cursor yields an empty page rather than an error.
	response = f.do(t, http.MethodGet, "/gizclaw/v1/contacts?cursor=zzz", "")
	if page := decodeJSON[peerhttp.ContactList](t, response); response.Code != http.StatusOK || len(page.Items) != 0 || page.HasNext {
		t.Fatalf("unknown cursor page = %d %+v", response.Code, page)
	}
	// Names are matched exactly and percent-decoded.
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/contacts/MOM", ""); response.Code != http.StatusNotFound {
		t.Fatalf("case-different name status = %d", response.Code)
	}
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/contacts", `{"name":"grand ma","phone_number":"+1"}`); response.Code != http.StatusCreated {
		t.Fatalf("create spaced name status = %d", response.Code)
	}
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/contacts/grand%20ma", ""); response.Code != http.StatusOK {
		t.Fatalf("encoded name status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceHTTPTelemetryIgnoresPeerSelectors(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	other := giznet.PublicKey{9}
	at := time.Date(2026, 9, 3, 7, 0, 0, 0, time.UTC)
	if err := f.metrics.Append(context.Background(), []metrics.Sample{{
		Name: peertelemetry.MetricBatteryPercent, Labels: map[string]string{"peer_id": other.String()}, Timestamp: at, Value: 5,
	}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/gizclaw/v1/device/telemetry/latest?public_key=" + other.String(),
		"/gizclaw/v1/device/telemetry?field=battery.percent&start_time_ms=" + itoa(at.Add(-time.Hour).UnixMilli()) + "&end_time_ms=" + itoa(at.Add(time.Hour).UnixMilli()) + "&peer=" + other.String(),
		"/gizclaw/v1/device/telemetry/aggregate?field=battery.percent&start_time_ms=" + itoa(at.Add(-time.Hour).UnixMilli()) + "&end_time_ms=" + itoa(at.Add(time.Hour).UnixMilli()) + "&bucket_ms=3600000&aggregate=max&peer_public_key=" + other.String(),
	} {
		response := f.do(t, http.MethodGet, path, "")
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d body=%s", path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if !strings.Contains(body, f.owner.String()) || strings.Contains(body, other.String()) || strings.Contains(body, `"value":5`) {
			t.Fatalf("GET %s leaked another Peer: %s", path, body)
		}
	}
}

func TestDeviceControlPreflightAllowsPut(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	request := httptest.NewRequest(http.MethodOptions, "/gizclaw/v1/device/volume", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPut)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatalf("preflight = %d methods=%q", response.Code, response.Header().Get("Access-Control-Allow-Methods"))
	}
}

type atomicTime struct {
	mu sync.Mutex
	t  time.Time
}

func (a *atomicTime) set(t time.Time) { a.mu.Lock(); a.t = t; a.mu.Unlock() }
func (a *atomicTime) now() time.Time  { a.mu.Lock(); defer a.mu.Unlock(); return a.t }

type atomicInt struct {
	mu sync.Mutex
	v  int
}

func (a *atomicInt) set(v int) { a.mu.Lock(); a.v = v; a.mu.Unlock() }
