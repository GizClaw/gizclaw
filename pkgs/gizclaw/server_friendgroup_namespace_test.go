package gizclaw

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type failingGroupWorkspaceRetirement struct {
	friendgroup.WorkspaceService
	err error
}

func (s failingGroupWorkspaceRetirement) RetireSystemWorkspaceByID(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, s.err
}

func TestServerFriendGroupNamespaceLifecycle(t *testing.T) {
	ctx := t.Context()
	root := kv.NewMemory(nil)
	t.Cleanup(func() { _ = root.Close() })
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	owner := keyPair.Public.String()
	const groupID = "namespace-group"
	newServer := func(namespace string) *Server {
		t.Helper()
		server := completeTestServer(t, &Server{
			LocalStatic:      *keyPair,
			FriendGroupStore: kv.Prefixed(kv.Prefixed(root, kv.Key{namespace}), kv.Key{"friend-groups"}),
			SFUURL:           "wss://sfu.test",
		})
		if err := server.init(); err != nil {
			t.Fatal(err)
		}
		if _, err := server.manager.Peers.SavePeer(ctx, apitypes.Peer{
			PublicKey: owner, Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive,
		}); err != nil {
			t.Fatal(err)
		}
		return server
	}
	dev, prod := newServer("dev"), newServer("prod")
	devGroups, prodGroups := dev.peerService.admin.FriendGroups, prod.peerService.admin.FriendGroups
	bindingKey := kv.Key{"social-workspace-bindings", "friend-groups", groupID}
	intentKey := kv.Key{"social-retirement-intents", "friend-groups", groupID}
	receiptKey := kv.Key{"social-retirement-receipts", "friend-groups", groupID}
	nameKey := kv.Key{"retired-group-names", socialutil.EscapeStoreSegment(owner), "room"}
	recovery := socialutil.RecoveryIndex{Root: kv.Key{"social-retirement-intents", "friend-groups"}}
	assertRecovery := func(store kv.Store, want []string) {
		t.Helper()
		var ids []string
		for id, err := range recovery.IDs(ctx, store) {
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		if !slices.Equal(ids, want) {
			t.Fatalf("recovery IDs = %v, want %v", ids, want)
		}
	}
	assertAbsent := func(store kv.Store, keys ...kv.Key) {
		t.Helper()
		for _, key := range keys {
			if _, err := store.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("unexpected key %s: %v", key, err)
			}
		}
	}
	// Equal IDs and names in separate environments must not compete for a binding.
	for _, server := range []*Server{dev, prod} {
		groups := server.peerService.admin.FriendGroups
		group, err := groups.AdminCreateFriendGroup(ctx, groupID, owner, "room", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.FriendGroupStore.Get(ctx, bindingKey); err != nil {
			t.Fatalf("configured namespace has no binding: %v", err)
		}
		workspaceName, err := groups.ResolveFriendGroupWorkspace(ctx, owner, groupID)
		if err != nil {
			t.Fatal(err)
		}
		workspaceID := *group.WorkspaceId
		if _, err := groups.ResolveSFUWorkspaceBinding(ctx, workspaceID, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := groups.ResolveSFUWorkspaceBindingByName(ctx, workspaceName, owner); err != nil {
			t.Fatal(err)
		}
		for _, key := range []kv.Key{socialutil.WorkspaceLocatorIDKey(workspaceID), socialutil.WorkspaceLocatorNameKey(workspaceName)} {
			if _, err := server.FriendGroupStore.Get(ctx, key); err != nil {
				t.Fatalf("configured namespace has no locator: %v", err)
			}
			assertAbsent(root, key)
		}
	}
	assertAbsent(root, bindingKey)
	// Leave a durable intent after the group transaction, before Workspace retirement.
	retirementErr := errors.New("injected Workspace retirement failure")
	devGroups.Workspaces = failingGroupWorkspaceRetirement{WorkspaceService: devGroups.Workspaces, err: retirementErr}
	if _, err := devGroups.DeleteFriendGroup(ctx, owner, rpcapi.FriendGroupDeleteRequest{Name: "room"}); !errors.Is(err, retirementErr) {
		t.Fatalf("delete = %v", err)
	}
	if _, err := dev.FriendGroupStore.Get(ctx, intentKey); err != nil {
		t.Fatal(err)
	}
	assertAbsent(dev.FriendGroupStore, bindingKey, receiptKey)
	assertRecovery(dev.FriendGroupStore, []string{groupID})
	assertRecovery(prod.FriendGroupStore, nil)
	assertRecovery(root, nil)
	assertAbsent(root, intentKey, receiptKey, nameKey)
	pending, err := pendingdeletion.GetByLocator(ctx, dev.FriendGroupStore, pendingdeletion.KindFriendGroup, groupID)
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []kv.Store{root, prod.FriendGroupStore} {
		if _, err := pendingdeletion.GetByLocator(ctx, store, pendingdeletion.KindFriendGroup, groupID); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("foreign pending deletion: %v", err)
		}
	}
	// Check the actual production registry, not a separately constructed source.
	list, err := dev.peerService.admin.PendingDeletions.List(ctx, pendingdeletion.ListRequest{Kind: pendingdeletion.KindFriendGroup})
	if err != nil || len(list.Tasks) != 1 {
		t.Fatalf("dev tasks = %v, %v", list.Tasks, err)
	}
	list, err = prod.peerService.admin.PendingDeletions.List(ctx, pendingdeletion.ListRequest{Kind: pendingdeletion.KindFriendGroup})
	if err != nil || len(list.Tasks) != 0 {
		t.Fatalf("prod tasks = %v, %v", list.Tasks, err)
	}
	// Reinitializing the other environment must not consume dev's recovery work.
	if err := prod.init(); err != nil {
		t.Fatal(err)
	}
	prodGroups = prod.peerService.admin.FriendGroups
	if _, err := prodGroups.AdminGetFriendGroup(ctx, groupID); err != nil {
		t.Fatal(err)
	}
	assertRecovery(dev.FriendGroupStore, []string{groupID})
	// Reinitialization uses the same durable stores and restores the real Workspace service.
	if err := dev.init(); err != nil {
		t.Fatal(err)
	}
	devGroups = dev.peerService.admin.FriendGroups
	assertRecovery(dev.FriendGroupStore, nil)
	assertAbsent(dev.FriendGroupStore, intentKey)
	for _, key := range []kv.Key{receiptKey, nameKey} {
		if _, err := dev.FriendGroupStore.Get(ctx, key); err != nil {
			t.Fatal(err)
		}
		assertAbsent(root, key)
		assertAbsent(prod.FriendGroupStore, key)
	}
	if _, err := devGroups.DeleteFriendGroup(ctx, owner, rpcapi.FriendGroupDeleteRequest{Name: "room"}); err != nil {
		t.Fatalf("idempotent deletion: %v", err)
	}
	// Finalize the scoped cleanup task and verify production admin sees it disappear.
	source := friendgroup.NewPendingDeletionSource(dev.FriendGroupStore)
	now := time.Now().UTC().Add(time.Second)
	refs, _, err := source.ScanDue(ctx, now, 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("due = %v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	if err := (friendgroup.DeletionHandler{Server: devGroups, Source: source, Now: func() time.Time { return now }}).Handle(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if _, err := source.GetTask(ctx, pending.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("finalized task: %v", err)
	}
	list, err = dev.peerService.admin.PendingDeletions.List(ctx, pendingdeletion.ListRequest{Kind: pendingdeletion.KindFriendGroup})
	if err != nil || len(list.Tasks) != 0 {
		t.Fatalf("finalized dev tasks = %v, %v", list.Tasks, err)
	}
	if _, err := prodGroups.ResolveFriendGroupWorkspace(ctx, owner, groupID); err != nil {
		t.Fatalf("prod binding after dev cleanup: %v", err)
	}
	if _, err := prodGroups.DeleteFriendGroup(ctx, owner, rpcapi.FriendGroupDeleteRequest{Name: "room"}); err != nil {
		t.Fatal(err)
	}
	if _, err := prod.FriendGroupStore.Get(ctx, receiptKey); err != nil {
		t.Fatal(err)
	}
	assertAbsent(root, intentKey, receiptKey, nameKey, bindingKey)
	assertRecovery(root, nil)
}

func TestServerFriendGroupNamespaceIgnoresUnscopedRecovery(t *testing.T) {
	root := kv.NewMemory(nil)
	t.Cleanup(func() { _ = root.Close() })
	key := kv.Key{"social-retirement-intents", "friend-groups", "foreign-group"}
	// Invalid foreign data must never be decoded by this environment's recovery.
	value := []byte("foreign retirement data")
	index := socialutil.RecoveryIndex{Root: kv.Key{"social-retirement-intents", "friend-groups"}}
	if _, err := root.ApplyMutation(t.Context(), kv.Mutation{
		Entries:    []kv.Entry{{Key: key, Value: value}},
		AddMembers: index.Add("foreign-group"),
	}); err != nil {
		t.Fatal(err)
	}
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := completeTestServer(t, &Server{
		LocalStatic:      *keyPair,
		FriendGroupStore: kv.Prefixed(root, kv.Key{"dev", "friend-groups"}),
	})
	if err := server.init(); err != nil {
		t.Fatalf("foreign recovery affected startup: %v", err)
	}
	got, err := root.Get(t.Context(), key)
	if err != nil || string(got) != string(value) {
		t.Fatalf("foreign record changed: %q, %v", got, err)
	}
}
