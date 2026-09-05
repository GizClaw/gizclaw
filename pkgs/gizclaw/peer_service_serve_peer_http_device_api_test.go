package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

// deviceHTTPFixture is one API-key-bound owner served by the Public HTTP
// device and contact extension without a real Giznet listener.
type deviceHTTPFixture struct {
	handler  http.Handler
	owner    giznet.PublicKey
	secret   string
	manager  *Manager
	peers    *peer.Server
	contacts *contact.Server
	control  *deviceController
	metrics  *metrics.MemoryStore
	apiKeys  *apikey.Server
	firmware *firmware.Server
}

func newDeviceHTTPFixture(t *testing.T) *deviceHTTPFixture {
	t.Helper()
	ctx := context.Background()
	ownerKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	keys := apikey.NewServer(kv.NewMemory(nil))
	created, err := keys.Create(ctx, ownerKey.Public.String(), "phone", false)
	if err != nil {
		t.Fatal(err)
	}
	peers := &peer.Server{Store: kv.NewMemory(nil)}
	manager := NewManager(peers)
	peers.PeerManager = manager
	manager.PeerRun = &peerrun.Server{Store: kv.NewMemory(nil)}
	manager.Metrics = metrics.NewMemoryStore()
	contacts := &contact.Server{Store: kv.NewMemory(nil)}
	manager.Contacts = contacts
	firmwares := &firmware.Server{Store: kv.NewMemory(nil)}
	manager.Firmwares = firmwares
	if _, err := peers.SavePeer(ctx, apitypes.Peer{
		PublicKey: ownerKey.Public.String(), Role: apitypes.PeerRoleClient,
		Status: apitypes.PeerRegistrationStatusActive,
		Device: apitypes.DeviceInfo{Name: new("kitchen-speaker"), Emoji: new("🔊"), Hardware: &apitypes.HardwareInfo{Model: new("H106")}},
	}); err != nil {
		t.Fatal(err)
	}
	profiles, _ := registrationServerAndToken(t, "profile-device-http")
	if err := profiles.BindOwnerProfile(ctx, ownerKey.Public.String(), "profile-device-http"); err != nil {
		t.Fatal(err)
	}
	manager.RuntimeProfiles = profiles
	control := newDeviceController(manager, manager.PeerRun)
	service := &PeerService{
		apiKeys: keys,
		manager: manager,
		public:  &peerHTTP{PeerHTTPService: peers, APIKeys: keys, Contacts: contacts, DeviceControl: control},
	}
	service.public.DeviceReads = service.deviceReadsForAPIKey
	return &deviceHTTPFixture{
		handler: service.publicHTTPHandler(keys), owner: ownerKey.Public, secret: created.Secret,
		manager: manager, peers: peers, contacts: contacts, control: control, metrics: manager.Metrics.(*metrics.MemoryStore),
		apiKeys: keys, firmware: firmwares,
	}
}

func (f *deviceHTTPFixture) do(t *testing.T, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Buffer
	if body != "" {
		reader = bytes.NewBufferString(body)
	} else {
		reader = bytes.NewBuffer(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+f.secret)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func decodeJSON[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatalf("decode %T: %v (body=%s)", value, err, response.Body.String())
	}
	return value
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	return decodeJSON[apitypes.ErrorResponse](t, response).Error.Code
}

func TestDeviceHTTPReadsAreOwnerBound(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	ctx := context.Background()
	reportedAt := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	if _, err := f.manager.PeerRun.PutStatus(ctx, f.owner, apitypes.PeerStatus{
		Volume: new(42), Muted: new(false), BatteryPercent: new(66), Charging: new(true),
		GnssLatitude: new(float32(31.2)), GnssLongitude: new(float32(121.5)), GnssAltitudeM: new(float32(12)), GnssAccuracyM: new(float32(3)),
		ReportedAt: &reportedAt,
	}); err != nil {
		t.Fatal(err)
	}
	otherKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.PeerRun.PutStatus(ctx, otherKey.Public, apitypes.PeerStatus{Volume: new(99)}); err != nil {
		t.Fatal(err)
	}

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET device status = %d body=%s", response.Code, response.Body.String())
	}
	info := decodeJSON[apitypes.DeviceInfo](t, response)
	if info.Name == nil || *info.Name != "kitchen-speaker" || info.Hardware == nil || *info.Hardware.Model != "H106" {
		t.Fatalf("device info = %+v", info)
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET runtime status = %d body=%s", response.Code, response.Body.String())
	}
	if runtime := decodeJSON[apitypes.Runtime](t, response); runtime.Online {
		t.Fatalf("runtime = %+v, want offline without an active connection", runtime)
	}

	// A caller-supplied Peer selector is ignored: the owner never changes.
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/status?public_key="+otherKey.Public.String(), "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", response.Code, response.Body.String())
	}
	status := decodeJSON[apitypes.PeerStatus](t, response)
	if status.Volume == nil || *status.Volume != 42 || status.BatteryPercent == nil || *status.BatteryPercent != 66 || status.Charging == nil || !*status.Charging {
		t.Fatalf("status = %+v", status)
	}
	if status.GnssLatitude == nil || *status.GnssLatitude != float32(31.2) || status.GnssAccuracyM == nil || *status.GnssAccuracyM != 3 || status.ReportedAt == nil || !status.ReportedAt.Equal(reportedAt) {
		t.Fatalf("status GNSS/report time = %+v", status)
	}

	// Reading never touches runtime or stored status.
	after, err := f.manager.PeerRun.GetStatus(ctx, f.owner)
	if err != nil || !after.ReportedAt.Equal(reportedAt) {
		t.Fatalf("stored status after read = %+v, %v", after, err)
	}
	if runtime := f.manager.PeerRuntime(ctx, f.owner); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after read = %+v", runtime)
	}

	response = httptest.NewRecorder()
	f.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/gizclaw/v1/device", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET device status = %d", response.Code)
	}
}

func TestDeviceHTTPTelemetryUsesOwnerAndValidatesQuery(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	other := giznet.PublicKey{5}
	for key, value := range map[giznet.PublicKey]float64{f.owner: 61, other: 7} {
		if err := f.metrics.Append(ctx, []metrics.Sample{{
			Name: peertelemetry.MetricBatteryPercent, Labels: map[string]string{"peer_id": key.String()}, Timestamp: at, Value: value,
		}}); err != nil {
			t.Fatal(err)
		}
	}

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/telemetry/latest?fields=battery.percent", "")
	if response.Code != http.StatusOK {
		t.Fatalf("latest status = %d body=%s", response.Code, response.Body.String())
	}
	latest := decodeJSON[apitypes.PeerTelemetryLatestResponse](t, response)
	if latest.PeerPublicKey != f.owner.String() || len(latest.Values) != 1 || latest.Values[0].Value != 61 || latest.Values[0].ObservedAtUnixMs != at.UnixMilli() {
		t.Fatalf("latest = %+v", latest)
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/telemetry/latest?fields=battery.unknown", "")
	if response.Code != http.StatusBadRequest || errorCode(t, response) != publicHTTPInvalidRequestCode {
		t.Fatalf("invalid field status = %d body=%s", response.Code, response.Body.String())
	}

	start := at.Add(-time.Minute).UnixMilli()
	end := at.Add(time.Minute).UnixMilli()
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/telemetry?field=battery.percent&start_time_ms="+itoa(start)+"&end_time_ms="+itoa(end)+"&limit=10&order=desc", "")
	if response.Code != http.StatusOK {
		t.Fatalf("range status = %d body=%s", response.Code, response.Body.String())
	}
	rangeResponse := decodeJSON[apitypes.PeerTelemetryRangeResponse](t, response)
	if rangeResponse.PeerPublicKey != f.owner.String() || rangeResponse.Field != apitypes.PeerTelemetryFieldBatteryPercent || len(rangeResponse.Points) == 0 || rangeResponse.Points[0].Value != 61 {
		t.Fatalf("range = %+v", rangeResponse)
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/telemetry?field=battery.percent&start_time_ms="+itoa(end)+"&end_time_ms="+itoa(start), "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("inverted range status = %d body=%s", response.Code, response.Body.String())
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/telemetry/aggregate?field=battery.percent&start_time_ms="+itoa(start)+"&end_time_ms="+itoa(end)+"&bucket_ms=60000&aggregate=max", "")
	if response.Code != http.StatusOK {
		t.Fatalf("aggregate status = %d body=%s", response.Code, response.Body.String())
	}
	aggregate := decodeJSON[apitypes.PeerTelemetryAggregateResponse](t, response)
	if aggregate.PeerPublicKey != f.owner.String() || aggregate.Aggregate != apitypes.PeerTelemetryAggregateMax || len(aggregate.Points) == 0 || aggregate.Points[0].Value != 61 {
		t.Fatalf("aggregate = %+v", aggregate)
	}
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}

func TestDeviceHTTPContactsCRUD(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	ctx := context.Background()

	response := f.do(t, http.MethodPost, "/gizclaw/v1/contacts", `{"name":"mom","display_name":"Mom","phone_number":"+8613800000001"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", response.Code, response.Body.String())
	}
	created := decodeJSON[peerhttp.Contact](t, response)
	if created.Name != "mom" || created.DisplayName == nil || *created.DisplayName != "Mom" || created.CreatedAt == nil {
		t.Fatalf("created contact = %+v", created)
	}
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/contacts", `{"name":"dad","phone_number":"+8613800000002"}`); response.Code != http.StatusCreated {
		t.Fatalf("create second status = %d body=%s", response.Code, response.Body.String())
	}

	response = f.do(t, http.MethodPost, "/gizclaw/v1/contacts", `{"name":"mom","display_name":"Again"}`)
	if response.Code != http.StatusConflict || errorCode(t, response) != publicHTTPContactExists {
		t.Fatalf("duplicate create status = %d body=%s", response.Code, response.Body.String())
	}
	response = f.do(t, http.MethodPost, "/gizclaw/v1/contacts", `{"name":"empty"}`)
	if response.Code != http.StatusBadRequest || errorCode(t, response) != publicHTTPInvalidRequestCode {
		t.Fatalf("invalid create status = %d body=%s", response.Code, response.Body.String())
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/contacts?limit=1", "")
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", response.Code, response.Body.String())
	}
	page := decodeJSON[peerhttp.ContactList](t, response)
	if len(page.Items) != 1 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	response = f.do(t, http.MethodGet, "/gizclaw/v1/contacts?limit=1&cursor="+*page.NextCursor, "")
	if response.Code != http.StatusOK {
		t.Fatalf("second page status = %d body=%s", response.Code, response.Body.String())
	}
	second := decodeJSON[peerhttp.ContactList](t, response)
	if len(second.Items) != 1 || second.HasNext || second.Items[0].Name == page.Items[0].Name {
		t.Fatalf("second page = %+v", second)
	}
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/contacts?limit=0", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("limit=0 status = %d", response.Code)
	}

	response = f.do(t, http.MethodPut, "/gizclaw/v1/contacts/mom", `{"display_name":"Mother"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("put status = %d body=%s", response.Code, response.Body.String())
	}
	updated := decodeJSON[peerhttp.Contact](t, response)
	if *updated.DisplayName != "Mother" || updated.PhoneNumber == nil || *updated.PhoneNumber != "+8613800000001" || updated.UpdatedAt == nil {
		t.Fatalf("updated contact = %+v", updated)
	}
	response = f.do(t, http.MethodPut, "/gizclaw/v1/contacts/mom", `{"phone_number":"+8613800000002"}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("put duplicate phone status = %d body=%s", response.Code, response.Body.String())
	}

	// Contacts of another owner are invisible: same 404 as a missing name.
	otherKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.contacts.CreateContact(ctx, otherKey.Public.String(), rpcapi.ContactCreateRequest{Name: "neighbour", DisplayName: new("Neighbour")}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"neighbour", "missing"} {
		response = f.do(t, http.MethodGet, "/gizclaw/v1/contacts/"+name, "")
		if response.Code != http.StatusNotFound || errorCode(t, response) != publicHTTPContactNotFound {
			t.Fatalf("GET %s status = %d body=%s", name, response.Code, response.Body.String())
		}
		if response := f.do(t, http.MethodDelete, "/gizclaw/v1/contacts/"+name, ""); response.Code != http.StatusNotFound {
			t.Fatalf("DELETE %s status = %d", name, response.Code)
		}
	}

	response = f.do(t, http.MethodGet, "/gizclaw/v1/contacts/mom", "")
	if response.Code != http.StatusOK || decodeJSON[peerhttp.Contact](t, response).Name != "mom" {
		t.Fatalf("get status = %d body=%s", response.Code, response.Body.String())
	}
	if response := f.do(t, http.MethodDelete, "/gizclaw/v1/contacts/mom", ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", response.Code, response.Body.String())
	}
	if response := f.do(t, http.MethodGet, "/gizclaw/v1/contacts/mom", ""); response.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d", response.Code)
	}
	items, err := f.contacts.ListContacts(ctx, otherKey.Public.String(), rpcapi.ContactListRequest{})
	if err != nil || len(items.Items) != 1 {
		t.Fatalf("other owner contacts after delete = %+v, %v", items, err)
	}
}
