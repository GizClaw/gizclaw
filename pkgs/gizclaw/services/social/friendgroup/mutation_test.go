package friendgroup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type groupMutationHookStore struct {
	kv.Store
	hook func(context.Context, kv.Mutation) error
}

func (store *groupMutationHookStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	if store.hook != nil {
		if err := store.hook(ctx, mutation); err != nil {
			return false, err
		}
	}
	return store.Store.ApplyMutation(ctx, mutation)
}

func groupServerWithMutationHook(t *testing.T) (*Server, *groupMutationHookStore) {
	t.Helper()
	server := newTestServer(t)
	store := &groupMutationHookStore{Store: server.RelationshipStore}
	server.Groups, server.Members, server.Belongs, server.InviteTokens, server.RelationshipStore = store, store, store, store, store
	return server, store
}

func TestGroupCreationPublishesOwnerAndIndexesAtomically(t *testing.T) {
	server, store := groupServerWithMutationHook(t)
	wanted := errors.New("injected owner write failure")
	store.hook = func(_ context.Context, mutation kv.Mutation) error {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, socialutil.GroupMemberKey("id-a", "owner")) {
				return wanted
			}
		}
		return nil
	}
	if _, err := server.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"}); !errors.Is(err, wanted) {
		t.Fatalf("create error=%v", err)
	}
	for _, key := range []kv.Key{socialutil.GroupKey("id-a"), groupRevisionKey("id-a"), workspaceBindingKey("id-a"), socialutil.GroupNameKey("owner", "room")} {
		if _, err := store.Get(t.Context(), key); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("partial record at %v: %v", key, err)
		}
	}
	page, err := listAdminGroupRecords(t.Context(), store, "", 10)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("partial management index: %+v %v", page, err)
	}
	if members, err := store.ListMembers(t.Context(), memberCollectionKey("id-a")); err != nil || len(members) != 0 {
		t.Fatalf("partial member index: %v %v", members, err)
	}
}

func TestGroupCreationRecoversLostWriteAcknowledgement(t *testing.T) {
	server, store := groupServerWithMutationHook(t)
	store.hook = func(ctx context.Context, mutation kv.Mutation) error {
		created, err := store.Store.ApplyMutation(ctx, mutation)
		if err != nil {
			return err
		}
		if !created {
			return errors.New("unexpected creation conflict")
		}
		return errors.New("lost write acknowledgement")
	}
	group, err := server.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	if len(server.Workspaces.(*recordingWorkspaceService).deleted) != 0 {
		t.Fatal("published group's Workspace was rolled back")
	}
	id := mustGroupID(t, server, "owner", group.Name)
	if _, err := server.groupMember(t.Context(), id, "owner"); err != nil {
		t.Fatal(err)
	}
}

func TestSharedGroupCapacityRejectsConcurrentAdmission(t *testing.T) {
	server, store := groupServerWithMutationHook(t)
	group, err := server.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	id := mustGroupID(t, server, "owner", group.Name)
	for i := 1; i < socialutil.FriendGroupMemberLimit-1; i++ {
		if _, err := server.createMember(t.Context(), id, fmt.Sprintf("member-%d", i), rpcapi.FriendGroupMemberRoleMember, "room"); err != nil {
			t.Fatal(err)
		}
	}
	var arrived atomic.Int32
	ready, release := make(chan struct{}), make(chan struct{})
	store.hook = func(ctx context.Context, _ kv.Mutation) error {
		if arrived.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	other := *server
	done := make(chan error, 2)
	// Exercise the storage admission boundary directly, without sharing the
	// process-local request locks that independent Server processes cannot share.
	go func() {
		_, err := server.createMember(t.Context(), id, "contender-a", rpcapi.FriendGroupMemberRoleMember, "room")
		done <- err
	}()
	go func() {
		_, err := other.createMember(t.Context(), id, "contender-b", rpcapi.FriendGroupMemberRoleMember, "room")
		done <- err
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("concurrent admissions did not reach the transaction boundary")
	}
	close(release)
	winners, conflicts := 0, 0
	for range 2 {
		err := <-done
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrGroupChanged):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", winners, conflicts)
	}
	keys, err := server.memberPublicKeys(t.Context(), id)
	if err != nil || len(keys) != socialutil.FriendGroupMemberLimit {
		t.Fatalf("members=%v error=%v", keys, err)
	}
	for _, peer := range []string{"contender-a", "contender-b"} {
		found, err := store.HasMember(t.Context(), belongCollectionKey(peer), id)
		if err != nil || found != slices.Contains(keys, peer) {
			t.Fatalf("belongs index for %s: %v %v", peer, found, err)
		}
	}
}

func TestGroupRetirementRejectsChangedMemberSnapshot(t *testing.T) {
	server, store := groupServerWithMutationHook(t)
	group, err := server.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	id := mustGroupID(t, server, "owner", group.Name)
	ready, release := make(chan struct{}), make(chan struct{})
	store.hook = func(ctx context.Context, mutation kv.Mutation) error {
		for _, key := range mutation.DeleteKeys {
			if slices.Equal(key, socialutil.GroupKey(id)) {
				close(ready)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return nil
	}
	done := make(chan error, 1)
	go func() { _, err := server.deleteFriendGroup(t.Context(), id); done <- err }()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("retirement did not reach the transaction boundary")
	}
	other := *server
	_, joinErr := other.createMember(t.Context(), id, "new-member", rpcapi.FriendGroupMemberRoleMember, "room")
	close(release)
	if joinErr != nil {
		t.Fatal(joinErr)
	}
	if err := <-done; !errors.Is(err, ErrGroupChanged) {
		t.Fatalf("stale retirement error=%v", err)
	}
	if _, err := server.groupMember(t.Context(), id, "new-member"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.AdminGetFriendGroup(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), groupRetirementIntentKey(id)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("stale retirement published an intent: %v", err)
	}
}

func TestGroupRetirementUsesActualStorePrefixes(t *testing.T) {
	s := newTestServer(t)
	root := kv.NewMemory(nil)
	s.Groups = kv.Prefixed(root, kv.Key{"actual", "groups"})
	s.Members = kv.Prefixed(root, kv.Key{"actual", "members"})
	s.Belongs = kv.Prefixed(root, kv.Key{"actual", "belongs"})
	s.InviteTokens = kv.Prefixed(root, kv.Key{"actual", "invites"})
	s.RelationshipStore = kv.Prefixed(root, kv.Key{"actual", "relationships"})
	group, err := s.AdminCreateFriendGroup(t.Context(), "prefix-group", "owner", "room", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminPutFriendGroupMember(t.Context(), group.Id, "guest", "guest-room", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := s.deleteFriendGroup(t.Context(), group.Id); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		store kv.Store
		key   kv.Key
	}{
		{s.Groups, socialutil.GroupKey(group.Id)},
		{s.Groups, groupRevisionKey(group.Id)},
		{s.Members, socialutil.GroupMemberKey(group.Id, "guest")},
		{s.Members, socialutil.GroupMemberKey(group.Id, "owner")},
		{s.Belongs, socialutil.GroupBelongKey("guest", group.Id)},
		{s.Belongs, socialutil.GroupNameKey("guest", "guest-room")},
		{s.RelationshipStore, workspaceBindingKey(group.Id)},
	} {
		if _, err := row.store.Get(t.Context(), row.key); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("retained %v: %v", row.key, err)
		}
	}
	if members, err := s.Members.ListMembers(t.Context(), memberCollectionKey(group.Id)); err != nil || len(members) != 0 {
		t.Fatalf("members=%v err=%v", members, err)
	}
	for _, peer := range []string{"owner", "guest"} {
		if groups, err := s.Belongs.ListMembers(t.Context(), belongCollectionKey(peer)); err != nil || len(groups) != 0 {
			t.Fatalf("belongs=%v err=%v", groups, err)
		}
	}
	page, err := listAdminGroupRecords(t.Context(), s.Groups, "", 10)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("admin index=%+v err=%v", page, err)
	}
}
