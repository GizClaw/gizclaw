package peerrun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: kv.NewMemory(nil)}
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
	server := &Server{Store: kv.NewMemory(nil)}
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
	server := &Server{Store: kv.NewMemory(nil)}
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
	ctx := context.Background()
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	publicKey := testPublicKey(t)
	statusKey, err := statusKey(publicKey)
	if err != nil {
		t.Fatalf("statusKey() error = %v", err)
	}
	if err := store.Set(ctx, statusKey, []byte("{")); err != nil {
		t.Fatalf("Set(status) error = %v", err)
	}
	if _, err := server.GetStatus(ctx, publicKey); err == nil {
		t.Fatal("GetStatus(corrupt) error = nil")
	}
	runAgentKey, err := runAgentKey(publicKey)
	if err != nil {
		t.Fatalf("runAgentKey() error = %v", err)
	}
	if err := store.Set(ctx, runAgentKey, []byte("{")); err != nil {
		t.Fatalf("Set(run-agent) error = %v", err)
	}
	if _, err := server.GetRunAgent(ctx, publicKey); err == nil {
		t.Fatal("GetRunAgent(corrupt) error = nil")
	}
}

func testPublicKey(t *testing.T) giznet.PublicKey {
	t.Helper()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	return keyPair.Public
}
