package peer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
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
	adapters := &peerDeletionAdapters{publicKey: key.String()}
	handler := DeletionHandler{
		Server: server, Source: source, Social: adapters, Workspaces: adapters,
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
	if found, err := store.HasMember(ctx, snPrefix(sn), key.String()); err != nil || found {
		t.Fatalf("SN index membership = %v, %v", found, err)
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
	publicKey    string
	sessionCalls int
	bindingCalls int
	quiesceCalls int
}

func (a *peerDeletionAdapters) SnapshotPeerSocial(context.Context, string) (social.PeerSnapshot, error) {
	return social.PeerSnapshot{PublicKey: a.publicKey}, nil
}
func (a *peerDeletionAdapters) RetirePeerSocial(context.Context, social.PeerSnapshot) (social.PeerRetirementResult, error) {
	return social.PeerRetirementResult{}, nil
}
func (a *peerDeletionAdapters) SnapshotPeerWorkspaces(context.Context, string) (workspace.PeerRetirementSnapshot, error) {
	return workspace.PeerRetirementSnapshot{PublicKey: a.publicKey}, nil
}
func (a *peerDeletionAdapters) RetirePeerWorkspaces(context.Context, workspace.PeerRetirementSnapshot) ([]string, error) {
	return nil, nil
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

// TestPeerDeletionRejectsLegacyRetirementPlan pins the upgrade boundary: a plan
// persisted by an earlier release recorded Pet system Workspaces in
// WorkspaceIDs that a retirement pass this handler no longer has was expected
// to remove. Accepting it would observe no pending marker for those Workspaces,
// treat them as complete, and tombstone the Peer while they remain.
func TestPeerDeletionRejectsLegacyRetirementPlan(t *testing.T) {
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
	record, err := server.LoadPeer(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	encodedPeer, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	legacy := fmt.Appendf(nil, `{
		"version": 1,
		"marker_fingerprint": %q,
		"peer": %s,
		"social": {"public_key": %q},
		"workspaces": {
			"public_key": %q,
			"workspaces": [],
			"pet_workspaces": [{"id": "workspace-pet", "name": "pet-1", "has_icon": false}]
		},
		"workspace_ids": ["workspace-pet"],
		"friend_group_ids": []
	}`, claim.MarkerFingerprint, encodedPeer, key.String(), key.String())
	if err := store.Set(ctx, peerRetirementPlanKey(claim.Record.DeletionID), legacy); err != nil {
		t.Fatal(err)
	}
	adapters := &peerDeletionAdapters{publicKey: key.String()}
	handler := DeletionHandler{
		Server: server, Source: source, Social: adapters, Workspaces: adapters,
		APIKeys: adapters, RuntimeProfiles: adapters, Quiescer: adapters,
		WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{},
		Now: func() time.Time { return claim.UpdatedAt.Add(time.Second) },
	}
	var outcome *pendingdeletion.OutcomeError
	err = handler.Handle(ctx, claim)
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeTerminal || outcome.Code != "retirement_plan_unsupported" {
		t.Fatalf("Handle(legacy plan) error = %v", err)
	}
	if _, err := server.LoadPeer(ctx, key); err != nil {
		t.Fatalf("Peer tombstoned despite the unretired Workspace: %v", err)
	}
}

type pendingWorkspaceAdapters struct {
	*peerDeletionAdapters
}

func (a pendingWorkspaceAdapters) SnapshotPeerWorkspaces(context.Context, string) (workspace.PeerRetirementSnapshot, error) {
	return workspace.PeerRetirementSnapshot{
		PublicKey:  a.publicKey,
		Workspaces: []workspace.PeerRetirementWorkspace{{ID: "workspace-a", Name: "room-a"}},
	}, nil
}

type pendingWorkspaceLookup struct{ emptyPeerLookup }

func (pendingWorkspaceLookup) HasLocator(_ context.Context, locator pendingdeletion.Locator) (bool, error) {
	return locator.Kind == pendingdeletion.KindWorkspace && locator.ResourceID == "workspace-a", nil
}

// Workspace cleanup resolves the Memory binding it purges through the owner's
// RuntimeProfile, so the Peer keeps that binding until no child Workspace
// deletion is pending.
func TestPeerDeletionKeepsRuntimeProfileBindingWhileWorkspaceCleanupIsPending(t *testing.T) {
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
	base := &peerDeletionAdapters{publicKey: key.String()}
	adapters := pendingWorkspaceAdapters{peerDeletionAdapters: base}
	handler := DeletionHandler{
		Server: server, Source: source, Social: base, Workspaces: adapters,
		APIKeys: base, RuntimeProfiles: base, Quiescer: base,
		WorkspaceLookup: pendingWorkspaceLookup{}, FriendGroupLookup: emptyPeerLookup{},
		Now: func() time.Time { return claim.UpdatedAt.Add(time.Second) },
	}
	err := handler.Handle(ctx, claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeDeferred || outcome.Code != "workspace_cleanup_pending" {
		t.Fatalf("Handle() error = %#v, want deferred workspace_cleanup_pending", err)
	}
	if base.bindingCalls != 0 {
		t.Fatalf("RuntimeProfile binding deleted %d times while Workspace cleanup was pending", base.bindingCalls)
	}
}
