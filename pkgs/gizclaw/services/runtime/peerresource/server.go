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
	"reflect"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
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
	Contacts     *contact.Server
	Friends      *friend.Server
	FriendGroups *friendgroup.Server
	Gameplay     *gameplay.Runtime
	Tools        *toolkit.Server
	ResourceACL  ResourceACLService
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
		rpcapi.RPCMethodServerFriendGroupMessagesSend,
		rpcapi.RPCMethodServerGameRulesetGet,
		rpcapi.RPCMethodServerBadgeDefPixaDownload,
		rpcapi.RPCMethodServerPetList,
		rpcapi.RPCMethodServerPetGet,
		rpcapi.RPCMethodServerPetActionsGet,
		rpcapi.RPCMethodServerPetPixaDownload,
		rpcapi.RPCMethodServerPetAdopt,
		rpcapi.RPCMethodServerPetPut,
		rpcapi.RPCMethodServerPetDelete,
		rpcapi.RPCMethodServerPetDrive,
		rpcapi.RPCMethodServerPointsGet,
		rpcapi.RPCMethodServerPointsTransactionsList,
		rpcapi.RPCMethodServerPointsTransactionsGet,
		rpcapi.RPCMethodServerBadgeList,
		rpcapi.RPCMethodServerBadgeGet,
		rpcapi.RPCMethodServerGameResultList,
		rpcapi.RPCMethodServerGameResultGet,
		rpcapi.RPCMethodServerRewardGrantList,
		rpcapi.RPCMethodServerRewardGrantGet,
		rpcapi.RPCMethodServerToolList,
		rpcapi.RPCMethodServerToolGet,
		rpcapi.RPCMethodServerToolCreate,
		rpcapi.RPCMethodServerToolPut,
		rpcapi.RPCMethodServerToolDelete:
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
	case rpcapi.RPCMethodServerGameRulesetGet:
		return s.handleGameRulesetGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerBadgeDefPixaDownload:
		return s.handleBadgeDefPixaDownload(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetList:
		return s.handlePetList(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetGet:
		return s.handlePetGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetActionsGet:
		return s.handlePetActionsGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetPixaDownload:
		return s.handlePetPixaDownload(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetAdopt:
		return s.handlePetAdopt(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetPut:
		return s.handlePetPut(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetDelete:
		return s.handlePetDelete(ctx, req), true, nil
	case rpcapi.RPCMethodServerPetDrive:
		return s.handlePetDrive(ctx, req), true, nil
	case rpcapi.RPCMethodServerPointsGet:
		return s.handlePointsGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerPointsTransactionsList:
		return s.handlePointsTransactionsList(ctx, req), true, nil
	case rpcapi.RPCMethodServerPointsTransactionsGet:
		return s.handlePointsTransactionsGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerBadgeList:
		return s.handleBadgeList(ctx, req), true, nil
	case rpcapi.RPCMethodServerBadgeGet:
		return s.handleBadgeGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerGameResultList:
		return s.handleGameResultList(ctx, req), true, nil
	case rpcapi.RPCMethodServerGameResultGet:
		return s.handleGameResultGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerRewardGrantList:
		return s.handleRewardGrantList(ctx, req), true, nil
	case rpcapi.RPCMethodServerRewardGrantGet:
		return s.handleRewardGrantGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerToolList:
		return s.handleToolList(ctx, req), true, nil
	case rpcapi.RPCMethodServerToolGet:
		return s.handleToolGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerToolCreate:
		return s.handleToolCreate(ctx, req), true, nil
	case rpcapi.RPCMethodServerToolPut:
		return s.handleToolPut(ctx, req), true, nil
	case rpcapi.RPCMethodServerToolDelete:
		return s.handleToolDelete(ctx, req), true, nil
	default:
		return nil, false, nil
	}
}

func (s *Server) handleWorkspaceList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsWorkspaceListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if strings.TrimSpace(valueOrZero(params.Prefix)) != "" {
		return s.handleWorkspaceListByPrefix(ctx, req.Id, params)
	}
	resp, err := s.Workspaces.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{
		Params: adminhttp.ListWorkspacesParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminhttp.WorkspaceList](resp.VisitListWorkspacesResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	items := make([]apitypes.Workspace, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, acl.WorkspaceResource(item.Name), apitypes.ACLPermissionRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, adminhttp.WorkspaceList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor}, (*rpcapi.RPCPayload).FromWorkspaceListResponse)
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
			Permission:       apitypes.ACLPermissionRead,
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
			err := s.authorizeErr(ctx, acl.WorkspaceResource(resourceID), apitypes.ACLPermissionRead)
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
	return resultResponse(requestID, adminhttp.WorkspaceList{Items: items, HasNext: hasNext, NextCursor: nextCursor}, (*rpcapi.RPCPayload).FromWorkspaceListResponse)
}

func (s *Server) getWorkspaceForList(ctx context.Context, requestID, name string) (apitypes.Workspace, *rpcapi.RPCResponse, error) {
	resp, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: name})
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
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.WorkspaceResource(params.Name), apitypes.ACLPermissionRead); resp != nil {
		return resp
	}
	adminResp, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return workspaceAdminRPCResponse(req.Id, adminResp.VisitGetWorkspaceResponse, (*rpcapi.RPCPayload).FromWorkspaceGetResponse)
}

func (s *Server) handleWorkspaceCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CollectionResource(acl.ResourceKindWorkspace), apitypes.ACLPermissionCreate); resp != nil {
		return resp, true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.WorkflowName), apitypes.ACLPermissionUse); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminhttp.CreateWorkspaceJSONRequestBody](params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workspaces.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	if _, ok := adminResp.(adminhttp.CreateWorkspace200JSONResponse); ok {
		if err := s.grantResourceOwner(ctx, acl.WorkspaceResource(params.Name)); err != nil {
			_, _ = s.Workspaces.DeleteWorkspace(
				context.WithoutCancel(ctx),
				adminhttp.DeleteWorkspaceRequestObject{Name: params.Name},
			)
			return internalError(req.Id, err.Error()), true, nil
		}
	}
	return workspaceAdminRPCResponse(req.Id, adminResp.VisitCreateWorkspaceResponse, (*rpcapi.RPCPayload).FromWorkspaceCreateResponse), true, nil
}

func (s *Server) handleWorkspacePut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspacePutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.WorkspaceResource(params.Name), apitypes.ACLPermissionAdmin); resp != nil {
		return resp, true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Body.WorkflowName), apitypes.ACLPermissionUse); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminhttp.PutWorkspaceJSONRequestBody](params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workspaces.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: params.Name, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return workspaceAdminRPCResponse(req.Id, adminResp.VisitPutWorkspaceResponse, (*rpcapi.RPCPayload).FromWorkspacePutResponse), true, nil
}

func (s *Server) handleWorkspaceDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.WorkspaceResource(params.Name), apitypes.ACLPermissionAdmin); resp != nil {
		return resp
	}
	deletedBinding, err := s.deleteResourceOwnerBinding(context.WithoutCancel(ctx), acl.WorkspaceResource(params.Name))
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	adminResp, err := s.Workspaces.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
			return internalError(req.Id, fmt.Sprintf("%v; Workspace owner binding rollback failed: %v", err, restoreErr))
		}
		return internalError(req.Id, err.Error())
	}
	if _, ok := adminResp.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		if _, notFound := adminResp.(adminhttp.DeleteWorkspace404JSONResponse); !notFound {
			if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
				return internalError(req.Id, fmt.Sprintf("Workspace delete failed; owner binding rollback failed: %v", restoreErr))
			}
		}
	}
	return workspaceAdminRPCResponse(req.Id, adminResp.VisitDeleteWorkspaceResponse, (*rpcapi.RPCPayload).FromWorkspaceDeleteResponse)
}

func (s *Server) handleWorkspaceHistoryList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	history, resp := s.workspaceHistoryService(req.Id)
	if resp != nil {
		return resp
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceHistoryListRequest)
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
	return resultResponse(req.Id, list, (*rpcapi.RPCPayload).FromWorkspaceHistoryListResponse)
}

func (s *Server) handleWorkspaceHistoryGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	history, resp := s.workspaceHistoryService(req.Id)
	if resp != nil {
		return resp
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceHistoryGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	entry, err := history.GetWorkspaceHistory(ctx, acl.PublicKeySubject(s.Caller.String()), params.WorkspaceName, params.HistoryId)
	if err != nil {
		return authOrBadRequest(req.Id, err)
	}
	return resultResponse(req.Id, entry.Public(), (*rpcapi.RPCPayload).FromWorkspaceHistoryGetResponse)
}

func (s *Server) handleWorkspaceHistoryAudioGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceHistoryAudioGetRequest)
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
	return resultResponse(req.Id, respValue, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioGetResponse)
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
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsWorkflowListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.Workflows.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{
		Params: adminhttp.ListWorkflowsParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminhttp.WorkflowList](resp.VisitListWorkflowsResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	items := make([]apitypes.WorkflowDocument, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, workflowResource(item.Metadata.Name), apitypes.ACLPermissionRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, adminhttp.WorkflowList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor}, (*rpcapi.RPCPayload).FromWorkflowListResponse)
}

func (s *Server) handleWorkflowGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkflowGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Name), apitypes.ACLPermissionRead); resp != nil {
		return resp
	}
	adminResp, err := s.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetWorkflowResponse, (*rpcapi.RPCPayload).FromWorkflowGetResponse)
}

func (s *Server) handleWorkflowCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkflowCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CollectionResource(acl.ResourceKindWorkflow), apitypes.ACLPermissionCreate); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminhttp.CreateWorkflowJSONRequestBody](params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workflows.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	result, rpcResp, err := adminResult[apitypes.WorkflowDocument](adminResp.VisitCreateWorkflowResponse)
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp), true, nil
	}
	if err := s.grantResourceOwner(ctx, workflowResource(result.Metadata.Name)); err != nil {
		_, _ = s.Workflows.DeleteWorkflow(
			context.WithoutCancel(ctx),
			adminhttp.DeleteWorkflowRequestObject{Name: result.Metadata.Name},
		)
		return internalError(req.Id, err.Error()), true, nil
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromWorkflowCreateResponse), true, nil
}

func (s *Server) handleWorkflowPut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkflowPutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Name), apitypes.ACLPermissionAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminhttp.PutWorkflowJSONRequestBody](params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Workflows.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Name: params.Name, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitPutWorkflowResponse, (*rpcapi.RPCPayload).FromWorkflowPutResponse), true, nil
}

func (s *Server) handleWorkflowDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkflowDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, workflowResource(params.Name), apitypes.ACLPermissionAdmin); resp != nil {
		return resp
	}
	deletedBinding, err := s.deleteResourceOwnerBinding(context.WithoutCancel(ctx), workflowResource(params.Name))
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	adminResp, err := s.Workflows.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Name: params.Name})
	if err != nil {
		if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
			return internalError(req.Id, fmt.Sprintf("%v; Workflow owner binding rollback failed: %v", err, restoreErr))
		}
		return internalError(req.Id, err.Error())
	}
	if _, notFound := adminResp.(adminhttp.DeleteWorkflow404JSONResponse); notFound {
		return adminRPCResponse(req.Id, adminResp.VisitDeleteWorkflowResponse, (*rpcapi.RPCPayload).FromWorkflowDeleteResponse)
	}
	result, rpcResp, err := adminResult[apitypes.WorkflowDocument](adminResp.VisitDeleteWorkflowResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
			return internalError(req.Id, fmt.Sprintf("Workflow delete failed; owner binding rollback failed: %v", restoreErr))
		}
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromWorkflowDeleteResponse)
}

func (s *Server) handleModelList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsModelListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.ListModels(ctx, adminhttp.ListModelsRequestObject{
		Params: adminhttp.ListModelsParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminhttp.ModelList](resp.VisitListModelsResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, list, (*rpcapi.RPCPayload).FromModelListResponse)
}

func (s *Server) handleModelGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsModelGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	adminResp, err := s.GetModel(ctx, adminhttp.GetModelRequestObject{Id: params.Id})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetModelResponse, (*rpcapi.RPCPayload).FromModelGetResponse)
}

func (s *Server) handleModelCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsModelCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CollectionResource(acl.ResourceKindModel), apitypes.ACLPermissionCreate); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminhttp.CreateModelJSONRequestBody](params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Models.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	result, rpcResp, err := adminResult[apitypes.Model](adminResp.VisitCreateModelResponse)
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp), true, nil
	}
	if err := s.grantResourceOwner(ctx, acl.ModelResource(result.Id)); err != nil {
		_, _ = s.Models.DeleteModel(
			context.WithoutCancel(ctx),
			adminhttp.DeleteModelRequestObject{Id: result.Id},
		)
		return internalError(req.Id, err.Error()), true, nil
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromModelCreateResponse), true, nil
}

func (s *Server) handleModelPut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsModelPutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.ModelResource(params.Id), apitypes.ACLPermissionAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := convertType[adminhttp.PutModelJSONRequestBody](params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Models.PutModel(ctx, adminhttp.PutModelRequestObject{Id: params.Id, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return adminRPCResponse(req.Id, adminResp.VisitPutModelResponse, (*rpcapi.RPCPayload).FromModelPutResponse), true, nil
}

func (s *Server) handleModelDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsModelDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.ModelResource(params.Id), apitypes.ACLPermissionAdmin); resp != nil {
		return resp
	}
	deletedBinding, err := s.deleteResourceOwnerBinding(context.WithoutCancel(ctx), acl.ModelResource(params.Id))
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	adminResp, err := s.Models.DeleteModel(ctx, adminhttp.DeleteModelRequestObject{Id: params.Id})
	if err != nil {
		if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
			return internalError(req.Id, fmt.Sprintf("%v; Model owner binding rollback failed: %v", err, restoreErr))
		}
		return internalError(req.Id, err.Error())
	}
	if _, notFound := adminResp.(adminhttp.DeleteModel404JSONResponse); notFound {
		return adminRPCResponse(req.Id, adminResp.VisitDeleteModelResponse, (*rpcapi.RPCPayload).FromModelDeleteResponse)
	}
	result, rpcResp, err := adminResult[apitypes.Model](adminResp.VisitDeleteModelResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
			return internalError(req.Id, fmt.Sprintf("Model delete failed; owner binding rollback failed: %v", restoreErr))
		}
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromModelDeleteResponse)
}

func (s *Server) handleVoiceList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Voices == nil {
		return internalError(req.Id, "voice service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsVoiceListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.ListVoices(ctx, adminhttp.ListVoicesRequestObject{
		Params: adminhttp.ListVoicesParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminhttp.VoiceList](resp.VisitListVoicesResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, list, (*rpcapi.RPCPayload).FromVoiceListResponse)
}

func (s *Server) handleVoiceGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Voices == nil {
		return internalError(req.Id, "voice service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsVoiceGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	adminResp, err := s.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: params.Id})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return adminRPCResponse(req.Id, adminResp.VisitGetVoiceResponse, (*rpcapi.RPCPayload).FromVoiceGetResponse)
}

func (s *Server) handleCredentialList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsCredentialListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.Credentials.ListCredentials(ctx, adminhttp.ListCredentialsRequestObject{
		Params: adminhttp.ListCredentialsParams{Cursor: params.Cursor, Limit: int32Ptr(params.Limit)},
	})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	list, rpcResp, err := adminResult[adminhttp.CredentialList](resp.VisitListCredentialsResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	items := make([]apitypes.Credential, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, acl.CredentialResource(item.Name), apitypes.ACLPermissionRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	rpcList, err := apiCredentialListToRPC(adminhttp.CredentialList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, rpcList, (*rpcapi.RPCPayload).FromCredentialListResponse)
}

func (s *Server) handleCredentialGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsCredentialGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	adminResp, err := s.GetCredential(ctx, adminhttp.GetCredentialRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	result, rpcResp, err := adminResult[apitypes.Credential](adminResp.VisitGetCredentialResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	converted, err := apiCredentialToRPC(result)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, converted, (*rpcapi.RPCPayload).FromCredentialGetResponse)
}

func (s *Server) handleCredentialCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsCredentialCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CollectionResource(acl.ResourceKindCredential), apitypes.ACLPermissionCreate); resp != nil {
		return resp, true, nil
	}
	body, err := rpcCredentialUpsertToAdmin(params)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Credentials.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	result, rpcResp, err := adminResult[apitypes.Credential](adminResp.VisitCreateCredentialResponse)
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp), true, nil
	}
	if err := s.grantResourceOwner(ctx, acl.CredentialResource(result.Name)); err != nil {
		_, _ = s.Credentials.DeleteCredential(
			context.WithoutCancel(ctx),
			adminhttp.DeleteCredentialRequestObject{Name: result.Name},
		)
		return internalError(req.Id, err.Error()), true, nil
	}
	converted, err := apiCredentialToRPC(result)
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return resultResponse(req.Id, converted, (*rpcapi.RPCPayload).FromCredentialCreateResponse), true, nil
}

func (s *Server) handleCredentialPut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsCredentialPutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CredentialResource(params.Name), apitypes.ACLPermissionAdmin); resp != nil {
		return resp, true, nil
	}
	body, err := rpcCredentialUpsertToAdmin(params.Body)
	if err != nil {
		return nil, true, err
	}
	adminResp, err := s.Credentials.PutCredential(ctx, adminhttp.PutCredentialRequestObject{Name: params.Name, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	result, rpcResp, err := adminResult[apitypes.Credential](adminResp.VisitPutCredentialResponse)
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp), true, nil
	}
	converted, err := apiCredentialToRPC(result)
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return resultResponse(req.Id, converted, (*rpcapi.RPCPayload).FromCredentialPutResponse), true, nil
}

func (s *Server) handleCredentialDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Credentials == nil {
		return internalError(req.Id, "credential service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsCredentialDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.CredentialResource(params.Name), apitypes.ACLPermissionAdmin); resp != nil {
		return resp
	}
	deletedBinding, err := s.deleteResourceOwnerBinding(context.WithoutCancel(ctx), acl.CredentialResource(params.Name))
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	adminResp, err := s.Credentials.DeleteCredential(ctx, adminhttp.DeleteCredentialRequestObject{Name: params.Name})
	if err != nil {
		if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
			return internalError(req.Id, fmt.Sprintf("%v; Credential owner binding rollback failed: %v", err, restoreErr))
		}
		return internalError(req.Id, err.Error())
	}
	if _, notFound := adminResp.(adminhttp.DeleteCredential404JSONResponse); notFound {
		return adminRPCResponse(req.Id, adminResp.VisitDeleteCredentialResponse, (*rpcapi.RPCPayload).FromCredentialDeleteResponse)
	}
	result, rpcResp, err := adminResult[apitypes.Credential](adminResp.VisitDeleteCredentialResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		if restoreErr := s.restoreResourceOwnerBinding(context.WithoutCancel(ctx), deletedBinding); restoreErr != nil {
			return internalError(req.Id, fmt.Sprintf("Credential delete failed; owner binding rollback failed: %v", restoreErr))
		}
		return withRequestID(req.Id, rpcResp)
	}
	converted, err := apiCredentialToRPC(result)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, converted, (*rpcapi.RPCPayload).FromCredentialDeleteResponse)
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
	return err
}

func adminRPCResponse[T any](id string, visit func(*fiber.Ctx) error, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCResponse {
	result, rpcResp, err := adminResult[T](visit)
	if err != nil {
		return internalError(id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(id, rpcResp)
	}
	return resultResponse(id, result, encode)
}

func workspaceAdminRPCResponse[T any](id string, visit func(*fiber.Ctx) error, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCResponse {
	result, rpcResp, err := adminResult[apitypes.Workspace](visit)
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

func resultResponse[T any](id string, value any, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCResponse {
	result, err := convertType[T](value)
	if err != nil {
		return internalError(id, err.Error())
	}
	var body rpcapi.RPCPayload
	if err := encode(&body, result); err != nil {
		return internalError(id, err.Error())
	}
	return &rpcapi.RPCResponse{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Result: &body,
	}
}

func decodeRequiredParams[T any](req *rpcapi.RPCRequest, decode func(rpcapi.RPCPayload) (T, error)) (T, bool) {
	var zero T
	if req == nil || req.Params == nil {
		return zero, false
	}
	value, err := decode(*req.Params)
	return value, err == nil
}

func decodeOptionalParams[T any](req *rpcapi.RPCRequest, decode func(rpcapi.RPCPayload) (T, error)) (T, bool) {
	var zero T
	if req == nil || req.Params == nil {
		return zero, true
	}
	value, err := decode(*req.Params)
	return value, err == nil
}

func convertType[T any](value any) (T, error) {
	var out T
	if err := convertValue(reflect.ValueOf(&out).Elem(), reflect.ValueOf(value)); err != nil {
		return out, err
	}
	return out, nil
}

func convertValue(dst reflect.Value, src reflect.Value) error {
	if !src.IsValid() {
		return nil
	}
	for src.Kind() == reflect.Interface {
		if src.IsNil() {
			return nil
		}
		src = src.Elem()
	}
	if dst.Type() == reflect.TypeOf(apitypes.CredentialBody{}) && src.Type() == reflect.TypeOf(rpcapi.CredentialBody{}) {
		body, err := rpcCredentialBodyToAPI(src.Interface().(rpcapi.CredentialBody))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if dst.Type() == reflect.TypeOf(apitypes.ModelProviderData{}) && src.Type() == reflect.TypeOf(rpcapi.ModelProviderData{}) {
		body, err := rpcModelProviderDataToAPI(src.Interface().(rpcapi.ModelProviderData))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if dst.Type() == reflect.TypeOf(rpcapi.ModelProviderData{}) && src.Type() == reflect.TypeOf(apitypes.ModelProviderData{}) {
		body, err := apiModelProviderDataToRPC(src.Interface().(apitypes.ModelProviderData))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if dst.Type() == reflect.TypeOf(apitypes.VoiceProviderData{}) && src.Type() == reflect.TypeOf(rpcapi.VoiceProviderData{}) {
		body, err := rpcVoiceProviderDataToAPI(src.Interface().(rpcapi.VoiceProviderData))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if dst.Type() == reflect.TypeOf(rpcapi.VoiceProviderData{}) && src.Type() == reflect.TypeOf(apitypes.VoiceProviderData{}) {
		body, err := apiVoiceProviderDataToRPC(src.Interface().(apitypes.VoiceProviderData))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if dst.Type() == reflect.TypeOf(apitypes.WorkspaceParameters{}) && src.Type() == reflect.TypeOf(rpcapi.WorkspaceParameters{}) {
		body, err := rpcWorkspaceParametersToAPI(src.Interface().(rpcapi.WorkspaceParameters))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if dst.Type() == reflect.TypeOf(rpcapi.WorkspaceParameters{}) && src.Type() == reflect.TypeOf(apitypes.WorkspaceParameters{}) {
		body, err := apiWorkspaceParametersToRPC(src.Interface().(apitypes.WorkspaceParameters))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if dst.Type() == reflect.TypeOf(apitypes.WorkflowI18n{}) && src.Type() == reflect.TypeOf(rpcapi.WorkflowI18n{}) {
		dst.Set(reflect.ValueOf(rpcWorkflowI18nToAPI(src.Interface().(rpcapi.WorkflowI18n))))
		return nil
	}
	if dst.Type() == reflect.TypeOf(rpcapi.WorkflowI18n{}) && src.Type() == reflect.TypeOf(apitypes.WorkflowI18n{}) {
		dst.Set(reflect.ValueOf(apiWorkflowI18nToRPC(src.Interface().(apitypes.WorkflowI18n))))
		return nil
	}
	if dst.Type() == reflect.TypeOf(rpcapi.Pet{}) && src.Type() == reflect.TypeOf(apitypes.Pet{}) {
		dst.Set(reflect.ValueOf(apiPetToRPC(src.Interface().(apitypes.Pet))))
		return nil
	}
	if dst.Type() == reflect.TypeOf(apitypes.Pet{}) && src.Type() == reflect.TypeOf(rpcapi.Pet{}) {
		dst.Set(reflect.ValueOf(rpcPetToAPI(src.Interface().(rpcapi.Pet))))
		return nil
	}
	if dst.Type() == reflect.TypeOf(rpcapi.PetDriveResponse{}) && src.Type() == reflect.TypeOf(apitypes.PetDriveResponse{}) {
		body, err := apiPetDriveResponseToRPC(src.Interface().(apitypes.PetDriveResponse))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(body))
		return nil
	}
	if src.Type().AssignableTo(dst.Type()) {
		dst.Set(src)
		return nil
	}
	if src.Type().ConvertibleTo(dst.Type()) {
		dst.Set(src.Convert(dst.Type()))
		return nil
	}
	switch dst.Kind() {
	case reflect.Pointer:
		if src.Kind() == reflect.Pointer {
			if src.IsNil() {
				return nil
			}
			src = src.Elem()
		}
		dst.Set(reflect.New(dst.Type().Elem()))
		return convertValue(dst.Elem(), src)
	case reflect.Struct:
		src = indirectReflectValue(src)
		if !src.IsValid() || src.Kind() != reflect.Struct {
			return fmt.Errorf("cannot convert %s to %s", src.Type(), dst.Type())
		}
		for i := 0; i < dst.NumField(); i++ {
			field := dst.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			srcField := src.FieldByName(field.Name)
			if !srcField.IsValid() {
				continue
			}
			if handled, err := convertUnionFieldWithParent(dst.Field(i), srcField, src); handled {
				if err != nil {
					return fmt.Errorf("%s: %w", field.Name, err)
				}
				continue
			}
			if err := convertValue(dst.Field(i), srcField); err != nil {
				return fmt.Errorf("%s: %w", field.Name, err)
			}
		}
		return nil
	case reflect.Slice:
		src = indirectReflectValue(src)
		if !src.IsValid() || src.Kind() != reflect.Slice {
			return fmt.Errorf("cannot convert %s to %s", src.Type(), dst.Type())
		}
		out := reflect.MakeSlice(dst.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			if err := convertValue(out.Index(i), src.Index(i)); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
		dst.Set(out)
		return nil
	case reflect.Map:
		src = indirectReflectValue(src)
		if !src.IsValid() || src.Kind() != reflect.Map {
			return fmt.Errorf("cannot convert %s to %s", src.Type(), dst.Type())
		}
		out := reflect.MakeMapWithSize(dst.Type(), src.Len())
		iter := src.MapRange()
		for iter.Next() {
			key := reflect.New(dst.Type().Key()).Elem()
			if err := convertValue(key, iter.Key()); err != nil {
				return err
			}
			item := reflect.New(dst.Type().Elem()).Elem()
			if err := convertValue(item, iter.Value()); err != nil {
				return err
			}
			out.SetMapIndex(key, item)
		}
		dst.Set(out)
		return nil
	default:
		return fmt.Errorf("cannot convert %s to %s", src.Type(), dst.Type())
	}
}

func rpcWorkflowI18nToAPI(in rpcapi.WorkflowI18n) apitypes.WorkflowI18n {
	catalogs := make(map[string]apitypes.WorkflowI18nCatalog, len(in.Value))
	for locale, catalog := range in.Value {
		catalogs[locale] = apitypes.WorkflowI18nCatalog{
			Description: catalog.Description,
			Name:        catalog.Name,
		}
	}
	return apitypes.WorkflowI18n{
		DefaultLocale:        in.DefaultLocale,
		AdditionalProperties: catalogs,
	}
}

func apiWorkflowI18nToRPC(in apitypes.WorkflowI18n) rpcapi.WorkflowI18n {
	catalogs := make(map[string]rpcapi.WorkflowI18nCatalog, len(in.AdditionalProperties))
	for locale, catalog := range in.AdditionalProperties {
		catalogs[locale] = rpcapi.WorkflowI18nCatalog{
			Description: catalog.Description,
			Name:        catalog.Name,
		}
	}
	return rpcapi.WorkflowI18n{
		DefaultLocale: in.DefaultLocale,
		Value:         catalogs,
	}
}

func apiPetDriveResponseToRPC(in apitypes.PetDriveResponse) (rpcapi.PetDriveResponse, error) {
	var out rpcapi.PetDriveResponse
	out.Pet = apiPetToRPC(in.Pet)
	if err := convertValue(reflect.ValueOf(&out.Points).Elem(), reflect.ValueOf(in.Points)); err != nil {
		return rpcapi.PetDriveResponse{}, fmt.Errorf("Points: %w", err)
	}
	if in.GameResult != nil {
		out.GameResult = &rpcapi.GameResult{}
		if err := convertValue(reflect.ValueOf(out.GameResult).Elem(), reflect.ValueOf(*in.GameResult)); err != nil {
			return rpcapi.PetDriveResponse{}, fmt.Errorf("GameResult: %w", err)
		}
	}
	if err := convertValue(reflect.ValueOf(&out.Badges).Elem(), reflect.ValueOf(in.Badges)); err != nil {
		return rpcapi.PetDriveResponse{}, fmt.Errorf("Badges: %w", err)
	}
	if err := convertValue(reflect.ValueOf(&out.RewardGrants).Elem(), reflect.ValueOf(in.RewardGrants)); err != nil {
		return rpcapi.PetDriveResponse{}, fmt.Errorf("RewardGrants: %w", err)
	}
	if err := convertValue(reflect.ValueOf(&out.Transactions).Elem(), reflect.ValueOf(in.Transactions)); err != nil {
		return rpcapi.PetDriveResponse{}, fmt.Errorf("Transactions: %w", err)
	}
	return out, nil
}

func apiPetToRPC(in apitypes.Pet) rpcapi.Pet {
	return rpcapi.Pet{
		CreatedAt:      in.CreatedAt,
		DisplayName:    in.DisplayName,
		Id:             in.Id,
		LastActiveAt:   in.LastActiveAt,
		Life:           rpcapi.PetLife(in.Life),
		OwnerPublicKey: in.OwnerPublicKey,
		PetdefId:       in.PetdefId,
		Progression:    rpcapi.PetProgression(in.Progression),
		RulesetName:    in.RulesetName,
		UpdatedAt:      in.UpdatedAt,
		WorkflowName:   in.WorkflowName,
		WorkspaceName:  in.WorkspaceName,
	}
}

func rpcPetToAPI(in rpcapi.Pet) apitypes.Pet {
	return apitypes.Pet{
		CreatedAt:      in.CreatedAt,
		DisplayName:    in.DisplayName,
		Id:             in.Id,
		LastActiveAt:   in.LastActiveAt,
		Life:           apitypes.PetLife(in.Life),
		OwnerPublicKey: in.OwnerPublicKey,
		PetdefId:       in.PetdefId,
		Progression:    apitypes.PetProgression(in.Progression),
		RulesetName:    in.RulesetName,
		UpdatedAt:      in.UpdatedAt,
		WorkflowName:   in.WorkflowName,
		WorkspaceName:  in.WorkspaceName,
	}
}

func indirectReflectValue(value reflect.Value) reflect.Value {
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
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
