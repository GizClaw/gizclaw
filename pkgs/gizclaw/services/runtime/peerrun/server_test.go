package peerrun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestServerStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)
	publicKey := testPublicKey(t)
	if got, err := server.GetStatus(ctx, publicKey); err != nil || got.Volume != nil {
		t.Fatalf("GetStatus(empty) = %+v, %v", got, err)
	}
	volume := 42
	reportedAt := time.Unix(100, 0).UTC()
	status, err := server.PutStatus(ctx, publicKey, apitypes.PeerStatus{
		ReportedAt: &reportedAt,
		Volume:     &volume,
		Labels:     &map[string]string{"mode": "test"},
	})
	if err != nil {
		t.Fatalf("PutStatus() error = %v", err)
	}
	if status.Volume == nil || *status.Volume != volume {
		t.Fatalf("PutStatus() = %+v", status)
	}
	got, err := server.GetStatus(ctx, publicKey)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if got.Volume == nil || *got.Volume != volume || got.ReportedAt == nil || !got.ReportedAt.Equal(reportedAt) {
		t.Fatalf("GetStatus() = %+v", got)
	}
}

func TestServerRunAgentRoundTrip(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)
	publicKey := testPublicKey(t)
	if got, err := server.GetRunAgent(ctx, publicKey); err != nil || got.Pending != nil || got.Active != nil {
		t.Fatalf("GetRunAgent(empty) = %+v, %v", got, err)
	}
	agent, err := server.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"})
	if err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	if agent.Active != nil || agent.Pending == nil || agent.Pending.WorkspaceName != "demo" {
		t.Fatalf("SetRunAgent() = %+v", agent)
	}
	got, err := server.GetRunAgent(ctx, publicKey)
	if err != nil {
		t.Fatalf("GetRunAgent() error = %v", err)
	}
	if got.Pending == nil || got.Pending.WorkspaceName != "demo" {
		t.Fatalf("GetRunAgent() = %+v", got)
	}
	selection, err := server.ResolveRunAgent(ctx, publicKey)
	if err != nil {
		t.Fatalf("ResolveRunAgent() error = %v", err)
	}
	if selection.WorkspaceName != "demo" {
		t.Fatalf("ResolveRunAgent() = %+v", selection)
	}
	activated, err := server.ActivateRunAgent(ctx, publicKey, selection)
	if err != nil {
		t.Fatalf("ActivateRunAgent() error = %v", err)
	}
	if activated.Pending != nil || activated.Active == nil || activated.Active.WorkspaceName != "demo" {
		t.Fatalf("ActivateRunAgent() = %+v", activated)
	}
	selection, err = server.ResolveRunAgent(ctx, publicKey)
	if err != nil {
		t.Fatalf("ResolveRunAgent(active) error = %v", err)
	}
	if selection.WorkspaceName != "demo" {
		t.Fatalf("ResolveRunAgent(active) = %+v", selection)
	}
	if _, err := server.ActivateRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "other"}); !errors.Is(err, ErrRunAgentChanged) {
		t.Fatalf("ActivateRunAgent(changed) error = %v, want %v", err, ErrRunAgentChanged)
	}
	emptyKey := testPublicKey(t)
	if _, err := server.ResolveRunAgent(ctx, emptyKey); !errors.Is(err, ErrRunAgentNotConfigured) {
		t.Fatalf("ResolveRunAgent(empty) error = %v, want %v", err, ErrRunAgentNotConfigured)
	}
	if _, err := server.ActivateRunAgent(ctx, emptyKey, apitypes.AgentSelection{WorkspaceName: "demo"}); !errors.Is(err, ErrRunAgentNotConfigured) {
		t.Fatalf("ActivateRunAgent(empty) error = %v, want %v", err, ErrRunAgentNotConfigured)
	}
}

func TestValidation(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t)
	publicKey := testPublicKey(t)
	badVolume := 101
	if _, err := server.PutStatus(ctx, publicKey, apitypes.PeerStatus{Volume: &badVolume}); err == nil {
		t.Fatal("PutStatus(bad volume) error = nil")
	}
	badBattery := -1
	if _, err := server.PutStatus(ctx, publicKey, apitypes.PeerStatus{BatteryPercent: &badBattery}); err == nil {
		t.Fatal("PutStatus(bad battery) error = nil")
	}
	if _, err := server.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{}); err == nil {
		t.Fatal("SetRunAgent(empty workspace) error = nil")
	}
	if _, err := server.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: " demo "}); err == nil {
		t.Fatal("SetRunAgent(trimmed workspace) error = nil")
	}
	if err := validateRunAgent(apitypes.PeerRunAgent{
		Active: &apitypes.AgentSelection{WorkspaceName: "active"},
	}); err != nil {
		t.Fatalf("validateRunAgent(active) error = %v", err)
	}
	if err := validateRunAgent(apitypes.PeerRunAgent{
		Active: &apitypes.AgentSelection{},
	}); err == nil {
		t.Fatal("validateRunAgent(invalid active) error = nil")
	}
	if _, err := server.GetStatus(ctx, giznet.PublicKey{}); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("GetStatus(empty public key) error = %v, want %v", err, ErrInvalidPublicKey)
	}
	if _, err := (*Server)(nil).GetStatus(ctx, publicKey); !errors.Is(err, ErrNilServer) {
		t.Fatalf("GetStatus(nil server) error = %v, want %v", err, ErrNilServer)
	}
	if _, err := (&Server{}).GetStatus(ctx, publicKey); !errors.Is(err, ErrNilStore) {
		t.Fatalf("GetStatus(nil store) error = %v, want %v", err, ErrNilStore)
	}
}

func TestCorruptStoreData(t *testing.T) {
	server := newTestServer(t)
	publicKey := testPublicKey(t)
	if _, err := server.DB.ExecContext(t.Context(), `INSERT INTO peer_runs(public_key,status_json) VALUES (?,?)`, publicKey.String(), "{"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.GetStatus(t.Context(), publicKey); err == nil {
		t.Fatal("corrupt status accepted")
	}
	if _, err := server.DB.ExecContext(t.Context(), `UPDATE peer_runs SET pending_workspace='' WHERE public_key=?`, publicKey.String()); err == nil {
		t.Fatal("empty selection accepted by schema")
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	server := &Server{DB: db}
	if err := server.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return server
}

func testPublicKey(t *testing.T) giznet.PublicKey {
	t.Helper()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	return keyPair.Public
}

func TestStaleActivationPreservesNewSelection(t *testing.T) {
	s := newTestServer(t)
	peer := testPublicKey(t)
	first := apitypes.AgentSelection{WorkspaceName: "first"}
	second := apitypes.AgentSelection{WorkspaceName: "second"}
	if _, err := s.SetRunAgent(t.Context(), peer, first); err != nil {
		t.Fatal(err)
	}
	selected, err := s.ResolveRunAgent(t.Context(), peer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRunAgent(t.Context(), peer, second); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateRunAgent(t.Context(), peer, selected); !errors.Is(err, ErrRunAgentChanged) {
		t.Fatalf("stale activation: %v", err)
	}
	got, err := s.GetRunAgent(t.Context(), peer)
	if err != nil || got.Pending == nil || got.Pending.WorkspaceName != "second" || got.Active != nil {
		t.Fatalf("new selection lost: %+v, %v", got, err)
	}
	if _, err := s.ActivateRunAgent(t.Context(), peer, second); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRunAgent(t.Context(), peer, first); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetRunAgent(t.Context(), peer)
	if err != nil || got.Pending == nil || got.Active == nil || got.Active.WorkspaceName != "second" {
		t.Fatalf("pending write lost active: %+v, %v", got, err)
	}
}

func TestRequestsDoNotInitializeRuntimeSchema(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	s := &Server{DB: db}
	if _, err := s.GetStatus(t.Context(), testPublicKey(t)); err == nil {
		t.Fatal("read unexpectedly initialized schema")
	}
	var tables int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_master WHERE name='peer_runs'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("request created runtime table")
	}
}
