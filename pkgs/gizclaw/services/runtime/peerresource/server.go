package peerresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/reward"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/wallet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/gofiber/fiber/v2"
)

type Authorizer interface {
	Authorize(context.Context, acl.AuthorizeRequest) error
}

type policyBindingLister interface {
	ListPolicyBindings(context.Context, acl.ListPolicyBindingsRequest) ([]apitypes.ACLPolicyBinding, bool, *string, error)
}

type Server struct {
	Caller       giznet.PublicKey
	ACL          Authorizer
	Firmwares    *firmware.Server
	Workspaces   workspace.WorkspaceAdminService
	Workflows    workflow.WorkflowAdminService
	Models       model.ModelAdminService
	Credentials  credential.CredentialAdminService
	Voices       voice.VoiceAdminService
	Pets         *pet.Server
	Wallets      *wallet.Server
	Rewards      *reward.Server
	Contacts     *contact.Server
	Friends      *friend.Server
	FriendGroups *friendgroup.Server
}

type WorkspaceHistoryService interface {
	ListWorkspaceHistory(context.Context, apitypes.ACLSubject, string, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error)
	GetWorkspaceHistory(context.Context, apitypes.ACLSubject, string, string) (workspace.HistoryEntry, error)
	ReadWorkspaceHistoryAsset(context.Context, apitypes.ACLSubject, string, string) (io.ReadCloser, error)
}

func IsMethod(method rpcapi.RPCMethod) bool {
	switch method {
	case rpcapi.RPCMethodServerFirmwareList,
		rpcapi.RPCMethodServerFirmwareGet,
		rpcapi.RPCMethodServerFirmwareFilesDownload,
		rpcapi.RPCMethodServerWorkspaceList,
		rpcapi.RPCMethodServerWorkspaceGet,
		rpcapi.RPCMethodServerWorkspaceCreate,
		rpcapi.RPCMethodServerWorkspacePut,
		rpcapi.RPCMethodServerWorkspaceDelete,
		rpcapi.RPCMethodServerWorkspaceHistoryList,
		rpcapi.RPCMethodServerWorkspaceHistoryGet,
		rpcapi.RPCMethodServerWorkspaceHistoryAudioGet,
		rpcapi.RPCMethodServerWorkflowList,
		rpcapi.RPCMethodServerWorkflowGet,
		rpcapi.RPCMethodServerWorkflowCreate,
		rpcapi.RPCMethodServerWorkflowPut,
		rpcapi.RPCMethodServerWorkflowDelete,
		rpcapi.RPCMethodServerModelList,
		rpcapi.RPCMethodServerModelGet,
		rpcapi.RPCMethodServerModelCreate,
		rpcapi.RPCMethodServerModelPut,
		rpcapi.RPCMethodServerModelDelete,
		rpcapi.RPCMethodServerVoiceList,
		rpcapi.RPCMethodServerVoiceGet,
		rpcapi.RPCMethodServerCredentialList,
		rpcapi.RPCMethodServerCredentialGet,
		rpcapi.RPCMethodServerCredentialCreate,
		rpcapi.RPCMethodServerCredentialPut,
		rpcapi.RPCMethodServerCredentialDelete,
		rpcapi.RPCMethodServerPetList,
		rpcapi.RPCMethodServerPetGet,
		rpcapi.RPCMethodServerPetAdopt,
		rpcapi.RPCMethodServerPetPut,
		rpcapi.RPCMethodServerPetDelete,
		rpcapi.RPCMethodServerPetFeed,
		rpcapi.RPCMethodServerPetWash,
		rpcapi.RPCMethodServerPetPlay,
		rpcapi.RPCMethodServerWalletGet,
		rpcapi.RPCMethodServerWalletTransactionsList,
		rpcapi.RPCMethodServerWalletTransactionsGet,
		rpcapi.RPCMethodServerRewardList,
		rpcapi.RPCMethodServerRewardGet,
		rpcapi.RPCMethodServerRewardClaim,
		rpcapi.RPCMethodServerContactList,
		rpcapi.RPCMethodServerContactGet,
		rpcapi.RPCMethodServerContactCreate,
		rpcapi.RPCMethodServerContactPut,
		rpcapi.RPCMethodServerContactDelete,
		rpcapi.RPCMethodServerFriendInviteTokenGet,
		rpcapi.RPCMethodServerFriendInviteTokenCreate,
		rpcapi.RPCMethodServerFriendInviteTokenClear,
		rpcapi.RPCMethodServerFriendAdd,
		rpcapi.RPCMethodServerFriendList,
		rpcapi.RPCMethodServerFriendDelete,
		rpcapi.RPCMethodServerFriendGroupList,
		rpcapi.RPCMethodServerFriendGroupGet,
		rpcapi.RPCMethodServerFriendGroupCreate,
		rpcapi.RPCMethodServerFriendGroupPut,
		rpcapi.RPCMethodServerFriendGroupDelete,
		rpcapi.RPCMethodServerFriendGroupInviteTokenGet,
		rpcapi.RPCMethodServerFriendGroupInviteTokenCreate,
		rpcapi.RPCMethodServerFriendGroupInviteTokenClear,
		rpcapi.RPCMethodServerFriendGroupJoin,
		rpcapi.RPCMethodServerFriendGroupMembersList,
		rpcapi.RPCMethodServerFriendGroupMembersAdd,
		rpcapi.RPCMethodServerFriendGroupMembersPut,
		rpcapi.RPCMethodServerFriendGroupMembersDelete,
		rpcapi.RPCMethodServerFriendGroupMessagesList,
		rpcapi.RPCMethodServerFriendGroupMessagesGet,
		rpcapi.RPCMethodServerFriendGroupMessagesSend:
		return true
	default:
		return false
	}
}

func (s *Server) Dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if req == nil || !IsMethod(req.Method) {
		return nil, false, nil
	}
	switch req.Method {
	case rpcapi.RPCMethodServerFirmwareList:
		return s.handleFirmwareList(ctx, req), true, nil
	case rpcapi.RPCMethodServerFirmwareGet:
		return s.handleFirmwareGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerFirmwareFilesDownload:
		return s.handleFirmwareDownload(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkspaceList:
		return s.handleWorkspaceList(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkspaceGet:
		return s.handleWorkspaceGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkspaceCreate:
		return s.handleWorkspaceCreate(ctx, req)
	case rpcapi.RPCMethodServerWorkspacePut:
		return s.handleWorkspacePut(ctx, req)
	case rpcapi.RPCMethodServerWorkspaceDelete:
		return s.handleWorkspaceDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkspaceHistoryList:
		return s.handleWorkspaceHistoryList(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkspaceHistoryGet:
		return s.handleWorkspaceHistoryGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkspaceHistoryAudioGet:
		return s.handleWorkspaceHistoryAudioGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkflowList:
		return s.handleWorkflowList(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkflowGet:
		return s.handleWorkflowGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerWorkflowCreate:
		return s.handleWorkflowCreate(ctx, req)
	case rpcapi.RPCMethodServerWorkflowPut:
		return s.handleWorkflowPut(ctx, req)
	case rpcapi.RPCMethodServerWorkflowDelete:
		return s.handleWorkflowDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerModelList:
		return s.handleModelList(ctx, req), true, nil
	case rpcapi.RPCMethodServerModelGet:
		return s.handleModelGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerModelCreate:
		return s.handleModelCreate(ctx, req)
	case rpcapi.RPCMethodServerModelPut:
		return s.handleModelPut(ctx, req)
	case rpcapi.RPCMethodServerModelDelete:
		return s.handleModelDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerVoiceList:
		return s.handleVoiceList(ctx, req), true, nil
	case rpcapi.RPCMethodServerVoiceGet:
		return s.handleVoiceGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerCredentialList:
		return s.handleCredentialList(ctx, req), true, nil
	case rpcapi.RPCMethodServerCredentialGet:
		return s.handleCredentialGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerCredentialCreate:
		return s.handleCredentialCreate(ctx, req)
	case rpcapi.RPCMethodServerCredentialPut:
		return s.handleCredentialPut(ctx, req)
	case rpcapi.RPCMethodServerCredentialDelete:
		return s.handleCredentialDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetList:
		return s.handlePetList(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetGet:
		return s.handlePetGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetAdopt:
		return s.handlePetAdopt(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetPut:
		return s.handlePetPut(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetDelete:
		return s.handlePetDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetFeed:
		return s.handlePetFeed(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetWash:
		return s.handlePetWash(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetPlay:
		return s.handlePetPlay(ctx, req), true, nil
	case rpcapi.RPCMethodServerWalletGet:
		return s.handleWalletGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerWalletTransactionsList:
		return s.handleWalletTransactionsList(ctx, req), true, nil
	case rpcapi.RPCMethodServerWalletTransactionsGet:
		return s.handleWalletTransactionsGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerRewardList:
		return s.handleRewardList(ctx, req), true, nil
	case rpcapi.RPCMethodServerRewardGet:
		return s.handleRewardGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerRewardClaim:
		return s.handleRewardClaim(ctx, req), true, nil
	case rpcapi.RPCMethodServerContactList:
		return s.handleContactList(ctx, req), true, nil
	case rpcapi.RPCMethodServerContactGet:
		return s.handleContactGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerContactCreate:
		return s.handleContactCreate(ctx, req), true, nil
	case rpcapi.RPCMethodServerContactPut:
		return s.handleContactPut(ctx, req), true, nil
	case rpcapi.RPCMethodServerContactDelete:
		return s.handleContactDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendInviteTokenGet:
		return s.handleFriendInviteTokenGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendInviteTokenCreate:
		return s.handleFriendInviteTokenCreate(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendInviteTokenClear:
		return s.handleFriendInviteTokenClear(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendAdd:
		return s.handleFriendAdd(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendList:
		return s.handleFriendList(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendDelete:
		return s.handleFriendDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupList:
		return s.handleFriendGroupList(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupGet:
		return s.handleFriendGroupGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupCreate:
		return s.handleFriendGroupCreate(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupPut:
		return s.handleFriendGroupPut(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupDelete:
		return s.handleFriendGroupDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupInviteTokenGet:
		return s.handleFriendGroupInviteTokenGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupInviteTokenCreate:
		return s.handleFriendGroupInviteTokenCreate(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupInviteTokenClear:
		return s.handleFriendGroupInviteTokenClear(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupJoin:
		return s.handleFriendGroupJoin(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupMembersList:
		return s.handleFriendGroupMembersList(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupMembersAdd:
		return s.handleFriendGroupMembersAdd(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupMembersPut:
		return s.handleFriendGroupMembersPut(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupMembersDelete:
		return s.handleFriendGroupMembersDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupMessagesList:
		return s.handleFriendGroupMessagesList(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupMessagesGet:
		return s.handleFriendGroupMessagesGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerFriendGroupMessagesSend:
		return s.handleFriendGroupMessagesSend(ctx, req), true, nil
	default:
		return nil, false, nil
	}
}

func (s *Server) handleWorkspaceList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsWorkspaceListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if strings.TrimSpace(valueOrZero(params.Prefix)) != "" {
		return s.handleWorkspaceListByPrefix(ctx, req.Id, params)
	}
	resp, err := s.Workspaces.ListWorkspaces(ctx, adminservice.ListWorkspacesRequestObject{
		Params: adminservice.ListWorkspacesParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminservice.WorkspaceList](resp.VisitListWorkspacesResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	items := make([]apitypes.Workspace, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, acl.WorkspaceResource(item.Name), apitypes.ACLPermissionWorkspaceRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, adminservice.WorkspaceList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor}, (*rpcapi.RPCResponse_Result).FromWorkspaceListResponse)
}

func (s *Server) handleWorkspaceListByPrefix(ctx context.Context, requestID string, params rpcapi.WorkspaceListRequest) *rpcapi.RPCResponse {
	lister, ok := s.ACL.(policyBindingLister)
	if !ok {
		return internalError(requestID, "acl policy binding listing not configured")
	}
	cursor := strings.TrimSpace(valueOrZero(params.Cursor))
	limit := peerListLimit(params.Limit)
	prefix := strings.TrimSpace(valueOrZero(params.Prefix))
	items := make([]apitypes.Workspace, 0, limit)
	seen := make(map[string]struct{})
	var nextCursor *string
	hasNext := false
	for len(items) < limit {
		bindings, bindingHasNext, bindingCursor, err := lister.ListPolicyBindings(ctx, acl.ListPolicyBindingsRequest{
			Cursor:           cursor,
			Limit:            limit,
			SubjectKind:      acl.SubjectKindPublicKey,
			SubjectID:        s.Caller.String(),
			ResourceKind:     acl.ResourceKindWorkspace,
			ResourceIDPrefix: prefix,
			Permission:       apitypes.ACLPermissionWorkspaceRead,
		})
		if err != nil {
			return internalError(requestID, err.Error())
		}
		hasNext = bindingHasNext
		nextCursor = bindingCursor
		for _, binding := range bindings {
			resourceID := strings.TrimSpace(binding.Policy.Resource.Id)
			if resourceID == "" || resourceID == acl.CollectionResourceID {
				continue
			}
			if _, ok := seen[resourceID]; ok {
				continue
			}
			seen[resourceID] = struct{}{}
			err := s.authorizeErr(ctx, acl.WorkspaceResource(resourceID), apitypes.ACLPermissionWorkspaceRead)
			if errors.Is(err, acl.ErrDenied) {
				continue
			}
			if err != nil {
				return authError(requestID, err)
			}
			workspace, rpcResp, err := s.getWorkspaceForList(ctx, requestID, resourceID)
			if err != nil {
				return internalError(requestID, err.Error())
			}
			if rpcResp != nil {
				if rpcResp.Error != nil && rpcResp.Error.Code == rpcapi.RPCErrorCodeNotFound {
					continue
				}
				return rpcResp
			}
			items = append(items, workspace)
			if len(items) == limit {
				break
			}
		}
		if !hasNext || nextCursor == nil || *nextCursor == cursor {
			break
		}
		cursor = *nextCursor
	}
	return resultResponse(requestID, adminservice.WorkspaceList{Items: items, HasNext: hasNext, NextCursor: nextCursor}, (*rpcapi.RPCResponse_Result).FromWorkspaceListResponse)
}

func (s *Server) getWorkspaceForList(ctx context.Context, requestID, name string) (apitypes.Workspace, *rpcapi.RPCResponse, error) {
	resp, err := s.Workspaces.GetWorkspace(ctx, adminservice.GetWorkspaceRequestObject{Name: name})
	if err != nil {
		return apitypes.Workspace{}, nil, err
	}
	workspace, rpcResp, err := adminResult[apitypes.Workspace](resp.VisitGetWorkspaceResponse)
	if rpcResp != nil {
		rpcResp = withRequestID(requestID, rpcResp)
	}
	return workspace, rpcResp, err
}

func (s *Server) handleWorkspaceGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkspaceGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.WorkspaceResource(params.Name), apitypes.ACLPermissionWorkspaceRead); resp != nil {
		return resp
	}
	adminResp, err := s.Workspaces.GetWorkspace(ctx, adminservice.GetWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetWorkspaceResponse, (*rpcapi.RPCResponse_Result).FromWorkspaceGetResponse)
}

func (s *Server) handleWorkspaceCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkspaceCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.WorkspaceResource(params.Name), apitypes.ACLPermissionWorkspaceAdmin); resp != nil {
		return resp, true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.WorkflowName), apitypes.ACLPermissionWorkflowUse); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.CreateWorkspaceJSONRequestBody](params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workspaces.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitCreateWorkspaceResponse, (*rpcapi.RPCResponse_Result).FromWorkspaceCreateResponse), true, nil
}

func (s *Server) handleWorkspacePut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkspacePutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.WorkspaceResource(params.Name), apitypes.ACLPermissionWorkspaceAdmin); resp != nil {
		return resp, true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Body.WorkflowName), apitypes.ACLPermissionWorkflowUse); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.PutWorkspaceJSONRequestBody](params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workspaces.PutWorkspace(ctx, adminservice.PutWorkspaceRequestObject{Name: params.Name, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitPutWorkspaceResponse, (*rpcapi.RPCResponse_Result).FromWorkspacePutResponse), true, nil
}

func (s *Server) handleWorkspaceDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkspaceDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.WorkspaceResource(params.Name), apitypes.ACLPermissionWorkspaceAdmin); resp != nil {
		return resp
	}
	adminResp, err := s.Workspaces.DeleteWorkspace(ctx, adminservice.DeleteWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitDeleteWorkspaceResponse, (*rpcapi.RPCResponse_Result).FromWorkspaceDeleteResponse)
}

func (s *Server) handleWorkspaceHistoryList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	history, resp := s.workspaceHistoryService(req.Id)
	if resp != nil {
		return resp
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkspaceHistoryListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	var order *apitypes.PeerRunHistoryListRequestOrder
	if params.Order != nil {
		converted := apitypes.PeerRunHistoryListRequestOrder(*params.Order)
		order = &converted
	}
	list, err := history.ListWorkspaceHistory(ctx, acl.PublicKeySubject(s.Caller.String()), params.WorkspaceName, apitypes.PeerRunHistoryListRequest{
		Cursor: params.Cursor,
		Limit:  params.Limit,
		Order:  order,
	})
	if err != nil {
		return authOrBadRequest(req.Id, err)
	}
	return resultResponse(req.Id, list, (*rpcapi.RPCResponse_Result).FromWorkspaceHistoryListResponse)
}

func (s *Server) handleWorkspaceHistoryGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	history, resp := s.workspaceHistoryService(req.Id)
	if resp != nil {
		return resp
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkspaceHistoryGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	entry, err := history.GetWorkspaceHistory(ctx, acl.PublicKeySubject(s.Caller.String()), params.WorkspaceName, params.HistoryId)
	if err != nil {
		return authOrBadRequest(req.Id, err)
	}
	return resultResponse(req.Id, entry.Public(), (*rpcapi.RPCResponse_Result).FromWorkspaceHistoryGetResponse)
}

func (s *Server) handleWorkspaceHistoryAudioGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkspaceHistoryAudioGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	respValue, reader, rpcErr, err := s.PrepareWorkspaceHistoryAudioGet(ctx, params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcErr != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcErr.Code, Message: rpcErr.Message}.RPCResponse()
	}
	if reader != nil {
		_ = reader.Close()
	}
	return resultResponse(req.Id, respValue, (*rpcapi.RPCResponse_Result).FromWorkspaceHistoryAudioGetResponse)
}

func (s *Server) PrepareWorkspaceHistoryAudioGet(ctx context.Context, params rpcapi.WorkspaceHistoryAudioGetRequest) (rpcapi.WorkspaceHistoryAudioGetResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	history, resp := s.workspaceHistoryService("")
	if resp != nil {
		return rpcapi.WorkspaceHistoryAudioGetResponse{}, nil, &rpcapi.RPCError{Code: resp.Error.Code, Message: resp.Error.Message}, nil
	}
	entry, err := history.GetWorkspaceHistory(ctx, acl.PublicKeySubject(s.Caller.String()), params.WorkspaceName, params.HistoryId)
	if err != nil {
		return rpcapi.WorkspaceHistoryAudioGetResponse{}, nil, historyRPCError(err), nil
	}
	var asset workspace.HistoryAsset
	mimeType := ""
	for _, candidate := range entry.Assets {
		candidateMIMEType := workspaceHistoryAssetMIMEType(candidate.Name, candidate.MIMEType)
		if strings.HasPrefix(strings.ToLower(candidateMIMEType), "audio/") {
			asset = candidate
			mimeType = candidateMIMEType
			break
		}
	}
	if mimeType == "" {
		return rpcapi.WorkspaceHistoryAudioGetResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "workspace history entry has no audio"}, nil
	}
	r, err := history.ReadWorkspaceHistoryAsset(ctx, acl.PublicKeySubject(s.Caller.String()), params.WorkspaceName, asset.Name)
	if err != nil {
		return rpcapi.WorkspaceHistoryAudioGetResponse{}, nil, historyRPCError(err), nil
	}
	return rpcapi.WorkspaceHistoryAudioGetResponse{
		WorkspaceName: params.WorkspaceName,
		HistoryId:     params.HistoryId,
		MimeType:      mimeType,
		SizeBytes:     asset.Bytes,
	}, r, nil, nil
}

func historyRPCError(err error) *rpcapi.RPCError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, acl.ErrDenied):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: err.Error()}
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}
	default:
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}
	}
}

func (s *Server) workspaceHistoryService(requestID string) (WorkspaceHistoryService, *rpcapi.RPCResponse) {
	if s.Workspaces == nil {
		return nil, internalError(requestID, "workspace service not configured")
	}
	history, ok := s.Workspaces.(WorkspaceHistoryService)
	if !ok {
		return nil, internalError(requestID, "workspace history service not configured")
	}
	return history, nil
}

func workspaceHistoryAssetMIMEType(name, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".opus"):
		return "audio/opus"
	case strings.HasSuffix(strings.ToLower(name), ".ogg"):
		return "audio/ogg"
	case strings.HasSuffix(strings.ToLower(name), ".mp3"):
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func (s *Server) handleWorkflowList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsWorkflowListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.Workflows.ListWorkflows(ctx, adminservice.ListWorkflowsRequestObject{
		Params: adminservice.ListWorkflowsParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminservice.WorkflowList](resp.VisitListWorkflowsResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	items := make([]apitypes.WorkflowDocument, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, workflowResource(item.Metadata.Name), apitypes.ACLPermissionWorkflowRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, adminservice.WorkflowList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor}, (*rpcapi.RPCResponse_Result).FromWorkflowListResponse)
}

func (s *Server) handleWorkflowGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkflowGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Name), apitypes.ACLPermissionWorkflowRead); resp != nil {
		return resp
	}
	adminResp, err := s.Workflows.GetWorkflow(ctx, adminservice.GetWorkflowRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetWorkflowResponse, (*rpcapi.RPCResponse_Result).FromWorkflowGetResponse)
}

func (s *Server) handleWorkflowCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkflowCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Metadata.Name), apitypes.ACLPermissionWorkflowAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.CreateWorkflowJSONRequestBody](params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workflows.CreateWorkflow(ctx, adminservice.CreateWorkflowRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitCreateWorkflowResponse, (*rpcapi.RPCResponse_Result).FromWorkflowCreateResponse), true, nil
}

func (s *Server) handleWorkflowPut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkflowPutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Name), apitypes.ACLPermissionWorkflowAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.PutWorkflowJSONRequestBody](params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workflows.PutWorkflow(ctx, adminservice.PutWorkflowRequestObject{Name: params.Name, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitPutWorkflowResponse, (*rpcapi.RPCResponse_Result).FromWorkflowPutResponse), true, nil
}

func (s *Server) handleWorkflowDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWorkflowDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Name), apitypes.ACLPermissionWorkflowAdmin); resp != nil {
		return resp
	}
	adminResp, err := s.Workflows.DeleteWorkflow(ctx, adminservice.DeleteWorkflowRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitDeleteWorkflowResponse, (*rpcapi.RPCResponse_Result).FromWorkflowDeleteResponse)
}

func (s *Server) handleModelList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsModelListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.ListModels(ctx, adminservice.ListModelsRequestObject{
		Params: adminservice.ListModelsParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminservice.ModelList](resp.VisitListModelsResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, list, (*rpcapi.RPCResponse_Result).FromModelListResponse)
}

func (s *Server) handleModelGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsModelGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	adminResp, err := s.GetModel(ctx, adminservice.GetModelRequestObject{Id: params.Id})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetModelResponse, (*rpcapi.RPCResponse_Result).FromModelGetResponse)
}

func (s *Server) handleModelCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsModelCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.ModelResource(params.Id), apitypes.ACLPermissionModelAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.CreateModelJSONRequestBody](params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Models.CreateModel(ctx, adminservice.CreateModelRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitCreateModelResponse, (*rpcapi.RPCResponse_Result).FromModelCreateResponse), true, nil
}

func (s *Server) handleModelPut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsModelPutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.ModelResource(params.Id), apitypes.ACLPermissionModelAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.PutModelJSONRequestBody](params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Models.PutModel(ctx, adminservice.PutModelRequestObject{Id: params.Id, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitPutModelResponse, (*rpcapi.RPCResponse_Result).FromModelPutResponse), true, nil
}

func (s *Server) handleModelDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsModelDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.ModelResource(params.Id), apitypes.ACLPermissionModelAdmin); resp != nil {
		return resp
	}
	adminResp, err := s.Models.DeleteModel(ctx, adminservice.DeleteModelRequestObject{Id: params.Id})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitDeleteModelResponse, (*rpcapi.RPCResponse_Result).FromModelDeleteResponse)
}

func (s *Server) handleVoiceList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Voices == nil {
		return internalError(req.Id, "voice service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsVoiceListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.ListVoices(ctx, adminservice.ListVoicesRequestObject{
		Params: adminservice.ListVoicesParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminservice.VoiceList](resp.VisitListVoicesResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, list, (*rpcapi.RPCResponse_Result).FromVoiceListResponse)
}

func (s *Server) handleVoiceGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Voices == nil {
		return internalError(req.Id, "voice service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsVoiceGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	adminResp, err := s.GetVoice(ctx, adminservice.GetVoiceRequestObject{Id: params.Id})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetVoiceResponse, (*rpcapi.RPCResponse_Result).FromVoiceGetResponse)
}

func (s *Server) handleCredentialList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsCredentialListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.Credentials.ListCredentials(ctx, adminservice.ListCredentialsRequestObject{
		Params: adminservice.ListCredentialsParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminservice.CredentialList](resp.VisitListCredentialsResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	items := make([]apitypes.Credential, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, acl.CredentialResource(item.Name), apitypes.ACLPermissionCredentialRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, adminservice.CredentialList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor}, (*rpcapi.RPCResponse_Result).FromCredentialListResponse)
}

func (s *Server) handleCredentialGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsCredentialGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	adminResp, err := s.GetCredential(ctx, adminservice.GetCredentialRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetCredentialResponse, (*rpcapi.RPCResponse_Result).FromCredentialGetResponse)
}

func (s *Server) handleCredentialCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsCredentialCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CredentialResource(params.Name), apitypes.ACLPermissionCredentialAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.CreateCredentialJSONRequestBody](params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Credentials.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitCreateCredentialResponse, (*rpcapi.RPCResponse_Result).FromCredentialCreateResponse), true, nil
}

func (s *Server) handleCredentialPut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsCredentialPutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CredentialResource(params.Name), apitypes.ACLPermissionCredentialAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminservice.PutCredentialJSONRequestBody](params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Credentials.PutCredential(ctx, adminservice.PutCredentialRequestObject{Name: params.Name, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitPutCredentialResponse, (*rpcapi.RPCResponse_Result).FromCredentialPutResponse), true, nil
}

func (s *Server) handleCredentialDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsCredentialDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CredentialResource(params.Name), apitypes.ACLPermissionCredentialAdmin); resp != nil {
		return resp
	}
	adminResp, err := s.Credentials.DeleteCredential(ctx, adminservice.DeleteCredentialRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitDeleteCredentialResponse, (*rpcapi.RPCResponse_Result).FromCredentialDeleteResponse)
}

func (s *Server) authorizeResponse(ctx context.Context, requestID string, resource apitypes.ACLResource, permission apitypes.ACLPermission) *rpcapi.RPCResponse {
	if err := s.authorizeErr(ctx, resource, permission); err != nil {
		return authError(requestID, err)
	}
	return nil
}

func (s *Server) authorizeErr(ctx context.Context, resource apitypes.ACLResource, permission apitypes.ACLPermission) error {
	if s == nil || s.ACL == nil {
		return errors.New("acl service not configured")
	}
	request := acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(s.Caller.String()),
		Resource:   resource,
		Permission: permission,
	}
	err := s.ACL.Authorize(ctx, request)
	if err == nil || !errors.Is(err, acl.ErrDenied) || !isCollectionFallbackResource(resource) {
		return err
	}
	request.Resource.Id = acl.CollectionResourceID
	return s.ACL.Authorize(ctx, request)
}

func isCollectionFallbackResource(resource apitypes.ACLResource) bool {
	switch resource.Kind {
	case apitypes.ACLResourceKindWorkflow, apitypes.ACLResourceKindWorkspace:
		return resource.Id != "" && resource.Id != acl.CollectionResourceID
	default:
		return false
	}
}

func adminRPCResponse[T any](id string, visit func(*fiber.Ctx) error, encode func(*rpcapi.RPCResponse_Result, T) error) *rpcapi.RPCResponse {
	result, rpcResp, err := adminResult[T](visit)
	if err != nil {
		return internalError(id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(id, rpcResp)
	}
	return resultResponse(id, result, encode)
}

func adminResult[T any](visit func(*fiber.Ctx) error) (T, *rpcapi.RPCResponse, error) {
	var result T
	status, body, err := renderAdminResponse(visit)
	if err != nil {
		return result, nil, err
	}
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &result); err != nil {
			return result, nil, err
		}
		return result, nil, nil
	}
	var apiErr apitypes.ErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
		return result, statusError("", status, apiErr.Error.Message), nil
	}
	return result, statusError("", status, http.StatusText(status)), nil
}

func renderAdminResponse(visit func(*fiber.Ctx) error) (int, []byte, error) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.All("/", visit)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func resultResponse[T any](id string, value any, encode func(*rpcapi.RPCResponse_Result, T) error) *rpcapi.RPCResponse {
	result, err := convertType[T](value)
	if err != nil {
		return internalError(id, err.Error())
	}
	var body rpcapi.RPCResponse_Result
	if err := encode(&body, result); err != nil {
		return internalError(id, err.Error())
	}
	return &rpcapi.RPCResponse{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Result: &body,
	}
}

func decodeRequiredParams[T any](req *rpcapi.RPCRequest, decode func(rpcapi.RPCRequest_Params) (T, error)) (T, bool) {
	var zero T
	if req == nil || req.Params == nil {
		return zero, false
	}
	value, err := decode(*req.Params)
	return value, err == nil
}

func decodeOptionalParams[T any](req *rpcapi.RPCRequest, decode func(rpcapi.RPCRequest_Params) (T, error)) (T, bool) {
	var zero T
	if req == nil || req.Params == nil {
		return zero, true
	}
	value, err := decode(*req.Params)
	return value, err == nil
}

func convertType[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func int32Ptr(value *int) *int32 {
	if value == nil {
		return nil
	}
	converted := int32(*value)
	return &converted
}

func peerListLimit(value *int) int {
	if value == nil || *value <= 0 {
		return 50
	}
	if *value > 200 {
		return 200
	}
	return *value
}

func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

func workflowResource(name string) apitypes.ACLResource {
	return apitypes.ACLResource{
		Kind: acl.ResourceKindWorkflow,
		Id:   name,
	}
}

func invalidParams(id string) *rpcapi.RPCResponse {
	return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid params"}.RPCResponse()
}

func internalError(id, message string) *rpcapi.RPCResponse {
	return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCodeInternalError, Message: message}.RPCResponse()
}

func authError(id string, err error) *rpcapi.RPCResponse {
	code := rpcapi.RPCErrorCodeBadRequest
	if err != nil && err.Error() == "acl service not configured" {
		code = rpcapi.RPCErrorCodeInternalError
	}
	return rpcapi.Error{RequestID: id, Code: code, Message: err.Error()}.RPCResponse()
}

func authOrBadRequest(id string, err error) *rpcapi.RPCResponse {
	if errors.Is(err, acl.ErrDenied) {
		return authError(id, err)
	}
	return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse()
}

func statusError(id string, statusCode int, message string) *rpcapi.RPCResponse {
	if message == "" {
		message = http.StatusText(statusCode)
	}
	code := rpcapi.RPCErrorCode(statusCode)
	if !code.Valid() {
		code = rpcapi.RPCErrorCodeInternalError
	}
	return rpcapi.Error{RequestID: id, Code: code, Message: message}.RPCResponse()
}

func withRequestID(id string, resp *rpcapi.RPCResponse) *rpcapi.RPCResponse {
	if resp == nil {
		return nil
	}
	resp.Id = id
	if resp.V == 0 {
		resp.V = rpcapi.RPCVersionV1
	}
	return resp
}

func (s *Server) String() string {
	return fmt.Sprintf("peerresource.Server{%s}", s.Caller.String())
}
