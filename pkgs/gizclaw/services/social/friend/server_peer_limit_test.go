package friend

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

func newPeerLimitTestServer() *Server {
	s := newTestServer()
	nextID := 0
	s.NewID = func() string {
		nextID++
		return fmt.Sprintf("id-%03d", nextID)
	}
	return s
}

func friendCount(t *testing.T, s *Server, peer string) int {
	t.Helper()
	friends, err := s.Friends.ListMembers(t.Context(), friendCollectionKey(peer))
	if err != nil {
		t.Fatal(err)
	}
	return len(friends)
}

func TestPeerFriendLimitCountsBothDirections(t *testing.T) {
	ctx := t.Context()
	s := newPeerLimitTestServer()
	for index := range socialutil.PeerFriendLimit {
		other := fmt.Sprintf("other-%02d", index)
		inviter, invitee := other, "peer"
		if index%2 == 1 {
			inviter, invitee = "peer", other
		}
		token, err := s.CreateFriendInviteToken(ctx, inviter, rpcapi.FriendInviteTokenCreateRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddFriend(ctx, invitee, rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err != nil {
			t.Fatalf("AddFriend(%s -> %s): %v", invitee, inviter, err)
		}
	}
	if got := friendCount(t, s, "peer"); got != socialutil.PeerFriendLimit {
		t.Fatalf("peer friends = %d, want %d", got, socialutil.PeerFriendLimit)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	createdBefore := len(workspaces.created)

	strangerToken, err := s.CreateFriendInviteToken(ctx, "stranger", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFriend(ctx, "peer", rpcapi.FriendAddRequest{InviteToken: strangerToken.InviteToken}); !errors.Is(err, ErrPeerFriendLimit) {
		t.Fatalf("peer adds 11th friend error = %v, want %v", err, ErrPeerFriendLimit)
	}
	peerToken, err := s.CreateFriendInviteToken(ctx, "peer", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFriend(ctx, "stranger", rpcapi.FriendAddRequest{InviteToken: peerToken.InviteToken}); !errors.Is(err, ErrPeerFriendLimit) {
		t.Fatalf("stranger adds full peer error = %v, want %v", err, ErrPeerFriendLimit)
	}
	if _, err := s.AdminCreateFriend(ctx, "stranger", "peer"); !errors.Is(err, ErrPeerFriendLimit) {
		t.Fatalf("AdminCreateFriend(full peer) error = %v, want %v", err, ErrPeerFriendLimit)
	}
	if len(workspaces.created) != createdBefore {
		t.Fatalf("rejected creations made Workspaces: %v", workspaces.created[createdBefore:])
	}
	if friendCount(t, s, "stranger") != 0 {
		t.Fatal("rejected creation stored a relationship")
	}
	// Re-adding an existing Friend at the limit stays idempotent.
	if _, err := s.AdminCreateFriend(ctx, "peer", "other-00"); err != nil {
		t.Fatalf("AdminCreateFriend(existing at limit) error = %v", err)
	}

	if _, err := s.DeleteFriend(ctx, "peer", rpcapi.FriendDeleteRequest{Name: "other-00"}); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	if _, err := s.AddFriend(ctx, "stranger", rpcapi.FriendAddRequest{InviteToken: peerToken.InviteToken}); err != nil {
		t.Fatalf("AddFriend after deleting one: %v", err)
	}
}

func TestPeerFriendLimitAtCommitCancelsPendingCreation(t *testing.T) {
	ctx := t.Context()
	s := newPeerLimitTestServer()
	intent, err := s.getOrCreateCreationIntent(ctx, s.Friends, "peer", "late", "peer")
	if err != nil {
		t.Fatal(err)
	}
	for index := range socialutil.PeerFriendLimit {
		if _, err := s.AdminCreateFriend(ctx, "peer", fmt.Sprintf("other-%02d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReconcileCreationIntents(ctx); err != nil {
		t.Fatalf("ReconcileCreationIntents: %v", err)
	}
	if _, err := readCreationIntent(ctx, s.Friends, intent.RelationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("creation intent after limit rejection error = %v, want not found", err)
	}
	if _, active, err := readActiveRelationship(ctx, s.Friends, "peer", "late"); err != nil || active {
		t.Fatalf("relationship after limit rejection active = %v, %v", active, err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	if !slices.Contains(workspaces.deleted, intent.Workspace) {
		t.Fatalf("deleted Workspaces = %v, want %s", workspaces.deleted, intent.Workspace)
	}
	if got := friendCount(t, s, "peer"); got != socialutil.PeerFriendLimit {
		t.Fatalf("peer friends = %d, want %d", got, socialutil.PeerFriendLimit)
	}
}

// friendRevisionRacingStore moves a Peer's Friend revision just before the
// first creation for that Peer commits, as a concurrent creation on another
// Server would.
type friendRevisionRacingStore struct {
	kv.Store
	key  kv.Key
	once sync.Once
	hits int
}

func (s *friendRevisionRacingStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	for _, condition := range mutation.Conditions {
		if slices.Equal(condition.Key, s.key) {
			var err error
			s.once.Do(func() {
				s.hits++
				err = s.Store.Set(ctx, s.key, []byte("raced"))
			})
			if err != nil {
				return false, err
			}
		}
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestPeerFriendCreationRetriesStalePeerRevision(t *testing.T) {
	ctx := t.Context()
	s := newPeerLimitTestServer()
	racing := &friendRevisionRacingStore{Store: s.Friends, key: peerFriendRevisionKey("peer")}
	s.Friends = racing
	if _, err := s.AdminCreateFriend(ctx, "peer", "other"); err != nil {
		t.Fatalf("AdminCreateFriend after revision race: %v", err)
	}
	if racing.hits != 1 {
		t.Fatalf("revision race hits = %d, want 1", racing.hits)
	}
	if _, active, err := readActiveRelationship(ctx, s.Friends, "peer", "other"); err != nil || !active {
		t.Fatalf("relationship active = %v, %v", active, err)
	}
	revision, err := s.Friends.Get(ctx, peerFriendRevisionKey("peer"))
	if err != nil || string(revision) == "raced" {
		t.Fatalf("peer revision = %q, %v, want advanced by the commit", revision, err)
	}
}
