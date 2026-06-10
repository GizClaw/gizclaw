package gizclaw

import (
	"context"
	"net"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func TestRPCClientResourceMethods(t *testing.T) {
	server := &rpcServer{gearResources: &fakeRPCGearResourceService{t: t}}
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
	if len(workflowList.Items) != 1 || workflowList.Items[0].Metadata.Name != "flow-a" {
		t.Fatalf("ListWorkflows() = %+v", workflowList)
	}
	workflow := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
		return client.GetWorkflow(context.Background(), conn, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow-a"})
	})
	if workflow.Metadata.Name != "flow-a" {
		t.Fatalf("GetWorkflow() = %+v", workflow)
	}
	workflow = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowCreateResponse, error) {
		return client.CreateWorkflow(context.Background(), conn, "workflow-create", resourceWorkflowDoc("flow-a"))
	})
	if workflow.Metadata.Name != "flow-a" {
		t.Fatalf("CreateWorkflow() = %+v", workflow)
	}
	workflow = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowPutResponse, error) {
		return client.PutWorkflow(context.Background(), conn, "workflow-put", rpcapi.WorkflowPutRequest{Name: "flow-a", Body: resourceWorkflowDoc("flow-a")})
	})
	if workflow.Metadata.Name != "flow-a" {
		t.Fatalf("PutWorkflow() = %+v", workflow)
	}
	workflow = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.WorkflowDeleteResponse, error) {
		return client.DeleteWorkflow(context.Background(), conn, "workflow-delete", rpcapi.WorkflowDeleteRequest{Name: "flow-a"})
	})
	if workflow.Metadata.Name != "flow-a" {
		t.Fatalf("DeleteWorkflow() = %+v", workflow)
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

type fakeRPCGearResourceService struct {
	t *testing.T
}

func (f *fakeRPCGearResourceService) Dispatch(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
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
	case rpcapi.RPCMethodWorkspaceList:
		if _, err := req.Params.AsWorkspaceListRequest(); err != nil {
			f.t.Fatalf("workspace.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.WorkspaceListResponse{Items: []rpcapi.Workspace{resourceWorkspace("workspace-a")}}, (*rpcapi.RPCResponse_Result).FromWorkspaceListResponse), true, nil
	case rpcapi.RPCMethodWorkspaceGet:
		params, err := req.Params.AsWorkspaceGetRequest()
		if err != nil || params.Name != "workspace-a" {
			f.t.Fatalf("workspace.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCResponse_Result).FromWorkspaceGetResponse), true, nil
	case rpcapi.RPCMethodWorkspaceCreate:
		params, err := req.Params.AsWorkspaceCreateRequest()
		if err != nil || params.Name != "workspace-a" || params.WorkflowName != "flow-a" {
			f.t.Fatalf("workspace.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCResponse_Result).FromWorkspaceCreateResponse), true, nil
	case rpcapi.RPCMethodWorkspacePut:
		params, err := req.Params.AsWorkspacePutRequest()
		if err != nil || params.Name != "workspace-a" || params.Body.WorkflowName != "flow-a" {
			f.t.Fatalf("workspace.put params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCResponse_Result).FromWorkspacePutResponse), true, nil
	case rpcapi.RPCMethodWorkspaceDelete:
		params, err := req.Params.AsWorkspaceDeleteRequest()
		if err != nil || params.Name != "workspace-a" {
			f.t.Fatalf("workspace.delete params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkspace("workspace-a"), (*rpcapi.RPCResponse_Result).FromWorkspaceDeleteResponse), true, nil
	case rpcapi.RPCMethodWorkflowList:
		if _, err := req.Params.AsWorkflowListRequest(); err != nil {
			f.t.Fatalf("workflow.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.WorkflowListResponse{Items: []rpcapi.WorkflowDocument{resourceWorkflowDoc("flow-a")}}, (*rpcapi.RPCResponse_Result).FromWorkflowListResponse), true, nil
	case rpcapi.RPCMethodWorkflowGet:
		params, err := req.Params.AsWorkflowGetRequest()
		if err != nil || params.Name != "flow-a" {
			f.t.Fatalf("workflow.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkflowDoc("flow-a"), (*rpcapi.RPCResponse_Result).FromWorkflowGetResponse), true, nil
	case rpcapi.RPCMethodWorkflowCreate:
		params, err := req.Params.AsWorkflowCreateRequest()
		if err != nil || params.Metadata.Name != "flow-a" {
			f.t.Fatalf("workflow.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkflowDoc("flow-a"), (*rpcapi.RPCResponse_Result).FromWorkflowCreateResponse), true, nil
	case rpcapi.RPCMethodWorkflowPut:
		params, err := req.Params.AsWorkflowPutRequest()
		if err != nil || params.Name != "flow-a" || params.Body.Metadata.Name != "flow-a" {
			f.t.Fatalf("workflow.put params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkflowDoc("flow-a"), (*rpcapi.RPCResponse_Result).FromWorkflowPutResponse), true, nil
	case rpcapi.RPCMethodWorkflowDelete:
		params, err := req.Params.AsWorkflowDeleteRequest()
		if err != nil || params.Name != "flow-a" {
			f.t.Fatalf("workflow.delete params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceWorkflowDoc("flow-a"), (*rpcapi.RPCResponse_Result).FromWorkflowDeleteResponse), true, nil
	case rpcapi.RPCMethodModelList:
		if _, err := req.Params.AsModelListRequest(); err != nil {
			f.t.Fatalf("model.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.ModelListResponse{Items: []rpcapi.Model{resourceModel("model-a")}}, (*rpcapi.RPCResponse_Result).FromModelListResponse), true, nil
	case rpcapi.RPCMethodModelGet:
		params, err := req.Params.AsModelGetRequest()
		if err != nil || params.Id != "model-a" {
			f.t.Fatalf("model.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCResponse_Result).FromModelGetResponse), true, nil
	case rpcapi.RPCMethodModelCreate:
		params, err := req.Params.AsModelCreateRequest()
		if err != nil || params.Id != "model-a" {
			f.t.Fatalf("model.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCResponse_Result).FromModelCreateResponse), true, nil
	case rpcapi.RPCMethodModelPut:
		params, err := req.Params.AsModelPutRequest()
		if err != nil || params.Id != "model-a" || params.Body.Id != "model-a" {
			f.t.Fatalf("model.put params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCResponse_Result).FromModelPutResponse), true, nil
	case rpcapi.RPCMethodModelDelete:
		params, err := req.Params.AsModelDeleteRequest()
		if err != nil || params.Id != "model-a" {
			f.t.Fatalf("model.delete params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceModel("model-a"), (*rpcapi.RPCResponse_Result).FromModelDeleteResponse), true, nil
	case rpcapi.RPCMethodCredentialList:
		if _, err := req.Params.AsCredentialListRequest(); err != nil {
			f.t.Fatalf("credential.list params: %v", err)
		}
		return resourceResponse(req.Id, rpcapi.CredentialListResponse{Items: []rpcapi.Credential{resourceCredential("credential-a")}}, (*rpcapi.RPCResponse_Result).FromCredentialListResponse), true, nil
	case rpcapi.RPCMethodCredentialGet:
		params, err := req.Params.AsCredentialGetRequest()
		if err != nil || params.Name != "credential-a" {
			f.t.Fatalf("credential.get params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCResponse_Result).FromCredentialGetResponse), true, nil
	case rpcapi.RPCMethodCredentialCreate:
		params, err := req.Params.AsCredentialCreateRequest()
		if err != nil || params.Name != "credential-a" {
			f.t.Fatalf("credential.create params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCResponse_Result).FromCredentialCreateResponse), true, nil
	case rpcapi.RPCMethodCredentialPut:
		params, err := req.Params.AsCredentialPutRequest()
		if err != nil || params.Name != "credential-a" || params.Body.Name != "credential-a" {
			f.t.Fatalf("credential.put params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCResponse_Result).FromCredentialPutResponse), true, nil
	case rpcapi.RPCMethodCredentialDelete:
		params, err := req.Params.AsCredentialDeleteRequest()
		if err != nil || params.Name != "credential-a" {
			f.t.Fatalf("credential.delete params = %+v, %v", params, err)
		}
		return resourceResponse(req.Id, resourceCredential("credential-a"), (*rpcapi.RPCResponse_Result).FromCredentialDeleteResponse), true, nil
	default:
		f.t.Fatalf("unexpected method %s", req.Method)
		return nil, false, nil
	}
}

func resourceResponse[T any](id string, value T, encode func(*rpcapi.RPCResponse_Result, T) error) *rpcapi.RPCResponse {
	resp, err := newRPCResultResponse(id, value, encode)
	if err != nil {
		panic(err)
	}
	return resp
}

func resourceWorkspace(name string) rpcapi.Workspace {
	return rpcapi.Workspace{Name: name, WorkflowName: "flow-a"}
}

func resourceWorkflowDoc(name string) rpcapi.WorkflowDocument {
	return rpcapi.WorkflowDocument{
		ApiVersion: rpcapi.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
		Kind:       rpcapi.FlowcraftWorkflowKindFlowcraftWorkflow,
		Metadata:   rpcapi.WorkflowMetadata{Name: name},
		Spec:       rpcapi.FlowcraftWorkflowSpec{"entry_agent": ""},
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
		Method:   rpcapi.CredentialMethodApiKey,
		Body:     rpcapi.CredentialBody{"api_key": "sk-test"},
	}
}
