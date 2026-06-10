package gearresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/credential"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/model"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/gofiber/fiber/v2"
)

type Authorizer interface {
	Authorize(context.Context, acl.AuthorizeRequest) error
}

type Server struct {
	Caller      giznet.PublicKey
	ACL         Authorizer
	Workspaces  workspace.WorkspaceAdminService
	Workflows   workflow.WorkflowAdminService
	Models      model.ModelAdminService
	Credentials credential.CredentialAdminService
}

func IsMethod(method rpcapi.RPCMethod) bool {
	switch method {
	case rpcapi.RPCMethodWorkspaceList,
		rpcapi.RPCMethodWorkspaceGet,
		rpcapi.RPCMethodWorkspaceCreate,
		rpcapi.RPCMethodWorkspacePut,
		rpcapi.RPCMethodWorkspaceDelete,
		rpcapi.RPCMethodWorkflowList,
		rpcapi.RPCMethodWorkflowGet,
		rpcapi.RPCMethodWorkflowCreate,
		rpcapi.RPCMethodWorkflowPut,
		rpcapi.RPCMethodWorkflowDelete,
		rpcapi.RPCMethodModelList,
		rpcapi.RPCMethodModelGet,
		rpcapi.RPCMethodModelCreate,
		rpcapi.RPCMethodModelPut,
		rpcapi.RPCMethodModelDelete,
		rpcapi.RPCMethodCredentialList,
		rpcapi.RPCMethodCredentialGet,
		rpcapi.RPCMethodCredentialCreate,
		rpcapi.RPCMethodCredentialPut,
		rpcapi.RPCMethodCredentialDelete:
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
	case rpcapi.RPCMethodWorkspaceList:
		return s.handleWorkspaceList(ctx, req), true, nil
	case rpcapi.RPCMethodWorkspaceGet:
		return s.handleWorkspaceGet(ctx, req), true, nil
	case rpcapi.RPCMethodWorkspaceCreate:
		return s.handleWorkspaceCreate(ctx, req)
	case rpcapi.RPCMethodWorkspacePut:
		return s.handleWorkspacePut(ctx, req)
	case rpcapi.RPCMethodWorkspaceDelete:
		return s.handleWorkspaceDelete(ctx, req), true, nil
	case rpcapi.RPCMethodWorkflowList:
		return s.handleWorkflowList(ctx, req), true, nil
	case rpcapi.RPCMethodWorkflowGet:
		return s.handleWorkflowGet(ctx, req), true, nil
	case rpcapi.RPCMethodWorkflowCreate:
		return s.handleWorkflowCreate(ctx, req)
	case rpcapi.RPCMethodWorkflowPut:
		return s.handleWorkflowPut(ctx, req)
	case rpcapi.RPCMethodWorkflowDelete:
		return s.handleWorkflowDelete(ctx, req), true, nil
	case rpcapi.RPCMethodModelList:
		return s.handleModelList(ctx, req), true, nil
	case rpcapi.RPCMethodModelGet:
		return s.handleModelGet(ctx, req), true, nil
	case rpcapi.RPCMethodModelCreate:
		return s.handleModelCreate(ctx, req)
	case rpcapi.RPCMethodModelPut:
		return s.handleModelPut(ctx, req)
	case rpcapi.RPCMethodModelDelete:
		return s.handleModelDelete(ctx, req), true, nil
	case rpcapi.RPCMethodCredentialList:
		return s.handleCredentialList(ctx, req), true, nil
	case rpcapi.RPCMethodCredentialGet:
		return s.handleCredentialGet(ctx, req), true, nil
	case rpcapi.RPCMethodCredentialCreate:
		return s.handleCredentialCreate(ctx, req)
	case rpcapi.RPCMethodCredentialPut:
		return s.handleCredentialPut(ctx, req)
	case rpcapi.RPCMethodCredentialDelete:
		return s.handleCredentialDelete(ctx, req), true, nil
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
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsModelListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := s.Models.ListModels(ctx, adminservice.ListModelsRequestObject{
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
	items := make([]apitypes.Model, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, acl.ModelResource(item.Id), apitypes.ACLPermissionModelRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
		}
		items = append(items, item)
	}
	return resultResponse(req.Id, adminservice.ModelList{Items: items, HasNext: list.HasNext, NextCursor: list.NextCursor}, (*rpcapi.RPCResponse_Result).FromModelListResponse)
}

func (s *Server) handleModelGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Models == nil {
		return internalError(req.Id, "model service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsModelGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if resp := s.authorizeResponse(ctx, req.Id, acl.ModelResource(params.Id), apitypes.ACLPermissionModelRead); resp != nil {
		return resp
	}
	adminResp, err := s.Models.GetModel(ctx, adminservice.GetModelRequestObject{Id: params.Id})
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
	if resp := s.authorizeResponse(ctx, req.Id, acl.CredentialResource(params.Name), apitypes.ACLPermissionCredentialRead); resp != nil {
		return resp
	}
	adminResp, err := s.Credentials.GetCredential(ctx, adminservice.GetCredentialRequestObject{Name: params.Name})
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
	return s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(s.Caller.String()),
		Resource:   resource,
		Permission: permission,
	})
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
	return fmt.Sprintf("gearresource.Server{%s}", s.Caller.String())
}
