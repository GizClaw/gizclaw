package friendgroup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// fillPeerGroups makes peer belong to socialutil.PeerFriendGroupLimit groups:
// some it created, some it joined, and some another Peer added it to.
func fillPeerGroups(t *testing.T, s *Server, peer string) {
	t.Helper()
	ctx := t.Context()
	for index := range socialutil.PeerFriendGroupLimit {
		name := fmt.Sprintf("room-%02d", index)
		switch index % 3 {
		case 0:
			if _, err := s.CreateFriendGroup(ctx, peer, rpcapi.FriendGroupCreateRequest{Name: name}); err != nil {
				t.Fatalf("CreateFriendGroup(%s): %v", name, err)
			}
		case 1:
			if _, err := s.CreateFriendGroup(ctx, "other", rpcapi.FriendGroupCreateRequest{Name: name}); err != nil {
				t.Fatal(err)
			}
			token, err := s.CreateFriendGroupInviteToken(ctx, "other", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: name})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.JoinFriendGroup(ctx, peer, rpcapi.FriendGroupJoinRequest{Name: name, InviteToken: token.InviteToken}); err != nil {
				t.Fatalf("JoinFriendGroup(%s): %v", name, err)
			}
		default:
			if _, err := s.CreateFriendGroup(ctx, "other", rpcapi.FriendGroupCreateRequest{Name: name}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AddFriendGroupMember(ctx, "other", rpcapi.FriendGroupMemberAddRequest{
				FriendGroupName: name, PeerPublicKey: peer, MemberName: name, Role: rpcapi.FriendGroupMemberMutableRole("member"),
			}); err != nil {
				t.Fatalf("AddFriendGroupMember(%s): %v", name, err)
			}
		}
	}
}

func TestPeerFriendGroupLimitCountsCreatedJoinedAndAddedGroups(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	fillPeerGroups(t, s, "peer")
	groups, err := s.Belongs.ListMembers(ctx, belongCollectionKey("peer"))
	if err != nil || len(groups) != socialutil.PeerFriendGroupLimit {
		t.Fatalf("peer groups = %d, %v, want %d", len(groups), err, socialutil.PeerFriendGroupLimit)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	createdBefore := len(workspaces.created)
	if _, err := s.CreateFriendGroup(ctx, "peer", rpcapi.FriendGroupCreateRequest{Name: "extra"}); !errors.Is(err, ErrPeerFriendGroupLimit) {
		t.Fatalf("CreateFriendGroup(11th) error = %v, want %v", err, ErrPeerFriendGroupLimit)
	}
	if _, err := s.AdminCreateFriendGroup(ctx, "admin-extra", "peer", "extra", nil, nil); !errors.Is(err, ErrPeerFriendGroupLimit) {
		t.Fatalf("AdminCreateFriendGroup(11th) error = %v, want %v", err, ErrPeerFriendGroupLimit)
	}
	if len(workspaces.created) != createdBefore {
		t.Fatalf("rejected creations made Workspaces: %v", workspaces.created[createdBefore:])
	}

	if _, err := s.CreateFriendGroup(ctx, "other", rpcapi.FriendGroupCreateRequest{Name: "extra"}); err != nil {
		t.Fatal(err)
	}
	extraID := mustGroupID(t, s, "other", "extra")
	token, err := s.CreateFriendGroupInviteToken(ctx, "other", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "extra"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinFriendGroup(ctx, "peer", rpcapi.FriendGroupJoinRequest{Name: "extra", InviteToken: token.InviteToken}); !errors.Is(err, ErrPeerFriendGroupLimit) {
		t.Fatalf("JoinFriendGroup(11th) error = %v, want %v", err, ErrPeerFriendGroupLimit)
	}
	if _, err := s.AddFriendGroupMember(ctx, "other", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: "extra", PeerPublicKey: "peer", MemberName: "extra", Role: rpcapi.FriendGroupMemberMutableRole("member"),
	}); !errors.Is(err, ErrPeerFriendGroupLimit) {
		t.Fatalf("AddFriendGroupMember(11th) error = %v, want %v", err, ErrPeerFriendGroupLimit)
	}
	if _, err := s.AdminCreateFriendGroupMember(ctx, extraID, "peer", "extra", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, ErrPeerFriendGroupLimit) {
		t.Fatalf("AdminCreateFriendGroupMember(11th) error = %v, want %v", err, ErrPeerFriendGroupLimit)
	}
	if _, err := s.AdminPutFriendGroupMember(ctx, extraID, "peer", "extra", rpcapi.FriendGroupMemberRoleMember); !errors.Is(err, ErrPeerFriendGroupLimit) {
		t.Fatalf("AdminPutFriendGroupMember(11th) error = %v, want %v", err, ErrPeerFriendGroupLimit)
	}
	if _, err := s.groupMember(ctx, extraID, "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("rejected member stored: %v", err)
	}
	// Existing memberships stay editable at the limit.
	if _, err := s.AddFriendGroupMember(ctx, "other", rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: "room-02", PeerPublicKey: "peer", MemberName: "room-02", Role: rpcapi.FriendGroupMemberMutableRole("admin"),
	}); err != nil {
		t.Fatalf("role change at limit: %v", err)
	}

	if _, err := s.DeleteFriendGroupMember(ctx, "peer", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: "room-01", Name: "peer"}); err != nil {
		t.Fatalf("leave group: %v", err)
	}
	if _, err := s.JoinFriendGroup(ctx, "peer", rpcapi.FriendGroupJoinRequest{Name: "extra", InviteToken: token.InviteToken}); err != nil {
		t.Fatalf("JoinFriendGroup after leaving one: %v", err)
	}
}

// revisionRacingStore moves a Peer's group revision just before the first
// admission for that Peer commits, as a concurrent admission on another
// Server would.
type revisionRacingStore struct {
	kv.Store
	key  kv.Key
	once sync.Once
}

func (s *revisionRacingStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	for _, condition := range mutation.Conditions {
		if slices.Equal(condition.Key, s.key) {
			var err error
			s.once.Do(func() { err = s.Store.Set(ctx, s.key, []byte("raced")) })
			if err != nil {
				return false, err
			}
		}
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestPeerFriendGroupAdmissionRejectsStalePeerRevision(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	if _, err := s.CreateFriendGroup(ctx, "owner", rpcapi.FriendGroupCreateRequest{Name: "room"}); err != nil {
		t.Fatal(err)
	}
	racing := &revisionRacingStore{Store: s.Groups, key: peerGroupRevisionKey("peer")}
	s.Groups, s.InviteTokens, s.Members, s.Belongs, s.RelationshipStore = racing, racing, racing, racing, racing
	request := rpcapi.FriendGroupMemberAddRequest{FriendGroupName: "room", PeerPublicKey: "peer", MemberName: "room", Role: rpcapi.FriendGroupMemberMutableRole("member")}
	if _, err := s.AddFriendGroupMember(ctx, "owner", request); !errors.Is(err, ErrGroupChanged) {
		t.Fatalf("AddFriendGroupMember(stale peer revision) error = %v, want %v", err, ErrGroupChanged)
	}
	if _, err := s.AddFriendGroupMember(ctx, "owner", request); err != nil {
		t.Fatalf("AddFriendGroupMember(retry) error = %v", err)
	}

	racing.key, racing.once = peerGroupRevisionKey("creator"), sync.Once{}
	if _, err := s.CreateFriendGroup(ctx, "creator", rpcapi.FriendGroupCreateRequest{Name: "mine"}); !errors.Is(err, ErrGroupChanged) {
		t.Fatalf("CreateFriendGroup(stale peer revision) error = %v, want %v", err, ErrGroupChanged)
	}
	if _, err := s.CreateFriendGroup(ctx, "creator", rpcapi.FriendGroupCreateRequest{Name: "mine"}); err != nil {
		t.Fatalf("CreateFriendGroup(retry) error = %v", err)
	}
}
