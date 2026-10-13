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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPeerDeletionFinalizesExactPermanentTombstone(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
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
		Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: adapters, Workspaces: adapters,
		APIKeys: adapters, RuntimeProfiles: adapters, Memory: adapters, Quiescer: adapters,
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
	memoryAbsent *bool
}

func (*peerDeletionAdapters) PurgePeerMemory(context.Context, string) error { return nil }

func (a *peerDeletionAdapters) PeerMemoryAbsent(context.Context, string) (bool, error) {
	if a.memoryAbsent != nil {
		return *a.memoryAbsent, nil
	}
	return true, nil
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
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
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
		Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: adapters, Workspaces: adapters,
		APIKeys: adapters, RuntimeProfiles: adapters, Memory: adapters, Quiescer: adapters,
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
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
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
		Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: base, Workspaces: adapters,
		APIKeys: base, RuntimeProfiles: base, Memory: base, Quiescer: base,
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

func TestPeerDeletionKeepsBindingWhileSharedMemoryRemains(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
	key := giznet.PublicKey{23}
	saveTestPeer(t, server, key, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource(store)
	claim := claimPeerDeletion(t, source, time.Now().Add(time.Second))
	remaining := false
	adapters := &peerDeletionAdapters{publicKey: key.String(), memoryAbsent: &remaining}
	handler := DeletionHandler{
		Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: adapters, Workspaces: adapters,
		APIKeys: adapters, RuntimeProfiles: adapters, Memory: adapters, Quiescer: adapters,
		WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{},
		Now: func() time.Time { return claim.UpdatedAt.Add(time.Second) },
	}
	err := handler.Handle(ctx, claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Code != "memory_cleanup_pending" {
		t.Fatalf("Handle() error = %v, want pending shared memory", err)
	}
	if adapters.bindingCalls != 0 {
		t.Fatalf("RuntimeProfile binding removed before shared memory: %d", adapters.bindingCalls)
	}
}

func TestPeerDeletionDoesNotCompareRegistrationFirmwareProjection(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
	key := giznet.PublicKey{24}
	saveTestPeer(t, server, key, apitypes.DeviceInfo{})
	firmware := new("registration-firmware")
	server.RegistrationFirmware = func(context.Context, string) (*string, error) { return firmware, nil }
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource(store)
	claim := claimPeerDeletion(t, source, time.Now().Add(time.Second))
	adapters := &peerDeletionAdapters{publicKey: key.String()}
	handler := DeletionHandler{
		Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: adapters, Workspaces: adapters,
		APIKeys: adapters, RuntimeProfiles: projectionBindingCleanup{remove: func() { firmware = nil }}, Memory: adapters, Quiescer: adapters,
		WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{},
		Now: func() time.Time { return claim.UpdatedAt.Add(time.Second) },
	}
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if err := server.EnsureAvailable(ctx, key); !errors.Is(err, ErrPeerDeleted) {
		t.Fatalf("availability = %v", err)
	}
}

type projectionBindingCleanup struct{ remove func() }

func (c projectionBindingCleanup) DeleteOwnerProfileBinding(context.Context, string) error {
	c.remove()
	return nil
}

type testDeletionAssignments struct{}

func (testDeletionAssignments) Lookup(_ context.Context, key giznet.PublicKey) (apitypes.PeerAssignment, error) {
	return apitypes.PeerAssignment{PeerPublicKey: key.String(), ServerPublicKey: giznet.PublicKey{100}.String(), Role: apitypes.PeerRoleClient, Version: 1, UpdatedAt: time.Now()}, nil
}

func newDeletionFixture(t *testing.T) (DeletionHandler, pendingdeletion.Claim, *peerDeletionAdapters, giznet.PublicKey) {
	t.Helper()
	store := kv.NewMemory(nil)
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
	key := giznet.PublicKey{25}
	saveTestPeer(t, server, key, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource(store)
	claim := claimPeerDeletion(t, source, time.Now().Add(time.Second))
	adapters := &peerDeletionAdapters{publicKey: key.String()}
	return DeletionHandler{Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: adapters, Workspaces: adapters, APIKeys: adapters, RuntimeProfiles: adapters, Memory: adapters, Quiescer: adapters, WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{}, Now: func() time.Time { return claim.UpdatedAt.Add(time.Second) }}, claim, adapters, key
}

func TestPeerDeletionRejectsReplacementBeforeDomainSideEffects(t *testing.T) {
	h, claim, adapters, key := newDeletionFixture(t)
	if _, err := h.loadOrCreatePlan(t.Context(), claim, key); err != nil {
		t.Fatal(err)
	}
	current, err := h.Server.getStored(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	current.UpdatedAt = current.UpdatedAt.Add(time.Second)
	data, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Server.Store.Set(t.Context(), peerKey(key.String()), data); err != nil {
		t.Fatal(err)
	}
	requireDeletionOutcome(t, h.Handle(t.Context(), claim), "peer_replacement_ambiguous")
	if adapters.quiesceCalls+adapters.sessionCalls+adapters.bindingCalls != 0 {
		t.Fatal("replacement triggered domain cleanup")
	}
}

func TestPeerDeletionAdoptsOnlyUnambiguousLegacyFirmwareProjection(t *testing.T) {
	for _, scenario := range []string{"projection_only", "persisted_firmware_changed", "hidden_home_workspace", "record_missing", "legacy_children"} {
		t.Run(scenario, func(t *testing.T) {
			h, claim, adapters, key := newDeletionFixture(t)
			plan, err := h.loadOrCreatePlan(t.Context(), claim, key)
			if err != nil {
				t.Fatal(err)
			}
			plan.Version = 2
			plan.HomeServer, plan.PeerFingerprint = "", ""
			plan.Peer.FirmwareId = new("projected-firmware")
			want := ""
			switch scenario {
			case "persisted_firmware_changed":
				current, err := h.Server.getStored(t.Context(), key)
				if err != nil {
					t.Fatal(err)
				}
				current.FirmwareId = new("different-persisted-firmware")
				data, err := json.Marshal(current)
				if err != nil {
					t.Fatal(err)
				}
				if err := h.Server.Store.Set(t.Context(), peerKey(key.String()), data); err != nil {
					t.Fatal(err)
				}
				want = "peer_replacement_ambiguous"
			case "hidden_home_workspace":
				h.Workspaces = pendingWorkspaceAdapters{adapters}
				want = "retirement_scope_ambiguous"
			case "record_missing":
				if err := h.Server.Store.Delete(t.Context(), peerKey(key.String())); err != nil {
					t.Fatal(err)
				}
				want = "peer_record_missing"
			case "legacy_children":
				plan.WorkspaceIDs = []string{"foreign-child"}
				want = "retirement_scope_ambiguous"
			}
			data, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.Server.Store.Set(t.Context(), peerRetirementPlanKey(claim.Record.DeletionID), data); err != nil {
				t.Fatal(err)
			}
			err = h.Handle(t.Context(), claim)
			if want == "" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := h.Source.GetTask(t.Context(), claim.Record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
					t.Fatalf("task remains: %v", err)
				}
			} else {
				requireDeletionOutcome(t, err, want)
				if adapters.sessionCalls+adapters.bindingCalls+adapters.quiesceCalls != 0 {
					t.Fatal("ambiguous legacy plan triggered domain cleanup")
				}
				if _, err := h.Source.GetTask(t.Context(), claim.Record.DeletionID); err != nil {
					t.Fatalf("unsafe task lost: %v", err)
				}
			}
		})
	}
}

type deletionAssignmentFunc func(context.Context, giznet.PublicKey) (apitypes.PeerAssignment, error)

func (f deletionAssignmentFunc) Lookup(ctx context.Context, key giznet.PublicKey) (apitypes.PeerAssignment, error) {
	return f(ctx, key)
}

func TestPeerDeletionRequiresValidatedHomeBeforeAnyRegionalAccess(t *testing.T) {
	for _, scenario := range []string{"foreign", "unknown", "malformed", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			h, claim, adapters, _ := newDeletionFixture(t)
			h.Assignments = deletionAssignmentFunc(func(ctx context.Context, key giznet.PublicKey) (apitypes.PeerAssignment, error) {
				assignment, _ := (testDeletionAssignments{}).Lookup(ctx, key)
				switch scenario {
				case "foreign":
					assignment.ServerPublicKey = giznet.PublicKey{101}.String()
				case "unknown":
					return apitypes.PeerAssignment{}, peerroute.ErrAssignmentNotFound
				case "malformed":
					assignment.PeerPublicKey = "wrong-peer"
				case "unavailable":
					return apitypes.PeerAssignment{}, errors.New("storage unavailable")
				}
				return assignment, nil
			})
			err := h.Handle(t.Context(), claim)
			if scenario == "foreign" {
				if !errors.Is(err, pendingdeletion.ErrNotLocal) {
					t.Fatalf("foreign error = %v", err)
				}
			} else {
				requireDeletionOutcome(t, err, map[string]string{"unknown": "peer_home_unknown", "malformed": "peer_home_invalid", "unavailable": "peer_home_unavailable"}[scenario])
			}
			if adapters.sessionCalls+adapters.bindingCalls+adapters.quiesceCalls != 0 {
				t.Fatal("unowned task triggered cleanup")
			}
			if _, err := h.Server.Store.Get(t.Context(), peerRetirementPlanKey(claim.Record.DeletionID)); !errors.Is(err, kv.ErrNotFound) {
				t.Fatal("unowned task created a plan")
			}
		})
	}
}

func requireDeletionOutcome(t *testing.T, err error, code string) {
	t.Helper()
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Code != code {
		t.Fatalf("outcome = %v, want %s", err, code)
	}
}

type replaceOnFinalizeStore struct {
	kv.Store
	key         kv.Key
	replacement []byte
}

func (s *replaceOnFinalizeStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	for _, entry := range mutation.Entries {
		if string(entry.Value) == string(encodedPeerTombstone) {
			if err := s.Store.Set(ctx, s.key, s.replacement); err != nil {
				return false, err
			}
		}
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestPeerFinalizationAtomicallyRejectsConcurrentReplacement(t *testing.T) {
	h, claim, _, key := newDeletionFixture(t)
	current, err := h.Server.getStored(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	current.UpdatedAt = current.UpdatedAt.Add(time.Second)
	replacement, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.loadOrCreatePlan(t.Context(), claim, key); err != nil {
		t.Fatal(err)
	}
	store := &replaceOnFinalizeStore{Store: h.Server.Store, key: peerKey(key.String()), replacement: replacement}
	h.Server.Store, h.Source.Store = store, store
	if err := h.Handle(t.Context(), claim); !errors.Is(err, pendingdeletion.ErrConflict) {
		t.Fatalf("finalization = %v", err)
	}
	data, err := store.Get(t.Context(), peerKey(key.String()))
	if err != nil || string(data) != string(replacement) {
		t.Fatalf("replacement overwritten: %s, %v", data, err)
	}
	if _, err := h.Source.GetTask(t.Context(), claim.Record.DeletionID); err != nil {
		t.Fatalf("task consumed: %v", err)
	}
}

func TestPeerRetirementRequiresReplicaProofOnlyForSelectedSocialWorkspaces(t *testing.T) {
	plan := retirementPlan{Peer: apitypes.Peer{PublicKey: "retiring"}, Social: social.PeerSnapshot{Groups: []friendgroup.PeerRetirementGroup{{OwnerPublicKey: "foreign", PeerPublicKey: "retiring"}}}}
	if err := checkSocialRetirementScope(plan); err != nil {
		t.Fatalf("foreign membership only: %v", err)
	}
	plan.Social.Groups[0].OwnerPublicKey = "retiring"
	requireDeletionOutcome(t, checkSocialRetirementScope(plan), "social_replica_scope_unknown")
	plan.Social.Groups = nil
	plan.Social.Friends = []friend.PeerRetirementFriend{{FirstPeer: "retiring", SecondPeer: "foreign"}}
	requireDeletionOutcome(t, checkSocialRetirementScope(plan), "social_replica_scope_unknown")
}
