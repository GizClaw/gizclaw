package gearresource

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/credential"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/model"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerAllowedCRUD(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}

	flowCreate := callRPC(t, srv, "workflow-create", rpcapi.RPCMethodWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-a")))
	requireNoRPCError(t, flowCreate)
	if got := mustResult(t, flowCreate.Result.AsWorkflowCreateResponse).Metadata.Name; got != "flow-a" {
		t.Fatalf("workflow.create name = %q", got)
	}

	flowList := callRPC(t, srv, "workflow-list", rpcapi.RPCMethodWorkflowList, nil)
	if got := mustResult(t, flowList.Result.AsWorkflowListResponse); len(got.Items) != 1 || got.Items[0].Metadata.Name != "flow-a" {
		t.Fatalf("workflow.list = %#v", got)
	}

	flowGet := callRPC(t, srv, "workflow-get", rpcapi.RPCMethodWorkflowGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "flow-a"}))
	if got := mustResult(t, flowGet.Result.AsWorkflowGetResponse).Metadata.Name; got != "flow-a" {
		t.Fatalf("workflow.get name = %q", got)
	}

	flowPut := callRPC(t, srv, "workflow-put", rpcapi.RPCMethodWorkflowPut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowPutRequest, rpcapi.WorkflowPutRequest{
		Name: "flow-a",
		Body: workflowDoc("flow-a"),
	}))
	requireNoRPCError(t, flowPut)

	workspaceCreate := callRPC(t, srv, "workspace-create", rpcapi.RPCMethodWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "flow-a",
	}))
	if got := mustResult(t, workspaceCreate.Result.AsWorkspaceCreateResponse); got.Name != "workspace-a" || got.WorkflowName != "flow-a" {
		t.Fatalf("workspace.create = %#v", got)
	}

	workspaceList := callRPC(t, srv, "workspace-list", rpcapi.RPCMethodWorkspaceList, nil)
	if got := mustResult(t, workspaceList.Result.AsWorkspaceListResponse); len(got.Items) != 1 || got.Items[0].Name != "workspace-a" {
		t.Fatalf("workspace.list = %#v", got)
	}

	workspaceGet := callRPC(t, srv, "workspace-get", rpcapi.RPCMethodWorkspaceGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "workspace-a"}))
	if got := mustResult(t, workspaceGet.Result.AsWorkspaceGetResponse).Name; got != "workspace-a" {
		t.Fatalf("workspace.get name = %q", got)
	}

	workspacePut := callRPC(t, srv, "workspace-put", rpcapi.RPCMethodWorkspacePut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspacePutRequest, rpcapi.WorkspacePutRequest{
		Name: "workspace-a",
		Body: rpcapi.Workspace{Name: "workspace-a", WorkflowName: "flow-a"},
	}))
	requireNoRPCError(t, workspacePut)

	modelCreate := callRPC(t, srv, "model-create", rpcapi.RPCMethodModelCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelCreateRequest, rpcModel("model-a")))
	if got := mustResult(t, modelCreate.Result.AsModelCreateResponse).Id; got != "model-a" {
		t.Fatalf("model.create id = %q", got)
	}

	modelList := callRPC(t, srv, "model-list", rpcapi.RPCMethodModelList, nil)
	if got := mustResult(t, modelList.Result.AsModelListResponse); len(got.Items) != 1 || got.Items[0].Id != "model-a" {
		t.Fatalf("model.list = %#v", got)
	}

	modelGet := callRPC(t, srv, "model-get", rpcapi.RPCMethodModelGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-a"}))
	if got := mustResult(t, modelGet.Result.AsModelGetResponse).Id; got != "model-a" {
		t.Fatalf("model.get id = %q", got)
	}

	updatedModel := rpcModel("model-a")
	modelName := "updated model"
	updatedModel.Name = &modelName
	modelPut := callRPC(t, srv, "model-put", rpcapi.RPCMethodModelPut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelPutRequest, rpcapi.ModelPutRequest{
		Id:   "model-a",
		Body: updatedModel,
	}))
	if got := mustResult(t, modelPut.Result.AsModelPutResponse); got.Name == nil || *got.Name != modelName {
		t.Fatalf("model.put = %#v", got)
	}

	credentialCreate := callRPC(t, srv, "credential-create", rpcapi.RPCMethodCredentialCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialCreateRequest, rpcCredential("credential-a", "sk-a")))
	if got := mustResult(t, credentialCreate.Result.AsCredentialCreateResponse).Name; got != "credential-a" {
		t.Fatalf("credential.create name = %q", got)
	}

	credentialList := callRPC(t, srv, "credential-list", rpcapi.RPCMethodCredentialList, nil)
	if got := mustResult(t, credentialList.Result.AsCredentialListResponse); len(got.Items) != 1 || got.Items[0].Name != "credential-a" {
		t.Fatalf("credential.list = %#v", got)
	}

	credentialGet := callRPC(t, srv, "credential-get", rpcapi.RPCMethodCredentialGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "credential-a"}))
	if got := mustResult(t, credentialGet.Result.AsCredentialGetResponse).Name; got != "credential-a" {
		t.Fatalf("credential.get name = %q", got)
	}

	credentialPut := callRPC(t, srv, "credential-put", rpcapi.RPCMethodCredentialPut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialPutRequest, rpcapi.CredentialPutRequest{
		Name: "credential-a",
		Body: rpcCredential("credential-a", "sk-b"),
	}))
	if got := mustResult(t, credentialPut.Result.AsCredentialPutResponse).Body["api_key"]; got != "sk-b" {
		t.Fatalf("credential.put body api_key = %#v", got)
	}

	requireNoRPCError(t, callRPC(t, srv, "credential-delete", rpcapi.RPCMethodCredentialDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialDeleteRequest, rpcapi.CredentialDeleteRequest{Name: "credential-a"})))
	requireNoRPCError(t, callRPC(t, srv, "model-delete", rpcapi.RPCMethodModelDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelDeleteRequest, rpcapi.ModelDeleteRequest{Id: "model-a"})))
	requireNoRPCError(t, callRPC(t, srv, "workspace-delete", rpcapi.RPCMethodWorkspaceDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceDeleteRequest, rpcapi.WorkspaceDeleteRequest{Name: "workspace-a"})))
	requireNoRPCError(t, callRPC(t, srv, "workflow-delete", rpcapi.RPCMethodWorkflowDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowDeleteRequest, rpcapi.WorkflowDeleteRequest{Name: "flow-a"})))
}

func TestServerACLBoundaries(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowAdmin)
	requireNoRPCError(t, callRPC(t, srv, "workflow-create-a", rpcapi.RPCMethodWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-a"))))
	auth.allow(acl.ResourceKindWorkflow, "flow-b", apitypes.ACLPermissionWorkflowAdmin)
	requireNoRPCError(t, callRPC(t, srv, "workflow-create-b", rpcapi.RPCMethodWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-b"))))

	auth.allow(acl.ResourceKindWorkspace, "workspace-a", apitypes.ACLPermissionWorkspaceAdmin)
	denied := callRPC(t, srv, "workspace-create-denied", rpcapi.RPCMethodWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "flow-a",
	}))
	if denied.Error == nil || denied.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("workspace.create denied response = %#v", denied)
	}

	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowUse)
	requireNoRPCError(t, callRPC(t, srv, "workspace-create-allowed", rpcapi.RPCMethodWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "flow-a",
	})))

	auth.allow(acl.ResourceKindWorkspace, "workspace-b", apitypes.ACLPermissionWorkspaceAdmin)
	auth.allow(acl.ResourceKindWorkflow, "flow-b", apitypes.ACLPermissionWorkflowUse)
	requireNoRPCError(t, callRPC(t, srv, "workspace-create-b", rpcapi.RPCMethodWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-b",
		WorkflowName: "flow-b",
	})))

	auth.allow(acl.ResourceKindWorkspace, "workspace-a", apitypes.ACLPermissionWorkspaceRead)
	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowRead)

	workspaceList := callRPC(t, srv, "workspace-list-filtered", rpcapi.RPCMethodWorkspaceList, nil)
	if got := mustResult(t, workspaceList.Result.AsWorkspaceListResponse); len(got.Items) != 1 || got.Items[0].Name != "workspace-a" {
		t.Fatalf("filtered workspace.list = %#v", got)
	}
	workflowList := callRPC(t, srv, "workflow-list-filtered", rpcapi.RPCMethodWorkflowList, nil)
	if got := mustResult(t, workflowList.Result.AsWorkflowListResponse); len(got.Items) != 1 || got.Items[0].Metadata.Name != "flow-a" {
		t.Fatalf("filtered workflow.list = %#v", got)
	}

	if got := auth.count(ctx, acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowUse); got == 0 {
		t.Fatal("workspace.create did not check workflow.use")
	}
}

func TestServerErrorPaths(t *testing.T) {
	requiredMethods := []rpcapi.RPCMethod{
		rpcapi.RPCMethodWorkspaceGet,
		rpcapi.RPCMethodWorkspaceCreate,
		rpcapi.RPCMethodWorkspacePut,
		rpcapi.RPCMethodWorkspaceDelete,
		rpcapi.RPCMethodWorkflowGet,
		rpcapi.RPCMethodWorkflowCreate,
		rpcapi.RPCMethodWorkflowPut,
		rpcapi.RPCMethodWorkflowDelete,
		rpcapi.RPCMethodModelGet,
		rpcapi.RPCMethodModelCreate,
		rpcapi.RPCMethodModelPut,
		rpcapi.RPCMethodModelDelete,
		rpcapi.RPCMethodCredentialGet,
		rpcapi.RPCMethodCredentialCreate,
		rpcapi.RPCMethodCredentialPut,
		rpcapi.RPCMethodCredentialDelete,
	}

	for _, method := range []rpcapi.RPCMethod{
		rpcapi.RPCMethodWorkspaceList,
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
		rpcapi.RPCMethodCredentialDelete,
	} {
		resp, handled, err := (&Server{}).Dispatch(context.Background(), &rpcapi.RPCRequest{Id: string(method), Method: method})
		if err != nil || !handled {
			t.Fatalf("unconfigured Dispatch(%s) handled=%v err=%v", method, handled, err)
		}
		requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)
	}

	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}
	for _, method := range requiredMethods {
		resp := callRPC(t, srv, "invalid-"+string(method), method, nil)
		requireRPCError(t, resp, rpcapi.RPCErrorCodeInvalidParams)
	}
	for _, method := range []rpcapi.RPCMethod{
		rpcapi.RPCMethodWorkspaceList,
		rpcapi.RPCMethodWorkflowList,
		rpcapi.RPCMethodModelList,
		rpcapi.RPCMethodCredentialList,
	} {
		resp := callRPC(t, srv, "invalid-"+string(method), method, &rpcapi.RPCRequest_Params{})
		requireRPCError(t, resp, rpcapi.RPCErrorCodeInvalidParams)
	}

	for _, tc := range []struct {
		name   string
		method rpcapi.RPCMethod
		params *rpcapi.RPCRequest_Params
	}{
		{"workspace", rpcapi.RPCMethodWorkspaceGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "missing"})},
		{"workflow", rpcapi.RPCMethodWorkflowGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "missing"})},
		{"model", rpcapi.RPCMethodModelGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "missing"})},
		{"credential", rpcapi.RPCMethodCredentialGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "missing"})},
	} {
		t.Run(tc.name+"-not-found", func(t *testing.T) {
			resp := callRPC(t, srv, tc.name+"-not-found", tc.method, tc.params)
			requireRPCError(t, resp, rpcapi.RPCErrorCodeNotFound)
		})
	}

	authless := newTestResourceServer()
	resp := callRPC(t, authless, "acl-missing", rpcapi.RPCMethodModelGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-a"}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)
}

func TestHelpers(t *testing.T) {
	if IsMethod(rpcapi.RPCMethodPeerPing) {
		t.Fatal("IsMethod(peer.ping) = true")
	}
	value := 7
	if got := int32Ptr(&value); got == nil || *got != 7 {
		t.Fatalf("int32Ptr() = %#v", got)
	}
	if got := int32Ptr(nil); got != nil {
		t.Fatalf("int32Ptr(nil) = %#v", got)
	}
	if resp := statusError("status", http.StatusTeapot, ""); resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("statusError(418) = %#v", resp)
	}
	if resp := withRequestID("id", nil); resp != nil {
		t.Fatalf("withRequestID(nil) = %#v", resp)
	}
	if got := newTestResourceServer().String(); got == "" {
		t.Fatal("String() = empty")
	}
	if _, err := convertType[struct{}](func() {}); err == nil {
		t.Fatal("convertType(function) error = nil")
	}
}

func newTestResourceServer() *Server {
	workflowStore := kv.NewMemory(nil)
	return &Server{
		Caller:      giznet.PublicKey{1},
		Workflows:   &workflow.Server{Store: workflowStore},
		Workspaces:  &workspace.Server{Store: kv.NewMemory(nil), WorkflowStore: workflowStore},
		Models:      &model.Server{Store: kv.NewMemory(nil), Now: func() time.Time { return time.Unix(1, 0).UTC() }},
		Credentials: &credential.Server{Store: kv.NewMemory(nil)},
	}
}

func callRPC(t *testing.T, srv *Server, id string, method rpcapi.RPCMethod, params *rpcapi.RPCRequest_Params) *rpcapi.RPCResponse {
	t.Helper()

	resp, handled, err := srv.Dispatch(context.Background(), &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: id, Method: method, Params: params})
	if err != nil {
		t.Fatalf("Dispatch(%s) error = %v", method, err)
	}
	if !handled {
		t.Fatalf("Dispatch(%s) handled = false", method)
	}
	if resp == nil {
		t.Fatalf("Dispatch(%s) response = nil", method)
	}
	return resp
}

func rpcParams[T any](t *testing.T, encode func(*rpcapi.RPCRequest_Params, T) error, value T) *rpcapi.RPCRequest_Params {
	t.Helper()

	var params rpcapi.RPCRequest_Params
	if err := encode(&params, value); err != nil {
		t.Fatalf("encode params error = %v", err)
	}
	return &params
}

func mustResult[T any](t *testing.T, decode func() (T, error)) T {
	t.Helper()

	value, err := decode()
	if err != nil {
		t.Fatalf("decode result error = %v", err)
	}
	return value
}

func requireNoRPCError(t *testing.T, resp *rpcapi.RPCResponse) {
	t.Helper()

	if resp.Error != nil {
		t.Fatalf("RPC error = %#v", resp.Error)
	}
}

func requireRPCError(t *testing.T, resp *rpcapi.RPCResponse, code rpcapi.RPCErrorCode) {
	t.Helper()

	if resp == nil || resp.Error == nil {
		t.Fatalf("RPC response error = nil, response = %#v", resp)
	}
	if resp.Error.Code != code {
		t.Fatalf("RPC error code = %v, want %v, response = %#v", resp.Error.Code, code, resp)
	}
}

func workflowDoc(name string) rpcapi.WorkflowDocument {
	return rpcapi.WorkflowDocument{
		ApiVersion: rpcapi.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
		Kind:       rpcapi.FlowcraftWorkflowKindFlowcraftWorkflow,
		Metadata:   rpcapi.WorkflowMetadata{Name: name},
		Spec:       rpcapi.FlowcraftWorkflowSpec{"entry_agent": ""},
	}
}

func rpcModel(id string) rpcapi.Model {
	return rpcapi.Model{
		Id:     id,
		Kind:   rpcapi.ModelKindLlm,
		Source: rpcapi.ModelSourceManual,
		Provider: rpcapi.ModelProvider{
			Kind: rpcapi.ModelProviderKind("openai-tenant"),
			Name: "global",
		},
	}
}

func rpcCredential(name, key string) rpcapi.Credential {
	return rpcapi.Credential{
		Name:     name,
		Provider: "openai",
		Method:   rpcapi.CredentialMethodApiKey,
		Body:     rpcapi.CredentialBody{"api_key": key},
	}
}

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Authorize(context.Context, acl.AuthorizeRequest) error {
	return nil
}

type ruleAuthorizer struct {
	allowed map[authKey]struct{}
	calls   map[authKey]int
}

type authKey struct {
	kind       apitypes.ACLResourceKind
	id         string
	permission apitypes.ACLPermission
}

func newRuleAuthorizer() *ruleAuthorizer {
	return &ruleAuthorizer{
		allowed: make(map[authKey]struct{}),
		calls:   make(map[authKey]int),
	}
}

func (a *ruleAuthorizer) allow(kind apitypes.ACLResourceKind, id string, permission apitypes.ACLPermission) {
	a.allowed[authKey{kind: kind, id: id, permission: permission}] = struct{}{}
}

func (a *ruleAuthorizer) count(_ context.Context, kind apitypes.ACLResourceKind, id string, permission apitypes.ACLPermission) int {
	return a.calls[authKey{kind: kind, id: id, permission: permission}]
}

func (a *ruleAuthorizer) Authorize(_ context.Context, request acl.AuthorizeRequest) error {
	key := authKey{kind: request.Resource.Kind, id: request.Resource.Id, permission: request.Permission}
	a.calls[key]++
	if _, ok := a.allowed[key]; !ok {
		return acl.ErrDenied
	}
	return nil
}
