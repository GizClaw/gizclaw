package gizclaw

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
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkspaceList, request, (*rpcapi.RPCRequest_Params).FromWorkspaceListRequest, rpcapi.RPCResponse_Result.AsWorkspaceListResponse, "workspace list")
}

func (c *rpcClient) GetWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceGetRequest) (*rpcapi.WorkspaceGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkspaceGet, request, (*rpcapi.RPCRequest_Params).FromWorkspaceGetRequest, rpcapi.RPCResponse_Result.AsWorkspaceGetResponse, "workspace get")
}

func (c *rpcClient) CreateWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceCreateRequest) (*rpcapi.WorkspaceCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkspaceCreate, request, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.RPCResponse_Result.AsWorkspaceCreateResponse, "workspace create")
}

func (c *rpcClient) PutWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspacePutRequest) (*rpcapi.WorkspacePutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkspacePut, request, (*rpcapi.RPCRequest_Params).FromWorkspacePutRequest, rpcapi.RPCResponse_Result.AsWorkspacePutResponse, "workspace put")
}

func (c *rpcClient) DeleteWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceDeleteRequest) (*rpcapi.WorkspaceDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkspaceDelete, request, (*rpcapi.RPCRequest_Params).FromWorkspaceDeleteRequest, rpcapi.RPCResponse_Result.AsWorkspaceDeleteResponse, "workspace delete")
}

func (c *rpcClient) ListWorkflows(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowListRequest) (*rpcapi.WorkflowListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkflowList, request, (*rpcapi.RPCRequest_Params).FromWorkflowListRequest, rpcapi.RPCResponse_Result.AsWorkflowListResponse, "workflow list")
}

func (c *rpcClient) GetWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowGetRequest) (*rpcapi.WorkflowGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkflowGet, request, (*rpcapi.RPCRequest_Params).FromWorkflowGetRequest, rpcapi.RPCResponse_Result.AsWorkflowGetResponse, "workflow get")
}

func (c *rpcClient) CreateWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowCreateRequest) (*rpcapi.WorkflowCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkflowCreate, request, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, rpcapi.RPCResponse_Result.AsWorkflowCreateResponse, "workflow create")
}

func (c *rpcClient) PutWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowPutRequest) (*rpcapi.WorkflowPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkflowPut, request, (*rpcapi.RPCRequest_Params).FromWorkflowPutRequest, rpcapi.RPCResponse_Result.AsWorkflowPutResponse, "workflow put")
}

func (c *rpcClient) DeleteWorkflow(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowDeleteRequest) (*rpcapi.WorkflowDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodWorkflowDelete, request, (*rpcapi.RPCRequest_Params).FromWorkflowDeleteRequest, rpcapi.RPCResponse_Result.AsWorkflowDeleteResponse, "workflow delete")
}

func (c *rpcClient) ListModels(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelListRequest) (*rpcapi.ModelListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodModelList, request, (*rpcapi.RPCRequest_Params).FromModelListRequest, rpcapi.RPCResponse_Result.AsModelListResponse, "model list")
}

func (c *rpcClient) GetModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelGetRequest) (*rpcapi.ModelGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodModelGet, request, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.RPCResponse_Result.AsModelGetResponse, "model get")
}

func (c *rpcClient) CreateModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelCreateRequest) (*rpcapi.ModelCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodModelCreate, request, (*rpcapi.RPCRequest_Params).FromModelCreateRequest, rpcapi.RPCResponse_Result.AsModelCreateResponse, "model create")
}

func (c *rpcClient) PutModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelPutRequest) (*rpcapi.ModelPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodModelPut, request, (*rpcapi.RPCRequest_Params).FromModelPutRequest, rpcapi.RPCResponse_Result.AsModelPutResponse, "model put")
}

func (c *rpcClient) DeleteModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelDeleteRequest) (*rpcapi.ModelDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodModelDelete, request, (*rpcapi.RPCRequest_Params).FromModelDeleteRequest, rpcapi.RPCResponse_Result.AsModelDeleteResponse, "model delete")
}

func (c *rpcClient) ListCredentials(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialListRequest) (*rpcapi.CredentialListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodCredentialList, request, (*rpcapi.RPCRequest_Params).FromCredentialListRequest, rpcapi.RPCResponse_Result.AsCredentialListResponse, "credential list")
}

func (c *rpcClient) GetCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialGetRequest) (*rpcapi.CredentialGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodCredentialGet, request, (*rpcapi.RPCRequest_Params).FromCredentialGetRequest, rpcapi.RPCResponse_Result.AsCredentialGetResponse, "credential get")
}

func (c *rpcClient) CreateCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialCreateRequest) (*rpcapi.CredentialCreateResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodCredentialCreate, request, (*rpcapi.RPCRequest_Params).FromCredentialCreateRequest, rpcapi.RPCResponse_Result.AsCredentialCreateResponse, "credential create")
}

func (c *rpcClient) PutCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialPutRequest) (*rpcapi.CredentialPutResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodCredentialPut, request, (*rpcapi.RPCRequest_Params).FromCredentialPutRequest, rpcapi.RPCResponse_Result.AsCredentialPutResponse, "credential put")
}

func (c *rpcClient) DeleteCredential(ctx context.Context, conn net.Conn, id string, request rpcapi.CredentialDeleteRequest) (*rpcapi.CredentialDeleteResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodCredentialDelete, request, (*rpcapi.RPCRequest_Params).FromCredentialDeleteRequest, rpcapi.RPCResponse_Result.AsCredentialDeleteResponse, "credential delete")
}
