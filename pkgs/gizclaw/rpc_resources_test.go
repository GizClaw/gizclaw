package gizclaw

import (
	"context"
	"net"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCClientSafeResourceMethods(t *testing.T) {
	server := &rpcServer{serverResources: &fakeRPCServerResourceService{t: t}}
	client := &rpcClient{}

	workspaceList := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspaceListResponse, error) {
		return client.ListWorkspaces(context.Background(), conn, "workspace-list", rpcapi.WorkspaceListRequest{Collection: "assistants"})
	})
	if len(workspaceList.Items) != 1 || workspaceList.Items[0].Name != "workspace-a" {
		t.Fatalf("ListWorkspaces() = %+v", workspaceList)
	}
	workspace := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspaceGetResponse, error) {
		return client.GetWorkspace(context.Background(), conn, "workspace-get", rpcapi.WorkspaceGetRequest{Name: "workspace-a"})
	})
	if workspace.Value.Name != "workspace-a" || workspace.Value.WorkflowAlias != "flow-a" {
		t.Fatalf("GetWorkspace() = %+v", workspace)
	}
	created := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspaceCreateResponse, error) {
		return client.CreateWorkspace(context.Background(), conn, "workspace-create", rpcapi.WorkspaceCreateRequest{Name: "workspace-a", Collection: "assistants", WorkflowAlias: "flow-a"})
	})
	if created.WorkflowAlias != "flow-a" {
		t.Fatalf("CreateWorkspace() = %+v", created)
	}
	updated := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspacePutResponse, error) {
		return client.PutWorkspace(context.Background(), conn, "workspace-put", rpcapi.WorkspacePutRequest{Name: "workspace-a", Body: rpcapi.WorkspacePutBody{}})
	})
	if updated.Name != "workspace-a" {
		t.Fatalf("PutWorkspace() = %+v", updated)
	}

	workflowList := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowListResponse, error) {
		return client.ListWorkflows(context.Background(), conn, "workflow-list", rpcapi.WorkflowListRequest{Collection: "assistants"})
	})
	if len(workflowList.Items) != 1 || workflowList.Items[0].Alias != "flow-a" {
		t.Fatalf("ListWorkflows() = %+v", workflowList)
	}
	workflow := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
		return client.GetWorkflow(context.Background(), conn, "workflow-get", rpcapi.WorkflowGetRequest{Alias: "flow-a"})
	})
	if workflow.Value.Alias != "flow-a" {
		t.Fatalf("GetWorkflow() = %+v", workflow)
	}

	modelList := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ModelListResponse, error) {
		return client.ListModels(context.Background(), conn, "model-list", rpcapi.ModelListRequest{})
	})
	if len(modelList.Items) != 1 || modelList.Items[0].Alias != "model-a" {
		t.Fatalf("ListModels() = %+v", modelList)
	}
	model := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ModelGetResponse, error) {
		return client.GetModel(context.Background(), conn, "model-get", rpcapi.ModelGetRequest{Alias: "model-a"})
	})
	if model.Value.Alias != "model-a" {
		t.Fatalf("GetModel() = %+v", model)
	}
}

type fakeRPCServerResourceService struct{ t *testing.T }

func (f *fakeRPCServerResourceService) Dispatch(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	f.t.Helper()
	if req == nil || req.Params == nil {
		f.t.Fatal("resource request or params is nil")
	}
	switch req.Method {
	case rpcapi.RPCMethodServerWorkspaceList:
		params, err := req.Params.AsWorkspaceListRequest()
		if err != nil || params.Collection != "assistants" {
			f.t.Fatalf("workspace.list params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, rpcapi.WorkspaceListResponse{Items: []rpcapi.Workspace{resourceWorkspace("workspace-a")}, RuntimeProfileName: "default", RuntimeProfileRevision: "rev"}, (*rpcapi.RPCPayload).FromWorkspaceListResponse), true, nil
	case rpcapi.RPCMethodServerWorkspaceGet:
		return resourceResponse(req.Id, rpcapi.WorkspaceGetResponse{Value: resourceWorkspace("workspace-a"), RuntimeProfileName: "default", RuntimeProfileRevision: "rev"}, (*rpcapi.RPCPayload).FromWorkspaceGetResponse), true, nil
	case rpcapi.RPCMethodServerWorkspaceCreate:
		params, err := req.Params.AsWorkspaceCreateRequest()
		if err != nil || params.Collection != "assistants" || params.WorkflowAlias != "flow-a" {
			f.t.Fatalf("workspace.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCPayload).FromWorkspaceCreateResponse), true, nil
	case rpcapi.RPCMethodServerWorkspacePut:
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCPayload).FromWorkspacePutResponse), true, nil
	case rpcapi.RPCMethodServerWorkflowList:
		return resourceResponse(req.Id, rpcapi.WorkflowListResponse{Items: []rpcapi.Workflow{resourceWorkflow("flow-a")}, RuntimeProfileName: "default", RuntimeProfileRevision: "rev"}, (*rpcapi.RPCPayload).FromWorkflowListResponse), true, nil
	case rpcapi.RPCMethodServerWorkflowGet:
		return resourceResponse(req.Id, rpcapi.WorkflowGetResponse{Value: resourceWorkflow("flow-a"), RuntimeProfileName: "default", RuntimeProfileRevision: "rev"}, (*rpcapi.RPCPayload).FromWorkflowGetResponse), true, nil
	case rpcapi.RPCMethodServerModelList:
		return resourceResponse(req.Id, rpcapi.ModelListResponse{Items: []rpcapi.Model{resourceModel("model-a")}, RuntimeProfileName: "default", RuntimeProfileRevision: "rev"}, (*rpcapi.RPCPayload).FromModelListResponse), true, nil
	case rpcapi.RPCMethodServerModelGet:
		return resourceResponse(req.Id, rpcapi.ModelGetResponse{Value: resourceModel("model-a"), RuntimeProfileName: "default", RuntimeProfileRevision: "rev"}, (*rpcapi.RPCPayload).FromModelGetResponse), true, nil
	default:
		f.t.Fatalf("unexpected method %s", req.Method)
		return nil, false, nil
	}
}

func resourceResponse[T any](id string, value T, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCResponse {
	resp, err := newRPCResultResponse(id, value, encode)
	if err != nil {
		panic(err)
	}
	return resp
}

func resourceWorkspace(name string) rpcapi.Workspace {
	return rpcapi.Workspace{Name: name, WorkflowAlias: "flow-a"}
}

func resourceWorkflow(alias string) rpcapi.Workflow {
	return rpcapi.Workflow{Alias: alias, Collection: "assistants", Driver: rpcapi.WorkflowDriverFlowcraft, I18n: resourceI18n(alias)}
}

func resourceModel(alias string) rpcapi.Model {
	return rpcapi.Model{Alias: alias, Kind: rpcapi.ModelKindLlm, I18n: resourceI18n(alias)}
}

func resourceI18n(name string) map[string]rpcapi.AliasI18nText {
	return map[string]rpcapi.AliasI18nText{"en": {DisplayName: name}, "zh-CN": {DisplayName: name}}
}
