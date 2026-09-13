package friend

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestFriendLocatorDoesNotScanOrReuseOldWorkspace(t *testing.T) {
	s := newTestServer()
	if _, err := s.AdminCreateFriend(t.Context(), "peer-a", "peer-b"); err != nil {
		t.Fatal(err)
	}
	relation := socialutil.RelationID("peer-a", "peer-b")
	binding, err := readWorkspaceBinding(t.Context(), s.Friends, relation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(t.Context(), binding.WorkspaceID, "peer-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBindingByName(t.Context(), binding.WorkspaceName, "peer-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListSFUWorkspaceBindingsForPeer(t.Context(), "peer-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(t.Context(), "missing-workspace", "peer-a"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatal(err)
	}
	old := binding
	binding.WorkspaceID = "replacement-workspace"
	binding.WorkspaceName = "replacement-name"
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Friends.Set(t.Context(), workspaceBindingKey(relation), data); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(t.Context(), old.WorkspaceID, "peer-a"); !errors.Is(err, sfu.ErrRevoked) {
		t.Fatalf("old Workspace resolved to replacement: %v", err)
	}
	if err := s.Friends.Delete(t.Context(), workspaceBindingKey(relation)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBindingByName(t.Context(), old.WorkspaceName, "peer-a"); !errors.Is(err, sfu.ErrRevoked) {
		t.Fatalf("retired name: %v", err)
	}
}

func TestInviteTokenLookupDoesNotScan(t *testing.T) {
	s := newTestServer()
	created, err := s.CreateFriendInviteToken(t.Context(), "peer-a", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	found, err := s.findInviteToken(t.Context(), created.InviteToken)
	if err != nil || found.PeerPublicKey != "peer-a" {
		t.Fatalf("lookup = %#v, %v", found, err)
	}
	if _, err := s.findInviteToken(t.Context(), "unknown"); !errors.Is(err, ErrInviteTokenUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.ClearFriendInviteToken(t.Context(), "peer-a", rpcapi.FriendInviteTokenClearRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.findInviteToken(t.Context(), created.InviteToken); !errors.Is(err, ErrInviteTokenUnavailable) {
		t.Fatal(err)
	}
}
