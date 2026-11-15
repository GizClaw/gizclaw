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
	claim := claimPeerDeletionFinalPass(t, source, time.Now().Add(time.Second))
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

type peerMarkerReadStore struct {
	kv.Store
	markerReads  int
	locatorReads int
}

func (s *peerMarkerReadStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if len(key) > 1 && key[0] == "pending-deletion" {
		switch key[1] {
		case "by-id":
			s.markerReads++
		case "by-locator":
			s.locatorReads++
		}
	}
	return s.Store.Get(ctx, key)
}

func TestPeerRetirementReusesValidatedRecordButFinalizationRechecksLocator(t *testing.T) {
	handler, claim, _, key := newDeletionFixture(t)
	ctx := t.Context()
	plan, err := handler.loadOrCreatePlan(ctx, claim, key)
	if err != nil {
		t.Fatal(err)
	}
	store := &peerMarkerReadStore{Store: handler.Server.Store}
	handler.Server.Store, handler.Source = store, PendingDeletionSource(store)
	for range 32 {
		if err := handler.CheckTask(ctx, claim.Task); err != nil {
			t.Fatal(err)
		}
		if _, err := handler.planPeer(ctx, claim.Record, plan, key); err != nil {
			t.Fatal(err)
		}
	}
	if store.markerReads != 0 || store.locatorReads != 0 {
		t.Fatalf("validated immutable record was reread: markers=%d locators=%d", store.markerReads, store.locatorReads)
	}
	invalid := claim.Task
	invalid.Record.Descriptor = json.RawMessage(`{"public_key":"wrong"}`)
	if err := handler.CheckTask(ctx, invalid); err == nil {
		t.Fatal("unvalidated replacement descriptor was accepted")
	}
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if store.markerReads == 0 || store.locatorReads == 0 {
		t.Fatalf("finalization skipped fresh authority checks: markers=%d locators=%d", store.markerReads, store.locatorReads)
	}
}

func retainedPeerForTest(ctx context.Context, server *Server, key giznet.PublicKey) (apitypes.Peer, error) {
	record, err := pendingdeletion.GetByLocator(ctx, server.Store, pendingdeletion.KindPeer, key.String())
	if err != nil {
		return apitypes.Peer{}, err
	}
	item, _, err := (DeletionHandler{Server: server}).retainedPeer(ctx, record, key)
	return item, err
}

// claimPeerDeletionFinalPass seeds prior completed executions for tests of
// finalization and safety guards. Scheduling tests run the real handler instead.
func claimPeerDeletionFinalPass(t *testing.T, source pendingdeletion.KVSource, now time.Time) pendingdeletion.Claim {
	t.Helper()
	for range peerDeletionMinExecutions - 1 {
		claim := claimPeerDeletion(t, source, now)
		if err := source.Defer(t.Context(), claim, "prior_execution", "Prior cleanup execution completed", now.Add(time.Hour), now); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Hour)
	}
	return claimPeerDeletion(t, source, now)
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
func (a *peerDeletionAdapters) PreparePeerSocialRetirement(ctx context.Context, owner string) (social.PeerSnapshot, error) {
	return a.SnapshotPeerSocial(ctx, owner)
}
func (a *peerDeletionAdapters) RetirePeerSocial(context.Context, social.PeerSnapshot) (social.PeerRetirementResult, error) {
	return social.PeerRetirementResult{}, nil
}
func (a *peerDeletionAdapters) SnapshotPeerWorkspaces(context.Context, string) (workspace.PeerRetirementSnapshot, error) {
	return workspace.PeerRetirementSnapshot{PublicKey: a.publicKey}, nil
}
func (a *peerDeletionAdapters) PreparePeerWorkspacesRetirement(ctx context.Context, owner string) (workspace.PeerRetirementSnapshot, error) {
	return a.SnapshotPeerWorkspaces(ctx, owner)
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
	claim := claimPeerDeletionFinalPass(t, source, time.Now().Add(time.Second))
	record, err := retainedPeerForTest(ctx, server, key)
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
	if _, err := server.LoadPeer(ctx, key); !errors.Is(err, ErrPeerDeleted) {
		t.Fatalf("logical deletion was lost while retaining the unsafe task: %v", err)
	}
	if _, err := source.GetTask(ctx, claim.Record.DeletionID); err != nil {
		t.Fatalf("unsafe task was removed: %v", err)
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

func (a pendingWorkspaceAdapters) PreparePeerWorkspacesRetirement(ctx context.Context, owner string) (workspace.PeerRetirementSnapshot, error) {
	return a.SnapshotPeerWorkspaces(ctx, owner)
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
	claim := claimPeerDeletionFinalPass(t, source, time.Now().Add(time.Second))
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
	claim := claimPeerDeletionFinalPass(t, source, time.Now().Add(time.Second))
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
	claim := claimPeerDeletionFinalPass(t, source, time.Now().Add(time.Second))
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
	claim := claimPeerDeletionFinalPass(t, source, time.Now().Add(time.Second))
	adapters := &peerDeletionAdapters{publicKey: key.String()}
	return DeletionHandler{Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: adapters, Workspaces: adapters, APIKeys: adapters, RuntimeProfiles: adapters, Memory: adapters, Quiescer: adapters, WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{}, Now: func() time.Time { return claim.UpdatedAt.Add(time.Second) }}, claim, adapters, key
}

func TestPeerDeletionRejectsReplacementBeforeDomainSideEffects(t *testing.T) {
	h, claim, adapters, key := newDeletionFixture(t)
	if _, err := h.loadOrCreatePlan(t.Context(), claim, key); err != nil {
		t.Fatal(err)
	}
	current, err := retainedPeerForTest(t.Context(), h.Server, key)
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
				current, err := retainedPeerForTest(t.Context(), h.Server, key)
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
	finalizing := false
	for _, key := range mutation.DeleteKeys {
		if len(key) > 1 && key[0] == "pending-deletion" && key[1] == "task" {
			finalizing = true
			break
		}
	}
	if !finalizing {
		return s.Store.ApplyMutation(ctx, mutation)
	}
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
	current, err := retainedPeerForTest(t.Context(), h.Server, key)
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

func TestPeerRetirementProcessesSocialResourcesWithoutGlobalReplicaLedger(t *testing.T) {
	for _, ownedGroup := range []bool{false, true} {
		t.Run(fmt.Sprint(ownedGroup), func(t *testing.T) {
			h, claim, base, key := newDeletionFixture(t)
			prepared := social.PeerSnapshot{PublicKey: key.String()}
			if ownedGroup {
				prepared.Groups = []friendgroup.PeerRetirementGroup{{FriendGroupID: "owned-group", WorkspaceID: "owned-workspace", OwnerPublicKey: key.String(), PeerPublicKey: key.String()}}
			} else {
				prepared.Friends = []friend.PeerRetirementFriend{{RelationID: "shared-relation", FirstPeer: key.String(), SecondPeer: "foreign", WorkspaceID: "shared-workspace"}}
			}
			h.Social = &preparedPeerSocial{peerDeletionAdapters: base, prepared: prepared}
			if err := h.Handle(t.Context(), claim); err != nil {
				t.Fatalf("observed Social resource was not processed: %v", err)
			}
			if _, err := h.Source.GetTask(t.Context(), claim.Record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
				t.Fatalf("completed local cleanup did not finish: %v", err)
			}
		})
	}
}

func TestAutomaticPeerRecoveryRequiresExactLegacyProjectionAndHomeProof(t *testing.T) {
	for _, scenario := range []string{"eligible", "new_plan", "missing_projection", "different_error", "wrong_phase", "replacement", "persisted_firmware", "unknown_home", "foreign_home", "hidden_home_workspace", "legacy_children"} {
		t.Run(scenario, func(t *testing.T) {
			h, claim, adapters, key := newDeletionFixture(t)
			plan, err := h.loadOrCreatePlan(t.Context(), claim, key)
			if err != nil {
				t.Fatal(err)
			}
			claim, err = h.Source.Checkpoint(t.Context(), claim, pendingdeletion.PhaseFinalize, claim.UpdatedAt.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			plan.Version = 2
			plan.HomeServer = ""
			plan.PeerFingerprint = ""
			plan.Peer.FirmwareId = new("registration-firmware")
			code := "peer_replacement_ambiguous"
			switch scenario {
			case "new_plan":
				plan.Version = retirementPlanVersion
				plan.HomeServer = h.Server.ServerPublicKey.String()
			case "missing_projection":
				plan.Peer.FirmwareId = nil
			case "different_error":
				code = "social_cleanup_failed"
			case "wrong_phase":
				claim, err = h.Source.Checkpoint(t.Context(), claim, pendingdeletion.PhaseValidate, claim.UpdatedAt.Add(time.Second))
				if err != nil {
					t.Fatal(err)
				}
			case "replacement", "persisted_firmware":
				current, err := retainedPeerForTest(t.Context(), h.Server, key)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "replacement" {
					current.UpdatedAt = current.UpdatedAt.Add(time.Second)
				} else {
					current.FirmwareId = new("different-persisted-firmware")
				}
				data, err := json.Marshal(current)
				if err != nil {
					t.Fatal(err)
				}
				if err := h.Server.Store.Set(t.Context(), peerKey(key.String()), data); err != nil {
					t.Fatal(err)
				}
			case "unknown_home":
				h.Assignments = deletionAssignmentFunc(func(context.Context, giznet.PublicKey) (apitypes.PeerAssignment, error) {
					return apitypes.PeerAssignment{}, peerroute.ErrAssignmentNotFound
				})
			case "foreign_home":
				h.Server.ServerPublicKey = giznet.PublicKey{101}
			case "hidden_home_workspace":
				h.Workspaces = pendingWorkspaceAdapters{adapters}
			case "legacy_children":
				plan.WorkspaceIDs = []string{"unproven-child"}
			}
			data, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.Server.Store.Set(t.Context(), peerRetirementPlanKey(claim.Record.DeletionID), data); err != nil {
				t.Fatal(err)
			}
			if err := h.Source.Fail(t.Context(), claim, code, "retained diagnostic", true, claim.UpdatedAt.Add(time.Second), claim.UpdatedAt.Add(time.Second), 3); err != nil {
				t.Fatal(err)
			}
			task, err := h.Source.GetTask(t.Context(), claim.Record.DeletionID)
			if err != nil {
				t.Fatal(err)
			}
			rawBefore, err := h.Server.Store.Get(t.Context(), kv.Key{"pending-deletion", "task", claim.Record.DeletionID})
			if err != nil {
				t.Fatal(err)
			}
			eligible, probeErr := h.CanRecoverFailed(t.Context(), task)
			if scenario == "eligible" {
				if !eligible || probeErr != nil {
					t.Fatalf("eligible=%v error=%v", eligible, probeErr)
				}
			} else if eligible {
				t.Fatal("unsafe task eligible for automatic recovery")
			}
			rawAfter, err := h.Server.Store.Get(t.Context(), kv.Key{"pending-deletion", "task", claim.Record.DeletionID})
			if err != nil {
				t.Fatal(err)
			}
			if string(rawAfter) != string(rawBefore) || adapters.sessionCalls+adapters.bindingCalls+adapters.quiesceCalls != 0 {
				t.Fatal("eligibility probe mutated task or domain data")
			}
		})
	}
}

func TestPeerRetirementPlanRetriesStaleGenerationAtPrimary(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{99}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	writer := &Server{Store: authority, ServerPublicKey: giznet.PublicKey{100}}
	saveTestPeer(t, writer, key, apitypes.DeviceInfo{})
	old, err := authority.Get(ctx, peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, peerKey(key.String()), old); err != nil {
		t.Fatal(err)
	}
	name := "newer-generation"
	current, err := writer.putInfo(ctx, key, apitypes.DeviceInfo{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	claim := claimPeerDeletionFinalPass(t, PendingDeletionSource(authority), time.Now().Add(time.Second))
	store := newStalePeerStore(t, authority, replica)
	adapters := &peerDeletionAdapters{publicKey: key.String()}
	handler := DeletionHandler{Server: &Server{Store: store, ServerPublicKey: writer.ServerPublicKey}, Social: adapters, Workspaces: adapters}
	plan, err := handler.loadOrCreatePlan(ctx, claim, key)
	if err != nil || plan.Peer.Device.Name == nil || *plan.Peer.Device.Name != name || !plan.Peer.UpdatedAt.Equal(current.UpdatedAt) {
		t.Fatalf("stale plan accepted: %+v %v", plan, err)
	}
	if store.attempts.Load() != 1 || store.rejected.Load() != 0 || store.ordinary.Load() == 0 || store.primary.Load() == 0 {
		t.Fatalf("plan fallback attempts=%d rejected=%d reads=%d/%d", store.attempts.Load(), store.rejected.Load(), store.ordinary.Load(), store.primary.Load())
	}
	store.ordinary.Store(0)
	store.primary.Store(0)
	raw, err := handler.planPeer(ctx, claim.Record, plan, key)
	if err != nil || peerFingerprint(raw) != plan.PeerFingerprint || store.ordinary.Load() == 0 || store.primary.Load() == 0 {
		t.Fatalf("known-plan fallback err=%v reads=%d/%d", err, store.ordinary.Load(), store.primary.Load())
	}
	if adapters.sessionCalls != 0 || adapters.bindingCalls != 0 || adapters.quiesceCalls != 0 {
		t.Fatal("plan retry performed cleanup side effects")
	}
	data, err := authority.Get(ctx, peerRetirementPlanKey(claim.Record.DeletionID))
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, peerRetirementPlanKey(claim.Record.DeletionID), data); err != nil {
		t.Fatal(err)
	}
	store.ordinary.Store(0)
	store.primary.Store(0)
	if _, err := handler.tryLoadOrCreatePlan(ctx, claim, key); err != nil {
		t.Fatal(err)
	}
	if store.ordinary.Load() == 0 || store.primary.Load() == 0 {
		t.Fatalf("existing plan skipped configured reads or mismatch recovery: reads=%d/%d", store.ordinary.Load(), store.primary.Load())
	}
}
