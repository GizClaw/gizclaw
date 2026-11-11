package friendgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
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

// socialReadSplitStore keeps a frozen replica while mutations target authority.
// WithPrimaryReads is idempotent, so comparing its returned context lets this
// test double recognize the explicit fallback without reaching into KV internals.
type socialReadSplitStore struct {
	kv.Store
	reader                                 kv.Store
	replicate                              bool
	ordinary, primary, mutations, rejected int
	beforeWrite                            func(context.Context, kv.Mutation)
	writeErr                               error
}

func (s *socialReadSplitStore) read(ctx context.Context) kv.Store {
	if kv.WithPrimaryReads(ctx) == ctx {
		s.primary++
		return s.Store
	}
	s.ordinary++
	return s.reader
}
func (s *socialReadSplitStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	return s.read(ctx).Get(ctx, key)
}
func (s *socialReadSplitStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	return s.read(ctx).ListMembers(ctx, key)
}
func (s *socialReadSplitStore) HasMember(ctx context.Context, key kv.Key, member string) (bool, error) {
	return s.read(ctx).HasMember(ctx, key, member)
}
func (s *socialReadSplitStore) RangeOrderedMembers(ctx context.Context, key kv.Key, query kv.OrderedRange) ([]string, error) {
	return s.read(ctx).RangeOrderedMembers(ctx, key, query)
}
func (s *socialReadSplitStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.mutations++
	if s.beforeWrite != nil {
		s.beforeWrite(ctx, mutation)
	}
	applied, err := s.Store.ApplyMutation(ctx, mutation)
	if err != nil {
		return applied, err
	}
	if !applied {
		s.rejected++
	}
	if applied && s.replicate {
		if _, err := s.reader.ApplyMutation(ctx, mutation); err != nil {
			return false, err
		}
	}
	if applied && s.writeErr != nil {
		return false, s.writeErr
	}
	return applied, nil
}

func newReadSplitGroupServer(t *testing.T) (*Server, *socialReadSplitStore) {
	t.Helper()
	s := newTestServer(t)
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Groups, s.Members, s.Belongs, s.InviteTokens, s.RelationshipStore = store, store, store, store, store
	return s, store
}

func TestGroupStaleReadsAndConflictOnlyFallback(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	if _, err := s.AdminPutFriendGroup(kv.WithPrimaryReads(t.Context()), "group", new("current"), nil); err != nil {
		t.Fatal(err)
	}
	store.primary, store.ordinary = 0, 0
	group, err := s.GetFriendGroup(t.Context(), "owner", rpcapi.FriendGroupGetRequest{Name: "room"})
	if err != nil || group.DisplayName != nil || store.primary != 0 {
		t.Fatalf("ordinary group=%+v primary=%d err=%v", group, store.primary, err)
	}
	before, rejected := store.mutations, store.rejected
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	group, err = s.PutFriendGroup(ctx, "owner", rpcapi.FriendGroupPutRequest{Name: "room", Description: new("updated")})
	if err != nil {
		t.Fatal(err)
	}
	if store.mutations-before != 2 || store.rejected-rejected != 1 || store.primary == 0 || socialutil.StringValue(group.DisplayName) != "current" {
		t.Fatalf("fallback group=%+v primary=%d writes=%d rejected=%d", group, store.primary, store.mutations-before, store.rejected-rejected)
	}
	primary := store.primary
	group, err = s.AdminGetFriendGroup(t.Context(), "group")
	if err != nil || group.DisplayName != nil || store.primary != primary {
		t.Fatalf("fallback pinned store: group=%+v primary=%d err=%v", group, store.primary, err)
	}
}

func TestGroupConflictFallbackRechecksPermission(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminPutFriendGroupMember(t.Context(), "group", "actor", "actor-room", rpcapi.FriendGroupMemberRoleAdmin); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	if _, err := s.AdminPutFriendGroupMember(kv.WithPrimaryReads(t.Context()), "group", "actor", "actor-room", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := s.AddFriendGroupMember(ctx, "actor", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: "actor-room", PeerPublicKey: "guest", MemberName: "guest-room", Role: rpcapi.FriendGroupMemberMutableRole("member")})
	if !errors.Is(err, ErrFriendGroupPermissionDenied) || store.rejected != 1 {
		t.Fatalf("permission fallback rejected=%d err=%v", store.rejected, err)
	}
	if _, err := s.groupMember(kv.WithPrimaryReads(t.Context()), "group", "guest"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("unauthorized admission: %v", err)
	}
}

func TestGroupMutationFallbackIsBounded(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	before := store.mutations
	store.beforeWrite = func(ctx context.Context, _ kv.Mutation) {
		if err := store.Store.Set(ctx, groupRevisionKey("group"), []byte(fmt.Sprint(store.mutations))); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.AdminPutFriendGroup(t.Context(), "group", new("updated"), nil)
	if !errors.Is(err, ErrGroupChanged) || store.mutations-before != 2 {
		t.Fatalf("writes=%d err=%v", store.mutations-before, err)
	}
}

func TestGroupMutationDoesNotReplayUncertainCommit(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	store.writeErr = errors.New("lost acknowledgement")
	before := store.mutations
	_, err := s.AdminPutFriendGroup(t.Context(), "group", new("updated"), nil)
	if !errors.Is(err, store.writeErr) || store.mutations-before != 1 {
		t.Fatalf("writes=%d err=%v", store.mutations-before, err)
	}
}

func TestGroupCreationConfirmsLostAcknowledgementThroughPrimary(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	store.replicate = false
	store.writeErr = errors.New("lost acknowledgement")
	group, err := s.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil || group.Name != "room" || store.primary == 0 || store.mutations != 1 {
		t.Fatalf("creation=%+v primary=%d writes=%d err=%v", group, store.primary, store.mutations, err)
	}
	if len(s.Workspaces.(*recordingWorkspaceService).deleted) != 0 {
		t.Fatal("published Workspace was rolled back")
	}
}

func TestGroupJoinConfirmsPostwriteReplicaMiss(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	// Publish the same invite on authority and replica without advancing group
	// revisions. The new member's name index will remain absent on the replica.
	for _, target := range []kv.Store{store.Store, store.reader} {
		record := inviteTokenRecord{FriendGroupID: "group", InviteToken: "invite", CreatedAt: s.now(), ExpiresAt: s.now().Add(time.Hour)}
		if err := socialutil.WriteInviteToken(t.Context(), target, socialutil.GroupInviteTokenKey("group"), record); err != nil {
			t.Fatal(err)
		}
	}
	store.replicate = false
	before := store.mutations
	joined, err := s.JoinFriendGroup(t.Context(), "guest", rpcapi.FriendGroupJoinRequest{Name: "guest-room", InviteToken: "invite"})
	if err != nil || joined.Group.Name != "guest-room" || store.mutations-before != 1 || store.rejected != 0 || store.primary == 0 {
		t.Fatalf("join=%+v writes=%d rejected=%d primary=%d err=%v", joined, store.mutations-before, store.rejected, store.primary, err)
	}
	if _, err := s.groupMember(kv.WithPrimaryReads(t.Context()), "group", "guest"); err != nil {
		t.Fatalf("committed membership rolled back: %v", err)
	}
}

func TestGroupAdminCreateConfirmsPostwriteReplicaMiss(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	store.replicate = false
	group, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil)
	if err != nil || group.WorkspaceId == nil || store.primary == 0 || store.mutations != 1 {
		t.Fatalf("creation=%+v primary=%d writes=%d err=%v", group, store.primary, store.mutations, err)
	}
}

func TestGroupStaleDeletionFenceCannotReuseLifecycle(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	store.replicate = false
	ctx := kv.WithPrimaryReads(t.Context())
	original, err := s.AdminCreateFriendGroup(ctx, "group", "owner", "room", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminDeleteFriendGroup(ctx, "group"); err != nil {
		t.Fatal(err)
	}
	_, err = s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil)
	if !errors.Is(err, ErrFriendGroupPendingDeletion) {
		t.Fatalf("retired ID reused with a stale fence: %v", err)
	}
	if _, err := s.AdminGetFriendGroup(ctx, "group"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retired group was recreated: %v", err)
	}
	receipt, err := s.readRetirementReceipt(ctx, "group")
	if err != nil || receipt.WorkspaceID != socialutil.StringValue(original.WorkspaceId) {
		t.Fatalf("retirement identity changed: %+v %v", receipt, err)
	}
}

func TestNamedGroupMutationsKeepCanonicalIDAfterNameReuse(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *Server) error
	}{
		{"put", func(ctx context.Context, s *Server) error {
			_, err := s.PutFriendGroup(ctx, "owner", rpcapi.FriendGroupPutRequest{Name: "room", DisplayName: new("changed")})
			return err
		}},
		{"delete", func(ctx context.Context, s *Server) error {
			_, err := s.DeleteFriendGroup(ctx, "owner", rpcapi.FriendGroupDeleteRequest{Name: "room"})
			return err
		}},
		{"create invitation", func(ctx context.Context, s *Server) error {
			_, err := s.CreateFriendGroupInviteTokenWithTTL(ctx, "owner", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "room"}, 2*time.Hour)
			return err
		}},
		{"clear invitation", func(ctx context.Context, s *Server) error {
			_, err := s.ClearFriendGroupInviteToken(ctx, "owner", rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupName: "room"})
			return err
		}},
		{"add member", func(ctx context.Context, s *Server) error {
			_, err := s.AddFriendGroupMember(ctx, "owner", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: "room", PeerPublicKey: "candidate", MemberName: "candidate-room", Role: "member"})
			return err
		}},
		{"put member", func(ctx context.Context, s *Server) error {
			_, err := s.PutFriendGroupMember(ctx, "owner", rpcapi.FriendGroupMemberPutRequest{FriendGroupName: "room", Name: "guest", Role: "admin"})
			return err
		}},
		{"delete member", func(ctx context.Context, s *Server) error {
			_, err := s.DeleteFriendGroupMember(ctx, "owner", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: "room", Name: "guest"})
			return err
		}},
		{"leave", func(ctx context.Context, s *Server) error {
			_, err := s.LeaveFriendGroup(ctx, "guest", "guest-room")
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			s, store := newReadSplitGroupServer(t)
			if _, err := s.AdminCreateFriendGroup(t.Context(), "original", "owner", "room", nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AdminCreateFriendGroupMember(t.Context(), "original", "guest", "guest-room", rpcapi.FriendGroupMemberRoleMember); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AdminPutFriendGroupInviteToken(t.Context(), "original", "original-token", s.now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			store.replicate = false
			primary := kv.WithPrimaryReads(t.Context())
			if _, err := s.AdminDeleteFriendGroup(primary, "original"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AdminCreateFriendGroup(primary, "replacement", "owner", "room", nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AdminCreateFriendGroupMember(primary, "replacement", "guest", "guest-room", rpcapi.FriendGroupMemberRoleMember); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AdminPutFriendGroupInviteToken(primary, "replacement", "replacement-token", s.now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			rejected := store.rejected
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			operationErr := operation.run(ctx, s)
			if errors.Is(operationErr, context.DeadlineExceeded) || store.rejected-rejected < 1 {
				t.Fatalf("operation did not reject its stale attempt: rejected=%d error=%v", store.rejected-rejected, operationErr)
			}
			group, err := s.AdminGetFriendGroup(primary, "replacement")
			if err != nil || group.DisplayName != nil {
				t.Fatalf("replacement Group changed: %+v %v", group, err)
			}
			member, err := s.groupMember(primary, "replacement", "guest")
			if err != nil || socialutil.GroupRole(member) != rpcapi.FriendGroupMemberRoleMember {
				t.Fatalf("replacement membership changed: %+v %v", member, err)
			}
			if _, err := s.groupMember(primary, "replacement", "candidate"); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("candidate admitted to replacement Group: %v", err)
			}
			token, active, err := s.activeGroupInviteToken(primary, store, "replacement")
			if err != nil || !active || token.InviteToken != "replacement-token" {
				t.Fatalf("replacement invitation changed: %+v %v", token, err)
			}
		})
	}
}

func TestGroupJoinDoesNotFollowReassignedInvitation(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	for _, id := range []string{"original", "replacement"} {
		if _, err := s.AdminCreateFriendGroup(t.Context(), id, id+"-owner", "room", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AdminPutFriendGroupInviteToken(t.Context(), "original", "shared-token", s.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	primary := kv.WithPrimaryReads(t.Context())
	if _, err := s.AdminDeleteFriendGroup(primary, "original"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminPutFriendGroupInviteToken(primary, "replacement", "shared-token", s.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rejected := store.rejected
	_, err := s.JoinFriendGroup(t.Context(), "guest", rpcapi.FriendGroupJoinRequest{Name: "guest-room", InviteToken: "shared-token"})
	if !errors.Is(err, ErrInviteTokenUnavailable) || store.rejected-rejected != 1 {
		t.Fatalf("join retargeted invitation: rejected=%d error=%v", store.rejected-rejected, err)
	}
	if _, err := s.groupMember(primary, "replacement", "guest"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("guest admitted to replacement Group: %v", err)
	}
}

func TestGroupInvitationCannotResurrectDeletedGroup(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(fmt.Sprint("admin=", admin), func(t *testing.T) {
			s, store := newReadSplitGroupServer(t)
			if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
				t.Fatal(err)
			}
			store.replicate = false
			primary := kv.WithPrimaryReads(t.Context())
			if _, err := s.AdminDeleteFriendGroup(primary, "group"); err != nil {
				t.Fatal(err)
			}
			rejected := store.rejected
			var err error
			if admin {
				_, err = s.AdminPutFriendGroupInviteToken(t.Context(), "group", "resurrected", s.now().Add(time.Hour))
			} else {
				_, err = s.CreateFriendGroupInviteToken(t.Context(), "owner", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "room"})
			}
			if err == nil || store.rejected-rejected != 1 {
				t.Fatalf("deleted Group accepted invitation: rejected=%d error=%v", store.rejected-rejected, err)
			}
			if _, err := store.Get(primary, socialutil.GroupInviteTokenKey("group")); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("invitation resurrected: %v", err)
			}
			if _, err := store.Get(primary, socialutil.InviteTokenIndexKey(socialutil.GroupInviteTokensRoot, "resurrected")); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("invitation index resurrected: %v", err)
			}
		})
	}
}

func TestGroupInvitationRetryRecomputesCurrentTokenAndExpiry(t *testing.T) {
	for _, token := range []string{"original-token", "rotated-token"} {
		t.Run(token, func(t *testing.T) {
			s, store := newReadSplitGroupServer(t)
			if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AdminPutFriendGroupInviteToken(t.Context(), "group", "original-token", s.now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			store.replicate = false
			primary := kv.WithPrimaryReads(t.Context())
			expiry := s.now().Add(48 * time.Hour)
			if _, err := s.AdminPutFriendGroupInviteToken(primary, "group", token, expiry); err != nil {
				t.Fatal(err)
			}
			rejected := store.rejected
			response, err := s.CreateFriendGroupInviteTokenWithTTL(t.Context(), "owner", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "room"}, 2*time.Hour)
			if err != nil || response.InviteToken != token || !response.ExpiresAt.Equal(expiry) || store.rejected-rejected != 1 {
				t.Fatalf("stale extension response=%+v rejected=%d error=%v", response, store.rejected-rejected, err)
			}
			current, active, err := s.activeGroupInviteToken(primary, store, "group")
			if err != nil || !active || current.InviteToken != response.InviteToken || !current.ExpiresAt.Equal(response.ExpiresAt) {
				t.Fatalf("response differs from committed invitation: %+v %v", current, err)
			}
			if token != "original-token" {
				if _, err := socialutil.ReadInviteToken(primary, store, socialutil.GroupInviteTokensRoot, "original-token"); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("old invitation revived: %v", err)
				}
			}
			shorter := s.now().Add(time.Hour)
			response, err = s.AdminPutFriendGroupInviteToken(t.Context(), "group", "explicit-token", shorter)
			if err != nil || response.InviteToken != "explicit-token" || !response.ExpiresAt.Equal(shorter) {
				t.Fatalf("explicit replacement lost semantics: %+v %v", response, err)
			}
		})
	}
}

func TestGroupInvitationPreservesScopedLocatorWithAtomicGuards(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	s.Groups = kv.Prefixed(store, kv.Key{"actual", "groups"})
	s.Members = kv.Prefixed(store, kv.Key{"actual", "members"})
	s.Belongs = kv.Prefixed(store, kv.Key{"actual", "belongs"})
	s.InviteTokens = kv.Prefixed(store, kv.Key{"actual", "invites"})
	s.RelationshipStore = kv.Prefixed(store, kv.Key{"actual", "relationships"})
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	request := rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "room"}
	invitation, err := s.CreateFriendGroupInviteToken(t.Context(), "owner", request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.InviteTokens.Get(t.Context(), socialutil.InviteTokenIndexKey(socialutil.GroupInviteTokensRoot, invitation.InviteToken))
	if err != nil {
		t.Fatal(err)
	}
	var locator kv.Key
	if err := json.Unmarshal(data, &locator); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(locator, socialutil.GroupInviteTokenKey("group")) {
		t.Fatalf("invitation locator escaped its logical namespace: %v", locator)
	}
	joined, err := s.JoinFriendGroup(t.Context(), "guest", rpcapi.FriendGroupJoinRequest{Name: "guest-room", InviteToken: invitation.InviteToken})
	if err != nil || joined.Group.Name != "guest-room" || socialutil.GroupRole(joined.Member) != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("namespaced invitation could not be joined: %+v %v", joined, err)
	}
	store.replicate = false
	primary := kv.WithPrimaryReads(t.Context())
	expiry := s.now().Add(48 * time.Hour)
	if _, err := s.AdminPutFriendGroupInviteToken(primary, "group", "rotated-token", expiry); err != nil {
		t.Fatal(err)
	}
	rejected := store.rejected
	extended, err := s.CreateFriendGroupInviteTokenWithTTL(t.Context(), "owner", request, 2*time.Hour)
	if err != nil || extended.InviteToken != "rotated-token" || !extended.ExpiresAt.Equal(expiry) || store.rejected-rejected != 1 {
		t.Fatalf("namespaced retry lost current token: %+v rejected=%d error=%v", extended, store.rejected-rejected, err)
	}
	current, err := s.findGroupInviteToken(primary, "rotated-token")
	if err != nil || current.FriendGroupID != "group" || !current.ExpiresAt.Equal(expiry) {
		t.Fatalf("namespaced rotated-token lookup=%+v error=%v", current, err)
	}
	visible, err := s.GetFriendGroupInviteToken(primary, "owner", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupName: "room"})
	if err != nil || socialutil.StringValue(visible.InviteToken) != "rotated-token" || !socialutil.TimeValue(visible.ExpiresAt).Equal(expiry) {
		t.Fatalf("namespaced TTL lookup=%+v error=%v", visible, err)
	}
	if _, err := s.findGroupInviteToken(primary, invitation.InviteToken); !errors.Is(err, ErrInviteTokenUnavailable) {
		t.Fatalf("namespaced old token still resolves after rotation: %v", err)
	}
	if _, err := s.ClearFriendGroupInviteToken(primary, "owner", rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupName: "room"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.findGroupInviteToken(primary, "rotated-token"); !errors.Is(err, ErrInviteTokenUnavailable) {
		t.Fatalf("namespaced cleared token still resolves: %v", err)
	}
	if _, err := s.InviteTokens.Get(primary, socialutil.GroupInviteTokenKey("group")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("namespaced cleared token record remains: %v", err)
	}
	if _, err := s.AdminPutFriendGroupInviteToken(primary, "group", "deleting-token", expiry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminDeleteFriendGroup(primary, "group"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFriendGroupInviteToken(t.Context(), "owner", request); err == nil {
		t.Fatal("namespaced stale invitation resurrected deleted Group")
	}
	if _, err := s.findGroupInviteToken(primary, "deleting-token"); !errors.Is(err, ErrInviteTokenUnavailable) {
		t.Fatalf("namespaced deleted Group token still resolves: %v", err)
	}
	if _, err := s.InviteTokens.Get(primary, socialutil.GroupInviteTokenKey("group")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("namespaced stale invitation remains: %v", err)
	}
}

func TestGroupInvitationDoesNotReplayUncertainCommit(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	store.writeErr = errors.New("lost invitation acknowledgement")
	before := store.mutations
	_, err := s.CreateFriendGroupInviteToken(t.Context(), "owner", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "room"})
	if !errors.Is(err, store.writeErr) || store.mutations-before != 1 {
		t.Fatalf("invitation replayed: writes=%d error=%v", store.mutations-before, err)
	}
}

func TestPreparedPeerGroupInventoryIncludesEarlierAdmissions(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	store.replicate = false
	primary := kv.WithPrimaryReads(t.Context())
	if _, err := s.AdminCreateFriendGroup(primary, "owned", "retiring", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminCreateFriendGroup(primary, "foreign", "other", "other-room", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminCreateFriendGroupMember(primary, "foreign", "retiring", "foreign-room", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatal(err)
	}
	before, authority := store.mutations, store.primary
	ordinary, err := s.SnapshotPeerGroups(t.Context(), "retiring")
	if err != nil || len(ordinary) != 0 || store.mutations != before || store.primary != authority {
		t.Fatalf("ordinary snapshot changed read policy or wrote data: %+v %v", ordinary, err)
	}
	prepared, err := s.PreparePeerGroupsRetirement(t.Context(), "retiring")
	if err != nil || len(prepared) != 2 || prepared[0].CreatedAt == nil || prepared[1].CreatedAt == nil {
		t.Fatalf("prepared inventory missed earlier admissions: %+v %v", prepared, err)
	}
	if _, err := store.Get(primary, socialutil.PeerRetirementFenceKey("retiring")); err != nil {
		t.Fatalf("permanent retirement fence missing: %v", err)
	}
	if _, err := s.AdminCreateFriendGroup(t.Context(), "late-owned", "retiring", "late-room", nil, nil); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("new owner admission crossed fence: %v", err)
	}
	if _, err := s.AdminPutFriendGroupMember(primary, "foreign", "retiring", "foreign-room", rpcapi.FriendGroupMemberRoleAdmin); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("membership update crossed fence: %v", err)
	}
	if _, err := s.AdminPutFriendGroup(primary, "owned", new("changed"), nil); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("owner update crossed fence: %v", err)
	}
	for _, captured := range prepared {
		if err := s.RetirePeerGroup(t.Context(), captured); err != nil {
			t.Fatalf("fenced deletion could not proceed: %v", err)
		}
	}
	if _, err := s.AdminGetFriendGroup(primary, "foreign"); err != nil {
		t.Fatalf("foreign Group removed: %v", err)
	}
	if _, err := s.groupMember(primary, "foreign", "retiring"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retiring membership retained: %v", err)
	}
}

func TestPreparedPeerGroupInventoryKeepsReaderWhenFenceVisible(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	authority := store.primary
	prepared, err := s.PreparePeerGroupsRetirement(t.Context(), "owner")
	if err != nil || len(prepared) != 1 || store.primary != authority {
		t.Fatalf("visible fence forced authority reads: %+v primary=%d error=%v", prepared, store.primary-authority, err)
	}
}

func TestPreparedPeerGroupInventoryUsesFenceReadScope(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	store.replicate = false
	if _, err := s.AdminCreateFriendGroup(kv.WithPrimaryReads(t.Context()), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	// The belongs view observes current data while Groups remains a frozen
	// replica. Preparation must use one common read scope after proving its fence.
	s.Belongs = kv.WithStrongReads(store)
	prepared, err := s.PreparePeerGroupsRetirement(t.Context(), "owner")
	if err != nil || len(prepared) != 1 || prepared[0].FriendGroupID != "group" {
		t.Fatalf("mixed-delay views produced incomplete preparation: %+v %v", prepared, err)
	}
}

func TestPreparedPeerGroupFenceKeepsActualNamespace(t *testing.T) {
	s := newTestServer(t)
	root := kv.NewMemory(nil)
	s.Groups = kv.Prefixed(root, kv.Key{"actual", "groups"})
	s.Members = kv.Prefixed(root, kv.Key{"actual", "members"})
	s.Belongs = kv.Prefixed(root, kv.Key{"actual", "belongs"})
	s.RelationshipStore = kv.Prefixed(root, kv.Key{"actual", "relationships"})
	s.InviteTokens = kv.Prefixed(root, kv.Key{"actual", "invites"})
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	prepared, err := s.PreparePeerGroupsRetirement(t.Context(), "owner")
	if err != nil || len(prepared) != 1 {
		t.Fatalf("namespaced preparation=%+v error=%v", prepared, err)
	}
	if _, err := s.Belongs.Get(t.Context(), socialutil.PeerRetirementFenceKey("owner")); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Get(t.Context(), socialutil.PeerRetirementFenceKey("owner")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retirement fence escaped configured namespace: %v", err)
	}
	if _, err := s.AdminCreateFriendGroupMember(t.Context(), "group", "guest", "guest-room", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("owner fence did not guard guest admission: %v", err)
	}
}

func TestPeerGroupRetirementFenceRejectsInFlightAdmission(t *testing.T) {
	s, store := groupServerWithMutationHook(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	store.hook = func(ctx context.Context, mutation kv.Mutation) error {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, socialutil.GroupMemberKey("group", "retiring")) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.createMember(t.Context(), "group", "retiring", rpcapi.FriendGroupMemberRoleMember, "guest-room")
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("admission did not reach its transaction")
	}
	prepared, prepareErr := s.PreparePeerGroupsRetirement(t.Context(), "retiring")
	close(release)
	if err := <-done; !errors.Is(err, ErrGroupChanged) {
		t.Fatalf("in-flight admission crossed fence: %v", err)
	}
	if prepareErr != nil || len(prepared) != 0 {
		t.Fatalf("preparation included an uncommitted admission: %+v %v", prepared, prepareErr)
	}
	if _, err := s.groupMember(t.Context(), "group", "retiring"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("in-flight membership retained: %v", err)
	}
}

func TestPeerGroupRetirementFenceRejectsInFlightOwnerAdmission(t *testing.T) {
	s, store := groupServerWithMutationHook(t)
	owner, name, workspaceName := "retiring", "room", socialutil.GroupWorkspaceName("group")
	now := s.now()
	group := rpcapi.FriendGroupObject{Name: name, CreatedByPeerPublicKey: &owner, WorkspaceName: &workspaceName, CreatedAt: &now, UpdatedAt: &now}
	binding, err := s.newWorkspaceBinding("group", owner)
	if err != nil {
		t.Fatal(err)
	}
	binding.WorkspaceID, binding.WorkspaceName = "group-workspace", workspaceName
	entered, release := make(chan struct{}), make(chan struct{})
	store.hook = func(ctx context.Context, mutation kv.Mutation) error {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, socialutil.GroupKey("group")) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- s.createGroupWithOwnerAttempt(t.Context(), "group", group, binding, owner, name) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("owner admission did not reach its transaction")
	}
	prepared, prepareErr := s.PreparePeerGroupsRetirement(t.Context(), owner)
	close(release)
	if err := <-done; !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("in-flight owner admission crossed fence: %v", err)
	}
	if prepareErr != nil || len(prepared) != 0 {
		t.Fatalf("preparation included uncommitted owner admission: %+v %v", prepared, prepareErr)
	}
	for _, key := range []kv.Key{socialutil.GroupKey("group"), socialutil.GroupMemberKey("group", owner), workspaceBindingKey("group")} {
		if _, err := store.Get(t.Context(), key); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("owner admission left a partial record at %v: %v", key, err)
		}
	}
}

func TestPeerGroupRetirementRefreshesStaleMissingForeignMember(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	primary := kv.WithPrimaryReads(t.Context())
	if _, err := s.AdminCreateFriendGroupMember(primary, "group", "retiring", "guest-room", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatal(err)
	}
	captured, err := s.SnapshotPeerGroups(primary, "retiring")
	if err != nil || len(captured) != 1 {
		t.Fatalf("snapshot=%+v error=%v", captured, err)
	}
	before, rejected := store.mutations, store.rejected
	if err := s.RetirePeerGroup(t.Context(), captured[0]); err != nil {
		t.Fatal(err)
	}
	if store.mutations-before != 2 || store.rejected-rejected != 1 {
		t.Fatalf("missing replica member was accepted without proof: writes=%d rejected=%d", store.mutations-before, store.rejected-rejected)
	}
	if _, err := s.groupMember(primary, "group", "retiring"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("captured foreign membership remains: %v", err)
	}
	if _, err := s.groupMember(primary, "group", "owner"); err != nil {
		t.Fatalf("foreign owner was removed: %v", err)
	}
}

func TestPeerGroupRetirementRetryPreservesReplacementMembership(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminCreateFriendGroupMember(t.Context(), "group", "retiring", "guest-room", rpcapi.FriendGroupMemberRoleMember); err != nil {
		t.Fatal(err)
	}
	captured, err := s.SnapshotPeerGroups(t.Context(), "retiring")
	if err != nil || len(captured) != 1 {
		t.Fatalf("snapshot=%+v error=%v", captured, err)
	}
	store.replicate = false
	primary := kv.WithPrimaryReads(t.Context())
	if _, err := s.AdminDeleteFriendGroupMember(primary, "group", "retiring"); err != nil {
		t.Fatal(err)
	}
	later := s.now().Add(time.Second)
	s.Now = func() time.Time { return later }
	replacement, err := s.AdminCreateFriendGroupMember(primary, "group", "retiring", "guest-room", rpcapi.FriendGroupMemberRoleMember)
	if err != nil {
		t.Fatal(err)
	}
	rejected := store.rejected
	if err := s.RetirePeerGroup(t.Context(), captured[0]); err == nil || store.rejected-rejected != 1 {
		t.Fatalf("replacement membership accepted: rejected=%d error=%v", store.rejected-rejected, err)
	}
	current, err := s.groupMember(primary, "group", "retiring")
	if err != nil || !socialutil.TimeValue(current.CreatedAt).Equal(socialutil.TimeValue(replacement.CreatedAt)) {
		t.Fatalf("replacement membership removed: %+v %v", current, err)
	}
}

func TestPeerGroupRetirementRetryKeepsCapturedWorkspace(t *testing.T) {
	s, store := newReadSplitGroupServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	captured, err := s.SnapshotPeerGroups(t.Context(), "owner")
	if err != nil || len(captured) != 1 {
		t.Fatalf("snapshot=%+v error=%v", captured, err)
	}
	store.replicate = false
	primary := kv.WithPrimaryReads(t.Context())
	binding, err := s.readWorkspaceBinding(primary, "group")
	if err != nil {
		t.Fatal(err)
	}
	binding.WorkspaceID = "replacement-workspace"
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Store.ApplyMutation(primary, kv.Mutation{Entries: []kv.Entry{
		{Key: workspaceBindingKey("group"), Value: data}, {Key: groupRevisionKey("group"), Value: []byte("replacement-revision")},
	}}); err != nil {
		t.Fatal(err)
	}
	rejected := store.rejected
	if err := s.RetirePeerGroup(t.Context(), captured[0]); err == nil || store.rejected-rejected != 1 {
		t.Fatalf("replacement Workspace accepted: rejected=%d error=%v", store.rejected-rejected, err)
	}
	if _, err := s.AdminGetFriendGroup(primary, "group"); err != nil {
		t.Fatalf("replacement Group removed: %v", err)
	}
	if retired := s.Workspaces.(*recordingWorkspaceService).retired; len(retired) != 0 {
		t.Fatalf("replacement Workspace retired: %v", retired)
	}
}

func TestPeerGroupRetirementRejectsUnversionedExistingMembership(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.AdminCreateFriendGroup(t.Context(), "group", "owner", "room", nil, nil); err != nil {
		t.Fatal(err)
	}
	captured, err := s.SnapshotPeerGroups(t.Context(), "owner")
	if err != nil || len(captured) != 1 {
		t.Fatalf("snapshot=%+v error=%v", captured, err)
	}
	captured[0].CreatedAt = nil
	if err := s.RetirePeerGroup(t.Context(), captured[0]); err == nil {
		t.Fatal("old snapshot without membership incarnation deleted an existing Group")
	}
	if _, err := s.AdminGetFriendGroup(t.Context(), "group"); err != nil {
		t.Fatalf("unversioned snapshot removed Group: %v", err)
	}
}
