package friendgroup

import (
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestGroupLocatorAndMembersDoNotScan(t *testing.T) {
	s := newTestServer(t)
	group, err := s.CreateFriendGroup(t.Context(), "owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	id := mustGroupID(t, s, "owner", group.Name)
	binding, err := s.readWorkspaceBinding(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(t.Context(), binding.WorkspaceID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBindingByName(t.Context(), binding.WorkspaceName, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListSFUWorkspaceBindingsForPeer(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(t.Context(), "missing-workspace", "owner"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.RelationshipStore.Delete(t.Context(), workspaceBindingKey(id)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSFUWorkspaceBinding(t.Context(), binding.WorkspaceID, "owner"); !errors.Is(err, sfu.ErrRevoked) {
		t.Fatalf("retired group: %v", err)
	}
}
