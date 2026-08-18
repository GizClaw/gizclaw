package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPeerDeletionFinalizesExactPermanentTombstone(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	key := giznet.PublicKey{20}
	sn := "peer-sn"
	if _, err := server.SavePeer(ctx, apitypes.Peer{
		PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive,
		Device: apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}},
	}); err != nil {
		t.Fatal(err)
	}
	foreign := giznet.PublicKey{21}
	saveTestPeer(t, server, foreign, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource(store)
	claim := claimPeerDeletion(t, source, time.Now().Add(time.Second))
	adapters := &peerDeletionAdapters{publicKey: key.String(), gameplayReady: true}
	handler := DeletionHandler{
		Server: server, Source: source, Social: adapters, Workspaces: adapters, Gameplay: adapters,
		APIKeys: adapters, RuntimeProfiles: adapters, Quiescer: adapters,
		WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{},
		Now: func() time.Time { return claim.UpdatedAt.Add(time.Second) },
	}
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatal(err)
	}
	data, err := store.Get(ctx, peerKey(key.String()))
	if err != nil || string(data) != string(encodedPeerTombstone) {
		t.Fatalf("tombstone = %q, %v", data, err)
	}
	if _, err := store.Get(ctx, snKey(sn)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("SN index error = %v", err)
	}
	if _, err := server.LoadPeer(ctx, key); !errors.Is(err, ErrPeerDeleted) {
		t.Fatalf("LoadPeer(tombstone) error = %v", err)
	}
	if err := server.EnsureAvailable(ctx, key); !errors.Is(err, ErrPeerDeleted) {
		t.Fatalf("EnsureAvailable(tombstone) error = %v", err)
	}
	if _, err := server.EnsureConnectedPeer(ctx, key); !errors.Is(err, ErrPeerDeleted) {
		t.Fatalf("EnsureConnectedPeer(tombstone) error = %v", err)
	}
	getResponse, err := server.GetPeer(ctx, adminhttp.GetPeerRequestObject{PublicKey: key.String()})
	if err != nil {
		t.Fatal(err)
	}
	getOK, ok := getResponse.(adminhttp.GetPeer200JSONResponse)
	if !ok {
		t.Fatalf("GetPeer(tombstone) response = %T", getResponse)
	}
	tombstone, err := adminhttp.PeerRegistrationResult(getOK).AsExternalRef0RegistrationTombstone()
	if err != nil || tombstone.PublicKey != key.String() || tombstone.Status != apitypes.RegistrationTombstoneStatusDeleted {
		t.Fatalf("Admin tombstone = %#v, %v", tombstone, err)
	}
	deleteResponse, err := server.DeletePeer(ctx, adminhttp.DeletePeerRequestObject{PublicKey: key.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deleteResponse.(adminhttp.DeletePeer200JSONResponse); !ok {
		t.Fatalf("DeletePeer(tombstone) response = %T", deleteResponse)
	}
	if _, err := server.LoadPeer(ctx, foreign); err != nil {
		t.Fatalf("foreign Peer removed: %v", err)
	}
	if _, err := source.GetTask(ctx, claim.Record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("completed task error = %v", err)
	}
	if adapters.sessionCalls == 0 || adapters.bindingCalls == 0 || adapters.quiesceCalls == 0 {
		t.Fatalf("adapter calls = sessions:%d binding:%d quiesce:%d", adapters.sessionCalls, adapters.bindingCalls, adapters.quiesceCalls)
	}
	entries := 0
	for _, err := range store.List(ctx, peersPrefix()) {
		if err != nil {
			t.Fatal(err)
		}
		entries++
	}
	if entries != 2 {
		t.Fatalf("by-pubkey records = %d, want tombstone plus foreign Peer", entries)
	}
}

func TestPeerDeletionDefersBeforeTombstoneWhileChildCleanupPending(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	key := giznet.PublicKey{22}
	saveTestPeer(t, server, key, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource(store)
	claim := claimPeerDeletion(t, source, time.Now().Add(time.Second))
	adapters := &peerDeletionAdapters{publicKey: key.String(), gameplayReady: false}
	handler := DeletionHandler{
		Server: server, Source: source, Social: adapters, Workspaces: adapters, Gameplay: adapters,
		APIKeys: adapters, RuntimeProfiles: adapters, Quiescer: adapters,
		WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{},
	}
	err := handler.Handle(ctx, claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeDeferred {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, err := server.LoadPeer(ctx, key); err != nil {
		t.Fatalf("Peer finalized while child pending: %v", err)
	}
	if _, err := store.Get(ctx, peerRetirementPlanKey(claim.Record.DeletionID)); err != nil {
		t.Fatalf("retirement plan missing: %v", err)
	}
	if adapters.petWorkspaceCalls != 0 {
		t.Fatalf("Pet Workspace handoff ran before Pet completion: %d", adapters.petWorkspaceCalls)
	}
}

func claimPeerDeletion(t *testing.T, source pendingdeletion.KVSource, now time.Time) pendingdeletion.Claim {
	t.Helper()
	refs, _, err := source.ScanDue(t.Context(), now, 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(t.Context(), refs[0], now, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	return claim
}

type peerDeletionAdapters struct {
	publicKey         string
	gameplayReady     bool
	sessionCalls      int
	bindingCalls      int
	quiesceCalls      int
	petWorkspaceCalls int
}

func (a *peerDeletionAdapters) SnapshotPeerSocial(context.Context, string) (social.PeerSnapshot, error) {
	return social.PeerSnapshot{PublicKey: a.publicKey}, nil
}
func (a *peerDeletionAdapters) RetirePeerSocial(context.Context, social.PeerSnapshot) (social.PeerRetirementResult, error) {
	return social.PeerRetirementResult{}, nil
}
func (a *peerDeletionAdapters) SnapshotPeerWorkspaces(context.Context, string, []string) (workspace.PeerRetirementSnapshot, error) {
	return workspace.PeerRetirementSnapshot{PublicKey: a.publicKey}, nil
}
func (a *peerDeletionAdapters) RetirePeerWorkspaces(context.Context, workspace.PeerRetirementSnapshot) ([]string, error) {
	return nil, nil
}
func (a *peerDeletionAdapters) RetirePeerPetWorkspaces(context.Context, workspace.PeerRetirementSnapshot) ([]string, error) {
	a.petWorkspaceCalls++
	return nil, nil
}
func (a *peerDeletionAdapters) SnapshotPeerGameplay(context.Context, string) (gameplay.PeerGameplaySnapshot, error) {
	return gameplay.PeerGameplaySnapshot{PublicKey: a.publicKey}, nil
}
func (a *peerDeletionAdapters) RetirePeerGameplay(context.Context, gameplay.PeerGameplaySnapshot) (bool, error) {
	return a.gameplayReady, nil
}
func (a *peerDeletionAdapters) CleanupPeer(context.Context, string) error {
	a.sessionCalls++
	return nil
}
func (a *peerDeletionAdapters) DeleteOwnerProfileBinding(context.Context, string) error {
	a.bindingCalls++
	return nil
}
func (a *peerDeletionAdapters) QuiescePeer(context.Context, giznet.PublicKey) error {
	a.quiesceCalls++
	return nil
}

type emptyPeerLookup struct{}

func (emptyPeerLookup) Get(context.Context, string) (pendingdeletion.Record, error) {
	return pendingdeletion.Record{}, kv.ErrNotFound
}
func (emptyPeerLookup) HasLocator(context.Context, pendingdeletion.Locator) (bool, error) {
	return false, nil
}
