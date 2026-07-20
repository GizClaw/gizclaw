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
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/gofiber/fiber/v2"
)

type Server struct {
	Caller         giznet.PublicKey
	Workspaces     workspace.WorkspaceAdminService
	Workflows      workflow.WorkflowAdminService
	Models         model.ModelAdminService
	Voices         voice.VoiceAdminService
	Contacts       *contact.Server
	Friends        *friend.Server
	FriendGroups   *friendgroup.Server
	Gameplay       *gameplay.Runtime
	Tools          *toolkit.Server
	RuntimeProfile func() *apitypes.RuntimeProfile
}

type WorkspaceHistoryService interface {
	ListWorkspaceHistory(context.Context, string, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error)
	GetWorkspaceHistory(context.Context, string, string) (workspace.HistoryEntry, error)
	ReadWorkspaceHistoryAsset(context.Context, string, string) (io.ReadCloser, error)
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
		rpcapi.RPCMethodServerModelList,
		rpcapi.RPCMethodServerModelGet,
		rpcapi.RPCMethodServerVoiceList,
		rpcapi.RPCMethodServerVoiceGet,
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
		rpcapi.RPCMethodServerFriendInfoGet,
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
		rpcapi.RPCMethodServerBadgeDefPixaDownload,
		rpcapi.RPCMethodServerPetList,
		rpcapi.RPCMethodServerPetGet,
		rpcapi.RPCMethodServerPetActionsGet,
		rpcapi.RPCMethodServerPetPixaDownload,
		rpcapi.RPCMethodRuntimeAdopt,
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
		rpcapi.RPCMethodServerToolGet:
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
	case rpcapi.RPCMethodServerModelList:
		return s.handleModelList(ctx, req), true, nil
	case rpcapi.RPCMethodServerModelGet:
		return s.handleModelGet(ctx, req), true, nil
	case rpcapi.RPCMethodServerVoiceList:
		return s.handleVoiceList(ctx, req), true, nil
	case rpcapi.RPCMethodServerVoiceGet:
		return s.handleVoiceGet(ctx, req), true, nil
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
	case rpcapi.RPCMethodServerFriendInfoGet:
		return s.handleFriendInfoGet(ctx, req), true, nil
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
	case rpcapi.RPCMethodRuntimeAdopt:
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
	default:
		return nil, false, nil
	}
}

func (s *Server) handleWorkspaceList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceListRequest)
	collection := strings.TrimSpace(params.Collection)
	if !ok || collection == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	if _, exists := profile.Spec.Workflows.Collections[collection]; !exists {
		return statusError(req.Id, http.StatusNotFound, "workflow collection not found")
	}
	items, err := s.effectiveWorkspacesByLabels(ctx, map[string]string{"collection": collection})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	prefix := strings.TrimSpace(valueOrZero(params.Prefix))
	byName := make(map[string]apitypes.Workspace)
	names := make([]string, 0, len(items))
	for _, item := range items {
		if prefix != "" && !strings.HasPrefix(item.Name, prefix) {
			continue
		}
		byName[item.Name] = item
		names = append(names, item.Name)
	}
	sort.Strings(names)
	pageNames, hasNext, nextCursor, conflict := pageAliases(names, params.Cursor, params.Limit, profile.Revision)
	if conflict {
		return statusError(req.Id, http.StatusConflict, "runtime profile revision changed")
	}
	page := make([]rpcapi.Workspace, 0, len(pageNames))
	for _, name := range pageNames {
		projected, err := workspaceRPCProjection(byName[name], workspaceAvailable(profile, byName[name]))
		if err != nil {
			return internalError(req.Id, err.Error())
		}
		page = append(page, projected)
	}
	return resultResponse(req.Id, rpcapi.WorkspaceListResponse{
		Items: page, HasNext: hasNext, NextCursor: nextCursor,
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromWorkspaceListResponse)
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

func workspaceRPCProjection(item apitypes.Workspace, available bool) (rpcapi.Workspace, error) {
	out := rpcapi.Workspace{
		CreatedAt: item.CreatedAt, LastActiveAt: item.LastActiveAt, Name: item.Name,
		OwnerPublicKey: item.OwnerPublicKey, System: item.System != nil && *item.System,
		UpdatedAt: item.UpdatedAt, WorkflowAlias: item.WorkflowName, Available: available,
	}
	if item.Parameters != nil {
		parameters, err := convertType[rpcapi.WorkspaceParameters](*item.Parameters)
		if err != nil {
			return rpcapi.Workspace{}, err
		}
		out.Parameters = &parameters
	}
	if item.Toolkit != nil {
		policy, err := convertType[rpcapi.ToolkitPolicy](*item.Toolkit)
		if err != nil {
			return rpcapi.Workspace{}, err
		}
		out.Toolkit = &policy
	}
	if item.Icon != nil {
		icon, err := convertType[rpcapi.Icon](*item.Icon)
		if err != nil {
			return rpcapi.Workspace{}, err
		}
		out.Icon = &icon
	}
	return out, nil
}

func workspaceAvailable(profile *apitypes.RuntimeProfile, item apitypes.Workspace) bool {
	if profile == nil {
		return false
	}
	if item.System != nil && *item.System {
		return true
	}
	if item.Labels == nil {
		return false
	}
	collection := strings.TrimSpace((*item.Labels)["collection"])
	bindings, ok := profile.Spec.Workflows.Collections[collection]
	if !ok {
		return false
	}
	_, ok = bindings[strings.TrimSpace(item.WorkflowName)]
	return ok
}

// ValidateRunWorkspaceSelection resolves a workspace selection and verifies
// that the current peer may use the canonical workspace resource.
func (s *Server) ValidateRunWorkspaceSelection(ctx context.Context, name string) (string, *rpcapi.RPCError) {
	if s == nil || s.Workspaces == nil {
		return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "workspace service not configured"}
	}
	resp, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: name})
	if err != nil {
		return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	}
	workspace, rpcResp, err := adminResult[apitypes.Workspace](resp.VisitGetWorkspaceResponse)
	if err != nil {
		return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	}
	if rpcResp != nil {
		if rpcResp.Error == nil {
			return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "workspace lookup returned an invalid response"}
		}
		return "", &rpcapi.RPCError{Code: rpcResp.Error.Code, Message: rpcResp.Error.Message}
	}
	canonicalName := strings.TrimSpace(workspace.Name)
	if canonicalName == "" || canonicalName != workspace.Name {
		return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "workspace service returned an invalid canonical name"}
	}
	allowed, err := s.canAccessWorkspace(ctx, workspace)
	if err != nil {
		return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	}
	if !allowed {
		return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: "workspace is not accessible to the authenticated peer"}
	}
	if !workspaceAvailable(s.currentRuntimeProfile(), workspace) {
		return "", &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "workspace workflow is not available in the current runtime profile"}
	}
	return canonicalName, nil
}

func (s *Server) handleWorkspaceGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	adminResp, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	item, rpcResp, err := adminResult[apitypes.Workspace](adminResp.VisitGetWorkspaceResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	allowed, err := s.canAccessWorkspace(ctx, item)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if !allowed {
		return statusError(req.Id, http.StatusNotFound, "workspace not found")
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	projected, err := workspaceRPCProjection(item, workspaceAvailable(profile, item))
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, rpcapi.WorkspaceGetResponse{
		Value: projected, RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromWorkspaceGetResponse)
}

func (s *Server) handleWorkspaceCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceCreateRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	collection := strings.TrimSpace(params.Collection)
	alias := strings.TrimSpace(params.WorkflowAlias)
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured"), true, nil
	}
	bindings, exists := profile.Spec.Workflows.Collections[collection]
	if collection == "" || alias == "" || !exists {
		return invalidParams(req.Id), true, nil
	}
	_, exists = bindings[alias]
	if !exists {
		return statusError(req.Id, http.StatusNotFound, "workflow not found"), true, nil
	}
	observability.Annotate(ctx, observability.AnnotationWorkspaceName, params.Name)
	observability.Annotate(ctx, observability.AnnotationWorkflowName, alias)
	parameters, err := convertType[*apitypes.WorkspaceParameters](params.Parameters)
	if err != nil {
		return nil, true, err
	}
	toolkitPolicy, err := convertType[*apitypes.ToolkitPolicy](params.Toolkit)
	if err != nil {
		return nil, true, err
	}
	labels := map[string]string{"collection": collection}
	body := adminhttp.CreateWorkspaceJSONRequestBody{
		Name: params.Name, WorkflowName: alias,
		Parameters: parameters, Toolkit: toolkitPolicy, Labels: &labels,
	}
	workspaceCtx := workspace.WithRuntimeWorkflowBindings(s.ownerContext(ctx), s.profileBindings(profileWorkflows))
	adminResp, err := s.Workspaces.CreateWorkspace(workspaceCtx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return workspaceAdminRPCResponse(ctx, req.Id, adminResp.VisitCreateWorkspaceResponse, func(payload *rpcapi.RPCPayload, item apitypes.Workspace) error {
		projected, err := workspaceRPCProjection(item, true)
		if err != nil {
			return err
		}
		return payload.FromWorkspaceCreateResponse(projected)
	}), true, nil
}

func (s *Server) handleWorkspacePut(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured"), true, nil
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspacePutRequest)
	if !ok {
		return invalidParams(req.Id), true, nil
	}
	currentResp, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	current, rpcResp, err := adminResult[apitypes.Workspace](currentResp.VisitGetWorkspaceResponse)
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp), true, nil
	}
	if response := s.requireOwner(req.Id, current.OwnerPublicKey); response != nil {
		return response, true, nil
	}
	body := adminhttp.PutWorkspaceJSONRequestBody{
		Name: current.Name, WorkflowName: current.WorkflowName,
		Parameters: current.Parameters, Toolkit: current.Toolkit,
	}
	if params.Body.Parameters != nil {
		parameters, err := convertType[*apitypes.WorkspaceParameters](params.Body.Parameters)
		if err != nil {
			return nil, true, err
		}
		body.Parameters = parameters
	}
	if params.Body.Toolkit != nil {
		toolkitPolicy, err := convertType[*apitypes.ToolkitPolicy](params.Body.Toolkit)
		if err != nil {
			return nil, true, err
		}
		body.Toolkit = toolkitPolicy
	}
	workspaceCtx := workspace.WithRuntimeWorkflowBindings(s.ownerContext(ctx), s.profileBindings(profileWorkflows))
	adminResp, err := s.Workspaces.PutWorkspace(workspaceCtx, adminhttp.PutWorkspaceRequestObject{Name: params.Name, Body: &body})
	if err != nil {
		return internalError(req.Id, err.Error()), true, nil
	}
	return workspaceAdminRPCResponse(ctx, req.Id, adminResp.VisitPutWorkspaceResponse, func(payload *rpcapi.RPCPayload, item apitypes.Workspace) error {
		projected, err := workspaceRPCProjection(item, workspaceAvailable(s.currentRuntimeProfile(), item))
		if err != nil {
			return err
		}
		return payload.FromWorkspacePutResponse(projected)
	}), true, nil
}

func (s *Server) handleWorkspaceDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workspaces == nil {
		return internalError(req.Id, "workspace service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	currentResp, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	current, rpcResp, err := adminResult[apitypes.Workspace](currentResp.VisitGetWorkspaceResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		return withRequestID(req.Id, rpcResp)
	}
	if response := s.requireOwner(req.Id, current.OwnerPublicKey); response != nil {
		return response
	}
	adminResp, err := s.Workspaces.DeleteWorkspace(s.ownerContext(ctx), adminhttp.DeleteWorkspaceRequestObject{Name: params.Name})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return workspaceAdminRPCResponse(ctx, req.Id, adminResp.VisitDeleteWorkspaceResponse, func(payload *rpcapi.RPCPayload, item apitypes.Workspace) error {
		projected, err := workspaceRPCProjection(item, workspaceAvailable(s.currentRuntimeProfile(), item))
		if err != nil {
			return err
		}
		return payload.FromWorkspaceDeleteResponse(projected)
	})
}

func (s *Server) handleWorkspaceHistoryList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	history, resp := s.workspaceHistoryService(req.Id)
	if resp != nil {
		return resp
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceHistoryListRequest)
	if !ok || strings.TrimSpace(params.WorkspaceName) == "" {
		return invalidParams(req.Id)
	}
	if params.Order != nil && !params.Order.Valid() {
		return statusError(req.Id, http.StatusBadRequest, "unsupported workspace history order")
	}
	if resp := s.requireWorkspaceAccess(ctx, req.Id, params.WorkspaceName); resp != nil {
		return resp
	}
	var order *apitypes.PeerRunHistoryListRequestOrder
	if params.Order != nil {
		converted := apitypes.PeerRunHistoryListRequestOrder(*params.Order)
		order = &converted
	}
	list, err := history.ListWorkspaceHistory(ctx, params.WorkspaceName, apitypes.PeerRunHistoryListRequest{
		Cursor: params.Cursor,
		Limit:  params.Limit,
		Order:  order,
	})
	if err != nil {
		return historyRPCResponse(req.Id, err)
	}
	return resultResponse(req.Id, list, (*rpcapi.RPCPayload).FromWorkspaceHistoryListResponse)
}

func (s *Server) handleWorkspaceHistoryGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	history, resp := s.workspaceHistoryService(req.Id)
	if resp != nil {
		return resp
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceHistoryGetRequest)
	if !ok || strings.TrimSpace(params.WorkspaceName) == "" || strings.TrimSpace(params.HistoryId) == "" {
		return invalidParams(req.Id)
	}
	if resp := s.requireWorkspaceAccess(ctx, req.Id, params.WorkspaceName); resp != nil {
		return resp
	}
	entry, err := history.GetWorkspaceHistory(ctx, params.WorkspaceName, params.HistoryId)
	if err != nil {
		return historyRPCResponse(req.Id, err)
	}
	return resultResponse(req.Id, entry.Public(), (*rpcapi.RPCPayload).FromWorkspaceHistoryGetResponse)
}

func (s *Server) handleWorkspaceHistoryAudioGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkspaceHistoryAudioGetRequest)
	if !ok || strings.TrimSpace(params.WorkspaceName) == "" || strings.TrimSpace(params.HistoryId) == "" {
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
	if strings.TrimSpace(params.WorkspaceName) == "" || strings.TrimSpace(params.HistoryId) == "" {
		return rpcapi.WorkspaceHistoryAudioGetResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid params"}, nil
	}
	history, resp := s.workspaceHistoryService("")
	if resp != nil {
		return rpcapi.WorkspaceHistoryAudioGetResponse{}, nil, &rpcapi.RPCError{Code: resp.Error.Code, Message: resp.Error.Message}, nil
	}
	if resp := s.requireWorkspaceAccess(ctx, "", params.WorkspaceName); resp != nil {
		return rpcapi.WorkspaceHistoryAudioGetResponse{}, nil, &rpcapi.RPCError{Code: resp.Error.Code, Message: resp.Error.Message}, nil
	}
	entry, err := history.GetWorkspaceHistory(ctx, params.WorkspaceName, params.HistoryId)
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
	r, err := history.ReadWorkspaceHistoryAsset(ctx, params.WorkspaceName, asset.Name)
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
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}
	default:
		return &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}
	}
}

func historyRPCResponse(requestID string, err error) *rpcapi.RPCResponse {
	rpcErr := historyRPCError(err)
	return rpcapi.Error{RequestID: requestID, Code: rpcErr.Code, Message: rpcErr.Message}.RPCResponse()
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
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkflowListRequest)
	if !ok || strings.TrimSpace(params.Collection) == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	bindings, exists := profile.Spec.Workflows.Collections[strings.TrimSpace(params.Collection)]
	if !exists {
		return statusError(req.Id, http.StatusNotFound, "workflow collection not found")
	}
	aliases := sortedBindingAliases(bindings)
	page, hasNext, nextCursor, conflict := pageAliases(aliases, params.Cursor, params.Limit, profile.Revision)
	if conflict {
		return statusError(req.Id, http.StatusConflict, "runtime profile revision changed")
	}
	items, err := s.listRuntimeWorkflows(ctx, params.Collection, bindings, page)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, rpcapi.WorkflowListResponse{
		Items: items, HasNext: hasNext, NextCursor: nextCursor,
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromWorkflowListResponse)
}

func (s *Server) listRuntimeWorkflows(ctx context.Context, collection string, bindings map[string]apitypes.RuntimeProfileBinding, aliases []string) ([]rpcapi.Workflow, error) {
	items := make([]rpcapi.Workflow, 0, len(aliases))
	for _, alias := range aliases {
		binding := bindings[alias]
		resp, err := s.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: binding.ResourceId})
		if err != nil {
			return nil, err
		}
		item, rpcResp, err := adminResult[apitypes.Workflow](resp.VisitGetWorkflowResponse)
		if err != nil {
			return nil, err
		}
		if isNotFoundResponse(rpcResp) {
			continue
		}
		if rpcResp != nil {
			return nil, fmt.Errorf("get runtime Workflow %q: %s", alias, rpcResp.Error.Message)
		}
		items = append(items, workflowRPCProjection(item, alias, collection, binding))
	}
	return items, nil
}

func (s *Server) handleWorkflowGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Workflows == nil {
		return internalError(req.Id, "workflow service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsWorkflowGetRequest)
	if !ok || strings.TrimSpace(params.Alias) == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	collection, binding, exists := workflowBinding(profile, params.Alias)
	if !exists {
		return statusError(req.Id, http.StatusNotFound, "workflow not found")
	}
	adminResp, err := s.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: binding.ResourceId})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	result, rpcResp, err := adminResult[apitypes.Workflow](adminResp.VisitGetWorkflowResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		if isNotFoundResponse(rpcResp) {
			return statusError(req.Id, http.StatusNotFound, "workflow not found")
		}
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, rpcapi.WorkflowGetResponse{
		Value:              workflowRPCProjection(result, params.Alias, collection, binding),
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromWorkflowGetResponse)
}

func workflowRPCProjection(item apitypes.Workflow, alias, collection string, binding apitypes.RuntimeProfileBinding) rpcapi.Workflow {
	return rpcapi.Workflow{
		Alias: alias, Collection: collection, I18n: bindingI18n(binding),
		Driver: rpcapi.WorkflowDriver(item.Spec.Driver),
	}
}

func (s *Server) handleModelList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsModelListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	bindings := bindingMap(profile.Spec.Resources.Models)
	aliases := sortedBindingAliases(bindings)
	page, hasNext, nextCursor, conflict := pageAliases(aliases, params.Cursor, params.Limit, profile.Revision)
	if conflict {
		return statusError(req.Id, http.StatusConflict, "runtime profile revision changed")
	}
	items := make([]rpcapi.Model, 0, len(page))
	for _, alias := range page {
		binding := bindings[alias]
		item, response := s.getModelValue(ctx, binding.ResourceId)
		if response != nil {
			if response.Error != nil && response.Error.Code == rpcapi.RPCErrorCodeNotFound {
				continue
			}
			return withRequestID(req.Id, response)
		}
		projected, err := modelRPCProjection(alias, binding, item)
		if err != nil {
			return internalError(req.Id, err.Error())
		}
		items = append(items, projected)
	}
	return resultResponse(req.Id, rpcapi.ModelListResponse{
		Items: items, HasNext: hasNext, NextCursor: nextCursor,
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromModelListResponse)
}

func (s *Server) handleModelGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsModelGetRequest)
	if !ok || strings.TrimSpace(params.Alias) == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	binding, exists := bindingMap(profile.Spec.Resources.Models)[strings.TrimSpace(params.Alias)]
	if !exists {
		return statusError(req.Id, http.StatusNotFound, "model not found")
	}
	item, response := s.getModelValue(ctx, binding.ResourceId)
	if response != nil {
		if isNotFoundResponse(response) {
			return statusError(req.Id, http.StatusNotFound, "model not found")
		}
		return withRequestID(req.Id, response)
	}
	projected, err := modelRPCProjection(params.Alias, binding, item)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, rpcapi.ModelGetResponse{
		Value: projected, RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromModelGetResponse)
}

func modelRPCProjection(alias string, binding apitypes.RuntimeProfileBinding, item apitypes.Model) (rpcapi.Model, error) {
	out := rpcapi.Model{Alias: alias, I18n: bindingI18n(binding), Kind: rpcapi.ModelKind(item.Kind)}
	if item.Capabilities != nil {
		capabilities, err := convertType[rpcapi.ModelCapabilities](*item.Capabilities)
		if err != nil {
			return rpcapi.Model{}, err
		}
		out.Capabilities = &capabilities
	}
	return out, nil
}

func (s *Server) handleVoiceList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Voices == nil {
		return internalError(req.Id, "voice service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsVoiceListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	bindings := bindingMap(profile.Spec.Resources.Voices)
	aliases := sortedBindingAliases(bindings)
	page, hasNext, nextCursor, conflict := pageAliases(aliases, params.Cursor, params.Limit, profile.Revision)
	if conflict {
		return statusError(req.Id, http.StatusConflict, "runtime profile revision changed")
	}
	items := make([]rpcapi.Voice, 0, len(page))
	for _, alias := range page {
		binding := bindings[alias]
		resp, err := s.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: binding.ResourceId})
		if err != nil {
			return internalError(req.Id, err.Error())
		}
		_, rpcResp, err := adminResult[apitypes.Voice](resp.VisitGetVoiceResponse)
		if err != nil {
			return internalError(req.Id, err.Error())
		}
		if isNotFoundResponse(rpcResp) {
			continue
		}
		if rpcResp != nil {
			return withRequestID(req.Id, rpcResp)
		}
		items = append(items, rpcapi.Voice{Alias: alias, I18n: bindingI18n(binding)})
	}
	return resultResponse(req.Id, rpcapi.VoiceListResponse{
		Items: items, HasNext: hasNext, NextCursor: nextCursor,
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromVoiceListResponse)
}

func (s *Server) handleVoiceGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Voices == nil {
		return internalError(req.Id, "voice service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsVoiceGetRequest)
	if !ok || strings.TrimSpace(params.Alias) == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	binding, exists := bindingMap(profile.Spec.Resources.Voices)[strings.TrimSpace(params.Alias)]
	if !exists {
		return statusError(req.Id, http.StatusNotFound, "voice not found")
	}
	adminResp, err := s.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: binding.ResourceId})
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	_, rpcResp, err := adminResult[apitypes.Voice](adminResp.VisitGetVoiceResponse)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if rpcResp != nil {
		if isNotFoundResponse(rpcResp) {
			return statusError(req.Id, http.StatusNotFound, "voice not found")
		}
		return withRequestID(req.Id, rpcResp)
	}
	return resultResponse(req.Id, rpcapi.VoiceGetResponse{
		Value:              rpcapi.Voice{Alias: params.Alias, I18n: bindingI18n(binding)},
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromVoiceGetResponse)
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

func workspaceAdminRPCResponse[T any](ctx context.Context, id string, visit func(*fiber.Ctx) error, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCResponse {
	result, rpcResp, errorCode, err := adminResultWithCode[apitypes.Workspace](visit)
	if err != nil {
		return internalError(id, err.Error())
	}
	if rpcResp != nil {
		observability.SetErrorCode(ctx, errorCode)
		return withRequestID(id, rpcResp)
	}
	return resultResponse(id, result, encode)
}

func adminResult[T any](visit func(*fiber.Ctx) error) (T, *rpcapi.RPCResponse, error) {
	result, rpcResp, _, err := adminResultWithCode[T](visit)
	return result, rpcResp, err
}

func adminResultWithCode[T any](visit func(*fiber.Ctx) error) (T, *rpcapi.RPCResponse, string, error) {
	var result T
	status, body, err := renderAdminResponse(visit)
	if err != nil {
		return result, nil, "", err
	}
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &result); err != nil {
			return result, nil, "", err
		}
		return result, nil, "", nil
	}
	var apiErr apitypes.ErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && (apiErr.Error.Code != "" || apiErr.Error.Message != "") {
		message := apiErr.Error.Message
		if message == "" {
			message = http.StatusText(status)
		}
		return result, statusError("", status, message), apiErr.Error.Code, nil
	}
	return result, statusError("", status, http.StatusText(status)), "", nil
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
	if dst.Kind() == reflect.Bool && src.Kind() == reflect.Pointer && src.Type().Elem().Kind() == reflect.Bool {
		if !src.IsNil() {
			dst.SetBool(src.Elem().Bool())
		}
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
		CreatedAt:          in.CreatedAt,
		DisplayName:        in.DisplayName,
		Id:                 in.Id,
		LastActiveAt:       in.LastActiveAt,
		Life:               rpcapi.PetLife(in.Life),
		OwnerPublicKey:     in.OwnerPublicKey,
		PetdefId:           in.PetdefId,
		Progression:        rpcapi.PetProgression(in.Progression),
		RuntimeProfileName: in.RuntimeProfileName,
		UpdatedAt:          in.UpdatedAt,
		WorkspaceName:      in.WorkspaceName,
	}
}

func rpcPetToAPI(in rpcapi.Pet) apitypes.Pet {
	return apitypes.Pet{
		CreatedAt:          in.CreatedAt,
		DisplayName:        in.DisplayName,
		Id:                 in.Id,
		LastActiveAt:       in.LastActiveAt,
		Life:               apitypes.PetLife(in.Life),
		OwnerPublicKey:     in.OwnerPublicKey,
		PetdefId:           in.PetdefId,
		Progression:        apitypes.PetProgression(in.Progression),
		RuntimeProfileName: in.RuntimeProfileName,
		UpdatedAt:          in.UpdatedAt,
		WorkspaceName:      in.WorkspaceName,
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

func invalidParams(id string) *rpcapi.RPCResponse {
	return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid params"}.RPCResponse()
}

func internalError(id, message string) *rpcapi.RPCResponse {
	return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCodeInternalError, Message: message}.RPCResponse()
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
