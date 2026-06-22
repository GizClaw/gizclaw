package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func callResourceRPC[Req any, Resp any](
	ctx context.Context,
	conn net.Conn,
	id string,
	method rpcapi.RPCMethod,
	request Req,
	encode func(*rpcapi.RPCRequest_Params, Req) error,
	decode func(rpcapi.RPCResponse_Result) (Resp, error),
	name string,
) (*Resp, error) {
	params, err := newRPCRequestParams(request, encode)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, method, params), decode)
	if err != nil {
		return nil, wrapRPCResultError(name, err)
	}
	return result, nil
}

func (c *rpcClient) ListWorkspaces(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceListRequest) (*rpcapi.WorkspaceListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceList, request, (*rpcapi.RPCRequest_Params).FromWorkspaceListRequest, rpcapi.RPCResponse_Result.AsWorkspaceListResponse, "workspace list")
}

func (c *rpcClient) GetWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceGetRequest) (*rpcapi.WorkspaceGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceGet, request, (*rpcapi.RPCRequest_Params).FromWorkspaceGetRequest, rpcapi.RPCResponse_Result.AsWorkspaceGetResponse, "workspace get")
}

func (c *rpcClient) CreateWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceCreateRequest) (*rpcapi.WorkspaceCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceCreate, request, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.RPCResponse_Result.AsWorkspaceCreateResponse, "workspace create")
}

func (c *rpcClient) PutWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspacePutRequest) (*rpcapi.WorkspacePutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspacePut, request, (*rpcapi.RPCRequest_Params).FromWorkspacePutRequest, rpcapi.RPCResponse_Result.AsWorkspacePutResponse, "workspace put")
}

func (c *rpcClient) DeleteWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceDeleteRequest) (*rpcapi.WorkspaceDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceDelete, request, (*rpcapi.RPCRequest_Params).FromWorkspaceDeleteRequest, rpcapi.RPCResponse_Result.AsWorkspaceDeleteResponse, "workspace delete")
}

func (c *rpcClient) ListWorkspaceHistory(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceHistoryList, request, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryListRequest, rpcapi.RPCResponse_Result.AsWorkspaceHistoryListResponse, "workspace history list")
}

func (c *rpcClient) GetWorkspaceHistory(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceHistoryGetRequest) (*rpcapi.WorkspaceHistoryGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceHistoryGet, request, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryGetRequest, rpcapi.RPCResponse_Result.AsWorkspaceHistoryGetResponse, "workspace history get")
}

func (c *rpcClient) ListWorkflows(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowListRequest) (*rpcapi.WorkflowListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowList, request, (*rpcapi.RPCRequest_Params).FromWorkflowListRequest, rpcapi.RPCResponse_Result.AsWorkflowListResponse, "workflow list")
}

func (c *rpcClient) GetWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowGetRequest) (*rpcapi.WorkflowGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowGet, request, (*rpcapi.RPCRequest_Params).FromWorkflowGetRequest, rpcapi.RPCResponse_Result.AsWorkflowGetResponse, "workflow get")
}

func (c *rpcClient) CreateWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowCreateRequest) (*rpcapi.WorkflowCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowCreate, request, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, rpcapi.RPCResponse_Result.AsWorkflowCreateResponse, "workflow create")
}

func (c *rpcClient) PutWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowPutRequest) (*rpcapi.WorkflowPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowPut, request, (*rpcapi.RPCRequest_Params).FromWorkflowPutRequest, rpcapi.RPCResponse_Result.AsWorkflowPutResponse, "workflow put")
}

func (c *rpcClient) DeleteWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowDeleteRequest) (*rpcapi.WorkflowDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowDelete, request, (*rpcapi.RPCRequest_Params).FromWorkflowDeleteRequest, rpcapi.RPCResponse_Result.AsWorkflowDeleteResponse, "workflow delete")
}

func (c *rpcClient) ListModels(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelListRequest) (*rpcapi.ModelListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelList, request, (*rpcapi.RPCRequest_Params).FromModelListRequest, rpcapi.RPCResponse_Result.AsModelListResponse, "model list")
}

func (c *rpcClient) GetModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelGetRequest) (*rpcapi.ModelGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelGet, request, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.RPCResponse_Result.AsModelGetResponse, "model get")
}

func (c *rpcClient) CreateModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelCreateRequest) (*rpcapi.ModelCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelCreate, request, (*rpcapi.RPCRequest_Params).FromModelCreateRequest, rpcapi.RPCResponse_Result.AsModelCreateResponse, "model create")
}

func (c *rpcClient) PutModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelPutRequest) (*rpcapi.ModelPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelPut, request, (*rpcapi.RPCRequest_Params).FromModelPutRequest, rpcapi.RPCResponse_Result.AsModelPutResponse, "model put")
}

func (c *rpcClient) DeleteModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelDeleteRequest) (*rpcapi.ModelDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelDelete, request, (*rpcapi.RPCRequest_Params).FromModelDeleteRequest, rpcapi.RPCResponse_Result.AsModelDeleteResponse, "model delete")
}

func (c *rpcClient) ListCredentials(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialListRequest) (*rpcapi.CredentialListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialList, request, (*rpcapi.RPCRequest_Params).FromCredentialListRequest, rpcapi.RPCResponse_Result.AsCredentialListResponse, "credential list")
}

func (c *rpcClient) GetCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialGetRequest) (*rpcapi.CredentialGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialGet, request, (*rpcapi.RPCRequest_Params).FromCredentialGetRequest, rpcapi.RPCResponse_Result.AsCredentialGetResponse, "credential get")
}

func (c *rpcClient) CreateCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialCreateRequest) (*rpcapi.CredentialCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialCreate, request, (*rpcapi.RPCRequest_Params).FromCredentialCreateRequest, rpcapi.RPCResponse_Result.AsCredentialCreateResponse, "credential create")
}

func (c *rpcClient) PutCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialPutRequest) (*rpcapi.CredentialPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialPut, request, (*rpcapi.RPCRequest_Params).FromCredentialPutRequest, rpcapi.RPCResponse_Result.AsCredentialPutResponse, "credential put")
}

func (c *rpcClient) DeleteCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialDeleteRequest) (*rpcapi.CredentialDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialDelete, request, (*rpcapi.RPCRequest_Params).FromCredentialDeleteRequest, rpcapi.RPCResponse_Result.AsCredentialDeleteResponse, "credential delete")
}

func (c *rpcClient) ListPets(ctx context.Context, conn net.Conn, id string, request rpcapi.PetListRequest) (*rpcapi.PetListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetList, request, (*rpcapi.RPCRequest_Params).FromPetListRequest, rpcapi.RPCResponse_Result.AsPetListResponse, "pet list")
}

func (c *rpcClient) GetPet(ctx context.Context, conn net.Conn, id string, request rpcapi.PetGetRequest) (*rpcapi.PetGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetGet, request, (*rpcapi.RPCRequest_Params).FromPetGetRequest, rpcapi.RPCResponse_Result.AsPetGetResponse, "pet get")
}

func (c *rpcClient) AdoptPet(ctx context.Context, conn net.Conn, id string, request rpcapi.PetAdoptRequest) (*rpcapi.PetAdoptResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetAdopt, request, (*rpcapi.RPCRequest_Params).FromPetAdoptRequest, rpcapi.RPCResponse_Result.AsPetAdoptResponse, "pet adopt")
}

func (c *rpcClient) PutPet(ctx context.Context, conn net.Conn, id string, request rpcapi.PetPutRequest) (*rpcapi.PetPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetPut, request, (*rpcapi.RPCRequest_Params).FromPetPutRequest, rpcapi.RPCResponse_Result.AsPetPutResponse, "pet put")
}

func (c *rpcClient) DeletePet(ctx context.Context, conn net.Conn, id string, request rpcapi.PetDeleteRequest) (*rpcapi.PetDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetDelete, request, (*rpcapi.RPCRequest_Params).FromPetDeleteRequest, rpcapi.RPCResponse_Result.AsPetDeleteResponse, "pet delete")
}

func (c *rpcClient) FeedPet(ctx context.Context, conn net.Conn, id string, request rpcapi.PetFeedRequest) (*rpcapi.PetFeedResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetFeed, request, (*rpcapi.RPCRequest_Params).FromPetFeedRequest, rpcapi.RPCResponse_Result.AsPetFeedResponse, "pet feed")
}

func (c *rpcClient) WashPet(ctx context.Context, conn net.Conn, id string, request rpcapi.PetWashRequest) (*rpcapi.PetWashResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetWash, request, (*rpcapi.RPCRequest_Params).FromPetWashRequest, rpcapi.RPCResponse_Result.AsPetWashResponse, "pet wash")
}

func (c *rpcClient) PlayPet(ctx context.Context, conn net.Conn, id string, request rpcapi.PetPlayRequest) (*rpcapi.PetPlayResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetPlay, request, (*rpcapi.RPCRequest_Params).FromPetPlayRequest, rpcapi.RPCResponse_Result.AsPetPlayResponse, "pet play")
}

func (c *rpcClient) GetWallet(ctx context.Context, conn net.Conn, id string, request rpcapi.WalletGetRequest) (*rpcapi.WalletGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWalletGet, request, (*rpcapi.RPCRequest_Params).FromWalletGetRequest, rpcapi.RPCResponse_Result.AsWalletGetResponse, "wallet get")
}

func (c *rpcClient) ListWalletTransactions(ctx context.Context, conn net.Conn, id string, request rpcapi.WalletTransactionsListRequest) (*rpcapi.WalletTransactionsListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWalletTransactionsList, request, (*rpcapi.RPCRequest_Params).FromWalletTransactionsListRequest, rpcapi.RPCResponse_Result.AsWalletTransactionsListResponse, "wallet transactions list")
}

func (c *rpcClient) GetWalletTransaction(ctx context.Context, conn net.Conn, id string, request rpcapi.WalletTransactionsGetRequest) (*rpcapi.WalletTransactionsGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWalletTransactionsGet, request, (*rpcapi.RPCRequest_Params).FromWalletTransactionsGetRequest, rpcapi.RPCResponse_Result.AsWalletTransactionsGetResponse, "wallet transactions get")
}

func (c *rpcClient) ListRewards(ctx context.Context, conn net.Conn, id string, request rpcapi.RewardListRequest) (*rpcapi.RewardListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerRewardList, request, (*rpcapi.RPCRequest_Params).FromRewardListRequest, rpcapi.RPCResponse_Result.AsRewardListResponse, "reward list")
}

func (c *rpcClient) GetReward(ctx context.Context, conn net.Conn, id string, request rpcapi.RewardGetRequest) (*rpcapi.RewardGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerRewardGet, request, (*rpcapi.RPCRequest_Params).FromRewardGetRequest, rpcapi.RPCResponse_Result.AsRewardGetResponse, "reward get")
}

func (c *rpcClient) ClaimReward(ctx context.Context, conn net.Conn, id string, request rpcapi.RewardClaimRequest) (*rpcapi.RewardClaimResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerRewardClaim, request, (*rpcapi.RPCRequest_Params).FromRewardClaimRequest, rpcapi.RPCResponse_Result.AsRewardClaimResponse, "reward claim")
}

func (c *rpcClient) ListContacts(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactListRequest) (*rpcapi.ContactListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactList, request, (*rpcapi.RPCRequest_Params).FromContactListRequest, rpcapi.RPCResponse_Result.AsContactListResponse, "contact list")
}

func (c *rpcClient) GetContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactGetRequest) (*rpcapi.ContactGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactGet, request, (*rpcapi.RPCRequest_Params).FromContactGetRequest, rpcapi.RPCResponse_Result.AsContactGetResponse, "contact get")
}

func (c *rpcClient) CreateContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactCreateRequest) (*rpcapi.ContactCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactCreate, request, (*rpcapi.RPCRequest_Params).FromContactCreateRequest, rpcapi.RPCResponse_Result.AsContactCreateResponse, "contact create")
}

func (c *rpcClient) PutContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactPutRequest) (*rpcapi.ContactPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactPut, request, (*rpcapi.RPCRequest_Params).FromContactPutRequest, rpcapi.RPCResponse_Result.AsContactPutResponse, "contact put")
}

func (c *rpcClient) DeleteContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactDeleteRequest) (*rpcapi.ContactDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactDelete, request, (*rpcapi.RPCRequest_Params).FromContactDeleteRequest, rpcapi.RPCResponse_Result.AsContactDeleteResponse, "contact delete")
}

func (c *rpcClient) ListFriendRequests(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendRequestListRequest) (*rpcapi.FriendRequestListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendRequestsList, request, (*rpcapi.RPCRequest_Params).FromFriendRequestListRequest, rpcapi.RPCResponse_Result.AsFriendRequestListResponse, "friend request list")
}

func (c *rpcClient) CreateFriendRequest(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendRequestCreateRequest) (*rpcapi.FriendRequestCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendRequestsCreate, request, (*rpcapi.RPCRequest_Params).FromFriendRequestCreateRequest, rpcapi.RPCResponse_Result.AsFriendRequestCreateResponse, "friend request create")
}

func (c *rpcClient) AcceptFriendRequest(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendRequestAcceptRequest) (*rpcapi.FriendRequestAcceptResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendRequestsAccept, request, (*rpcapi.RPCRequest_Params).FromFriendRequestAcceptRequest, rpcapi.RPCResponse_Result.AsFriendRequestAcceptResponse, "friend request accept")
}

func (c *rpcClient) RejectFriendRequest(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendRequestRejectRequest) (*rpcapi.FriendRequestRejectResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendRequestsReject, request, (*rpcapi.RPCRequest_Params).FromFriendRequestRejectRequest, rpcapi.RPCResponse_Result.AsFriendRequestRejectResponse, "friend request reject")
}

func (c *rpcClient) ListFriends(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendListRequest) (*rpcapi.FriendListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendList, request, (*rpcapi.RPCRequest_Params).FromFriendListRequest, rpcapi.RPCResponse_Result.AsFriendListResponse, "friend list")
}

func (c *rpcClient) DeleteFriend(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendDeleteRequest) (*rpcapi.FriendDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendDelete, request, (*rpcapi.RPCRequest_Params).FromFriendDeleteRequest, rpcapi.RPCResponse_Result.AsFriendDeleteResponse, "friend delete")
}

func (c *rpcClient) ListFriendGroups(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupListRequest) (*rpcapi.FriendGroupListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupList, request, (*rpcapi.RPCRequest_Params).FromFriendGroupListRequest, rpcapi.RPCResponse_Result.AsFriendGroupListResponse, "friend group list")
}

func (c *rpcClient) GetFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupGetRequest) (*rpcapi.FriendGroupGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupGet, request, (*rpcapi.RPCRequest_Params).FromFriendGroupGetRequest, rpcapi.RPCResponse_Result.AsFriendGroupGetResponse, "friend group get")
}

func (c *rpcClient) CreateFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupCreateRequest) (*rpcapi.FriendGroupCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupCreate, request, (*rpcapi.RPCRequest_Params).FromFriendGroupCreateRequest, rpcapi.RPCResponse_Result.AsFriendGroupCreateResponse, "friend group create")
}

func (c *rpcClient) PutFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupPutRequest) (*rpcapi.FriendGroupPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupPut, request, (*rpcapi.RPCRequest_Params).FromFriendGroupPutRequest, rpcapi.RPCResponse_Result.AsFriendGroupPutResponse, "friend group put")
}

func (c *rpcClient) DeleteFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupDeleteRequest) (*rpcapi.FriendGroupDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupDelete, request, (*rpcapi.RPCRequest_Params).FromFriendGroupDeleteRequest, rpcapi.RPCResponse_Result.AsFriendGroupDeleteResponse, "friend group delete")
}

func (c *rpcClient) ListFriendGroupMembers(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberListRequest) (*rpcapi.FriendGroupMemberListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersList, request, (*rpcapi.RPCRequest_Params).FromFriendGroupMemberListRequest, rpcapi.RPCResponse_Result.AsFriendGroupMemberListResponse, "friend group member list")
}

func (c *rpcClient) AddFriendGroupMember(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberAddRequest) (*rpcapi.FriendGroupMemberAddResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersAdd, request, (*rpcapi.RPCRequest_Params).FromFriendGroupMemberAddRequest, rpcapi.RPCResponse_Result.AsFriendGroupMemberAddResponse, "friend group member add")
}

func (c *rpcClient) PutFriendGroupMember(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberPutRequest) (*rpcapi.FriendGroupMemberPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersPut, request, (*rpcapi.RPCRequest_Params).FromFriendGroupMemberPutRequest, rpcapi.RPCResponse_Result.AsFriendGroupMemberPutResponse, "friend group member put")
}

func (c *rpcClient) DeleteFriendGroupMember(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberDeleteRequest) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersDelete, request, (*rpcapi.RPCRequest_Params).FromFriendGroupMemberDeleteRequest, rpcapi.RPCResponse_Result.AsFriendGroupMemberDeleteResponse, "friend group member delete")
}

func (c *rpcClient) ListFriendGroupMessages(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageListRequest) (*rpcapi.FriendGroupMessageListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesList, request, (*rpcapi.RPCRequest_Params).FromFriendGroupMessageListRequest, rpcapi.RPCResponse_Result.AsFriendGroupMessageListResponse, "friend group message list")
}

func (c *rpcClient) GetFriendGroupMessage(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageGetRequest) (*rpcapi.FriendGroupMessageGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesGet, request, (*rpcapi.RPCRequest_Params).FromFriendGroupMessageGetRequest, rpcapi.RPCResponse_Result.AsFriendGroupMessageGetResponse, "friend group message get")
}

func (c *rpcClient) SendFriendGroupMessage(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageSendRequest) (*rpcapi.FriendGroupMessageSendResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesSend, request, (*rpcapi.RPCRequest_Params).FromFriendGroupMessageSendRequest, rpcapi.RPCResponse_Result.AsFriendGroupMessageSendResponse, "friend group message send")
}
