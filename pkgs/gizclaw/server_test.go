package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type testGiznetSecurityPolicy struct {
	allowService func(giznet.PublicKey, uint64) bool
}

type storeWithoutAtomicCreate struct {
	kv.Store
}

type storeWithoutAtomicCompare struct {
	kv.Store
}

func (s storeWithoutAtomicCompare) CreateIfAbsent(
	ctx context.Context,
	guard kv.Entry,
	entries []kv.Entry,
) ([]byte, bool, error) {
	return kv.CreateIfAbsent(ctx, s.Store, guard, entries)
}

func (p testGiznetSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (p testGiznetSecurityPolicy) AllowService(pk giznet.PublicKey, service uint64) bool {
	if p.allowService == nil {
		return service == 0
	}
	return p.allowService(pk, service)
}

func TestServerListenRequiresPeerStore(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	server := &Server{LocalStatic: *keyPair}
	err = server.Listen()
	if err == nil || !strings.Contains(err.Error(), "nil peer store") {
		t.Fatalf("Listen error = %v, want nil peer store", err)
	}
	server.PeerStore = kv.NewMemory(nil)
	err = server.Listen()
	if err == nil || !strings.Contains(err.Error(), "nil peer run store") {
		t.Fatalf("Listen error = %v, want nil peer run store", err)
	}
}

func TestServerInitRequiresAtomicStoreCapabilities(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	friendGroupStore := storeWithoutAtomicCreate{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name        string
		server      *Server
		wantMessage string
	}{
		{
			name: "peer store",
			server: &Server{
				LocalStatic: *keyPair,
				PeerStore:   storeWithoutAtomicCreate{Store: kv.NewMemory(nil)},
			},
			wantMessage: "peer store",
		},
		{
			name: "workspace store",
			server: &Server{
				LocalStatic:    *keyPair,
				PeerStore:      kv.NewMemory(nil),
				WorkspaceStore: storeWithoutAtomicCreate{Store: kv.NewMemory(nil)},
			},
			wantMessage: "workspace store",
		},
		{
			name: "friend store",
			server: &Server{
				LocalStatic: *keyPair,
				PeerStore:   kv.NewMemory(nil),
				FriendStore: storeWithoutAtomicCreate{Store: kv.NewMemory(nil)},
			},
			wantMessage: "friend store",
		},
		{
			name: "friend group relationship store",
			server: &Server{
				LocalStatic:      *keyPair,
				PeerStore:        kv.NewMemory(nil),
				FriendGroupStore: friendGroupStore,
			},
			wantMessage: "friend group relationship store",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			completeTestServer(t, tc.server)
			err := tc.server.init()
			if !errors.Is(err, kv.ErrCreateIfAbsentUnsupported) || !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("init() error = %v, want %q wrapping ErrCreateIfAbsentUnsupported", err, tc.wantMessage)
			}
		})
	}
	compareServer := &Server{
		LocalStatic: *keyPair,
		PeerStore:   kv.NewMemory(nil),
		FriendStore: storeWithoutAtomicCompare{Store: kv.NewMemory(nil)},
	}
	completeTestServer(t, compareServer)
	err = compareServer.init()
	if !errors.Is(err, kv.ErrCompareAndMutateUnsupported) ||
		!strings.Contains(err.Error(), "friend store") {
		t.Fatalf(
			"init() error = %v, want friend store wrapping ErrCompareAndMutateUnsupported",
			err,
		)
	}
}

func TestServerInitReconcilesFriendCreationIntents(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	friendStore := mustBadgerInMemory(t, nil)
	if err := kv.Prefixed(friendStore, kv.Key{"friends"}).Set(
		t.Context(),
		kv.Key{"friend-creation-intents", "invalid"},
		[]byte("{"),
	); err != nil {
		t.Fatalf("write malformed Friend creation intent: %v", err)
	}
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
		FriendStore: friendStore,
	}
	completeTestServer(t, server)
	err = server.init()
	if err == nil || !strings.Contains(err.Error(), "reconcile Friend creation intents") {
		t.Fatalf("init() error = %v, want Friend creation reconciliation error", err)
	}
}

func TestServerInitDerivesFriendGroupRelationshipViews(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	server := &Server{
		LocalStatic:      *keyPair,
		PeerStore:        kv.NewMemory(nil),
		FriendGroupStore: kv.NewMemory(nil),
	}
	completeTestServer(t, server)
	if err := server.init(); err != nil {
		t.Fatalf("init() error = %v", err)
	}
}

func TestServerListenValidatesReceiverAndLocalStatic(t *testing.T) {
	t.Run("nil server", func(t *testing.T) {
		var server *Server
		if err := server.Listen(); err == nil || !strings.Contains(err.Error(), "nil server") {
			t.Fatalf("Listen() err = %v", err)
		}
	})

	t.Run("nil key pair", func(t *testing.T) {
		server := &Server{}
		if err := server.Listen(); err == nil || !strings.Contains(err.Error(), "empty local static private key") {
			t.Fatalf("Listen() empty local static private key err = %v", err)
		}
	})
}

func TestServerServeReturnsNilAfterClose(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	server := &Server{
		LocalStatic:   *keyPair,
		PeerStore:     mustBadgerInMemory(t, nil),
		PeerListeners: []giznet.Listener{newTestGiznetListener()},
	}
	completeTestServer(t, server)
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve()
	}()

	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() after Close() error = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve() did not return after Close()")
	}
}

func TestServerListenDoesNotReadColdWorkspaceRecordsForRewards(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	workspaceStore := mustBadgerInMemory(t, nil)
	malformed := []byte("{")
	if err := workspaceStore.Set(t.Context(), kv.Key{"by-id", "workspace-cold"}, malformed); err != nil {
		t.Fatalf("seed malformed cold Workspace: %v", err)
	}
	listener := newTestGiznetListener()
	server := &Server{
		LocalStatic: *keyPair, PeerStore: mustBadgerInMemory(t, nil),
		WorkspaceStore: workspaceStore, PeerListeners: []giznet.Listener{listener},
	}
	completeTestServer(t, server)
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	select {
	case <-listener.closed:
		t.Fatal("listener was closed after reading a cold Workspace")
	default:
	}
	stored, err := workspaceStore.Get(t.Context(), kv.Key{"by-id", "workspace-cold"})
	if err != nil {
		t.Fatalf("read retained cold Workspace: %v", err)
	}
	if !bytes.Equal(stored, malformed) {
		t.Fatalf("cold Workspace was rewritten: got %q want %q", stored, malformed)
	}
}

func TestServerListenProcessesExistingPetDeletion(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	db, err := sqlx.Open("sqlite", "file:server-pending-deletion?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sqlx.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	runtime := &gameplay.Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	const owner = "peer-server-cleanup"
	const petID = "pet-server-cleanup"
	if _, err := db.ExecContext(ctx, `INSERT INTO gameplay_pets (
		owner_public_key, id, name, runtime_profile_id, pet_def_id, display_name, workspace_id,
		stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		owner, petID, "pet-main", "profile-main", "petdef-main", "Pet", "workspace-main",
		`{"life":100,"health":100,"satiety":100,"hygiene":100,"mood":100,"energy":100}`,
		`{"experience":0,"level":1}`, "alive", nil,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert Pet: %v", err)
	}
	if _, err := runtime.DeletePet(ctx, owner, petID); err != nil {
		t.Fatalf("DeletePet() error = %v", err)
	}
	var markers int
	if err := db.GetContext(ctx, &markers, `SELECT COUNT(*) FROM gameplay_pending_deletions`); err != nil {
		t.Fatalf("count pending deletions: %v", err)
	}
	if markers != 1 {
		t.Fatalf("pending deletions = %d, want 1 before Listen", markers)
	}

	server := &Server{
		LocalStatic:   *keyPair,
		PeerStore:     kv.NewMemory(nil),
		GameplayDB:    db,
		PeerListeners: []giznet.Listener{newTestGiznetListener()},
	}
	completeTestServer(t, server)
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	deadline := time.Now().Add(3 * time.Second)
	for {
		var pets int
		if err := db.GetContext(ctx, &pets, `SELECT COUNT(*) FROM gameplay_pets WHERE id = ?`, petID); err != nil {
			t.Fatalf("count Pet: %v", err)
		}
		if pets == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Pet still exists after startup scan deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := db.GetContext(ctx, &markers, `SELECT COUNT(*) FROM gameplay_pending_deletions`); err != nil {
		t.Fatalf("count completed pending deletions: %v", err)
	}
	if markers != 0 {
		t.Fatalf("pending deletions = %d, want 0 after cleanup", markers)
	}
}

func TestServerCanListenAgainAfterClose(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	first := newTestGiznetListener()
	second := newTestGiznetListener()
	server := &Server{
		LocalStatic:   *keyPair,
		PeerStore:     mustBadgerInMemory(t, nil),
		PeerListeners: []giznet.Listener{first},
	}
	completeTestServer(t, server)
	if err := server.Listen(); err != nil {
		t.Fatalf("first Listen() error = %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	server.PeerListeners = []giznet.Listener{second}
	if err := server.Listen(); err != nil {
		t.Fatalf("second Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve()
	}()
	if err := second.Close(); err != nil {
		t.Fatalf("second listener Close() error = %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() after second listener close error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve() did not use second listener")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() after second Listen() error = %v", err)
	}
}

func TestPeerHTTPWebRTCSignalingUsesGeneratedRoute(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	var gotBody []byte
	var gotContentType string
	var gotPublicKey string
	var gotTimestamp string
	var gotNonce string
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
		WebRTCSignalingHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("signaling method = %q", r.Method)
			}
			if r.URL.Path != gizwebrtc.SignalingPath {
				t.Fatalf("signaling path = %q", r.URL.Path)
			}
			gotContentType = r.Header.Get("Content-Type")
			gotPublicKey = r.Header.Get("X-Giznet-Public-Key")
			gotTimestamp = r.Header.Get("X-Giznet-Timestamp")
			gotNonce = r.Header.Get("X-Giznet-Nonce")
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("encrypted-answer"))
		}),
	}
	completeTestServer(t, server)
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, bytes.NewReader([]byte("encrypted-offer")))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Giznet-Public-Key", "peer-public")
	req.Header.Set("X-Giznet-Timestamp", "123456789")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "encrypted-answer" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if gotContentType != "application/octet-stream" {
		t.Fatalf("forwarded content-type = %q", gotContentType)
	}
	if gotPublicKey != "peer-public" || gotTimestamp != "123456789" || gotNonce != "nonce" {
		t.Fatalf("forwarded headers public=%q ts=%q nonce=%q", gotPublicKey, gotTimestamp, gotNonce)
	}
	if string(gotBody) != "encrypted-offer" {
		t.Fatalf("forwarded body = %q", string(gotBody))
	}
}

func TestPeerHTTPWebRTCSignalingUnavailable(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
	}
	completeTestServer(t, server)
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, strings.NewReader("encrypted-offer"))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Giznet-Public-Key", "peer-public")
	req.Header.Set("X-Giznet-Timestamp", "123456789")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if payload.Error != "webrtc_signaling_listener_unavailable" {
		t.Fatalf("error = %q", payload.Error)
	}
}

func TestPeerHTTPWebRTCSignalingRejectsRetiringPeerBeforeHandler(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{name: "pending", err: runtimepeer.ErrPeerPendingDeletion, code: runtimepeer.PeerPendingDeletionCode},
		{name: "deleted", err: runtimepeer.ErrPeerDeleted, code: runtimepeer.PeerDeletedCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			service := &peerHTTP{
				PeerAvailability: func(context.Context, giznet.PublicKey) error { return tc.err },
				WebRTCSignalingHandler: func() http.Handler {
					return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
				},
			}
			response, err := service.CreateGiznetWebRTCOffer(t.Context(), peerhttp.CreateGiznetWebRTCOfferRequestObject{
				Params: peerhttp.CreateGiznetWebRTCOfferParams{
					XGiznetPublicKey: keyPair.Public.String(),
					XGiznetTimestamp: 1,
					XGiznetNonce:     "nonce",
				},
				Body: strings.NewReader("encrypted-offer"),
			})
			if err != nil {
				t.Fatal(err)
			}
			conflict, ok := response.(peerhttp.CreateGiznetWebRTCOffer409JSONResponse)
			if !ok || conflict.Error != tc.code {
				t.Fatalf("response = %#v, want 409 %q", response, tc.code)
			}
			if called {
				t.Fatal("signaling handler was called for a retiring Peer")
			}
		})
	}
}

func TestPeerHTTPWebRTCSignalingPreservesContentType(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
		WebRTCSignalingHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("forwarded content-type = %q", r.Header.Get("Content-Type"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = w.Write([]byte(`{"error":"unsupported_media_type"}`))
		}),
	}
	completeTestServer(t, server)
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, strings.NewReader(`{"offer":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Giznet-Public-Key", "peer-public")
	req.Header.Set("X-Giznet-Timestamp", "123456789")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if payload.Error != "unsupported_media_type" {
		t.Fatalf("error = %q", payload.Error)
	}
}

func TestServerServeWithoutListenStillRequiresListenerAfterClose(t *testing.T) {
	server := &Server{}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := server.Serve(); !errors.Is(err, giznet.ErrNilListener) {
		t.Fatalf("Serve() error = %v, want %v", err, giznet.ErrNilListener)
	}
}

func TestServerPublicKeyAndPeerServiceAccessors(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &PeerService{}
	server := &Server{LocalStatic: *keyPair, peerService: service}
	if got := server.PublicKey(); got != keyPair.Public {
		t.Fatalf("PublicKey() = %v, want %v", got, keyPair.Public)
	}
	if got := server.PeerService(); got != service {
		t.Fatalf("PeerService() = %v, want %v", got, service)
	}
}

func TestServerInitConfiguresPeerRunService(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	peerRunRoot := kv.NewMemory(nil)
	battery := 73
	existingStatus, err := json.Marshal(apitypes.PeerStatus{BatteryPercent: &battery})
	if err != nil {
		t.Fatalf("json.Marshal existing PeerRun status: %v", err)
	}
	if err := kv.Prefixed(peerRunRoot, kv.Key{"runs"}).Set(
		t.Context(),
		kv.Key{"by-peer", keyPair.Public.String(), "status"},
		existingStatus,
	); err != nil {
		t.Fatalf("write existing PeerRun status: %v", err)
	}
	server := &Server{
		LocalStatic:  *keyPair,
		PeerStore:    mustBadgerInMemory(t, nil),
		PeerRunStore: peerRunRoot,
	}
	completeTestServer(t, server)
	if err := server.init(); err != nil {
		t.Fatalf("init() error = %v", err)
	}
	if server.manager == nil || server.manager.PeerRun == nil || server.manager.AgentHost == nil || server.manager.Voices == nil || server.manager.ProviderTenants == nil {
		t.Fatalf("manager peer run runtime services not configured: %+v", server.manager)
	}
	status, err := server.manager.PeerRun.GetStatus(t.Context(), keyPair.Public)
	if err != nil || status.BatteryPercent == nil || *status.BatteryPercent != battery {
		t.Fatalf("existing runs/ status = %+v, %v; want battery %d", status, err, battery)
	}
	conn := &PeerConn{Service: server.peerService}
	conn.initRPC()
	if conn.rpc == nil || conn.rpc.peerRun != server.manager.PeerRun {
		t.Fatalf("PeerConn rpc peerRun = %+v, want %+v", conn.rpc, server.manager.PeerRun)
	}
}

func TestServerInitConfiguresWorkspaceRuntimeStore(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{
		LocalStatic:    *keyPair,
		PeerStore:      mustBadgerInMemory(t, nil),
		AgentHostStore: newTestObjectStore(t),
	}
	completeTestServer(t, server)
	if err := server.init(); err != nil {
		t.Fatalf("init() error = %v", err)
	}
	workspaces, ok := server.manager.Workspaces.(*workspace.Server)
	if server.manager == nil || !ok || workspaces.RuntimeStore == nil {
		t.Fatalf("workspace runtime store not configured: %+v", server.manager)
	}
}

func TestServerServeHTTPAPIKeyAndRejectsLegacyLogin(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := completeTestServer(t, &Server{LocalStatic: *serverKey, BuildCommit: "test-build"})
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}
	if _, err := server.manager.Peers.SavePeer(t.Context(), apitypes.Peer{
		PublicKey: deviceKey.Public.String(), Role: apitypes.PeerRoleClient,
		Status: apitypes.PeerRegistrationStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	profiles, _ := registrationServerAndToken(t, "profile-server-http-key")
	if err := profiles.BindOwnerProfile(t.Context(), deviceKey.Public.String(), "profile-server-http-key"); err != nil {
		t.Fatal(err)
	}
	server.manager.RuntimeProfiles = profiles
	created, err := server.apiKeys.Create(t.Context(), deviceKey.Public.String(), "test client", true)
	if err != nil {
		t.Fatal(err)
	}

	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	for _, path := range []string{"/login", "/me", "/side-control/sessions"} {
		response, err := http.Get(httpServer.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, response.StatusCode)
		}
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, httpServer.URL+"/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+created.Secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET API key self status = %d body=%s", response.StatusCode, body)
	}

	request, err = http.NewRequestWithContext(t.Context(), http.MethodGet, httpServer.URL+"/openai/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+created.Secret)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET OpenAI models status = %d body=%s", response.StatusCode, body)
	}
}

func TestServerServeHTTPHandlesBrowserCORS(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := completeTestServer(t, &Server{LocalStatic: *serverKey, BuildCommit: "test-build"})
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	const origin = "https://app.example.com"
	preflight := httptest.NewRequest(http.MethodOptions, "/gizclaw/v1/api-keys/self", nil)
	preflight.Header.Set("Origin", origin)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-request-id")
	preflightResponse := httptest.NewRecorder()
	server.ServeHTTP(preflightResponse, preflight)

	if preflightResponse.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d body=%s", preflightResponse.Code, preflightResponse.Body.String())
	}
	if got := preflightResponse.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("OPTIONS Access-Control-Allow-Origin = %q", got)
	}
	if got := preflightResponse.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodDelete) {
		t.Fatalf("OPTIONS Access-Control-Allow-Methods = %q", got)
	}
	if got := preflightResponse.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") || !strings.Contains(got, requestIDHeader) {
		t.Fatalf("OPTIONS Access-Control-Allow-Headers = %q", got)
	}

	request := httptest.NewRequest(http.MethodGet, "/server-info", nil)
	request.Header.Set("Origin", origin)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("GET Access-Control-Allow-Origin = %q", got)
	}
	if got := response.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("GET Vary = %q", got)
	}
}
func TestServerPeerEventHandlerDoesNotClearActivePeer(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{manager: &Manager{}}
	server.manager.SetPeerUp(keyPair.Public, &testGiznetConn{})

	(*serverPeerEventHandler)(server).HandlePeerEvent(giznet.PeerEvent{PublicKey: keyPair.Public, State: giznet.PeerStateOffline})
	runtime := server.manager.PeerRuntime(context.Background(), keyPair.Public)
	if !runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after offline event = %+v, want active peer unchanged", runtime)
	}
}
