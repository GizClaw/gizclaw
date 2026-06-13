package gizcli

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func TestClientResourceMethodsRequireConnection(t *testing.T) {
	client := &Client{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() (any, error)
	}{
		{"workspace list", func() (any, error) {
			return client.ListWorkspaces(ctx, "workspace-list", rpcapi.WorkspaceListRequest{})
		}},
		{"workspace get", func() (any, error) {
			return client.GetWorkspace(ctx, "workspace-get", rpcapi.WorkspaceGetRequest{Name: "workspace-a"})
		}},
		{"workspace create", func() (any, error) {
			return client.CreateWorkspace(ctx, "workspace-create", rpcapi.WorkspaceCreateRequest{Name: "workspace-a", WorkflowName: "flow-a"})
		}},
		{"workspace put", func() (any, error) {
			return client.PutWorkspace(ctx, "workspace-put", rpcapi.WorkspacePutRequest{Name: "workspace-a", Body: rpcapi.Workspace{Name: "workspace-a", WorkflowName: "flow-a"}})
		}},
		{"workspace delete", func() (any, error) {
			return client.DeleteWorkspace(ctx, "workspace-delete", rpcapi.WorkspaceDeleteRequest{Name: "workspace-a"})
		}},
		{"workflow list", func() (any, error) { return client.ListWorkflows(ctx, "workflow-list", rpcapi.WorkflowListRequest{}) }},
		{"workflow get", func() (any, error) {
			return client.GetWorkflow(ctx, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow-a"})
		}},
		{"workflow create", func() (any, error) {
			return client.CreateWorkflow(ctx, "workflow-create", resourceWorkflowDoc("flow-a"))
		}},
		{"workflow put", func() (any, error) {
			return client.PutWorkflow(ctx, "workflow-put", rpcapi.WorkflowPutRequest{Name: "flow-a", Body: resourceWorkflowDoc("flow-a")})
		}},
		{"workflow delete", func() (any, error) {
			return client.DeleteWorkflow(ctx, "workflow-delete", rpcapi.WorkflowDeleteRequest{Name: "flow-a"})
		}},
		{"model list", func() (any, error) { return client.ListModels(ctx, "model-list", rpcapi.ModelListRequest{}) }},
		{"model get", func() (any, error) { return client.GetModel(ctx, "model-get", rpcapi.ModelGetRequest{Id: "model-a"}) }},
		{"model create", func() (any, error) { return client.CreateModel(ctx, "model-create", resourceModel("model-a")) }},
		{"model put", func() (any, error) {
			return client.PutModel(ctx, "model-put", rpcapi.ModelPutRequest{Id: "model-a", Body: resourceModel("model-a")})
		}},
		{"model delete", func() (any, error) {
			return client.DeleteModel(ctx, "model-delete", rpcapi.ModelDeleteRequest{Id: "model-a"})
		}},
		{"credential list", func() (any, error) {
			return client.ListCredentials(ctx, "credential-list", rpcapi.CredentialListRequest{})
		}},
		{"credential get", func() (any, error) {
			return client.GetCredential(ctx, "credential-get", rpcapi.CredentialGetRequest{Name: "credential-a"})
		}},
		{"credential create", func() (any, error) {
			return client.CreateCredential(ctx, "credential-create", resourceCredential("credential-a"))
		}},
		{"credential put", func() (any, error) {
			return client.PutCredential(ctx, "credential-put", rpcapi.CredentialPutRequest{Name: "credential-a", Body: resourceCredential("credential-a")})
		}},
		{"credential delete", func() (any, error) {
			return client.DeleteCredential(ctx, "credential-delete", rpcapi.CredentialDeleteRequest{Name: "credential-a"})
		}},
		{"contact list", func() (any, error) { return client.ListContacts(ctx, "contact-list", rpcapi.ContactListRequest{}) }},
		{"contact get", func() (any, error) {
			return client.GetContact(ctx, "contact-get", rpcapi.ContactGetRequest{Id: "contact-a"})
		}},
		{"contact create", func() (any, error) { return client.CreateContact(ctx, "contact-create", rpcapi.ContactCreateRequest{}) }},
		{"contact put", func() (any, error) {
			return client.PutContact(ctx, "contact-put", rpcapi.ContactPutRequest{Id: "contact-a"})
		}},
		{"contact delete", func() (any, error) {
			return client.DeleteContact(ctx, "contact-delete", rpcapi.ContactDeleteRequest{Id: "contact-a"})
		}},
		{"friend requests list", func() (any, error) {
			return client.ListFriendRequests(ctx, "friend-requests-list", rpcapi.FriendRequestListRequest{})
		}},
		{"friend requests create", func() (any, error) {
			return client.CreateFriendRequest(ctx, "friend-requests-create", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"})
		}},
		{"friend requests accept", func() (any, error) {
			return client.AcceptFriendRequest(ctx, "friend-requests-accept", rpcapi.FriendRequestAcceptRequest{Id: "request-a"})
		}},
		{"friend requests reject", func() (any, error) {
			return client.RejectFriendRequest(ctx, "friend-requests-reject", rpcapi.FriendRequestRejectRequest{Id: "request-a"})
		}},
		{"friend list", func() (any, error) { return client.ListFriends(ctx, "friend-list", rpcapi.FriendListRequest{}) }},
		{"friend delete", func() (any, error) {
			return client.DeleteFriend(ctx, "friend-delete", rpcapi.FriendDeleteRequest{Id: "friend-a"})
		}},
		{"friend group list", func() (any, error) {
			return client.ListFriendGroups(ctx, "friend-group-list", rpcapi.FriendGroupListRequest{})
		}},
		{"friend group get", func() (any, error) {
			return client.GetFriendGroup(ctx, "friend-group-get", rpcapi.FriendGroupGetRequest{Id: "group-a"})
		}},
		{"friend group create", func() (any, error) {
			return client.CreateFriendGroup(ctx, "friend-group-create", rpcapi.FriendGroupCreateRequest{Name: "family"})
		}},
		{"friend group put", func() (any, error) {
			return client.PutFriendGroup(ctx, "friend-group-put", rpcapi.FriendGroupPutRequest{Id: "group-a"})
		}},
		{"friend group delete", func() (any, error) {
			return client.DeleteFriendGroup(ctx, "friend-group-delete", rpcapi.FriendGroupDeleteRequest{Id: "group-a"})
		}},
		{"friend group members list", func() (any, error) {
			return client.ListFriendGroupMembers(ctx, "friend-group-members-list", rpcapi.FriendGroupMemberListRequest{})
		}},
		{"friend group members add", func() (any, error) {
			return client.AddFriendGroupMember(ctx, "friend-group-members-add", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: "group-a", PeerId: "peer-b"})
		}},
		{"friend group members put", func() (any, error) {
			return client.PutFriendGroupMember(ctx, "friend-group-members-put", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: "group-a", Id: "peer-b"})
		}},
		{"friend group members delete", func() (any, error) {
			return client.DeleteFriendGroupMember(ctx, "friend-group-members-delete", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: "group-a", Id: "peer-b"})
		}},
		{"friend group messages list", func() (any, error) {
			return client.ListFriendGroupMessages(ctx, "friend-group-messages-list", rpcapi.FriendGroupMessageListRequest{})
		}},
		{"friend group messages get", func() (any, error) {
			return client.GetFriendGroupMessage(ctx, "friend-group-messages-get", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: "group-a", Id: "message-a"})
		}},
		{"friend group messages send", func() (any, error) {
			return client.SendFriendGroupMessage(ctx, "friend-group-messages-send", rpcapi.FriendGroupMessageSendRequest{FriendGroupId: "group-a", AudioContentType: "audio/opus"})
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
