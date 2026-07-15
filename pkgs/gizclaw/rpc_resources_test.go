package gizclaw

import (
	"context"
	"net"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCClientResourceMethods(t *testing.T) {
	server := &rpcServer{serverResources: &fakeRPCServerResourceService{t: t}}
	client := &rpcClient{}

	workspaceList := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspaceListResponse, error) {
		return client.ListWorkspaces(context.Background(), conn, "workspace-list", rpcapi.WorkspaceListRequest{})
	})
	if len(workspaceList.Items) != 1 || workspaceList.Items[0].Name != "workspace-a" {
		t.Fatalf("ListWorkspaces() = %+v", workspaceList)
	}

	workspace := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspaceGetResponse, error) {
		return client.GetWorkspace(context.Background(), conn, "workspace-get", rpcapi.WorkspaceGetRequest{Name: "workspace-a"})
	})
	if workspace.Name != "workspace-a" {
		t.Fatalf("GetWorkspace() = %+v", workspace)
	}
	workspace = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspaceCreateResponse, error) {
		return client.CreateWorkspace(context.Background(), conn, "workspace-create", rpcapi.WorkspaceCreateRequest{Name: "workspace-a", WorkflowName: "flow-a"})
	})
	if workspace.WorkflowName != "flow-a" {
		t.Fatalf("CreateWorkspace() = %+v", workspace)
	}
	workspace = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspacePutResponse, error) {
		return client.PutWorkspace(context.Background(), conn, "workspace-put", rpcapi.WorkspacePutRequest{Name: "workspace-a", Body: resourceWorkspace("workspace-a")})
	})
	if workspace.Name != "workspace-a" {
		t.Fatalf("PutWorkspace() = %+v", workspace)
	}
	workspace = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkspaceDeleteResponse, error) {
		return client.DeleteWorkspace(context.Background(), conn, "workspace-delete", rpcapi.WorkspaceDeleteRequest{Name: "workspace-a"})
	})
	if workspace.Name != "workspace-a" {
		t.Fatalf("DeleteWorkspace() = %+v", workspace)
	}

	workflowList := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowListResponse, error) {
		return client.ListWorkflows(context.Background(), conn, "workflow-list", rpcapi.WorkflowListRequest{})
	})
	if len(workflowList.Items) != 1 || workflowList.Items[0].Name != "flow-a" {
		t.Fatalf("ListWorkflows() = %+v", workflowList)
	}
	workflow := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
		return client.GetWorkflow(context.Background(), conn, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow-a"})
	})
	if workflow.Name != "flow-a" {
		t.Fatalf("GetWorkflow() = %+v", workflow)
	}

	modelList := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ModelListResponse, error) {
		return client.ListModels(context.Background(), conn, "model-list", rpcapi.ModelListRequest{})
	})
	if len(modelList.Items) != 1 || modelList.Items[0].Id != "model-a" {
		t.Fatalf("ListModels() = %+v", modelList)
	}
	model := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ModelGetResponse, error) {
		return client.GetModel(context.Background(), conn, "model-get", rpcapi.ModelGetRequest{Id: "model-a"})
	})
	if model.Id != "model-a" {
		t.Fatalf("GetModel() = %+v", model)
	}
	model = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ModelCreateResponse, error) {
		return client.CreateModel(context.Background(), conn, "model-create", resourceModel("model-a"))
	})
	if model.Id != "model-a" {
		t.Fatalf("CreateModel() = %+v", model)
	}
	model = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ModelPutResponse, error) {
		return client.PutModel(context.Background(), conn, "model-put", rpcapi.ModelPutRequest{Id: "model-a", Body: resourceModel("model-a")})
	})
	if model.Id != "model-a" {
		t.Fatalf("PutModel() = %+v", model)
	}
	model = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ModelDeleteResponse, error) {
		return client.DeleteModel(context.Background(), conn, "model-delete", rpcapi.ModelDeleteRequest{Id: "model-a"})
	})
	if model.Id != "model-a" {
		t.Fatalf("DeleteModel() = %+v", model)
	}

	credentialList := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.CredentialListResponse, error) {
		return client.ListCredentials(context.Background(), conn, "credential-list", rpcapi.CredentialListRequest{})
	})
	if len(credentialList.Items) != 1 || credentialList.Items[0].Name != "credential-a" {
		t.Fatalf("ListCredentials() = %+v", credentialList)
	}
	credential := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.CredentialGetResponse, error) {
		return client.GetCredential(context.Background(), conn, "credential-get", rpcapi.CredentialGetRequest{Name: "credential-a"})
	})
	if credential.Name != "credential-a" {
		t.Fatalf("GetCredential() = %+v", credential)
	}
	credential = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.CredentialCreateResponse, error) {
		return client.CreateCredential(context.Background(), conn, "credential-create", resourceCredential("credential-a"))
	})
	if credential.Name != "credential-a" {
		t.Fatalf("CreateCredential() = %+v", credential)
	}
	credential = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.CredentialPutResponse, error) {
		return client.PutCredential(context.Background(), conn, "credential-put", rpcapi.CredentialPutRequest{Name: "credential-a", Body: resourceCredential("credential-a")})
	})
	if credential.Name != "credential-a" {
		t.Fatalf("PutCredential() = %+v", credential)
	}
	credential = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.CredentialDeleteResponse, error) {
		return client.DeleteCredential(context.Background(), conn, "credential-delete", rpcapi.CredentialDeleteRequest{Name: "credential-a"})
	})
	if credential.Name != "credential-a" {
		t.Fatalf("DeleteCredential() = %+v", credential)
	}
}

type fakeRPCServerResourceService struct {
	t *testing.T
}

func (f *fakeRPCServerResourceService) Dispatch(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	f.t.Helper()

	if req == nil {
		f.t.Fatal("resource request = nil")
	}
	if req.Id == "" {
		f.t.Fatal("resource request id = empty")
	}
	if req.Params == nil {
		f.t.Fatalf("%s params = nil", req.Method)
	}

	switch req.Method {
	case rpcapi.RPCMethodServerWorkspaceList:
		if _, err := req.Params.AsWorkspaceListRequest(); err != nil {
			f.t.Fatalf("workspace.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.WorkspaceListResponse{Items: []rpcapi.Workspace{resourceWorkspace("workspace-a")}}, (*rpcapi.RPCPayload).FromWorkspaceListResponse), true, nil
	case rpcapi.RPCMethodServerWorkspaceGet:
		params, err := req.Params.AsWorkspaceGetRequest()
		if err != nil || params.Name != "workspace-a" {
			f.t.Fatalf("workspace.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCPayload).FromWorkspaceGetResponse), true, nil
	case rpcapi.RPCMethodServerWorkspaceCreate:
		params, err := req.Params.AsWorkspaceCreateRequest()
		if err != nil || params.Name != "workspace-a" || params.WorkflowName != "flow-a" {
			f.t.Fatalf("workspace.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCPayload).FromWorkspaceCreateResponse), true, nil
	case rpcapi.RPCMethodServerWorkspacePut:
		params, err := req.Params.AsWorkspacePutRequest()
		if err != nil || params.Name != "workspace-a" || params.Body.WorkflowName != "flow-a" {
			f.t.Fatalf("workspace.put params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCPayload).FromWorkspacePutResponse), true, nil
	case rpcapi.RPCMethodServerWorkspaceDelete:
		params, err := req.Params.AsWorkspaceDeleteRequest()
		if err != nil || params.Name != "workspace-a" {
			f.t.Fatalf("workspace.delete params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCPayload).FromWorkspaceDeleteResponse), true, nil
	case rpcapi.RPCMethodServerWorkflowList:
		if _, err := req.Params.AsWorkflowListRequest(); err != nil {
			f.t.Fatalf("workflow.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.WorkflowListResponse{Items: []rpcapi.Workflow{resourceWorkflowDoc("flow-a")}}, (*rpcapi.RPCPayload).FromWorkflowListResponse), true, nil
	case rpcapi.RPCMethodServerWorkflowGet:
		params, err := req.Params.AsWorkflowGetRequest()
		if err != nil || params.Name != "flow-a" {
			f.t.Fatalf("workflow.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkflowDoc("flow-a"), (*rpcapi.RPCPayload).FromWorkflowGetResponse), true, nil
	case rpcapi.RPCMethodServerModelList:
		if _, err := req.Params.AsModelListRequest(); err != nil {
			f.t.Fatalf("model.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.ModelListResponse{Items: []rpcapi.Model{resourceModel("model-a")}}, (*rpcapi.RPCPayload).FromModelListResponse), true, nil
	case rpcapi.RPCMethodServerModelGet:
		params, err := req.Params.AsModelGetRequest()
		if err != nil || params.Id != "model-a" {
			f.t.Fatalf("model.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCPayload).FromModelGetResponse), true, nil
	case rpcapi.RPCMethodServerModelCreate:
		params, err := req.Params.AsModelCreateRequest()
		if err != nil || params.Id != "model-a" {
			f.t.Fatalf("model.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCPayload).FromModelCreateResponse), true, nil
	case rpcapi.RPCMethodServerModelPut:
		params, err := req.Params.AsModelPutRequest()
		if err != nil || params.Id != "model-a" || params.Body.Id != "model-a" {
			f.t.Fatalf("model.put params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCPayload).FromModelPutResponse), true, nil
	case rpcapi.RPCMethodServerModelDelete:
		params, err := req.Params.AsModelDeleteRequest()
		if err != nil || params.Id != "model-a" {
			f.t.Fatalf("model.delete params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCPayload).FromModelDeleteResponse), true, nil
	case rpcapi.RPCMethodServerCredentialList:
		if _, err := req.Params.AsCredentialListRequest(); err != nil {
			f.t.Fatalf("credential.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.CredentialListResponse{Items: []rpcapi.Credential{resourceCredential("credential-a")}}, (*rpcapi.RPCPayload).FromCredentialListResponse), true, nil
	case rpcapi.RPCMethodServerCredentialGet:
		params, err := req.Params.AsCredentialGetRequest()
		if err != nil || params.Name != "credential-a" {
			f.t.Fatalf("credential.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCPayload).FromCredentialGetResponse), true, nil
	case rpcapi.RPCMethodServerCredentialCreate:
		params, err := req.Params.AsCredentialCreateRequest()
		if err != nil || params.Name != "credential-a" {
			f.t.Fatalf("credential.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCPayload).FromCredentialCreateResponse), true, nil
	case rpcapi.RPCMethodServerCredentialPut:
		params, err := req.Params.AsCredentialPutRequest()
		if err != nil || params.Name != "credential-a" || params.Body.Name != "credential-a" {
			f.t.Fatalf("credential.put params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCPayload).FromCredentialPutResponse), true, nil
	case rpcapi.RPCMethodServerCredentialDelete:
		params, err := req.Params.AsCredentialDeleteRequest()
		if err != nil || params.Name != "credential-a" {
			f.t.Fatalf("credential.delete params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCPayload).FromCredentialDeleteResponse), true, nil
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
	return rpcapi.Workspace{Name: name, WorkflowName: "flow-a"}
}

func resourceWorkflowDoc(name string) rpcapi.Workflow {
	spec := rpcapi.FlowcraftWorkflowSpec{"entry_agent": ""}
	return rpcapi.Workflow{
		Name: name,
		Spec: rpcapi.WorkflowSpec{
			Driver:    rpcapi.WorkflowDriverFlowcraft,
			Flowcraft: &spec,
		},
	}
}

func resourceModel(id string) rpcapi.Model {
	return rpcapi.Model{
		Id:     id,
		Kind:   rpcapi.ModelKindLlm,
		Source: rpcapi.ModelSourceManual,
		Provider: rpcapi.ModelProvider{
			Kind: rpcapi.ModelProviderKindOpenaiTenant,
			Name: "global",
		},
	}
}

func resourceCredential(name string) rpcapi.Credential {
	return rpcapi.Credential{
		Name:     name,
		Provider: "openai",
		Body:     testRPCOpenAICredentialBody("sk-test"),
	}
}
