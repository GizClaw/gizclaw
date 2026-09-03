package gizcli

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestClientResourceMethodsRequireConnection(t *testing.T) {
	client := &Client{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() (any, error)
	}{
		{"API key create", func() (any, error) {
			return client.CreateAPIKey(ctx, "api-key-create", rpcapi.APIKeyCreateRequest{DisplayName: "phone"})
		}},
		{"API key list", func() (any, error) {
			return client.ListAPIKeys(ctx, "api-key-list", rpcapi.APIKeyListRequest{Limit: 25})
		}},
		{"API key revoke", func() (any, error) {
			return client.RevokeAPIKey(ctx, "api-key-revoke", rpcapi.APIKeyRevokeRequest{Name: "key-a"})
		}},
		{"workspace list", func() (any, error) {
			return client.ListWorkspaces(ctx, "workspace-list", rpcapi.WorkspaceListRequest{Collection: "assistants"})
		}},
		{"workspace get", func() (any, error) {
			return client.GetWorkspace(ctx, "workspace-get", rpcapi.WorkspaceGetRequest{Name: "workspace-a"})
		}},
		{"workspace create", func() (any, error) {
			return client.CreateWorkspace(ctx, "workspace-create", rpcapi.WorkspaceCreateRequest{Name: "workspace-a", Collection: "assistants", WorkflowName: "flow-a"})
		}},
		{"workspace put", func() (any, error) {
			return client.PutWorkspace(ctx, "workspace-put", rpcapi.WorkspacePutRequest{Name: "workspace-a", Body: rpcapi.WorkspacePutBody{}})
		}},
		{"workspace delete", func() (any, error) {
			return client.DeleteWorkspace(ctx, "workspace-delete", rpcapi.WorkspaceDeleteRequest{Name: "workspace-a"})
		}},
		{"workspace history list", func() (any, error) {
			return client.ListWorkspaceHistory(ctx, "workspace-history-list", rpcapi.WorkspaceHistoryListRequest{WorkspaceName: "workspace-a"})
		}},
		{"workspace history get", func() (any, error) {
			return client.GetWorkspaceHistory(ctx, "workspace-history-get", rpcapi.WorkspaceHistoryGetRequest{WorkspaceName: "workspace-a", HistoryName: "history-a"})
		}},
		{"workspace history audio download", func() (any, error) {
			var out strings.Builder
			return client.DownloadWorkspaceHistoryAudio(ctx, "workspace-history-audio-download", rpcapi.WorkspaceHistoryAudioDownloadRequest{WorkspaceName: "workspace-a", HistoryName: "history-a"}, &out)
		}},
		{"workflow list", func() (any, error) {
			return client.ListWorkflows(ctx, "workflow-list", rpcapi.WorkflowListRequest{Collection: "assistants"})
		}},
		{"workflow get", func() (any, error) {
			return client.GetWorkflow(ctx, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow-a"})
		}},
		{"model list", func() (any, error) { return client.ListModels(ctx, "model-list", rpcapi.ModelListRequest{}) }},
		{"model get", func() (any, error) {
			return client.GetModel(ctx, "model-get", rpcapi.ModelGetRequest{Name: "model-a"})
		}},
		{"contact list", func() (any, error) { return client.ListContacts(ctx, "contact-list", rpcapi.ContactListRequest{}) }},
		{"contact get", func() (any, error) {
			return client.GetContact(ctx, "contact-get", rpcapi.ContactGetRequest{Name: "contact-a"})
		}},
		{"contact create", func() (any, error) { return client.CreateContact(ctx, "contact-create", rpcapi.ContactCreateRequest{}) }},
		{"contact put", func() (any, error) {
			return client.PutContact(ctx, "contact-put", rpcapi.ContactPutRequest{Name: "contact-a"})
		}},
		{"contact delete", func() (any, error) {
			return client.DeleteContact(ctx, "contact-delete", rpcapi.ContactDeleteRequest{Name: "contact-a"})
		}},
		{"friend invite token get", func() (any, error) {
			return client.GetFriendInviteToken(ctx, "friend-invite-token-get", rpcapi.FriendInviteTokenGetRequest{})
		}},
		{"friend invite token create", func() (any, error) {
			return client.CreateFriendInviteToken(ctx, "friend-invite-token-create", rpcapi.FriendInviteTokenCreateRequest{})
		}},
		{"friend invite token clear", func() (any, error) {
			return client.ClearFriendInviteToken(ctx, "friend-invite-token-clear", rpcapi.FriendInviteTokenClearRequest{})
		}},
		{"friend add", func() (any, error) {
			return client.AddFriend(ctx, "friend-add", rpcapi.FriendAddRequest{InviteToken: "token-a"})
		}},
		{"friend list", func() (any, error) { return client.ListFriends(ctx, "friend-list", rpcapi.FriendListRequest{}) }},
		{"friend delete", func() (any, error) {
			return client.DeleteFriend(ctx, "friend-delete", rpcapi.FriendDeleteRequest{Name: "friend-a"})
		}},
		{"friend group list", func() (any, error) {
			return client.ListFriendGroups(ctx, "friend-group-list", rpcapi.FriendGroupListRequest{})
		}},
		{"friend group get", func() (any, error) {
			return client.GetFriendGroup(ctx, "friend-group-get", rpcapi.FriendGroupGetRequest{Name: "group-a"})
		}},
		{"friend group create", func() (any, error) {
			return client.CreateFriendGroup(ctx, "friend-group-create", rpcapi.FriendGroupCreateRequest{Name: "family"})
		}},
		{"friend group put", func() (any, error) {
			return client.PutFriendGroup(ctx, "friend-group-put", rpcapi.FriendGroupPutRequest{Name: "group-a"})
		}},
		{"friend group delete", func() (any, error) {
			return client.DeleteFriendGroup(ctx, "friend-group-delete", rpcapi.FriendGroupDeleteRequest{Name: "group-a"})
		}},
		{"friend group invite token get", func() (any, error) {
			return client.GetFriendGroupInviteToken(ctx, "friend-group-invite-token-get", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupName: "group-a"})
		}},
		{"friend group invite token create", func() (any, error) {
			return client.CreateFriendGroupInviteToken(ctx, "friend-group-invite-token-create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "group-a"})
		}},
		{"friend group invite token clear", func() (any, error) {
			return client.ClearFriendGroupInviteToken(ctx, "friend-group-invite-token-clear", rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupName: "group-a"})
		}},
		{"friend group join", func() (any, error) {
			return client.JoinFriendGroup(ctx, "friend-group-join", rpcapi.FriendGroupJoinRequest{InviteToken: "token-a"})
		}},
		{"friend group members list", func() (any, error) {
			return client.ListFriendGroupMembers(ctx, "friend-group-members-list", rpcapi.FriendGroupMemberListRequest{})
		}},
		{"friend group members add", func() (any, error) {
			return client.AddFriendGroupMember(ctx, "friend-group-members-add", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: "group-a", PeerPublicKey: "peer-b"})
		}},
		{"friend group members put", func() (any, error) {
			return client.PutFriendGroupMember(ctx, "friend-group-members-put", rpcapi.FriendGroupMemberPutRequest{FriendGroupName: "group-a", Name: "peer-b"})
		}},
		{"friend group members delete", func() (any, error) {
			return client.DeleteFriendGroupMember(ctx, "friend-group-members-delete", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: "group-a", Name: "peer-b"})
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.call(); err == nil || !strings.Contains(err.Error(), "client is not connected") {
				t.Fatalf("resource client call error = %v", err)
			}
		})
	}
}
