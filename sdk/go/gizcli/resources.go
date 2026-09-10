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

// CreateAPIKey creates a long-lived, recoverable API key for the connected device.
func (c *Client) CreateAPIKey(ctx context.Context, id string, request rpcapi.APIKeyCreateRequest) (*rpcapi.APIKeyCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.APIKeyCreateResponse, error) {
		return client.CreateAPIKey(ctx, conn, id, request)
	})
}

// ListAPIKeys lists API keys owned by the connected device, including complete keys.
func (c *Client) ListAPIKeys(ctx context.Context, id string, request rpcapi.APIKeyListRequest) (*rpcapi.APIKeyListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.APIKeyListResponse, error) {
		return client.ListAPIKeys(ctx, conn, id, request)
	})
}

// RevokeAPIKey revokes an API key owned by the connected device.
func (c *Client) RevokeAPIKey(ctx context.Context, id string, request rpcapi.APIKeyRevokeRequest) (*rpcapi.APIKeyRevokeResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.APIKeyRevokeResponse, error) {
		return client.RevokeAPIKey(ctx, conn, id, request)
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

// PingFriend rings the device of the Friend named request.Name. The result
// reports delivered, not_online (nothing was sent and no rate-limit window
// opened) or rate_limited with the seconds left.
func (c *Client) PingFriend(ctx context.Context, id string, request rpcapi.FriendPingRequest) (*rpcapi.FriendPingResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendPingResponse, error) {
		return client.PingFriend(ctx, conn, id, request)
	})
}

// PingFriendGroup rallies every other online member of the Friend Group named
// request.Name.
func (c *Client) PingFriendGroup(ctx context.Context, id string, request rpcapi.FriendGroupPingRequest) (*rpcapi.FriendGroupPingResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FriendGroupPingResponse, error) {
		return client.PingFriendGroup(ctx, conn, id, request)
	})
}

// GetProfiles reads the public display name and emoji of up to
// rpcapi.MaxProfileGetKeys Peers.
func (c *Client) GetProfiles(ctx context.Context, id string, request rpcapi.ProfileGetRequest) (*rpcapi.ProfileGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ProfileGetResponse, error) {
		return client.GetProfiles(ctx, conn, id, request)
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

// ListAppConfig pages the opaque app_config keys of the selected RuntimeProfile.
func (c *Client) ListAppConfig(ctx context.Context, id string, request rpcapi.AppConfigListRequest) (*rpcapi.AppConfigListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.AppConfigListResponse, error) {
		return client.ListAppConfig(ctx, conn, id, request)
	})
}

// GetAppConfig reads one opaque app_config value verbatim.
func (c *Client) GetAppConfig(ctx context.Context, id string, request rpcapi.AppConfigGetRequest) (*rpcapi.AppConfigGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.AppConfigGetResponse, error) {
		return client.GetAppConfig(ctx, conn, id, request)
	})
}

// SetWorkspaceParameters updates supported fields and ignores unsupported fields.
func (c *Client) SetWorkspaceParameters(ctx context.Context, id string, request rpcapi.WorkspaceParametersSetRequest) (*rpcapi.WorkspaceParametersSetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceParametersSetResponse, error) {
		return client.SetWorkspaceParameters(ctx, conn, id, request)
	})
}
