package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// Register applies a pre-distributed RegistrationToken to the current connection.
func (c *Client) Register(ctx context.Context, id, token string) (*rpcpb.ServerRegisterResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcpb.ServerRegisterResponse, error) {
		return client.Register(ctx, conn, id, token)
	})
}

// DeletePeer deletes the connected caller's active Peer. A successful call is
// terminal for the current Peer connection; reconnect before issuing more work.
func (c *Client) DeletePeer(ctx context.Context, id string, request rpcapi.ServerPeerDeleteRequest) (*rpcapi.ServerPeerDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPeerDeleteResponse, error) {
		return client.DeletePeer(ctx, conn, id, request)
	})
}

func (c *Client) ListWorkspaces(ctx context.Context, id string, request rpcapi.WorkspaceListRequest) (*rpcapi.WorkspaceListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceListResponse, error) {
		return client.ListWorkspaces(ctx, conn, id, request)
	})
}

func (c *Client) GetWorkspace(ctx context.Context, id string, request rpcapi.WorkspaceGetRequest) (*rpcapi.WorkspaceGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceGetResponse, error) {
		return client.GetWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) CreateWorkspace(ctx context.Context, id string, request rpcapi.WorkspaceCreateRequest) (*rpcapi.WorkspaceCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceCreateResponse, error) {
		return client.CreateWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) PutWorkspace(ctx context.Context, id string, request rpcapi.WorkspacePutRequest) (*rpcapi.WorkspacePutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspacePutResponse, error) {
		return client.PutWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) DeleteWorkspace(ctx context.Context, id string, request rpcapi.WorkspaceDeleteRequest) (*rpcapi.WorkspaceDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceDeleteResponse, error) {
		return client.DeleteWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) ListWorkspaceHistory(ctx context.Context, id string, request rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceHistoryListResponse, error) {
		return client.ListWorkspaceHistory(ctx, conn, id, request)
	})
}

func (c *Client) GetWorkspaceHistory(ctx context.Context, id string, request rpcapi.WorkspaceHistoryGetRequest) (*rpcapi.WorkspaceHistoryGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceHistoryGetResponse, error) {
		return client.GetWorkspaceHistory(ctx, conn, id, request)
	})
}

func (c *Client) ListWorkflows(ctx context.Context, id string, request rpcapi.WorkflowListRequest) (*rpcapi.WorkflowListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowListResponse, error) {
		return client.ListWorkflows(ctx, conn, id, request)
	})
}

func (c *Client) GetWorkflow(ctx context.Context, id string, request rpcapi.WorkflowGetRequest) (*rpcapi.WorkflowGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
		return client.GetWorkflow(ctx, conn, id, request)
	})
}

func (c *Client) ListModels(ctx context.Context, id string, request rpcapi.ModelListRequest) (*rpcapi.ModelListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelListResponse, error) {
		return client.ListModels(ctx, conn, id, request)
	})
}

func (c *Client) GetModel(ctx context.Context, id string, request rpcapi.ModelGetRequest) (*rpcapi.ModelGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelGetResponse, error) {
		return client.GetModel(ctx, conn, id, request)
	})
}

func (c *Client) ListContacts(ctx context.Context, id string, request rpcapi.ContactListRequest) (*rpcapi.ContactListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ContactListResponse, error) {
		return client.ListContacts(ctx, conn, id, request)
	})
}

func (c *Client) GetContact(ctx context.Context, id string, request rpcapi.ContactGetRequest) (*rpcapi.ContactGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ContactGetResponse, error) {
		return client.GetContact(ctx, conn, id, request)
	})
}

func (c *Client) CreateContact(ctx context.Context, id string, request rpcapi.ContactCreateRequest) (*rpcapi.ContactCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ContactCreateResponse, error) {
		return client.CreateContact(ctx, conn, id, request)
	})
}

func (c *Client) PutContact(ctx context.Context, id string, request rpcapi.ContactPutRequest) (*rpcapi.ContactPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ContactPutResponse, error) {
		return client.PutContact(ctx, conn, id, request)
	})
}

func (c *Client) DeleteContact(ctx context.Context, id string, request rpcapi.ContactDeleteRequest) (*rpcapi.ContactDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ContactDeleteResponse, error) {
		return client.DeleteContact(ctx, conn, id, request)
	})
}

func (c *Client) GetFriendInviteToken(ctx context.Context, id string, request rpcapi.FriendInviteTokenGetRequest) (*rpcapi.FriendInviteTokenGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendInviteTokenGetResponse, error) {
		return client.GetFriendInviteToken(ctx, conn, id, request)
	})
}

func (c *Client) CreateFriendInviteToken(ctx context.Context, id string, request rpcapi.FriendInviteTokenCreateRequest) (*rpcapi.FriendInviteTokenCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendInviteTokenCreateResponse, error) {
		return client.CreateFriendInviteToken(ctx, conn, id, request)
	})
}

func (c *Client) ClearFriendInviteToken(ctx context.Context, id string, request rpcapi.FriendInviteTokenClearRequest) (*rpcapi.FriendInviteTokenClearResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendInviteTokenClearResponse, error) {
		return client.ClearFriendInviteToken(ctx, conn, id, request)
	})
}

func (c *Client) AddFriend(ctx context.Context, id string, request rpcapi.FriendAddRequest) (*rpcapi.FriendAddResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendAddResponse, error) {
		return client.AddFriend(ctx, conn, id, request)
	})
}

func (c *Client) ListFriends(ctx context.Context, id string, request rpcapi.FriendListRequest) (*rpcapi.FriendListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendListResponse, error) {
		return client.ListFriends(ctx, conn, id, request)
	})
}

func (c *Client) GetFriendInfo(ctx context.Context, id string, request rpcapi.FriendInfoGetRequest) (*rpcapi.FriendInfoGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendInfoGetResponse, error) {
		return client.GetFriendInfo(ctx, conn, id, request)
	})
}

func (c *Client) DeleteFriend(ctx context.Context, id string, request rpcapi.FriendDeleteRequest) (*rpcapi.FriendDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendDeleteResponse, error) {
		return client.DeleteFriend(ctx, conn, id, request)
	})
}

func (c *Client) ListFriendGroups(ctx context.Context, id string, request rpcapi.FriendGroupListRequest) (*rpcapi.FriendGroupListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupListResponse, error) {
		return client.ListFriendGroups(ctx, conn, id, request)
	})
}

func (c *Client) GetFriendGroup(ctx context.Context, id string, request rpcapi.FriendGroupGetRequest) (*rpcapi.FriendGroupGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupGetResponse, error) {
		return client.GetFriendGroup(ctx, conn, id, request)
	})
}

func (c *Client) CreateFriendGroup(ctx context.Context, id string, request rpcapi.FriendGroupCreateRequest) (*rpcapi.FriendGroupCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupCreateResponse, error) {
		return client.CreateFriendGroup(ctx, conn, id, request)
	})
}

func (c *Client) PutFriendGroup(ctx context.Context, id string, request rpcapi.FriendGroupPutRequest) (*rpcapi.FriendGroupPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupPutResponse, error) {
		return client.PutFriendGroup(ctx, conn, id, request)
	})
}

func (c *Client) DeleteFriendGroup(ctx context.Context, id string, request rpcapi.FriendGroupDeleteRequest) (*rpcapi.FriendGroupDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupDeleteResponse, error) {
		return client.DeleteFriendGroup(ctx, conn, id, request)
	})
}

func (c *Client) GetFriendGroupInviteToken(ctx context.Context, id string, request rpcapi.FriendGroupInviteTokenGetRequest) (*rpcapi.FriendGroupInviteTokenGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupInviteTokenGetResponse, error) {
		return client.GetFriendGroupInviteToken(ctx, conn, id, request)
	})
}

func (c *Client) CreateFriendGroupInviteToken(ctx context.Context, id string, request rpcapi.FriendGroupInviteTokenCreateRequest) (*rpcapi.FriendGroupInviteTokenCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupInviteTokenCreateResponse, error) {
		return client.CreateFriendGroupInviteToken(ctx, conn, id, request)
	})
}

func (c *Client) ClearFriendGroupInviteToken(ctx context.Context, id string, request rpcapi.FriendGroupInviteTokenClearRequest) (*rpcapi.FriendGroupInviteTokenClearResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupInviteTokenClearResponse, error) {
		return client.ClearFriendGroupInviteToken(ctx, conn, id, request)
	})
}

func (c *Client) JoinFriendGroup(ctx context.Context, id string, request rpcapi.FriendGroupJoinRequest) (*rpcapi.FriendGroupJoinResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupJoinResponse, error) {
		return client.JoinFriendGroup(ctx, conn, id, request)
	})
}

func (c *Client) ListFriendGroupMembers(ctx context.Context, id string, request rpcapi.FriendGroupMemberListRequest) (*rpcapi.FriendGroupMemberListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupMemberListResponse, error) {
		return client.ListFriendGroupMembers(ctx, conn, id, request)
	})
}

func (c *Client) AddFriendGroupMember(ctx context.Context, id string, request rpcapi.FriendGroupMemberAddRequest) (*rpcapi.FriendGroupMemberAddResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupMemberAddResponse, error) {
		return client.AddFriendGroupMember(ctx, conn, id, request)
	})
}

func (c *Client) PutFriendGroupMember(ctx context.Context, id string, request rpcapi.FriendGroupMemberPutRequest) (*rpcapi.FriendGroupMemberPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupMemberPutResponse, error) {
		return client.PutFriendGroupMember(ctx, conn, id, request)
	})
}

func (c *Client) DeleteFriendGroupMember(ctx context.Context, id string, request rpcapi.FriendGroupMemberDeleteRequest) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
		return client.DeleteFriendGroupMember(ctx, conn, id, request)
	})
}

func (c *Client) ListFriendGroupMessages(ctx context.Context, id string, request rpcapi.FriendGroupMessageListRequest) (*rpcapi.FriendGroupMessageListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupMessageListResponse, error) {
		return client.ListFriendGroupMessages(ctx, conn, id, request)
	})
}

func (c *Client) GetFriendGroupMessage(ctx context.Context, id string, request rpcapi.FriendGroupMessageGetRequest) (*rpcapi.FriendGroupMessageGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupMessageGetResponse, error) {
		return client.GetFriendGroupMessage(ctx, conn, id, request)
	})
}

func (c *Client) SendFriendGroupMessage(ctx context.Context, id string, request rpcapi.FriendGroupMessageSendRequest) (*rpcapi.FriendGroupMessageSendResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupMessageSendResponse, error) {
		return client.SendFriendGroupMessage(ctx, conn, id, request)
	})
}

func (c *Client) ListPets(ctx context.Context, id string, request rpcapi.ServerPetListRequest) (*rpcapi.ServerPetListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPetListResponse, error) {
		return client.ListPets(ctx, conn, id, request)
	})
}

func (c *Client) GetPet(ctx context.Context, id string, request rpcapi.ServerPetGetRequest) (*rpcapi.ServerPetGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPetGetResponse, error) {
		return client.GetPet(ctx, conn, id, request)
	})
}

func (c *Client) GetPetActions(ctx context.Context, id string, request rpcapi.ServerPetActionsGetRequest) (*rpcapi.ServerPetActionsGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPetActionsGetResponse, error) {
		return client.GetPetActions(ctx, conn, id, request)
	})
}

func (c *Client) AdoptPet(ctx context.Context, id string, request rpcapi.RuntimeAdoptRequest) (*rpcapi.RuntimeAdoptResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.RuntimeAdoptResponse, error) {
		return client.AdoptPet(ctx, conn, id, request)
	})
}

func (c *Client) PutPet(ctx context.Context, id string, request rpcapi.ServerPetPutRequest) (*rpcapi.ServerPetPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPetPutResponse, error) {
		return client.PutPet(ctx, conn, id, request)
	})
}

func (c *Client) DeletePet(ctx context.Context, id string, request rpcapi.ServerPetDeleteRequest) (*rpcapi.ServerPetDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPetDeleteResponse, error) {
		return client.DeletePet(ctx, conn, id, request)
	})
}

func (c *Client) DrivePet(ctx context.Context, id string, request rpcapi.ServerPetDriveRequest) (*rpcapi.ServerPetDriveResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPetDriveResponse, error) {
		return client.DrivePet(ctx, conn, id, request)
	})
}

func (c *Client) GetPoints(ctx context.Context, id string, request rpcapi.ServerPointsGetRequest) (*rpcapi.ServerPointsGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPointsGetResponse, error) {
		return client.GetPoints(ctx, conn, id, request)
	})
}

func (c *Client) ListPointsTransactions(ctx context.Context, id string, request rpcapi.ServerPointsTransactionListRequest) (*rpcapi.ServerPointsTransactionListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPointsTransactionListResponse, error) {
		return client.ListPointsTransactions(ctx, conn, id, request)
	})
}

func (c *Client) GetPointsTransaction(ctx context.Context, id string, request rpcapi.ServerPointsTransactionGetRequest) (*rpcapi.ServerPointsTransactionGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPointsTransactionGetResponse, error) {
		return client.GetPointsTransaction(ctx, conn, id, request)
	})
}

func (c *Client) ListBadges(ctx context.Context, id string, request rpcapi.ServerBadgeListRequest) (*rpcapi.ServerBadgeListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerBadgeListResponse, error) {
		return client.ListBadges(ctx, conn, id, request)
	})
}

func (c *Client) GetBadge(ctx context.Context, id string, request rpcapi.ServerBadgeGetRequest) (*rpcapi.ServerBadgeGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerBadgeGetResponse, error) {
		return client.GetBadge(ctx, conn, id, request)
	})
}

func (c *Client) ListGameResults(ctx context.Context, id string, request rpcapi.ServerGameResultListRequest) (*rpcapi.ServerGameResultListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGameResultListResponse, error) {
		return client.ListGameResults(ctx, conn, id, request)
	})
}

func (c *Client) GetGameResult(ctx context.Context, id string, request rpcapi.ServerGameResultGetRequest) (*rpcapi.ServerGameResultGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGameResultGetResponse, error) {
		return client.GetGameResult(ctx, conn, id, request)
	})
}

func (c *Client) ListRewardGrants(ctx context.Context, id string, request rpcapi.ServerRewardGrantListRequest) (*rpcapi.ServerRewardGrantListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerRewardGrantListResponse, error) {
		return client.ListRewardGrants(ctx, conn, id, request)
	})
}

func (c *Client) GetRewardGrant(ctx context.Context, id string, request rpcapi.ServerRewardGrantGetRequest) (*rpcapi.ServerRewardGrantGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerRewardGrantGetResponse, error) {
		return client.GetRewardGrant(ctx, conn, id, request)
	})
}

func (c *Client) ListTools(ctx context.Context, id string, request rpcapi.ToolListRequest) (*rpcapi.ToolListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ToolListResponse, error) {
		return client.ListTools(ctx, conn, id, request)
	})
}

func (c *Client) GetTool(ctx context.Context, id string, request rpcapi.ToolGetRequest) (*rpcapi.ToolGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ToolGetResponse, error) {
		return client.GetTool(ctx, conn, id, request)
	})
}
