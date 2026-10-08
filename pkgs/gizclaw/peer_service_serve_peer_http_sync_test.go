package gizclaw

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peersync"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/getkin/kin-openapi/openapi3"
)

type peerSyncBatch struct {
	reset     bool
	timestamp int64
	upserts   map[string]json.RawMessage
	deleted   []string
	events    []peerhttp.SyncEvent
}

func newPeerSyncHTTPFixture(t *testing.T) *socialHTTPFixture {
	t.Helper()
	f := newSocialHTTPFixture(t)
	f.public.Sync = &peersync.Server{Store: f.peers.Store}
	f.public.PeerAvailability = f.peers.EnsureAvailable
	return f
}

func readPeerSync(t *testing.T, body io.Reader) peerSyncBatch {
	t.Helper()
	batch := peerSyncBatch{upserts: make(map[string]json.RawMessage)}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var name string
	for scanner.Scan() {
		line := scanner.Text()
		if value, ok := strings.CutPrefix(line, "event: "); ok {
			name = value
		}
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		if batch.timestamp != 0 {
			t.Fatal("event received after done")
		}
		var event peerhttp.SyncEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatal(err)
		}
		discriminator, err := event.Discriminator()
		if err != nil || discriminator != name {
			t.Fatalf("SSE name %q differs from payload discriminator %q: %v", name, discriminator, err)
		}
		batch.events = append(batch.events, event)
		switch name {
		case "reset":
			if len(batch.events) != 1 {
				t.Fatal("reset must be the first event")
			}
			batch.reset = true
		case "upsert":
			item, err := event.AsSyncUpsert()
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := batch.upserts[item.Key]; exists {
				t.Fatalf("duplicate item %q", item.Key)
			}
			data, err := item.Data.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			batch.upserts[item.Key] = data
		case "delete":
			item, err := event.AsSyncDelete()
			if err != nil {
				t.Fatal(err)
			}
			batch.deleted = append(batch.deleted, item.Key)
		case "done":
			done, err := event.AsSyncDone()
			if err != nil || done.Timestamp <= 0 {
				t.Fatalf("invalid completion %#v, %v", done, err)
			}
			batch.timestamp = done.Timestamp
		default:
			t.Fatalf("unknown sync event %q", name)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if batch.timestamp == 0 {
		t.Fatal("sync ended without done")
	}
	return batch
}

func syncAs(t *testing.T, f *socialHTTPFixture, peer socialHTTPPeer, timestamp int64) peerSyncBatch {
	t.Helper()
	response := f.as(t, peer, http.MethodGet, "/sync?timestamp="+strconv.FormatInt(timestamp, 10), "")
	expect(t, response, http.StatusOK, "")
	if response.Header().Get("Content-Type") != "text/event-stream" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("sync headers = %v", response.Header())
	}
	return readPeerSync(t, response.Body)
}

func TestPeerHTTPSyncOwnerIsolationAndSchema(t *testing.T) {
	f := newPeerSyncHTTPFixture(t)
	expect(t, f.as(t, f.a, http.MethodPost, "/contacts", `{"name":"alice","display_name":"Alice"}`), http.StatusCreated, "")
	expect(t, f.as(t, f.b, http.MethodPost, "/contacts", `{"name":"private-b","display_name":"Private B"}`), http.StatusCreated, "")
	response := f.as(t, f.a, http.MethodGet, "/sync?timestamp=0&public_key="+f.b.key.String(), "")
	expect(t, response, http.StatusOK, "")
	first := readPeerSync(t, response.Body)
	if !first.reset || first.upserts[syncPrefix+"contacts/alice"] == nil || strings.Contains(response.Body.String(), "Private B") {
		t.Fatalf("owner-scoped sync = %#v", first)
	}
	var info apitypes.DeviceInfo
	if err := json.Unmarshal(first.upserts[syncPrefix+"device"], &info); err != nil || info.Name == nil || *info.Name != "kitchen-speaker" {
		t.Fatalf("device projection = %#v, %v", info, err)
	}
	if first.upserts[syncPrefix+"device/firmware"] != nil {
		t.Fatal("unbound firmware appeared in sync")
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	spec, err := loader.LoadFromFile("../../api/http/peer.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, event := range first.events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		if err := spec.Components.Schemas["SyncEvent"].Value.VisitJSON(value); err != nil {
			t.Fatalf("event violates OpenAPI: %s: %v", data, err)
		}
	}
	unchanged := syncAs(t, f, f.a, first.timestamp)
	if unchanged.reset || len(unchanged.events) != 1 || unchanged.timestamp <= first.timestamp {
		t.Fatalf("unchanged sync = %#v", unchanged)
	}
	other := syncAs(t, f, f.b, first.timestamp)
	if !other.reset || len(other.deleted) != 0 || other.upserts[syncPrefix+"contacts/alice"] != nil || other.upserts[syncPrefix+"contacts/private-b"] == nil {
		t.Fatalf("foreign timestamp = %#v", other)
	}
}

type missingSyncFirmware struct {
	firmware.FirmwareAdminService
}

func (missingSyncFirmware) GetFirmware(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
	return nil, fmt.Errorf("firmware lookup: %w", kv.ErrNotFound)
}

func TestPeerHTTPSyncMissingFirmwareDeletesPreviousItem(t *testing.T) {
	f := newPeerSyncHTTPFixture(t)
	seedBoundFirmware(t, f.deviceHTTPFixture, "devkit")
	first := syncAs(t, f, f.a, 0)
	if first.upserts[syncPrefix+"device/firmware"] == nil {
		t.Fatal("bound firmware missing from initial sync")
	}
	deviceReads := f.public.DeviceReads
	f.public.DeviceReads = func(owner giznet.PublicKey) peerresource.DeviceReads {
		reads := deviceReads(owner)
		reads.Firmwares = missingSyncFirmware{FirmwareAdminService: f.firmware}
		return reads
	}
	expect(t, f.as(t, f.a, http.MethodGet, "/device/firmware", ""), http.StatusNotFound, publicHTTPFirmwareNotFound)
	next := syncAs(t, f, f.a, first.timestamp)
	if next.reset || len(next.upserts) != 0 || !slices.Equal(next.deleted, []string{syncPrefix + "device/firmware"}) {
		t.Fatalf("missing firmware changes = %#v", next)
	}
	full := syncAs(t, f, f.a, 0)
	if !full.reset || full.upserts[syncPrefix+"device/firmware"] != nil {
		t.Fatalf("missing firmware reset = %#v", full)
	}
}

func TestPeerHTTPSyncOnlineFinalStatusDeletionAndRetry(t *testing.T) {
	f := newPeerSyncHTTPFixture(t)
	expect(t, f.as(t, f.a, http.MethodPost, "/contacts", `{"name":"alice","display_name":"Alice"}`), http.StatusCreated, "")
	first := syncAs(t, f, f.a, 0)
	device := newFakeDeviceConn(func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		t.Error("sync must not contact the device")
		return nil, fmt.Errorf("unexpected device RPC")
	})
	f.manager.SetPeerUp(f.owner, device)
	for _, volume := range []int{30, 75, 90} {
		if _, err := f.manager.PeerRun.PutStatus(t.Context(), f.owner, apitypes.PeerStatus{Volume: &volume}); err != nil {
			t.Fatal(err)
		}
	}
	expect(t, f.as(t, f.a, http.MethodDelete, "/contacts/alice", ""), http.StatusNoContent, "")
	next := syncAs(t, f, f.a, first.timestamp)
	if next.reset || !slices.Equal(next.deleted, []string{syncPrefix + "contacts/alice"}) || len(next.upserts) != 2 {
		t.Fatalf("changes = %#v", next)
	}
	var runtime apitypes.Runtime
	if err := json.Unmarshal(next.upserts[syncPrefix+"device/runtime"], &runtime); err != nil || !runtime.Online {
		t.Fatalf("online state = %#v, %v", runtime, err)
	}
	var status apitypes.PeerStatus
	if err := json.Unmarshal(next.upserts[syncPrefix+"device/status"], &status); err != nil || status.Volume == nil || *status.Volume != 90 {
		t.Fatalf("final status = %#v, %v", status, err)
	}
	retry := syncAs(t, f, f.a, first.timestamp)
	if retry.reset || len(retry.upserts) != 2 || !slices.Equal(retry.deleted, next.deleted) {
		t.Fatalf("retry from completed checkpoint = %#v", retry)
	}
	// Replacing the service retains checkpoint continuity through shared KV.
	f.public.Sync = &peersync.Server{Store: f.peers.Store}
	resumed := syncAs(t, f, f.a, next.timestamp)
	if resumed.reset || len(resumed.events) != 1 {
		t.Fatalf("resumed = %#v", resumed)
	}
	f.manager.SetPeerDown(f.owner, device)
	offline := syncAs(t, f, f.a, resumed.timestamp)
	if err := json.Unmarshal(offline.upserts[syncPrefix+"device/runtime"], &runtime); err != nil || runtime.Online {
		t.Fatalf("offline state = %#v, %v", runtime, err)
	}
}

func TestPeerHTTPSyncGroupsUseOwnerNamesAndPermissions(t *testing.T) {
	f := newPeerSyncHTTPFixture(t)
	expect(t, f.as(t, f.a, http.MethodPost, "/friend-groups", `{"name":"family"}`), http.StatusCreated, "")
	token := decodeJSON[peerhttp.InviteToken](t, f.as(t, f.a, http.MethodPost, "/friend-groups/family/invite-token", ""))
	expect(t, f.as(t, f.b, http.MethodPost, "/friend-groups/@join", `{"invite_token":"`+token.InviteToken+`","name":"my-family"}`), http.StatusOK, "")
	owner := syncAs(t, f, f.a, 0)
	if owner.upserts[syncPrefix+"friend-groups/family/invite-token"] == nil {
		t.Fatal("owner invite token missing")
	}
	member := syncAs(t, f, f.b, 0)
	if member.upserts[syncPrefix+"friend-groups/my-family"] == nil || member.upserts[syncPrefix+"friend-groups/my-family/invite-token"] != nil || member.upserts[syncPrefix+"friend-groups/family"] != nil {
		t.Fatalf("member's scoped group projection = %#v", member)
	}
	memberPath := syncPrefix + "friend-groups/my-family/members/" + url.PathEscape(f.a.key.String())
	if member.upserts[memberPath] == nil {
		t.Fatal("visible group member missing")
	}
	expect(t, f.as(t, f.a, http.MethodDelete, "/friend-groups/family/members/"+url.PathEscape(f.b.key.String()), ""), http.StatusNoContent, "")
	lost := syncAs(t, f, f.b, member.timestamp)
	if !slices.Contains(lost.deleted, syncPrefix+"friend-groups/my-family") || !slices.Contains(lost.deleted, memberPath) {
		t.Fatalf("lost access did not remove group state: %#v", lost)
	}
}

type peerSyncFailingStore struct{ kv.Store }

func (s peerSyncFailingStore) ApplyMutation(context.Context, kv.Mutation) (bool, error) {
	return false, fmt.Errorf("backend private detail")
}

func TestPeerHTTPSyncErrorsStayJSONAndDoNotLeak(t *testing.T) {
	f := newPeerSyncHTTPFixture(t)
	for _, timestamp := range []string{"", "-1", "1.5", "abc", "9007199254740992"} {
		response := f.as(t, f.a, http.MethodGet, "/sync?timestamp="+timestamp, "")
		expect(t, response, http.StatusBadRequest, "")
		if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("invalid timestamp uses non-JSON error: %v", response.Header())
		}
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, syncPrefix+"sync?timestamp=0", nil))
	expect(t, response, http.StatusUnauthorized, "INVALID_API_KEY")
	f.public.Sync = &peersync.Server{Store: peerSyncFailingStore{Store: f.peers.Store}}
	failed := f.as(t, f.a, http.MethodGet, "/sync?timestamp=0", "")
	expect(t, failed, http.StatusInternalServerError, "INTERNAL_ERROR")
	if strings.Contains(failed.Body.String(), "private") || strings.HasPrefix(failed.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("failed sync leaked or started SSE: %s", failed.Body.String())
	}
	if err := f.peers.DeleteSelf(t.Context(), f.a.key); err != nil {
		t.Fatal(err)
	}
	expect(t, f.as(t, f.a, http.MethodGet, "/sync?timestamp=0", ""), http.StatusConflict, "PEER_PENDING_DELETION")
}

func TestPeerHTTPSyncGeneratedClientStreamsOverHTTP(t *testing.T) {
	f := newPeerSyncHTTPFixture(t)
	server := httptest.NewServer(f.handler)
	t.Cleanup(server.Close)
	client, err := peerhttp.NewClient(server.URL, peerhttp.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	response, err := client.SyncPeer(ctx, &peerhttp.SyncPeerParams{Timestamp: 0}, func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+f.a.secret)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream response = %d, %v", response.StatusCode, response.Header)
	}
	batch := readPeerSync(t, response.Body)
	if !batch.reset || batch.upserts[syncPrefix+"device/runtime"] == nil {
		t.Fatalf("generated HTTP stream = %#v", batch)
	}
}
