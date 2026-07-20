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
	sharedSetupSocialGroupID         = "family-circle"
)

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
	registerRuntimeProfile(t, h, peer, "shared-resources", sharedRuntimeProfileSpec())
	return &sharedSetupRPCHarness{ctx: ctx, h: h, peer: peer}
}

func TestSharedSetupRPCResourcesPagination(t *testing.T) {
	env := newSharedSetupRPCHarness(t)

	workflowNames := collectWorkflowNames(t, env.ctx, env.peer, 25)
	requireName(t, workflowNames, "shared")
	requireName(t, workflowNames, "chatroom")

	workspaceNames := collectWorkspaceNames(t, env.ctx, env.peer, 25)
	requirePrefixCount(t, workspaceNames, "social-", 2)
	if workspaceNames["support-desk-workspace"] {
		t.Fatalf("unowned support Workspace unexpectedly accessible: %#v", workspaceNames)
	}

	modelIDs := collectModelIDs(t, env.ctx, env.peer, 25)
	requireName(t, modelIDs, "fake-openai-chat-000")

	credentialNames := collectCredentialNames(t, env.ctx, env.peer, 25)
	if len(credentialNames) != 0 {
		t.Fatalf("unowned Credentials unexpectedly accessible: %#v", credentialNames)
	}

}

func TestSharedSetupRPCSocialFixtures(t *testing.T) {
	env := newSharedSetupRPCHarness(t)

	if got := env.h.ContextPublicKey("gear1"); got != sharedSetupSocialClientPublicKey {
		t.Skipf("shared social fixture targets default gear1 %s, got %s", sharedSetupSocialClientPublicKey, got)
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
	group := requireFriendGroupID(t, groups.Items, sharedSetupSocialGroupID)
	if group.MyRole == nil || *group.MyRole != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("shared group my_role = %#v, want member", group.MyRole)
	}

	gotGroup, err := env.peer.GetFriendGroup(env.ctx, "shared.social.friend_group.get", rpcapi.FriendGroupGetRequest{Id: sharedSetupSocialGroupID})
	if err != nil {
		t.Fatalf("friend_group.get shared fixture: %v", err)
	}
	if gotGroup.Name == nil || *gotGroup.Name != "Family Circle" {
		t.Fatalf("shared group = %#v", gotGroup)
	}

	members, err := env.peer.ListFriendGroupMembers(env.ctx, "shared.social.friend_group.members.list", rpcapi.FriendGroupMemberListRequest{
		FriendGroupId: testStringPtr(sharedSetupSocialGroupID),
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

func TestSharedSetupRPCMutationFixtures(t *testing.T) {
	env := newSharedSetupRPCHarness(t)

	_, _ = env.peer.DeleteModel(env.ctx, "shared.model.delete.preclean", rpcapi.ModelDeleteRequest{Id: "mutation-openai-model"})
	createdModel, err := env.peer.CreateModel(env.ctx, "shared.model.create", rpcModel("mutation-openai-model", "openai-main"))
	if err != nil {
		t.Fatalf("model.create mutation-openai-model: %v", err)
	}
	if createdModel.Id != "mutation-openai-model" {
		t.Fatalf("model.create = %#v", createdModel)
	}
	if _, err := env.peer.DeleteModel(env.ctx, "shared.model.delete", rpcapi.ModelDeleteRequest{Id: "mutation-openai-model"}); err != nil {
		t.Fatalf("model.delete mutation-openai-model: %v", err)
	}

	_, _ = env.peer.DeleteCredential(env.ctx, "shared.credential.delete.preclean", rpcapi.CredentialDeleteRequest{Name: "mutation-openai-credential"})
	createdCredential, err := env.peer.CreateCredential(env.ctx, "shared.credential.create", rpcCredential("mutation-openai-credential", "sk-mutation-openai"))
	if err != nil {
		t.Fatalf("credential.create mutation-openai-credential: %v", err)
	}
	if createdCredential.Name != "mutation-openai-credential" {
		t.Fatalf("credential.create = %#v", createdCredential)
	}
	if _, err := env.peer.DeleteCredential(env.ctx, "shared.credential.delete", rpcapi.CredentialDeleteRequest{Name: "mutation-openai-credential"}); err != nil {
		t.Fatalf("credential.delete mutation-openai-credential: %v", err)
	}
}

func collectWorkflowNames(t *testing.T, ctx context.Context, peer *gizcli.Client, limit int) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	var cursor *string
	for page := 0; page < 100; page++ {
		list, err := peer.ListWorkflows(ctx, "shared.workflow.list", rpcapi.WorkflowListRequest{Source: rpcapi.ResourceSourceRuntime, Cursor: cursor, Limit: &limit})
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
			names[item.Id] = true
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

func collectCredentialNames(t *testing.T, ctx context.Context, peer *gizcli.Client, limit int) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	var cursor *string
	for page := 0; page < 100; page++ {
		list, err := peer.ListCredentials(ctx, "shared.credential.list", rpcapi.CredentialListRequest{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("credential.list page %d: %v", page, err)
		}
		for _, item := range list.Items {
			names[item.Name] = true
		}
		if !list.HasNext {
			return names
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("credential.list page %d has_next without next cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatal("credential.list pagination did not terminate")
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

func requireFriendGroupID(t *testing.T, items []rpcapi.FriendGroupObject, id string) rpcapi.FriendGroupObject {
	t.Helper()
	for _, item := range items {
		if item.Id != nil && *item.Id == id {
			return item
		}
	}
	t.Fatalf("missing friend group %q in %#v", id, items)
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
