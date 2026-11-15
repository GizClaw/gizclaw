package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestPeerDeletionImmediatelyPublishesTombstoneAndRetainsOriginalBytes(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{71}
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	saveTestPeer(t, server, key, apitypes.DeviceInfo{})
	original, err := store.Get(ctx, peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, original, "", "  "); err != nil {
		t.Fatal(err)
	}
	original = formatted.Bytes()
	if err := store.Set(ctx, peerKey(key.String()), original); err != nil {
		t.Fatal(err)
	}
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	current, err := store.Get(ctx, peerKey(key.String()))
	if err != nil || !isPeerTombstone(current) {
		t.Fatalf("logical deletion = %s, %v", current, err)
	}
	if _, err := server.LoadPeer(ctx, key); !errors.Is(err, ErrPeerDeleted) {
		t.Fatalf("deleted Peer remains visible: %v", err)
	}
	record, err := pendingdeletion.GetByLocator(ctx, store, pendingdeletion.KindPeer, key.String())
	if err != nil {
		t.Fatal(err)
	}
	var descriptor peerDeletionDescriptor
	if err := json.Unmarshal(record.Descriptor, &descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor.PublicKey != key.String() || !bytes.Equal(descriptor.PeerRecord, original) {
		t.Fatalf("original Peer bytes were not retained: %+v", descriptor)
	}
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatalf("lost-response retry failed: %v", err)
	}
	retried, err := pendingdeletion.GetByLocator(ctx, store, pendingdeletion.KindPeer, key.String())
	if err != nil || retried.DeletionID != record.DeletionID || !retried.DeletedAt.Equal(record.DeletedAt) {
		t.Fatalf("retry replaced deletion request: %+v, %v", retried, err)
	}
}

type rescanPeerSocial struct {
	*peerDeletionAdapters
	groups       []friendgroup.PeerRetirementGroup
	prepareCalls int
	retired      int
}

func (a *rescanPeerSocial) PreparePeerSocialRetirement(context.Context, string) (social.PeerSnapshot, error) {
	a.prepareCalls++
	return social.PeerSnapshot{PublicKey: a.publicKey, Groups: append([]friendgroup.PeerRetirementGroup(nil), a.groups...)}, nil
}

func (a *rescanPeerSocial) RetirePeerSocial(_ context.Context, snapshot social.PeerSnapshot) (social.PeerRetirementResult, error) {
	a.retired += len(snapshot.Groups)
	a.groups = nil
	return social.PeerRetirementResult{}, nil
}

func TestPeerDeletionRuns32HourlyPassesAndRebuildsLateInventory(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{72}
	store := kv.NewMemory(nil)
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
	sn := "hourly-deletion"
	saveTestPeer(t, server, key, apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}})
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	record, err := pendingdeletion.GetByLocator(ctx, store, pendingdeletion.KindPeer, key.String())
	if err != nil {
		t.Fatal(err)
	}
	now := record.DeletedAt.Add(time.Second)
	base := &peerDeletionAdapters{publicKey: key.String()}
	socialAdapter := &rescanPeerSocial{peerDeletionAdapters: base}
	for execution := 1; execution <= 32; execution++ {
		// A new source and handler model worker restarts; scheduling and counts
		// must come from persistent state rather than process memory.
		source := PendingDeletionSource(store)
		claim := claimPeerDeletion(t, source, now)
		if claim.ExecutionCount != execution-1 {
			t.Fatalf("execution %d observed count %d", execution, claim.ExecutionCount)
		}
		if execution == 9 {
			socialAdapter.groups = []friendgroup.PeerRetirementGroup{{FriendGroupID: "late-group", WorkspaceID: "late-workspace", OwnerPublicKey: "foreign", PeerPublicKey: key.String()}}
		}
		handler := DeletionHandler{Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: socialAdapter, Workspaces: base, APIKeys: base, RuntimeProfiles: base, Memory: base, Quiescer: base, WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{}, Now: func() time.Time { return now }}
		err := handler.Handle(ctx, claim)
		if execution == 32 {
			if err != nil {
				t.Fatalf("final execution: %v", err)
			}
			if _, err := source.GetTask(ctx, record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
				t.Fatalf("completed request remains: %v", err)
			}
			break
		}
		var outcome *pendingdeletion.OutcomeError
		if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeDeferred || outcome.Code != "peer_cleanup_rescan_pending" || outcome.After != time.Hour {
			t.Fatalf("execution %d did not defer one hour: %v", execution, err)
		}
		if base.bindingCalls != 0 {
			t.Fatalf("profile binding removed during observation: execution %d", execution)
		}
		if err := source.Defer(ctx, claim, outcome.Code, outcome.Message, now.Add(outcome.After), now); err != nil {
			t.Fatal(err)
		}
		task, err := source.GetTask(ctx, record.DeletionID)
		if err != nil || task.ExecutionCount != execution || !task.NextAttemptAt.Equal(now.Add(time.Hour)) || task.FailureCount != 0 {
			t.Fatalf("execution %d state = %+v, %v", execution, task, err)
		}
		refs, _, err := source.ScanDue(ctx, task.NextAttemptAt.Add(-time.Nanosecond), 10, "")
		if err != nil || len(refs) != 0 {
			t.Fatalf("execution %d scheduled early: %+v, %v", execution, refs, err)
		}
		now = task.NextAttemptAt
		if execution == 1 {
			// Elapsed wall time alone must never consume the remaining passes.
			now = now.Add(72 * time.Hour)
		}
	}
	if socialAdapter.prepareCalls != 32 || socialAdapter.retired != 1 || base.sessionCalls != 32 || base.bindingCalls != 1 {
		t.Fatalf("passes=%d late-members=%d credential-cleanups=%d bindings=%d", socialAdapter.prepareCalls, socialAdapter.retired, base.sessionCalls, base.bindingCalls)
	}
	if found, err := store.HasMember(ctx, snPrefix(sn), key.String()); err != nil || found {
		t.Fatalf("identifier index remains after final pass: %v, %v", found, err)
	}
	current, err := store.Get(ctx, peerKey(key.String()))
	if err != nil || !isPeerTombstone(current) {
		t.Fatalf("permanent tombstone removed: %s, %v", current, err)
	}
}

type retirementSQLWorkflow struct{}

func (retirementSQLWorkflow) GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	var item apitypes.Workflow
	if err := json.Unmarshal([]byte(`{"id":"system-sfu","spec":{"driver":"sfu","sfu":{}}}`), &item); err != nil {
		return nil, err
	}
	return adminhttp.GetWorkflow200JSONResponse(item), nil
}

func TestPeerDeletionRetainsFailedFriendHandoffThroughRefreshAnd32Attempts(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{73}
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "retirement.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := workspace.Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	workspaces := &workspace.Server{DB: db, Workflows: retirementSQLWorkflow{}}
	contacts := &contact.Server{DB: db}
	if err := contacts.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	socialStore := kv.NewMemory(nil)
	friends := &friend.Server{Friends: socialStore, InviteTokens: socialStore, Workspaces: workspaces, SFUURL: "wss://sfu.test"}
	groups := &friendgroup.Server{Groups: socialStore, Members: socialStore, Belongs: socialStore, InviteTokens: socialStore, RelationshipStore: socialStore, Workspaces: workspaces}
	created, err := friends.AdminCreateFriendResource(ctx, socialutil.RelationID(key.String(), giznet.PublicKey{74}.String()), key.String(), giznet.PublicKey{74}.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER reject_workspace_handoff BEFORE INSERT ON workspace_pending_deletions BEGIN SELECT RAISE(FAIL, 'handoff unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	store := kv.NewMemory(nil)
	server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
	saveTestPeer(t, server, key, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(ctx, key); err != nil {
		t.Fatal(err)
	}
	record, err := pendingdeletion.GetByLocator(ctx, store, pendingdeletion.KindPeer, key.String())
	if err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource(store)
	base := &peerDeletionAdapters{publicKey: key.String()}
	children := workspace.NewPendingDeletionSource(db)
	now := record.DeletedAt.Add(time.Second)
	for attempt := 1; attempt <= 32; attempt++ {
		claim := claimPeerDeletion(t, source, now)
		handler := DeletionHandler{Server: server, Source: source, Assignments: testDeletionAssignments{},
			Social: social.PeerRetirement{Contacts: contacts, Friends: friends, FriendGroups: groups}, Workspaces: workspaces,
			APIKeys: base, RuntimeProfiles: base, Memory: base, Quiescer: base, WorkspaceLookup: children, FriendGroupLookup: emptyPeerLookup{}, Now: func() time.Time { return now }}
		err := handler.Handle(ctx, claim)
		requireDeletionOutcome(t, err, "social_cleanup_failed")
		if err := source.Fail(ctx, claim, "social_cleanup_failed", "Workspace handoff unavailable", false, now.Add(time.Hour), now, 32); err != nil {
			t.Fatal(err)
		}
		if err := friends.ReconcileRetirementIntents(ctx); err == nil {
			t.Fatal("injected independent recovery failure was ignored")
		}
		if pending, err := children.HasLocator(ctx, pendingdeletion.Locator{Kind: pendingdeletion.KindWorkspace, ResourceID: created.WorkspaceId}); err != nil || pending {
			t.Fatalf("handoff unexpectedly exists: %v, %v", pending, err)
		}
		data, err := store.Get(ctx, peerRetirementPlanKey(record.DeletionID))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := validateRetirementPlan(data, claim, key)
		if err != nil || len(plan.Social.Friends) != 1 || plan.Social.Friends[0].WorkspaceID != created.WorkspaceId {
			t.Fatalf("attempt %d lost exact unresolved Friend: %+v, %v", attempt, plan.Social, err)
		}
		if base.bindingCalls != 0 {
			t.Fatalf("attempt %d removed profile before handoff", attempt)
		}
		now = now.Add(time.Hour)
	}
	task, err := source.GetTask(ctx, record.DeletionID)
	if err != nil || task.ExecutionCount != 32 || task.FailureCount != 32 || task.Status != pendingdeletion.StatusFailed {
		t.Fatalf("unfinished request after 32 actual attempts = %+v, %v", task, err)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER reject_workspace_handoff"); err != nil {
		t.Fatal(err)
	}
	if err := friends.ReconcileRetirementIntents(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Retry(ctx, record.DeletionID, now); err != nil {
		t.Fatal(err)
	}
	claim := claimPeerDeletion(t, source, now)
	handler := DeletionHandler{Server: server, Source: source, Assignments: testDeletionAssignments{},
		Social: social.PeerRetirement{Contacts: contacts, Friends: friends, FriendGroups: groups}, Workspaces: workspaces,
		APIKeys: base, RuntimeProfiles: base, Memory: base, Quiescer: base, WorkspaceLookup: children, FriendGroupLookup: emptyPeerLookup{}, Now: func() time.Time { return now }}
	err = handler.Handle(ctx, claim)
	requireDeletionOutcome(t, err, "workspace_cleanup_pending")
	if base.bindingCalls != 0 {
		t.Fatal("profile removed before child physically completed")
	}
	if err := source.Defer(ctx, claim, "workspace_cleanup_pending", "Workspace pending", now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	refs, _, err := children.ScanDue(ctx, now, 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("Workspace child = %+v, %v", refs, err)
	}
	childClaim, claimed, err := children.Claim(ctx, refs[0], now, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim Workspace = %v, %v", claimed, err)
	}
	if err := (workspace.DeletionHandler{Server: workspaces, Source: children, Now: func() time.Time { return now }}).Handle(ctx, childClaim); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	claim = claimPeerDeletion(t, source, now)
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatalf("cleanup after proven handoff and child completion: %v", err)
	}
	if _, err := source.GetTask(ctx, record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) || base.bindingCalls != 1 {
		t.Fatalf("completed request=%v binding calls=%d", err, base.bindingCalls)
	}
}

type progressPeerSocial struct {
	*peerDeletionAdapters
	groups []friendgroup.PeerRetirementGroup
	calls  map[string]int
	fail   bool
}

func (a *progressPeerSocial) PreparePeerSocialRetirement(context.Context, string) (social.PeerSnapshot, error) {
	return social.PeerSnapshot{PublicKey: a.publicKey, Groups: append([]friendgroup.PeerRetirementGroup(nil), a.groups...)}, nil
}

func (a *progressPeerSocial) RetirePeerSocial(_ context.Context, snapshot social.PeerSnapshot) (social.PeerRetirementResult, error) {
	for _, item := range snapshot.Groups {
		a.calls[item.FriendGroupID]++
		if item.FriendGroupID == "completed" && a.calls[item.FriendGroupID] > 1 {
			return social.PeerRetirementResult{}, errors.New("old receipt replaced")
		}
		if item.FriendGroupID == "unfinished" && a.fail {
			return social.PeerRetirementResult{}, errors.New("handoff unavailable")
		}
	}
	return social.PeerRetirementResult{}, nil
}

func TestPeerDeletionCheckpointsCompletedLifecyclesBeforeLaterFailure(t *testing.T) {
	handler, claim, base, key := newDeletionFixture(t)
	ctx, now := t.Context(), claim.UpdatedAt.Add(time.Second)
	created := time.Now().UTC()
	group := func(id string) friendgroup.PeerRetirementGroup {
		return friendgroup.PeerRetirementGroup{FriendGroupID: id, WorkspaceID: "workspace-" + id, WorkspaceName: "name-" + id,
			OwnerPublicKey: "foreign", PeerPublicKey: key.String(), FriendGroupName: "local-" + id, Role: "member", CreatedAt: &created}
	}
	adapter := &progressPeerSocial{peerDeletionAdapters: base, groups: []friendgroup.PeerRetirementGroup{group("completed"), group("unfinished")}, calls: make(map[string]int), fail: true}
	handler.Social, handler.Now = adapter, func() time.Time { return now }
	err := handler.Handle(ctx, claim)
	requireDeletionOutcome(t, err, "social_cleanup_failed")
	if err := handler.Source.Fail(ctx, claim, "social_cleanup_failed", "handoff unavailable", false, now.Add(time.Hour), now, 32); err != nil {
		t.Fatal(err)
	}
	// Neither old lifecycle is in the fresh inventory. A newly visible member
	// must still be retired, while only the unfinished old obligation is replayed.
	adapter.groups, adapter.fail = []friendgroup.PeerRetirementGroup{group("late")}, false
	now = now.Add(time.Hour)
	claim = claimPeerDeletion(t, handler.Source, now)
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if adapter.calls["completed"] != 1 || adapter.calls["unfinished"] != 2 || adapter.calls["late"] != 1 {
		t.Fatalf("exact lifecycle retries = %+v", adapter.calls)
	}
}

func TestPeerDeletionRefreshesKnownChildAbsenceBeforeFinalizing(t *testing.T) {
	handler, claim, base, key := newDeletionFixture(t)
	ctx := t.Context()
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	var ordinary, primary atomic.Int32
	view := kvfixture.ReadView(authority, replica, &ordinary, &primary)
	record, err := pendingdeletion.New(pendingdeletion.KindFriendGroup, "known-group", nil, pendingdeletion.ReasonResourceDelete, struct{}{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(ctx, authority, record); err != nil {
		t.Fatal(err)
	}
	if _, created, err := pendingdeletion.CreateOrGet(ctx, view, record); err != nil || created {
		t.Fatalf("acknowledged existing child = %v, %v", created, err)
	}
	handler.Social = &preparedPeerSocial{peerDeletionAdapters: base, prepared: social.PeerSnapshot{PublicKey: key.String(), Groups: []friendgroup.PeerRetirementGroup{{FriendGroupID: "known-group", WorkspaceID: "known-workspace", OwnerPublicKey: key.String(), PeerPublicKey: key.String()}}}}
	handler.FriendGroupLookup = pendingdeletion.KVSource{Store: view, SourceName: "friend-group", OwnedKinds: []pendingdeletion.Kind{pendingdeletion.KindFriendGroup}}
	ordinary.Store(0)
	primary.Store(0)
	err = handler.Handle(ctx, claim)
	requireDeletionOutcome(t, err, "friend_group_cleanup_pending")
	if base.bindingCalls != 0 || ordinary.Load() == 0 || primary.Load() == 0 {
		t.Fatalf("known child did not retain profile: bindings=%d reads=%d/%d", base.bindingCalls, ordinary.Load(), primary.Load())
	}
	if _, err := handler.Source.GetTask(ctx, claim.Record.DeletionID); err != nil {
		t.Fatalf("parent was finalized: %v", err)
	}
}

func TestPeerDeletionKeepsRequestWhen32ndPassCannotFinish(t *testing.T) {
	handler, claim, base, _ := newDeletionFixture(t)
	ctx := t.Context()
	now := claim.UpdatedAt.Add(time.Second)
	handler.Now = func() time.Time { return now }
	absent := false
	handler.Memory = &peerDeletionAdapters{publicKey: base.publicKey, memoryAbsent: &absent}
	err := handler.Handle(ctx, claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Code != "memory_cleanup_pending" || outcome.After != time.Hour {
		t.Fatalf("unfinished last pass = %v", err)
	}
	if err := handler.Source.Defer(ctx, claim, outcome.Code, outcome.Message, now.Add(outcome.After), now); err != nil {
		t.Fatal(err)
	}
	task, err := handler.Source.GetTask(ctx, claim.Record.DeletionID)
	if err != nil || task.ExecutionCount != 32 || base.bindingCalls != 0 {
		t.Fatalf("unfinished request = %+v binding calls=%d error=%v", task, base.bindingCalls, err)
	}
	absent = true
	now = task.NextAttemptAt
	claim = claimPeerDeletion(t, handler.Source, now)
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatalf("successful later pass: %v", err)
	}
	if _, err := handler.Source.GetTask(ctx, claim.Record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("request did not finish after recovery: %v", err)
	}
}
