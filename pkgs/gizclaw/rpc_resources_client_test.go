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

func (c *rpcClient) ListModels(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelListRequest) (*rpcapi.ModelListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelList, request, (*rpcapi.RPCPayload).FromModelListRequest, rpcapi.RPCPayload.AsModelListResponse, "model list")
}

func (c *rpcClient) GetModel(ctx context.Context, conn net.Conn, id string, request rpcapi.ModelGetRequest) (*rpcapi.ModelGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerModelGet, request, (*rpcapi.RPCPayload).FromModelGetRequest, rpcapi.RPCPayload.AsModelGetResponse, "model get")
}
