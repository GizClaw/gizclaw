package friend

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPrepareFriendRetirementDoesNotSealLaggedEmptyInventory(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil)}
	s.Friends = store
	created, err := s.AdminCreateFriend(kv.WithPrimaryReads(ctx), "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	primary, mutations := store.primary, store.mutations
	if snapshot, err := s.SnapshotPeerFriends(ctx, "owner"); err != nil || len(snapshot) != 0 {
		t.Fatalf("ordinary snapshot = %+v, %v", snapshot, err)
	}
	if store.primary != primary || store.mutations != mutations {
		t.Fatal("ordinary snapshot changed read policy or wrote a fence")
	}
	snapshot, err := s.PreparePeerFriendsRetirement(ctx, "owner")
	if err != nil || len(snapshot) != 1 || snapshot[0].WorkspaceName != socialutil.StringValue(created.WorkspaceName) {
		t.Fatalf("prepared snapshot = %+v, %v", snapshot, err)
	}
	if _, err := s.CreateFriendInviteToken(ctx, "owner", rpcapi.FriendInviteTokenCreateRequest{}); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("invitation after retirement fence = %v", err)
	}
	for _, pair := range [][2]string{{"owner", "new-first"}, {"new-second", "owner"}} {
		if _, err := s.AdminCreateFriend(ctx, pair[0], pair[1]); !errors.Is(err, ownership.ErrOwnerRetired) {
			t.Fatalf("admission for %v after fence = %v", pair, err)
		}
	}
}

func TestFriendInFlightCreationCannotPublishAfterRetirementFence(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	intent, err := s.getOrCreateCreationIntent(ctx, s.Friends, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := s.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot, err := s.PreparePeerFriendsRetirement(ctx, "peer"); err != nil || len(snapshot) != 0 {
		t.Fatalf("prepare pending relation = %+v, %v", snapshot, err)
	}
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("late commit = %v", err)
	}
	if _, err := s.GetFriendRelation(ctx, "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("late relationship published = %v", err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	if !slices.Equal(workspaces.deleted, []string{intent.Workspace}) {
		t.Fatalf("cancelled Workspace cleanup = %v", workspaces.deleted)
	}
}

func TestFriendInviteRenewalRecomputesAfterCASRejection(t *testing.T) {
	for _, change := range []string{"rotated", "extended"} {
		t.Run(change, func(t *testing.T) {
			ctx := t.Context()
			s := newTestServer()
			store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
			s.InviteTokens = store
			old := inviteTokenRecord{PeerPublicKey: "owner", InviteToken: "old", CreatedAt: s.now(), ExpiresAt: s.now().Add(time.Hour)}
			if err := socialutil.WriteInviteToken(ctx, store, socialutil.FriendInviteTokenKey("owner"), old); err != nil {
				t.Fatal(err)
			}
			store.replicate = false
			current := old
			current.ExpiresAt = s.now().Add(6 * time.Hour)
			if change == "rotated" {
				current.InviteToken = "rotated"
			}
			store.beforeWrite = func(ctx context.Context, mutation kv.Mutation) {
				store.beforeWrite = nil
				if err := socialutil.WriteInviteToken(ctx, store.Store, socialutil.FriendInviteTokenKey("owner"), current); err != nil {
					t.Fatal(err)
				}
			}
			result, err := s.CreateFriendInviteTokenWithTTL(ctx, "owner", 2*time.Hour)
			if err != nil || result.InviteToken != current.InviteToken || !result.ExpiresAt.Equal(current.ExpiresAt) {
				t.Fatalf("renewal = %+v, %v; want token %q expiry %v", result, err, current.InviteToken, current.ExpiresAt)
			}
			if store.rejected != 1 || store.primary == 0 {
				t.Fatalf("renewal retry rejected=%d primary=%d", store.rejected, store.primary)
			}
			for _, token := range []string{"old", "rotated"} {
				_, err := socialutil.ReadInviteToken(kv.WithPrimaryReads(ctx), store, socialutil.FriendInviteTokensRoot, token)
				if (token == current.InviteToken) != (err == nil) {
					t.Fatalf("token %q index = %v", token, err)
				}
			}
		})
	}
}

func TestFriendStaleCreationRecoveryDoesNotRecreateRetiredWorkspace(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	intent, err := s.getOrCreateCreationIntent(ctx, store, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	workspace, err := s.ensureCreationWorkspace(kv.WithPrimaryReads(ctx), intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.commitFriendCreation(kv.WithPrimaryReads(ctx), store, "owner", "peer", intent, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(kv.WithPrimaryReads(ctx), "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	before := len(workspaces.owners)
	if err := s.ReconcileCreationIntents(ctx); err != nil {
		t.Fatalf("stale creation recovery = %v", err)
	}
	if len(workspaces.owners) != before {
		t.Fatal("stale intent recreated its retired SQL Workspace")
	}
	if _, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retired relationship restored = %v", err)
	}
}

func TestFriendRejectedCreatorCompensatesOnlyItsRetiredIncarnation(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	intent, err := s.getOrCreateCreationIntent(ctx, s.Friends, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := s.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	newRelation, err := s.AdminCreateFriend(ctx, "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); !errors.Is(err, errFriendCreationCancelled) {
		t.Fatalf("delayed old commit = %v", err)
	}
	active, err := s.GetFriendRelation(ctx, "owner", "peer")
	if err != nil || socialutil.StringValue(active.WorkspaceName) != socialutil.StringValue(newRelation.WorkspaceName) {
		t.Fatalf("new lifecycle changed = %+v, %v", active, err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	if workspaces.retired[len(workspaces.retired)-1] != workspace.Id || len(workspaces.deleted) != 0 {
		t.Fatalf("compensation retired=%v deleted=%v", workspaces.retired, workspaces.deleted)
	}
	// A primary availability rejection uses cancellation cleanup too. Its old
	// intent must still leave the replacement binding and relationship intact.
	s.PeerAvailability = func(context.Context, string) error { return ownership.ErrOwnerRetired }
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("unavailable stale creator = %v", err)
	}
	active, err = s.GetFriendRelation(ctx, "owner", "peer")
	if err != nil || socialutil.StringValue(active.WorkspaceName) != socialutil.StringValue(newRelation.WorkspaceName) ||
		workspaces.retired[len(workspaces.retired)-1] != workspace.Id {
		t.Fatalf("old cancellation touched replacement: active=%+v retired=%v err=%v", active, workspaces.retired, err)
	}
}

func TestFriendStaleReceiptCannotAcknowledgeNewActiveIncarnation(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	old, err := s.AdminCreateFriend(ctx, "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	current, err := s.AdminCreateFriend(kv.WithPrimaryReads(ctx), "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"})
	if err != nil || socialutil.StringValue(deleted.WorkspaceName) != socialutil.StringValue(current.WorkspaceName) ||
		socialutil.StringValue(deleted.WorkspaceName) == socialutil.StringValue(old.WorkspaceName) {
		t.Fatalf("delete acknowledged stale receipt = %+v, %v", deleted, err)
	}
	if _, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("new incarnation left active = %v", err)
	}
}

func TestFriendAdminCreateProjectionMatchesCommittedLifecycle(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	if _, err := s.AdminCreateFriend(ctx, "owner", "peer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	id := socialutil.RelationID("owner", "peer")
	created, err := s.AdminCreateFriendResource(ctx, id, "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := readWorkspaceBinding(kv.WithPrimaryReads(ctx), store, id)
	if err != nil || created.WorkspaceId != binding.WorkspaceID {
		t.Fatalf("Admin creation = %+v binding=%+v err=%v", created, binding, err)
	}
}

func TestFriendCapacityRetryRechecksBothPeersOnAuthority(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	retired := false
	var refreshed []string
	s.PeerAvailability = func(ctx context.Context, peer string) error {
		if kv.WithPrimaryReads(ctx) == ctx && retired {
			refreshed = append(refreshed, peer)
			if peer == "peer" {
				return ownership.ErrOwnerRetired
			}
		}
		return nil
	}
	store.beforeWrite = func(ctx context.Context, mutation kv.Mutation) {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, workspaceBindingKey(socialutil.RelationID("owner", "peer"))) {
				store.beforeWrite = nil
				store.replicate = false
				retired = true
				if err := store.Store.Set(ctx, peerFriendRevisionKey("owner"), []byte("concurrent-capacity")); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	if _, err := s.AdminCreateFriend(ctx, "owner", "peer"); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("capacity retry bypassed availability = %v", err)
	}
	if !slices.Equal(refreshed, []string{"owner", "peer"}) {
		t.Fatalf("fallback availability peers = %v", refreshed)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	if len(workspaces.created) != 1 || !slices.Equal(workspaces.deleted, []string{workspaces.created[0].Name}) {
		t.Fatalf("retry changed incarnation or leaked Workspace: created=%v deleted=%v", workspaces.created, workspaces.deleted)
	}
	if _, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("unavailable peer was admitted = %v", err)
	}
}
