package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func callResourceRPC[Req any, Resp any](
	ctx context.Context,
	conn net.Conn,
	id string,
	method rpcapi.RPCMethod,
	request Req,
	encode func(*rpcapi.RPCPayload, Req) error,
	decode func(rpcapi.RPCPayload) (Resp, error),
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
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceList, request, (*rpcapi.RPCPayload).FromWorkspaceListRequest, rpcapi.RPCPayload.AsWorkspaceListResponse, "workspace list")
}

func (c *rpcClient) GetWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceGetRequest) (*rpcapi.WorkspaceGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceGet, request, (*rpcapi.RPCPayload).FromWorkspaceGetRequest, rpcapi.RPCPayload.AsWorkspaceGetResponse, "workspace get")
}

func (c *rpcClient) CreateWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceCreateRequest) (*rpcapi.WorkspaceCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceCreate, request, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest, rpcapi.RPCPayload.AsWorkspaceCreateResponse, "workspace create")
}

func (c *rpcClient) PutWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspacePutRequest) (*rpcapi.WorkspacePutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspacePut, request, (*rpcapi.RPCPayload).FromWorkspacePutRequest, rpcapi.RPCPayload.AsWorkspacePutResponse, "workspace put")
}

func (c *rpcClient) DeleteWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceDeleteRequest) (*rpcapi.WorkspaceDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceDelete, request, (*rpcapi.RPCPayload).FromWorkspaceDeleteRequest, rpcapi.RPCPayload.AsWorkspaceDeleteResponse, "workspace delete")
}

func (c *rpcClient) ListWorkspaceHistory(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceHistoryList, request, (*rpcapi.RPCPayload).FromWorkspaceHistoryListRequest, rpcapi.RPCPayload.AsWorkspaceHistoryListResponse, "workspace history list")
}

func (c *rpcClient) GetWorkspaceHistory(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceHistoryGetRequest) (*rpcapi.WorkspaceHistoryGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceHistoryGet, request, (*rpcapi.RPCPayload).FromWorkspaceHistoryGetRequest, rpcapi.RPCPayload.AsWorkspaceHistoryGetResponse, "workspace history get")
}

func (c *rpcClient) ListWorkflows(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowListRequest) (*rpcapi.WorkflowListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowList, request, (*rpcapi.RPCPayload).FromWorkflowListRequest, rpcapi.RPCPayload.AsWorkflowListResponse, "workflow list")
}

func (c *rpcClient) GetWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowGetRequest) (*rpcapi.WorkflowGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowGet, request, (*rpcapi.RPCPayload).FromWorkflowGetRequest, rpcapi.RPCPayload.AsWorkflowGetResponse, "workflow get")
}

func (c *rpcClient) CreateWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowCreateRequest) (*rpcapi.WorkflowCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowCreate, request, (*rpcapi.RPCPayload).FromWorkflowCreateRequest, rpcapi.RPCPayload.AsWorkflowCreateResponse, "workflow create")
}

func (c *rpcClient) PutWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowPutRequest) (*rpcapi.WorkflowPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowPut, request, (*rpcapi.RPCPayload).FromWorkflowPutRequest, rpcapi.RPCPayload.AsWorkflowPutResponse, "workflow put")
}

func (c *rpcClient) DeleteWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowDeleteRequest) (*rpcapi.WorkflowDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerWorkflowDelete, request, (*rpcapi.RPCPayload).FromWorkflowDeleteRequest, rpcapi.RPCPayload.AsWorkflowDeleteResponse, "workflow delete")
}

func (c *rpcClient) ListModels(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelListRequest) (*rpcapi.ModelListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelList, request, (*rpcapi.RPCPayload).FromModelListRequest, rpcapi.RPCPayload.AsModelListResponse, "model list")
}

func (c *rpcClient) GetModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelGetRequest) (*rpcapi.ModelGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelGet, request, (*rpcapi.RPCPayload).FromModelGetRequest, rpcapi.RPCPayload.AsModelGetResponse, "model get")
}

func (c *rpcClient) CreateModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelCreateRequest) (*rpcapi.ModelCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelCreate, request, (*rpcapi.RPCPayload).FromModelCreateRequest, rpcapi.RPCPayload.AsModelCreateResponse, "model create")
}

func (c *rpcClient) PutModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelPutRequest) (*rpcapi.ModelPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelPut, request, (*rpcapi.RPCPayload).FromModelPutRequest, rpcapi.RPCPayload.AsModelPutResponse, "model put")
}

func (c *rpcClient) DeleteModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelDeleteRequest) (*rpcapi.ModelDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelDelete, request, (*rpcapi.RPCPayload).FromModelDeleteRequest, rpcapi.RPCPayload.AsModelDeleteResponse, "model delete")
}

func (c *rpcClient) ListCredentials(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialListRequest) (*rpcapi.CredentialListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialList, request, (*rpcapi.RPCPayload).FromCredentialListRequest, rpcapi.RPCPayload.AsCredentialListResponse, "credential list")
}

func (c *rpcClient) GetCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialGetRequest) (*rpcapi.CredentialGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialGet, request, (*rpcapi.RPCPayload).FromCredentialGetRequest, rpcapi.RPCPayload.AsCredentialGetResponse, "credential get")
}

func (c *rpcClient) CreateCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialCreateRequest) (*rpcapi.CredentialCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialCreate, request, (*rpcapi.RPCPayload).FromCredentialCreateRequest, rpcapi.RPCPayload.AsCredentialCreateResponse, "credential create")
}

func (c *rpcClient) PutCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialPutRequest) (*rpcapi.CredentialPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialPut, request, (*rpcapi.RPCPayload).FromCredentialPutRequest, rpcapi.RPCPayload.AsCredentialPutResponse, "credential put")
}

func (c *rpcClient) DeleteCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialDeleteRequest) (*rpcapi.CredentialDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerCredentialDelete, request, (*rpcapi.RPCPayload).FromCredentialDeleteRequest, rpcapi.RPCPayload.AsCredentialDeleteResponse, "credential delete")
}

func (c *rpcClient) ListContacts(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactListRequest) (*rpcapi.ContactListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactList, request, (*rpcapi.RPCPayload).FromContactListRequest, rpcapi.RPCPayload.AsContactListResponse, "contact list")
}

func (c *rpcClient) GetContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactGetRequest) (*rpcapi.ContactGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactGet, request, (*rpcapi.RPCPayload).FromContactGetRequest, rpcapi.RPCPayload.AsContactGetResponse, "contact get")
}

func (c *rpcClient) CreateContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactCreateRequest) (*rpcapi.ContactCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactCreate, request, (*rpcapi.RPCPayload).FromContactCreateRequest, rpcapi.RPCPayload.AsContactCreateResponse, "contact create")
}

func (c *rpcClient) PutContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactPutRequest) (*rpcapi.ContactPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactPut, request, (*rpcapi.RPCPayload).FromContactPutRequest, rpcapi.RPCPayload.AsContactPutResponse, "contact put")
}

func (c *rpcClient) DeleteContact(ctx context.Context, conn net.Conn, id string, request rpcapi.ContactDeleteRequest) (*rpcapi.ContactDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerContactDelete, request, (*rpcapi.RPCPayload).FromContactDeleteRequest, rpcapi.RPCPayload.AsContactDeleteResponse, "contact delete")
}

func (c *rpcClient) GetFriendInviteToken(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendInviteTokenGetRequest) (*rpcapi.FriendInviteTokenGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendInviteTokenGet, request, (*rpcapi.RPCPayload).FromFriendInviteTokenGetRequest, rpcapi.RPCPayload.AsFriendInviteTokenGetResponse, "friend invite token get")
}

func (c *rpcClient) CreateFriendInviteToken(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendInviteTokenCreateRequest) (*rpcapi.FriendInviteTokenCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendInviteTokenCreate, request, (*rpcapi.RPCPayload).FromFriendInviteTokenCreateRequest, rpcapi.RPCPayload.AsFriendInviteTokenCreateResponse, "friend invite token create")
}

func (c *rpcClient) ClearFriendInviteToken(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendInviteTokenClearRequest) (*rpcapi.FriendInviteTokenClearResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendInviteTokenClear, request, (*rpcapi.RPCPayload).FromFriendInviteTokenClearRequest, rpcapi.RPCPayload.AsFriendInviteTokenClearResponse, "friend invite token clear")
}

func (c *rpcClient) AddFriend(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendAddRequest) (*rpcapi.FriendAddResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendAdd, request, (*rpcapi.RPCPayload).FromFriendAddRequest, rpcapi.RPCPayload.AsFriendAddResponse, "friend add")
}

func (c *rpcClient) ListFriends(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendListRequest) (*rpcapi.FriendListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendList, request, (*rpcapi.RPCPayload).FromFriendListRequest, rpcapi.RPCPayload.AsFriendListResponse, "friend list")
}

func (c *rpcClient) DeleteFriend(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendDeleteRequest) (*rpcapi.FriendDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendDelete, request, (*rpcapi.RPCPayload).FromFriendDeleteRequest, rpcapi.RPCPayload.AsFriendDeleteResponse, "friend delete")
}

func (c *rpcClient) ListFriendGroups(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupListRequest) (*rpcapi.FriendGroupListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupList, request, (*rpcapi.RPCPayload).FromFriendGroupListRequest, rpcapi.RPCPayload.AsFriendGroupListResponse, "friend group list")
}

func (c *rpcClient) GetFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupGetRequest) (*rpcapi.FriendGroupGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupGet, request, (*rpcapi.RPCPayload).FromFriendGroupGetRequest, rpcapi.RPCPayload.AsFriendGroupGetResponse, "friend group get")
}

func (c *rpcClient) CreateFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupCreateRequest) (*rpcapi.FriendGroupCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupCreate, request, (*rpcapi.RPCPayload).FromFriendGroupCreateRequest, rpcapi.RPCPayload.AsFriendGroupCreateResponse, "friend group create")
}

func (c *rpcClient) PutFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupPutRequest) (*rpcapi.FriendGroupPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupPut, request, (*rpcapi.RPCPayload).FromFriendGroupPutRequest, rpcapi.RPCPayload.AsFriendGroupPutResponse, "friend group put")
}

func (c *rpcClient) DeleteFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupDeleteRequest) (*rpcapi.FriendGroupDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupDelete, request, (*rpcapi.RPCPayload).FromFriendGroupDeleteRequest, rpcapi.RPCPayload.AsFriendGroupDeleteResponse, "friend group delete")
}

func (c *rpcClient) GetFriendGroupInviteToken(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupInviteTokenGetRequest) (*rpcapi.FriendGroupInviteTokenGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupInviteTokenGet, request, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenGetRequest, rpcapi.RPCPayload.AsFriendGroupInviteTokenGetResponse, "friend group invite token get")
}

func (c *rpcClient) CreateFriendGroupInviteToken(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupInviteTokenCreateRequest) (*rpcapi.FriendGroupInviteTokenCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupInviteTokenCreate, request, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenCreateRequest, rpcapi.RPCPayload.AsFriendGroupInviteTokenCreateResponse, "friend group invite token create")
}

func (c *rpcClient) ClearFriendGroupInviteToken(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupInviteTokenClearRequest) (*rpcapi.FriendGroupInviteTokenClearResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupInviteTokenClear, request, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenClearRequest, rpcapi.RPCPayload.AsFriendGroupInviteTokenClearResponse, "friend group invite token clear")
}

func (c *rpcClient) JoinFriendGroup(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupJoinRequest) (*rpcapi.FriendGroupJoinResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupJoin, request, (*rpcapi.RPCPayload).FromFriendGroupJoinRequest, rpcapi.RPCPayload.AsFriendGroupJoinResponse, "friend group join")
}

func (c *rpcClient) ListFriendGroupMembers(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberListRequest) (*rpcapi.FriendGroupMemberListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersList, request, (*rpcapi.RPCPayload).FromFriendGroupMemberListRequest, rpcapi.RPCPayload.AsFriendGroupMemberListResponse, "friend group member list")
}

func (c *rpcClient) AddFriendGroupMember(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberAddRequest) (*rpcapi.FriendGroupMemberAddResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersAdd, request, (*rpcapi.RPCPayload).FromFriendGroupMemberAddRequest, rpcapi.RPCPayload.AsFriendGroupMemberAddResponse, "friend group member add")
}

func (c *rpcClient) PutFriendGroupMember(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberPutRequest) (*rpcapi.FriendGroupMemberPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersPut, request, (*rpcapi.RPCPayload).FromFriendGroupMemberPutRequest, rpcapi.RPCPayload.AsFriendGroupMemberPutResponse, "friend group member put")
}

func (c *rpcClient) DeleteFriendGroupMember(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMemberDeleteRequest) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMembersDelete, request, (*rpcapi.RPCPayload).FromFriendGroupMemberDeleteRequest, rpcapi.RPCPayload.AsFriendGroupMemberDeleteResponse, "friend group member delete")
}

func (c *rpcClient) GetGameRuleset(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerGameRulesetGetRequest) (*rpcapi.ServerGameRulesetGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerGameRulesetGet, request, (*rpcapi.RPCPayload).FromServerGameRulesetGetRequest, rpcapi.RPCPayload.AsServerGameRulesetGetResponse, "game ruleset get")
}

func (c *rpcClient) ListPets(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPetListRequest) (*rpcapi.ServerPetListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetList, request, (*rpcapi.RPCPayload).FromServerPetListRequest, rpcapi.RPCPayload.AsServerPetListResponse, "pet list")
}

func (c *rpcClient) GetPet(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPetGetRequest) (*rpcapi.ServerPetGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetGet, request, (*rpcapi.RPCPayload).FromServerPetGetRequest, rpcapi.RPCPayload.AsServerPetGetResponse, "pet get")
}

func (c *rpcClient) AdoptPet(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPetAdoptRequest) (*rpcapi.ServerPetAdoptResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetAdopt, request, (*rpcapi.RPCPayload).FromServerPetAdoptRequest, rpcapi.RPCPayload.AsServerPetAdoptResponse, "pet adopt")
}

func (c *rpcClient) PutPet(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPetPutRequest) (*rpcapi.ServerPetPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetPut, request, (*rpcapi.RPCPayload).FromServerPetPutRequest, rpcapi.RPCPayload.AsServerPetPutResponse, "pet put")
}

func (c *rpcClient) DeletePet(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPetDeleteRequest) (*rpcapi.ServerPetDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetDelete, request, (*rpcapi.RPCPayload).FromServerPetDeleteRequest, rpcapi.RPCPayload.AsServerPetDeleteResponse, "pet delete")
}

func (c *rpcClient) DrivePet(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPetDriveRequest) (*rpcapi.ServerPetDriveResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPetDrive, request, (*rpcapi.RPCPayload).FromServerPetDriveRequest, rpcapi.RPCPayload.AsServerPetDriveResponse, "pet drive")
}

func (c *rpcClient) GetPoints(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPointsGetRequest) (*rpcapi.ServerPointsGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPointsGet, request, (*rpcapi.RPCPayload).FromServerPointsGetRequest, rpcapi.RPCPayload.AsServerPointsGetResponse, "points get")
}

func (c *rpcClient) ListPointsTransactions(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPointsTransactionListRequest) (*rpcapi.ServerPointsTransactionListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPointsTransactionsList, request, (*rpcapi.RPCPayload).FromServerPointsTransactionListRequest, rpcapi.RPCPayload.AsServerPointsTransactionListResponse, "points transaction list")
}

func (c *rpcClient) GetPointsTransaction(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPointsTransactionGetRequest) (*rpcapi.ServerPointsTransactionGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPointsTransactionsGet, request, (*rpcapi.RPCPayload).FromServerPointsTransactionGetRequest, rpcapi.RPCPayload.AsServerPointsTransactionGetResponse, "points transaction get")
}

func (c *rpcClient) ListBadges(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerBadgeListRequest) (*rpcapi.ServerBadgeListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerBadgeList, request, (*rpcapi.RPCPayload).FromServerBadgeListRequest, rpcapi.RPCPayload.AsServerBadgeListResponse, "badge list")
}

func (c *rpcClient) GetBadge(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerBadgeGetRequest) (*rpcapi.ServerBadgeGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerBadgeGet, request, (*rpcapi.RPCPayload).FromServerBadgeGetRequest, rpcapi.RPCPayload.AsServerBadgeGetResponse, "badge get")
}

func (c *rpcClient) ListGameResults(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerGameResultListRequest) (*rpcapi.ServerGameResultListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerGameResultList, request, (*rpcapi.RPCPayload).FromServerGameResultListRequest, rpcapi.RPCPayload.AsServerGameResultListResponse, "game result list")
}

func (c *rpcClient) GetGameResult(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerGameResultGetRequest) (*rpcapi.ServerGameResultGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerGameResultGet, request, (*rpcapi.RPCPayload).FromServerGameResultGetRequest, rpcapi.RPCPayload.AsServerGameResultGetResponse, "game result get")
}

func (c *rpcClient) ListRewardGrants(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerRewardGrantListRequest) (*rpcapi.ServerRewardGrantListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerRewardGrantList, request, (*rpcapi.RPCPayload).FromServerRewardGrantListRequest, rpcapi.RPCPayload.AsServerRewardGrantListResponse, "reward grant list")
}

func (c *rpcClient) GetRewardGrant(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerRewardGrantGetRequest) (*rpcapi.ServerRewardGrantGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerRewardGrantGet, request, (*rpcapi.RPCPayload).FromServerRewardGrantGetRequest, rpcapi.RPCPayload.AsServerRewardGrantGetResponse, "reward grant get")
}

func (c *rpcClient) ListFriendGroupMessages(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageListRequest) (*rpcapi.FriendGroupMessageListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesList, request, (*rpcapi.RPCPayload).FromFriendGroupMessageListRequest, rpcapi.RPCPayload.AsFriendGroupMessageListResponse, "friend group message list")
}

func (c *rpcClient) GetFriendGroupMessage(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageGetRequest) (*rpcapi.FriendGroupMessageGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesGet, request, (*rpcapi.RPCPayload).FromFriendGroupMessageGetRequest, rpcapi.RPCPayload.AsFriendGroupMessageGetResponse, "friend group message get")
}

func (c *rpcClient) SendFriendGroupMessage(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageSendRequest) (*rpcapi.FriendGroupMessageSendResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesSend, request, (*rpcapi.RPCPayload).FromFriendGroupMessageSendRequest, rpcapi.RPCPayload.AsFriendGroupMessageSendResponse, "friend group message send")
}
