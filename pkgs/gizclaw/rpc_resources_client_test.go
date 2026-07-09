package gizclaw

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
