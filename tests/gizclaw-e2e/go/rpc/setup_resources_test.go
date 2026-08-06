//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

type sharedSetupRPCHarness struct {
	ctx  context.Context
	h    *clitest.Harness
	peer *gizcli.Client
}

const (
	sharedSetupSocialAdminPublicKey  = "6Ww6ANsXDCf91Yp7Tvi65hqpywjMmXqAoZDiq33kfCee"
	sharedSetupSocialClientPublicKey = "8rAUkTyxLHDa5o3VajtzWcQdNJq1thrjAGtpwQkEsaEu"
)

var sharedSetupSocialGroupName string

func newSharedSetupRPCHarness(t *testing.T) *sharedSetupRPCHarness {
	t.Helper()

	h := clitest.NewSetupHarness(t, "client-rpc-shared-resources")
	identitiesHome := getenvDefault("GIZCLAW_E2E_IDENTITIES_HOME", filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities"))
	contextName := getenvDefault("GIZCLAW_E2E_PEER_IDENTITY", "peer")
	h.SetContextDirAlias("gear1", filepath.Join(identitiesHome, contextName))
	adminContextName := getenvDefault("GIZCLAW_E2E_ADMIN_IDENTITY", "admin")
	h.SetContextDirAlias("admin-a", filepath.Join(identitiesHome, adminContextName))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("gear1")
	t.Cleanup(func() { peer.Close() })
	registerRuntimeProfile(t, h, peer, "shared-resources", sharedRuntimeProfileSpec(t))
	profileID, token := provisionRuntimeProfile(t, h, "shared-social-admin", sharedRuntimeProfileSpec(t))
	admin := h.ConnectClientFromContext("admin-a")
	t.Cleanup(func() { admin.Close() })
	registerWithRuntimeProfile(t, admin, "shared-social-admin", profileID, token)
	applySharedSocialFixtures(t, h)
	return &sharedSetupRPCHarness{ctx: ctx, h: h, peer: peer}
}

func applySharedSocialFixtures(t *testing.T, h *clitest.Harness) {
	t.Helper()
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create shared Social Admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ids, err := clitest.EnsureSocialFixture(ctx, api, sharedSetupSocialAdminPublicKey, sharedSetupSocialClientPublicKey)
	if err != nil {
		t.Fatalf("ensure shared Social fixtures: %v", err)
	}
	sharedSetupSocialGroupName = ids.FriendGroupName
}

func TestSharedSetupRPCResourcesPagination(t *testing.T) {
	env := newSharedSetupRPCHarness(t)

	workflowNames := collectWorkflowNames(t, env.ctx, env.peer, 25)
	requireName(t, workflowNames, "shared")
	requireName(t, workflowNames, "chatroom")

	modelIDs := collectModelIDs(t, env.ctx, env.peer, 25)
	requireName(t, modelIDs, "llm")
}

func TestSharedSetupRPCSocialFixtures(t *testing.T) {
	env := newSharedSetupRPCHarness(t)

	if got := env.h.ContextPublicKey("gear1"); got != sharedSetupSocialClientPublicKey {
		t.Fatalf("shared social fixture targets default gear1 %s, got %s", sharedSetupSocialClientPublicKey, got)
	}

	friends, err := env.peer.ListFriends(env.ctx, "shared.social.friend.list", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("friend.list shared fixture: %v", err)
	}
	friend := requireFriendPeer(t, friends.Items, sharedSetupSocialAdminPublicKey)
	if friend.WorkspaceName == nil || *friend.WorkspaceName == "" {
		t.Fatalf("shared friend workspace is empty: %#v", friend)
	}

	groups, err := env.peer.ListFriendGroups(env.ctx, "shared.social.friend_group.list", rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("friend_group.list shared fixture: %v", err)
	}
	group := requireFriendGroupName(t, groups.Items, sharedSetupSocialGroupName)
	if group.MyRole == nil || *group.MyRole != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("shared group my_role = %#v, want member", group.MyRole)
	}

	gotGroup, err := env.peer.GetFriendGroup(env.ctx, "shared.social.friend_group.get", rpcapi.FriendGroupGetRequest{Name: sharedSetupSocialGroupName})
	if err != nil {
		t.Fatalf("friend_group.get shared fixture: %v", err)
	}
	if gotGroup.Name != sharedSetupSocialGroupName || gotGroup.DisplayName == nil || *gotGroup.DisplayName != "Family Circle" {
		t.Fatalf("shared group = %#v", gotGroup)
	}

	members, err := env.peer.ListFriendGroupMembers(env.ctx, "shared.social.friend_group.members.list", rpcapi.FriendGroupMemberListRequest{
		FriendGroupName: testStringPtr(sharedSetupSocialGroupName),
	})
	if err != nil {
		t.Fatalf("friend_group.members.list shared fixture: %v", err)
	}
	member := requireFriendGroupMemberPeer(t, members.Items, sharedSetupSocialClientPublicKey)
	if member.Role == nil || *member.Role != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("shared member role = %#v, want member", member.Role)
	}
	if sharedStringValue(friend.WorkspaceName) == "" || sharedStringValue(group.WorkspaceName) == "" {
		t.Fatalf("shared social workspaces are empty: friend=%#v group=%#v", friend.WorkspaceName, group.WorkspaceName)
	}
}

func collectWorkflowNames(t *testing.T, ctx context.Context, peer *gizcli.Client, limit int) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	var cursor *string
	for page := 0; page < 100; page++ {
		list, err := peer.ListWorkflows(ctx, "shared.workflow.list", rpcapi.WorkflowListRequest{Collection: "assistants", Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("workflow.list page %d: %v", page, err)
		}
		for _, item := range list.Items {
			names[item.Name] = true
		}
		if !list.HasNext {
			return names
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("workflow.list page %d has_next without next cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatal("workflow.list pagination did not terminate")
	return names
}

func collectWorkspaceNames(t *testing.T, ctx context.Context, peer *gizcli.Client, limit int) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	var cursor *string
	for page := 0; page < 100; page++ {
		list, err := peer.ListWorkspaces(ctx, "shared.workspace.list", rpcapi.WorkspaceListRequest{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("workspace.list page %d: %v", page, err)
		}
		for _, item := range list.Items {
			names[item.Name] = true
		}
		if !list.HasNext {
			return names
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("workspace.list page %d has_next without next cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatal("workspace.list pagination did not terminate")
	return names
}

func collectModelIDs(t *testing.T, ctx context.Context, peer *gizcli.Client, limit int) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	var cursor *string
	for page := 0; page < 100; page++ {
		list, err := peer.ListModels(ctx, "shared.model.list", rpcapi.ModelListRequest{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("model.list page %d: %v", page, err)
		}
		for _, item := range list.Items {
			names[item.Name] = true
		}
		if !list.HasNext {
			return names
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("model.list page %d has_next without next cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatal("model.list pagination did not terminate")
	return names
}

func requireName(t *testing.T, names map[string]bool, name string) {
	t.Helper()
	if !names[name] {
		t.Fatalf("missing %q in names map with %d entries", name, len(names))
	}
}

func requireFriendPeer(t *testing.T, items []rpcapi.FriendObject, peerPublicKey string) rpcapi.FriendObject {
	t.Helper()
	for _, item := range items {
		if item.PeerPublicKey != nil && *item.PeerPublicKey == peerPublicKey {
			return item
		}
	}
	t.Fatalf("missing friend peer %q in %#v", peerPublicKey, items)
	return rpcapi.FriendObject{}
}

func requireFriendGroupName(t *testing.T, items []rpcapi.FriendGroupObject, name string) rpcapi.FriendGroupObject {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("missing friend group %q in %#v", name, items)
	return rpcapi.FriendGroupObject{}
}

func requireFriendGroupMemberPeer(t *testing.T, items []rpcapi.FriendGroupMemberObject, peerPublicKey string) rpcapi.FriendGroupMemberObject {
	t.Helper()
	for _, item := range items {
		if item.PeerPublicKey != nil && *item.PeerPublicKey == peerPublicKey {
			return item
		}
	}
	t.Fatalf("missing friend group member %q in %#v", peerPublicKey, items)
	return rpcapi.FriendGroupMemberObject{}
}

func sharedStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func requirePrefixCount(t *testing.T, names map[string]bool, prefix string, want int) {
	t.Helper()
	got := 0
	for name := range names {
		if strings.HasPrefix(name, prefix) {
			got++
		}
	}
	if got < want {
		t.Fatalf("prefix %q count = %d, want at least %d", prefix, got, want)
	}
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
