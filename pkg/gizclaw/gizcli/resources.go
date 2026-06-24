package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

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

func (c *Client) CreateWorkflow(ctx context.Context, id string, request rpcapi.WorkflowCreateRequest) (*rpcapi.WorkflowCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowCreateResponse, error) {
		return client.CreateWorkflow(ctx, conn, id, request)
	})
}

func (c *Client) PutWorkflow(ctx context.Context, id string, request rpcapi.WorkflowPutRequest) (*rpcapi.WorkflowPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowPutResponse, error) {
		return client.PutWorkflow(ctx, conn, id, request)
	})
}

func (c *Client) DeleteWorkflow(ctx context.Context, id string, request rpcapi.WorkflowDeleteRequest) (*rpcapi.WorkflowDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowDeleteResponse, error) {
		return client.DeleteWorkflow(ctx, conn, id, request)
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

func (c *Client) CreateModel(ctx context.Context, id string, request rpcapi.ModelCreateRequest) (*rpcapi.ModelCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelCreateResponse, error) {
		return client.CreateModel(ctx, conn, id, request)
	})
}

func (c *Client) PutModel(ctx context.Context, id string, request rpcapi.ModelPutRequest) (*rpcapi.ModelPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelPutResponse, error) {
		return client.PutModel(ctx, conn, id, request)
	})
}

func (c *Client) DeleteModel(ctx context.Context, id string, request rpcapi.ModelDeleteRequest) (*rpcapi.ModelDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelDeleteResponse, error) {
		return client.DeleteModel(ctx, conn, id, request)
	})
}

func (c *Client) ListCredentials(ctx context.Context, id string, request rpcapi.CredentialListRequest) (*rpcapi.CredentialListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialListResponse, error) {
		return client.ListCredentials(ctx, conn, id, request)
	})
}

func (c *Client) GetCredential(ctx context.Context, id string, request rpcapi.CredentialGetRequest) (*rpcapi.CredentialGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialGetResponse, error) {
		return client.GetCredential(ctx, conn, id, request)
	})
}

func (c *Client) CreateCredential(ctx context.Context, id string, request rpcapi.CredentialCreateRequest) (*rpcapi.CredentialCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialCreateResponse, error) {
		return client.CreateCredential(ctx, conn, id, request)
	})
}

func (c *Client) PutCredential(ctx context.Context, id string, request rpcapi.CredentialPutRequest) (*rpcapi.CredentialPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialPutResponse, error) {
		return client.PutCredential(ctx, conn, id, request)
	})
}

func (c *Client) DeleteCredential(ctx context.Context, id string, request rpcapi.CredentialDeleteRequest) (*rpcapi.CredentialDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialDeleteResponse, error) {
		return client.DeleteCredential(ctx, conn, id, request)
	})
}

func (c *Client) ListPets(ctx context.Context, id string, request rpcapi.PetListRequest) (*rpcapi.PetListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetListResponse, error) {
		return client.ListPets(ctx, conn, id, request)
	})
}

func (c *Client) GetPet(ctx context.Context, id string, request rpcapi.PetGetRequest) (*rpcapi.PetGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetGetResponse, error) {
		return client.GetPet(ctx, conn, id, request)
	})
}

func (c *Client) AdoptPet(ctx context.Context, id string, request rpcapi.PetAdoptRequest) (*rpcapi.PetAdoptResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetAdoptResponse, error) {
		return client.AdoptPet(ctx, conn, id, request)
	})
}

func (c *Client) PutPet(ctx context.Context, id string, request rpcapi.PetPutRequest) (*rpcapi.PetPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetPutResponse, error) {
		return client.PutPet(ctx, conn, id, request)
	})
}

func (c *Client) DeletePet(ctx context.Context, id string, request rpcapi.PetDeleteRequest) (*rpcapi.PetDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetDeleteResponse, error) {
		return client.DeletePet(ctx, conn, id, request)
	})
}

func (c *Client) FeedPet(ctx context.Context, id string, request rpcapi.PetFeedRequest) (*rpcapi.PetFeedResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetFeedResponse, error) {
		return client.FeedPet(ctx, conn, id, request)
	})
}

func (c *Client) WashPet(ctx context.Context, id string, request rpcapi.PetWashRequest) (*rpcapi.PetWashResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetWashResponse, error) {
		return client.WashPet(ctx, conn, id, request)
	})
}

func (c *Client) PlayPet(ctx context.Context, id string, request rpcapi.PetPlayRequest) (*rpcapi.PetPlayResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.PetPlayResponse, error) {
		return client.PlayPet(ctx, conn, id, request)
	})
}

func (c *Client) GetWallet(ctx context.Context, id string, request rpcapi.WalletGetRequest) (*rpcapi.WalletGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WalletGetResponse, error) {
		return client.GetWallet(ctx, conn, id, request)
	})
}

func (c *Client) ListWalletTransactions(ctx context.Context, id string, request rpcapi.WalletTransactionsListRequest) (*rpcapi.WalletTransactionsListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WalletTransactionsListResponse, error) {
		return client.ListWalletTransactions(ctx, conn, id, request)
	})
}

func (c *Client) GetWalletTransaction(ctx context.Context, id string, request rpcapi.WalletTransactionsGetRequest) (*rpcapi.WalletTransactionsGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WalletTransactionsGetResponse, error) {
		return client.GetWalletTransaction(ctx, conn, id, request)
	})
}

func (c *Client) ListRewards(ctx context.Context, id string, request rpcapi.RewardListRequest) (*rpcapi.RewardListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.RewardListResponse, error) {
		return client.ListRewards(ctx, conn, id, request)
	})
}

func (c *Client) GetReward(ctx context.Context, id string, request rpcapi.RewardGetRequest) (*rpcapi.RewardGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.RewardGetResponse, error) {
		return client.GetReward(ctx, conn, id, request)
	})
}

func (c *Client) ClaimReward(ctx context.Context, id string, request rpcapi.RewardClaimRequest) (*rpcapi.RewardClaimResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.RewardClaimResponse, error) {
		return client.ClaimReward(ctx, conn, id, request)
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
