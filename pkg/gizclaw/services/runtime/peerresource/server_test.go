package peerresource

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/pet"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/reward"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/wallet"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
	_ "modernc.org/sqlite"
)

func TestServerAllowedCRUD(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}

	flowCreate := callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-a")))
	requireNoRPCError(t, flowCreate)
	if got := mustResult(t, flowCreate.Result.AsWorkflowCreateResponse).Metadata.Name; got != "flow-a" {
		t.Fatalf("workflow.create name = %q", got)
	}

	flowList := callRPC(t, srv, "workflow-list", rpcapi.RPCMethodServerWorkflowList, nil)
	if got := mustResult(t, flowList.Result.AsWorkflowListResponse); len(got.Items) != 1 || got.Items[0].Metadata.Name != "flow-a" {
		t.Fatalf("workflow.list = %#v", got)
	}

	flowGet := callRPC(t, srv, "workflow-get", rpcapi.RPCMethodServerWorkflowGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "flow-a"}))
	if got := mustResult(t, flowGet.Result.AsWorkflowGetResponse).Metadata.Name; got != "flow-a" {
		t.Fatalf("workflow.get name = %q", got)
	}

	flowPut := callRPC(t, srv, "workflow-put", rpcapi.RPCMethodServerWorkflowPut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowPutRequest, rpcapi.WorkflowPutRequest{
		Name: "flow-a",
		Body: workflowDoc("flow-a"),
	}))
	requireNoRPCError(t, flowPut)

	workspaceCreate := callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "flow-a",
	}))
	if got := mustResult(t, workspaceCreate.Result.AsWorkspaceCreateResponse); got.Name != "workspace-a" || got.WorkflowName != "flow-a" {
		t.Fatalf("workspace.create = %#v", got)
	}

	workspaceList := callRPC(t, srv, "workspace-list", rpcapi.RPCMethodServerWorkspaceList, nil)
	if got := mustResult(t, workspaceList.Result.AsWorkspaceListResponse); len(got.Items) != 1 || got.Items[0].Name != "workspace-a" {
		t.Fatalf("workspace.list = %#v", got)
	}

	workspaceGet := callRPC(t, srv, "workspace-get", rpcapi.RPCMethodServerWorkspaceGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "workspace-a"}))
	if got := mustResult(t, workspaceGet.Result.AsWorkspaceGetResponse).Name; got != "workspace-a" {
		t.Fatalf("workspace.get name = %q", got)
	}

	workspacePut := callRPC(t, srv, "workspace-put", rpcapi.RPCMethodServerWorkspacePut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspacePutRequest, rpcapi.WorkspacePutRequest{
		Name: "workspace-a",
		Body: rpcapi.Workspace{Name: "workspace-a", WorkflowName: "flow-a"},
	}))
	requireNoRPCError(t, workspacePut)

	modelCreate := callRPC(t, srv, "model-create", rpcapi.RPCMethodServerModelCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelCreateRequest, rpcModel("model-a")))
	if got := mustResult(t, modelCreate.Result.AsModelCreateResponse).Id; got != "model-a" {
		t.Fatalf("model.create id = %q", got)
	}

	modelList := callRPC(t, srv, "model-list", rpcapi.RPCMethodServerModelList, nil)
	if got := mustResult(t, modelList.Result.AsModelListResponse); len(got.Items) != 1 || got.Items[0].Id != "model-a" {
		t.Fatalf("model.list = %#v", got)
	}

	modelGet := callRPC(t, srv, "model-get", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-a"}))
	if got := mustResult(t, modelGet.Result.AsModelGetResponse).Id; got != "model-a" {
		t.Fatalf("model.get id = %q", got)
	}

	updatedModel := rpcModel("model-a")
	modelName := "updated model"
	updatedModel.Name = &modelName
	modelPut := callRPC(t, srv, "model-put", rpcapi.RPCMethodServerModelPut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelPutRequest, rpcapi.ModelPutRequest{
		Id:   "model-a",
		Body: updatedModel,
	}))
	if got := mustResult(t, modelPut.Result.AsModelPutResponse); got.Name == nil || *got.Name != modelName {
		t.Fatalf("model.put = %#v", got)
	}

	credentialCreate := callRPC(t, srv, "credential-create", rpcapi.RPCMethodServerCredentialCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialCreateRequest, rpcCredential("credential-a", "sk-a")))
	if got := mustResult(t, credentialCreate.Result.AsCredentialCreateResponse).Name; got != "credential-a" {
		t.Fatalf("credential.create name = %q", got)
	}

	credentialList := callRPC(t, srv, "credential-list", rpcapi.RPCMethodServerCredentialList, nil)
	if got := mustResult(t, credentialList.Result.AsCredentialListResponse); len(got.Items) != 1 || got.Items[0].Name != "credential-a" {
		t.Fatalf("credential.list = %#v", got)
	}

	credentialGet := callRPC(t, srv, "credential-get", rpcapi.RPCMethodServerCredentialGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "credential-a"}))
	if got := mustResult(t, credentialGet.Result.AsCredentialGetResponse).Name; got != "credential-a" {
		t.Fatalf("credential.get name = %q", got)
	}

	credentialPut := callRPC(t, srv, "credential-put", rpcapi.RPCMethodServerCredentialPut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialPutRequest, rpcapi.CredentialPutRequest{
		Name: "credential-a",
		Body: rpcCredential("credential-a", "sk-b"),
	}))
	if got := testRPCCredentialBodyString(mustResult(t, credentialPut.Result.AsCredentialPutResponse).Body, "api_key"); got != "sk-b" {
		t.Fatalf("credential.put body api_key = %#v", got)
	}

	requireNoRPCError(t, callRPC(t, srv, "credential-delete", rpcapi.RPCMethodServerCredentialDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialDeleteRequest, rpcapi.CredentialDeleteRequest{Name: "credential-a"})))
	requireNoRPCError(t, callRPC(t, srv, "model-delete", rpcapi.RPCMethodServerModelDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelDeleteRequest, rpcapi.ModelDeleteRequest{Id: "model-a"})))
	requireNoRPCError(t, callRPC(t, srv, "workspace-delete", rpcapi.RPCMethodServerWorkspaceDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceDeleteRequest, rpcapi.WorkspaceDeleteRequest{Name: "workspace-a"})))
	requireNoRPCError(t, callRPC(t, srv, "workflow-delete", rpcapi.RPCMethodServerWorkflowDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowDeleteRequest, rpcapi.WorkflowDeleteRequest{Name: "flow-a"})))
}

func TestServerACLBoundaries(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowAdmin)
	requireNoRPCError(t, callRPC(t, srv, "workflow-create-a", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-a"))))
	auth.allow(acl.ResourceKindWorkflow, "flow-b", apitypes.ACLPermissionWorkflowAdmin)
	requireNoRPCError(t, callRPC(t, srv, "workflow-create-b", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-b"))))

	auth.allow(acl.ResourceKindWorkspace, "workspace-a", apitypes.ACLPermissionWorkspaceAdmin)
	denied := callRPC(t, srv, "workspace-create-denied", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "flow-a",
	}))
	if denied.Error == nil || denied.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("workspace.create denied response = %#v", denied)
	}

	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowUse)
	requireNoRPCError(t, callRPC(t, srv, "workspace-create-allowed", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-a",
		WorkflowName: "flow-a",
	})))

	auth.allow(acl.ResourceKindWorkspace, "workspace-b", apitypes.ACLPermissionWorkspaceAdmin)
	auth.allow(acl.ResourceKindWorkflow, "flow-b", apitypes.ACLPermissionWorkflowUse)
	requireNoRPCError(t, callRPC(t, srv, "workspace-create-b", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-b",
		WorkflowName: "flow-b",
	})))

	auth.allow(acl.ResourceKindWorkspace, "workspace-a", apitypes.ACLPermissionWorkspaceRead)
	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowRead)

	workspaceList := callRPC(t, srv, "workspace-list-filtered", rpcapi.RPCMethodServerWorkspaceList, nil)
	if got := mustResult(t, workspaceList.Result.AsWorkspaceListResponse); len(got.Items) != 1 || got.Items[0].Name != "workspace-a" {
		t.Fatalf("filtered workspace.list = %#v", got)
	}
	workflowList := callRPC(t, srv, "workflow-list-filtered", rpcapi.RPCMethodServerWorkflowList, nil)
	if got := mustResult(t, workflowList.Result.AsWorkflowListResponse); len(got.Items) != 1 || got.Items[0].Metadata.Name != "flow-a" {
		t.Fatalf("filtered workflow.list = %#v", got)
	}

	if got := auth.count(ctx, acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowUse); got == 0 {
		t.Fatal("workspace.create did not check workflow.use")
	}
}

func TestServerWorkspaceListPrefixUsesACLDiscovery(t *testing.T) {
	ctx := context.Background()
	auth := newListingAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowAdmin)
	auth.allow(acl.ResourceKindWorkflow, "flow-a", apitypes.ACLPermissionWorkflowUse)
	auth.allow(acl.ResourceKindWorkspace, "social-direct-visible", apitypes.ACLPermissionWorkspaceAdmin)
	auth.allow(acl.ResourceKindWorkspace, "social-direct-hidden", apitypes.ACLPermissionWorkspaceAdmin)
	auth.allow(acl.ResourceKindWorkspace, "social-group-visible", apitypes.ACLPermissionWorkspaceAdmin)
	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-a"))))
	for _, name := range []string{"social-direct-visible", "social-direct-hidden", "social-group-visible"} {
		requireNoRPCError(t, callRPC(t, srv, "workspace-create-"+name, rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
			Name:         name,
			WorkflowName: "flow-a",
		})))
	}

	auth.bindings = []apitypes.ACLPolicyBinding{
		{Id: "binding-hidden", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-direct-hidden"), Role: "workspace-member"}},
		{Id: "binding-missing", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-direct-missing"), Role: "workspace-member"}},
		{Id: "binding-visible", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-direct-visible"), Role: "workspace-member"}},
		{Id: "binding-group", Policy: apitypes.ACLPolicy{Subject: acl.PublicKeySubject(srv.Caller.String()), Resource: acl.WorkspaceResource("social-group-visible"), Role: "workspace-member"}},
	}
	auth.allow(acl.ResourceKindWorkspace, "social-direct-missing", apitypes.ACLPermissionWorkspaceRead)
	auth.allow(acl.ResourceKindWorkspace, "social-direct-visible", apitypes.ACLPermissionWorkspaceRead)

	limit := 1
	resp := callRPC(t, srv, "workspace-list-prefix", rpcapi.RPCMethodServerWorkspaceList, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceListRequest, rpcapi.WorkspaceListRequest{
		Prefix: stringPtr("social-direct-"),
		Limit:  &limit,
	}))
	got := mustResult(t, resp.Result.AsWorkspaceListResponse)
	if len(got.Items) != 1 || got.Items[0].Name != "social-direct-visible" {
		t.Fatalf("workspace.list prefix = %#v", got)
	}
	if got.HasNext || got.NextCursor != nil {
		t.Fatalf("workspace.list prefix pagination = hasNext:%v next:%v", got.HasNext, got.NextCursor)
	}
	if len(auth.listRequests) == 0 {
		t.Fatal("workspace.list prefix did not list ACL policy bindings")
	}
	req := auth.listRequests[0]
	if req.SubjectKind != acl.SubjectKindPublicKey || req.SubjectID != srv.Caller.String() || req.ResourceKind != acl.ResourceKindWorkspace ||
		req.ResourceIDPrefix != "social-direct-" || req.Permission != apitypes.ACLPermissionWorkspaceRead {
		t.Fatalf("ACL discovery request = %+v", req)
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, "social-direct-visible", apitypes.ACLPermissionWorkspaceRead); got == 0 {
		t.Fatal("workspace.list prefix did not authorize visible workspace")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, "social-direct-hidden", apitypes.ACLPermissionWorkspaceRead); got == 0 {
		t.Fatal("workspace.list prefix did not authorize hidden workspace")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, "social-group-visible", apitypes.ACLPermissionWorkspaceRead); got != 0 {
		t.Fatal("workspace.list prefix checked workspace outside requested prefix")
	}
}

func TestServerWorkspaceWorkflowCreateUsesCollectionACL(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	auth.allow(acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionWorkflowAdmin)
	auth.allow(acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionWorkflowUse)
	auth.allow(acl.ResourceKindWorkspace, acl.CollectionResourceID, apitypes.ACLPermissionWorkspaceAdmin)

	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-dynamic"))))
	requireNoRPCError(t, callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-dynamic",
		WorkflowName: "flow-dynamic",
	})))

	if got := auth.count(ctx, acl.ResourceKindWorkflow, "flow-dynamic", apitypes.ACLPermissionWorkflowAdmin); got == 0 {
		t.Fatal("workflow.create did not first check concrete workflow")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionWorkflowAdmin); got == 0 {
		t.Fatal("workflow.create did not fallback to workflow collection")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkspace, acl.CollectionResourceID, apitypes.ACLPermissionWorkspaceAdmin); got == 0 {
		t.Fatal("workspace.create did not fallback to workspace collection")
	}
	if got := auth.count(ctx, acl.ResourceKindWorkflow, acl.CollectionResourceID, apitypes.ACLPermissionWorkflowUse); got == 0 {
		t.Fatal("workspace.create did not fallback to workflow collection use")
	}
}

func TestServerWorkspaceHistoryRPC(t *testing.T) {
	workflowStore := kv.NewMemory(nil)
	objects := objectstore.Dir(t.TempDir())
	workspaceServer := &workspace.Server{
		Store:         kv.NewMemory(nil),
		WorkflowStore: workflowStore,
		RuntimeStore:  workspace.NewObjectRuntimeStore(objects),
	}
	srv := &Server{
		Caller:     giznet.PublicKey{1},
		ACL:        allowAllAuthorizer{},
		Workflows:  &workflow.Server{Store: workflowStore},
		Workspaces: workspaceServer,
	}
	requireNoRPCError(t, callRPC(t, srv, "workflow-create", rpcapi.RPCMethodServerWorkflowCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowCreateRequest, workflowDoc("flow-history"))))
	requireNoRPCError(t, callRPC(t, srv, "workspace-create", rpcapi.RPCMethodServerWorkspaceCreate, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceCreateRequest, rpcapi.WorkspaceCreateRequest{
		Name:         "workspace-history",
		WorkflowName: "flow-history",
	})))
	createdAt := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	entry, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:      "agent",
		Name:      "assistant",
		Text:      "历史回复",
		CreatedAt: createdAt,
		Asset:     &workspace.AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	latest, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:      "agent",
		Name:      "assistant",
		Text:      "最新回复",
		CreatedAt: createdAt.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory(latest) error = %v", err)
	}

	limit := 1
	list := callRPC(t, srv, "workspace-history-list", rpcapi.RPCMethodServerWorkspaceHistoryList, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryListRequest, rpcapi.WorkspaceHistoryListRequest{
		WorkspaceName: "workspace-history",
		Limit:         &limit,
	}))
	listResult := mustResult(t, list.Result.AsWorkspaceHistoryListResponse)
	if len(listResult.Items) != 1 || listResult.Items[0].Id != entry.ID || listResult.Items[0].Text != "历史回复" {
		t.Fatalf("workspace.history.list = %+v", listResult)
	}
	desc := rpcapi.WorkspaceHistoryListRequestOrderDesc
	list = callRPC(t, srv, "workspace-history-list-desc", rpcapi.RPCMethodServerWorkspaceHistoryList, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryListRequest, rpcapi.WorkspaceHistoryListRequest{
		WorkspaceName: "workspace-history",
		Limit:         &limit,
		Order:         &desc,
	}))
	listResult = mustResult(t, list.Result.AsWorkspaceHistoryListResponse)
	if len(listResult.Items) != 1 || listResult.Items[0].Id != latest.ID || listResult.Items[0].Text != "最新回复" {
		t.Fatalf("workspace.history.list desc = %+v", listResult)
	}

	get := callRPC(t, srv, "workspace-history-get", rpcapi.RPCMethodServerWorkspaceHistoryGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryGetRequest, rpcapi.WorkspaceHistoryGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     entry.ID,
	}))
	if got := mustResult(t, get.Result.AsWorkspaceHistoryGetResponse); got.Id != entry.ID || got.Text != "历史回复" {
		t.Fatalf("workspace.history.get = %+v", got)
	}

	asset := callRPC(t, srv, "workspace-history-audio-get", rpcapi.RPCMethodServerWorkspaceHistoryAudioGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryAudioGetRequest, rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     entry.ID,
	}))
	assetResult := mustResult(t, asset.Result.AsWorkspaceHistoryAudioGetResponse)
	if assetResult.WorkspaceName != "workspace-history" || assetResult.HistoryId != entry.ID || assetResult.MimeType != "audio/opus" || assetResult.SizeBytes != int64(len("opus")) {
		t.Fatalf("workspace.history.audio.get = %+v", assetResult)
	}
	assetMetadata, reader, rpcErr, err := srv.PrepareWorkspaceHistoryAudioGet(context.Background(), rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     entry.ID,
	})
	if err != nil || rpcErr != nil {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet() error = %v rpcErr = %+v", err, rpcErr)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(workspace history audio) error = %v", err)
	}
	if assetMetadata != assetResult || string(data) != "opus" {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet() = %+v data=%q", assetMetadata, data)
	}

	textAssetEntry, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "text asset",
		Asset: &workspace.AppendHistoryAsset{MIMEType: "application/octet-stream", Data: []byte("not audio")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory(text asset) error = %v", err)
	}
	_, reader, rpcErr, err = srv.PrepareWorkspaceHistoryAudioGet(context.Background(), rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     textAssetEntry.ID,
	})
	if err != nil || rpcErr == nil || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || reader != nil {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet(non-audio) err = %v rpcErr = %+v reader = %v", err, rpcErr, reader)
	}

	missingAssetEntry, err := workspaceServer.AppendWorkspaceHistory(context.Background(), "workspace-history", workspace.AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "missing audio",
		Asset: &workspace.AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("gone")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory(missing asset) error = %v", err)
	}
	if err := objects.Delete(missingAssetEntry.Assets[0].Name); err != nil {
		t.Fatalf("Delete missing asset fixture: %v", err)
	}
	_, reader, rpcErr, err = srv.PrepareWorkspaceHistoryAudioGet(context.Background(), rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "workspace-history",
		HistoryId:     missingAssetEntry.ID,
	})
	if err != nil || rpcErr == nil || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || reader != nil {
		t.Fatalf("PrepareWorkspaceHistoryAudioGet(missing asset) err = %v rpcErr = %+v reader = %v", err, rpcErr, reader)
	}
}

func TestServerListVoicesFiltersByACL(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	for _, id := range []string{"voice-a", "voice-b", "provider:tenant:voice-c"} {
		body := testVoiceUpsert(id)
		resp, err := srv.Voices.CreateVoice(ctx, adminservice.CreateVoiceRequestObject{Body: &body})
		if err != nil {
			t.Fatalf("CreateVoice(%s) error = %v", id, err)
		}
		if _, ok := resp.(adminservice.CreateVoice200JSONResponse); !ok {
			t.Fatalf("CreateVoice(%s) response = %#v", id, resp)
		}
	}

	auth.allow(acl.ResourceKindVoice, "voice-a", apitypes.ACLPermissionVoiceRead)
	auth.allow(acl.ResourceKindVoice, "provider:tenant:voice-c", apitypes.ACLPermissionVoiceRead)
	resp, err := srv.ListVoices(ctx, adminservice.ListVoicesRequestObject{})
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	list, ok := resp.(adminservice.ListVoices200JSONResponse)
	if !ok {
		t.Fatalf("ListVoices() response = %#v", resp)
	}
	if len(list.Items) != 2 || list.Items[0].Id != "provider:tenant:voice-c" || list.Items[1].Id != "voice-a" {
		t.Fatalf("ListVoices() items = %#v", list.Items)
	}
	if got := auth.count(ctx, acl.ResourceKindVoice, "voice-b", apitypes.ACLPermissionVoiceRead); got == 0 {
		t.Fatal("ListVoices() did not check denied voice")
	}
}

func TestServerBusinessDomainRPC(t *testing.T) {
	srv := newTestResourceServer()
	srv.ACL = allowAllAuthorizer{}

	petAdopt := callRPC(t, srv, "pet-adopt", rpcapi.RPCMethodServerPetAdopt, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetAdoptRequest, rpcapi.PetAdoptRequest{
		Id:   stringPtr("pet-a"),
		Name: "navi",
	}))
	if got := mustResult(t, petAdopt.Result.AsPetAdoptResponse); got.Id != "pet-a" || got.SpeciesId != "rabbit" || got.VoiceId != "voice-a" {
		t.Fatalf("pet.adopt = %#v", got)
	}
	petGet := callRPC(t, srv, "pet-get", rpcapi.RPCMethodServerPetGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetGetRequest, rpcapi.PetGetRequest{Id: "pet-a"}))
	if got := mustResult(t, petGet.Result.AsPetGetResponse); got.Id != "pet-a" {
		t.Fatalf("pet.get = %#v", got)
	}
	petPut := callRPC(t, srv, "pet-put", rpcapi.RPCMethodServerPetPut, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetPutRequest, rpcapi.PetPutRequest{Id: "pet-a", Name: "renamed"}))
	if got := mustResult(t, petPut.Result.AsPetPutResponse); got.Name != "renamed" {
		t.Fatalf("pet.put = %#v", got)
	}
	petFeed := callRPC(t, srv, "pet-feed", rpcapi.RPCMethodServerPetFeed, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetFeedRequest, rpcapi.PetFeedRequest{PetId: "pet-a", Prompt: "hungry"}))
	if got := mustResult(t, petFeed.Result.AsPetFeedResponse); got.Life.Satiety != 65 {
		t.Fatalf("pet.feed = %#v", got)
	}
	petWash := callRPC(t, srv, "pet-wash", rpcapi.RPCMethodServerPetWash, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetWashRequest, rpcapi.PetWashRequest{PetId: "pet-a", Prompt: "bath"}))
	requireNoRPCError(t, petWash)
	petPlay := callRPC(t, srv, "pet-play", rpcapi.RPCMethodServerPetPlay, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetPlayRequest, rpcapi.PetPlayRequest{PetId: "pet-a", Prompt: "game"}))
	requireNoRPCError(t, petPlay)
	petAdoptSecond := callRPC(t, srv, "pet-adopt-second", rpcapi.RPCMethodServerPetAdopt, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetAdoptRequest, rpcapi.PetAdoptRequest{
		Id:   stringPtr("pet-b"),
		Name: "delete-me",
	}))
	requireNoRPCError(t, petAdoptSecond)
	petDelete := callRPC(t, srv, "pet-delete", rpcapi.RPCMethodServerPetDelete, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetDeleteRequest, rpcapi.PetDeleteRequest{Id: "pet-b"}))
	if got := mustResult(t, petDelete.Result.AsPetDeleteResponse); got.Id != "pet-b" {
		t.Fatalf("pet.delete = %#v", got)
	}
	petList := callRPC(t, srv, "pet-list", rpcapi.RPCMethodServerPetList, nil)
	if got := mustResult(t, petList.Result.AsPetListResponse); len(got.Items) != 1 || got.Items[0].Id != "pet-a" {
		t.Fatalf("pet.list = %#v", got)
	}

	rewardClaim := callRPC(t, srv, "reward-claim", rpcapi.RPCMethodServerRewardClaim, rpcParams(t, (*rpcapi.RPCRequest_Params).FromRewardClaimRequest, rpcapi.RewardClaimRequest{Prompt: "won a game"}))
	reward := mustResult(t, rewardClaim.Result.AsRewardClaimResponse)
	if reward.PointAmount != 8 || reward.Prompt != "won a game" {
		t.Fatalf("reward.claim = %#v", reward)
	}
	rewardGet := callRPC(t, srv, "reward-get", rpcapi.RPCMethodServerRewardGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromRewardGetRequest, rpcapi.RewardGetRequest{Id: reward.Id}))
	if got := mustResult(t, rewardGet.Result.AsRewardGetResponse); got.Id != reward.Id {
		t.Fatalf("reward.get = %#v", got)
	}
	rewardList := callRPC(t, srv, "reward-list", rpcapi.RPCMethodServerRewardList, nil)
	if got := mustResult(t, rewardList.Result.AsRewardListResponse); len(got.Items) != 1 || got.Items[0].Id != reward.Id {
		t.Fatalf("reward.list = %#v", got)
	}
	walletGet := callRPC(t, srv, "wallet-get", rpcapi.RPCMethodServerWalletGet, nil)
	if got := mustResult(t, walletGet.Result.AsWalletGetResponse); got.PointBalance != 8 {
		t.Fatalf("wallet.get = %#v", got)
	}
	txList := callRPC(t, srv, "wallet-tx-list", rpcapi.RPCMethodServerWalletTransactionsList, nil)
	txs := mustResult(t, txList.Result.AsWalletTransactionsListResponse)
	if len(txs.Items) != 1 || txs.Items[0].Reason != rpcapi.WalletTransactionObjectReasonRewardClaim {
		t.Fatalf("wallet.transactions.list = %#v", txs)
	}
	txGet := callRPC(t, srv, "wallet-tx-get", rpcapi.RPCMethodServerWalletTransactionsGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWalletTransactionsGetRequest, rpcapi.WalletTransactionsGetRequest{Id: txs.Items[0].Id}))
	if got := mustResult(t, txGet.Result.AsWalletTransactionsGetResponse); got.Id != txs.Items[0].Id {
		t.Fatalf("wallet.transactions.get = %#v", got)
	}
}

func TestServerBusinessDomainDoesNotUseResourceACL(t *testing.T) {
	auth := newRuleAuthorizer()
	srv := newTestResourceServer()
	srv.ACL = auth

	walletGet := callRPC(t, srv, "wallet-get", rpcapi.RPCMethodServerWalletGet, nil)
	requireNoRPCError(t, walletGet)
	if len(auth.calls) != 0 {
		t.Fatalf("business RPC ACL checks = %#v, want none", auth.calls)
	}
}

func TestServerFirmwareRPCUsesFirmwareReadACL(t *testing.T) {
	ctx := context.Background()
	auth := newRuleAuthorizer()
	description := "main stable firmware"
	firmwareServer := &firmware.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir()), Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	create := adminservice.FirmwareUpsert{
		Name: "devkit",
		Slots: apitypes.FirmwareSlots{
			Stable: apitypes.FirmwareSlot{
				Description: &description,
			},
		},
	}
	if resp, err := firmwareServer.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: &create}); err != nil {
		t.Fatalf("CreateFirmware error = %v", err)
	} else if _, ok := resp.(adminservice.CreateFirmware200JSONResponse); !ok {
		t.Fatalf("CreateFirmware response = %T", resp)
	}
	other := adminservice.FirmwareUpsert{
		Name: "otherkit",
		Slots: apitypes.FirmwareSlots{
			Stable: apitypes.FirmwareSlot{Description: stringPtr("other stable firmware")},
		},
	}
	if resp, err := firmwareServer.CreateFirmware(ctx, adminservice.CreateFirmwareRequestObject{Body: &other}); err != nil {
		t.Fatalf("CreateFirmware other error = %v", err)
	} else if _, ok := resp.(adminservice.CreateFirmware200JSONResponse); !ok {
		t.Fatalf("CreateFirmware other response = %T", resp)
	}
	if resp, err := firmwareServer.UploadFirmwareArtifact(ctx, adminservice.UploadFirmwareArtifactRequestObject{
		Name:    "devkit",
		Channel: "stable",
		Body:    bytes.NewReader(peerresourceTarPayload(t, map[string]string{"firmware.bin": "firmware payload"})),
	}); err != nil {
		t.Fatalf("UploadFirmwareArtifact error = %v", err)
	} else if _, ok := resp.(adminservice.UploadFirmwareArtifact200JSONResponse); !ok {
		t.Fatalf("UploadFirmwareArtifact response = %T", resp)
	}

	srv := &Server{
		Caller:    giznet.PublicKey{1},
		ACL:       auth,
		Firmwares: firmwareServer,
	}

	denied := callRPC(t, srv, "firmware-get-denied", rpcapi.RPCMethodServerFirmwareGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromFirmwareGetRequest, rpcapi.FirmwareGetRequest{
		FirmwareId: "devkit",
	}))
	requireRPCError(t, denied, rpcapi.RPCErrorCodeForbidden)
	if got := auth.count(ctx, acl.ResourceKindFirmware, "devkit", apitypes.ACLPermissionFirmwareRead); got == 0 {
		t.Fatal("firmware.get did not check firmware.read")
	}

	auth.allow(acl.ResourceKindFirmware, "devkit", apitypes.ACLPermissionFirmwareRead)
	listResp := callRPC(t, srv, "firmware-list", rpcapi.RPCMethodServerFirmwareList, nil)
	gotList := mustResult(t, listResp.Result.AsFirmwareListResponse)
	if len(gotList.Items) != 1 || gotList.Items[0].Name != "devkit" {
		t.Fatalf("firmware.list = %#v", gotList)
	}
	if got := auth.count(ctx, acl.ResourceKindFirmware, "otherkit", apitypes.ACLPermissionFirmwareRead); got == 0 {
		t.Fatal("firmware.list did not check denied firmware")
	}

	getResp := callRPC(t, srv, "firmware-get", rpcapi.RPCMethodServerFirmwareGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromFirmwareGetRequest, rpcapi.FirmwareGetRequest{
		FirmwareId: "devkit",
	}))
	gotFirmware := mustResult(t, getResp.Result.AsFirmwareGetResponse)
	if gotFirmware.Name != "devkit" || gotFirmware.Slots.Stable.Description == nil || *gotFirmware.Slots.Stable.Description != description {
		t.Fatalf("firmware.get = %#v", gotFirmware)
	}
	if gotFirmware.Slots.Stable.Artifact == nil || gotFirmware.Slots.Stable.Artifact.Size == 0 {
		t.Fatalf("firmware.get artifact = %#v", gotFirmware.Slots.Stable.Artifact)
	}

	bin := callRPC(t, srv, "firmware-download", rpcapi.RPCMethodServerFirmwareFilesDownload, rpcParams(t, (*rpcapi.RPCRequest_Params).FromFirmwareFilesDownloadRequest, rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware.bin",
	}))
	gotBin := mustResult(t, bin.Result.AsFirmwareFilesDownloadResponse)
	if gotBin.FirmwareId != "devkit" || gotBin.File.Path != "firmware.bin" || gotBin.File.Size == 0 {
		t.Fatalf("firmware.download = %#v", gotBin)
	}

	missingBin := callRPC(t, srv, "firmware-artifact-missing", rpcapi.RPCMethodServerFirmwareFilesDownload, rpcParams(t, (*rpcapi.RPCRequest_Params).FromFirmwareFilesDownloadRequest, rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "missing.bin",
	}))
	requireRPCError(t, missingBin, rpcapi.RPCErrorCodeNotFound)
}

func TestServerErrorPaths(t *testing.T) {
	requiredMethods := []rpcapi.RPCMethod{
		rpcapi.RPCMethodServerWorkspaceGet,
		rpcapi.RPCMethodServerWorkspaceCreate,
		rpcapi.RPCMethodServerWorkspacePut,
		rpcapi.RPCMethodServerWorkspaceDelete,
		rpcapi.RPCMethodServerWorkflowGet,
		rpcapi.RPCMethodServerWorkflowCreate,
		rpcapi.RPCMethodServerWorkflowPut,
		rpcapi.RPCMethodServerWorkflowDelete,
		rpcapi.RPCMethodServerModelGet,
		rpcapi.RPCMethodServerModelCreate,
		rpcapi.RPCMethodServerModelPut,
		rpcapi.RPCMethodServerModelDelete,
		rpcapi.RPCMethodServerCredentialGet,
		rpcapi.RPCMethodServerCredentialCreate,
		rpcapi.RPCMethodServerCredentialPut,
		rpcapi.RPCMethodServerCredentialDelete,
		rpcapi.RPCMethodServerPetGet,
		rpcapi.RPCMethodServerPetAdopt,
		rpcapi.RPCMethodServerPetPut,
		rpcapi.RPCMethodServerPetDelete,
		rpcapi.RPCMethodServerPetFeed,
		rpcapi.RPCMethodServerPetWash,
		rpcapi.RPCMethodServerPetPlay,
		rpcapi.RPCMethodServerWalletTransactionsGet,
		rpcapi.RPCMethodServerRewardGet,
		rpcapi.RPCMethodServerRewardClaim,
	}

	for _, method := range []rpcapi.RPCMethod{
		rpcapi.RPCMethodServerWorkspaceList,
		rpcapi.RPCMethodServerWorkspaceGet,
		rpcapi.RPCMethodServerWorkspaceCreate,
		rpcapi.RPCMethodServerWorkspacePut,
		rpcapi.RPCMethodServerWorkspaceDelete,
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
		rpcapi.RPCMethodServerCredentialList,
		rpcapi.RPCMethodServerCredentialGet,
		rpcapi.RPCMethodServerCredentialCreate,
		rpcapi.RPCMethodServerCredentialPut,
		rpcapi.RPCMethodServerCredentialDelete,
		rpcapi.RPCMethodServerPetList,
		rpcapi.RPCMethodServerPetGet,
		rpcapi.RPCMethodServerPetAdopt,
		rpcapi.RPCMethodServerPetPut,
		rpcapi.RPCMethodServerPetDelete,
		rpcapi.RPCMethodServerWalletGet,
		rpcapi.RPCMethodServerWalletTransactionsList,
		rpcapi.RPCMethodServerWalletTransactionsGet,
		rpcapi.RPCMethodServerRewardList,
		rpcapi.RPCMethodServerRewardGet,
		rpcapi.RPCMethodServerRewardClaim,
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
		rpcapi.RPCMethodServerWorkspaceList,
		rpcapi.RPCMethodServerWorkflowList,
		rpcapi.RPCMethodServerModelList,
		rpcapi.RPCMethodServerCredentialList,
		rpcapi.RPCMethodServerPetList,
		rpcapi.RPCMethodServerWalletGet,
		rpcapi.RPCMethodServerWalletTransactionsList,
		rpcapi.RPCMethodServerRewardList,
	} {
		resp := callRPC(t, srv, "invalid-"+string(method), method, &rpcapi.RPCRequest_Params{})
		requireRPCError(t, resp, rpcapi.RPCErrorCodeInvalidParams)
	}

	for _, tc := range []struct {
		name   string
		method rpcapi.RPCMethod
		params *rpcapi.RPCRequest_Params
	}{
		{"workspace", rpcapi.RPCMethodServerWorkspaceGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkspaceGetRequest, rpcapi.WorkspaceGetRequest{Name: "missing"})},
		{"workflow", rpcapi.RPCMethodServerWorkflowGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWorkflowGetRequest, rpcapi.WorkflowGetRequest{Name: "missing"})},
		{"model", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "missing"})},
		{"credential", rpcapi.RPCMethodServerCredentialGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromCredentialGetRequest, rpcapi.CredentialGetRequest{Name: "missing"})},
		{"pet", rpcapi.RPCMethodServerPetGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromPetGetRequest, rpcapi.PetGetRequest{Id: "missing"})},
		{"reward", rpcapi.RPCMethodServerRewardGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromRewardGetRequest, rpcapi.RewardGetRequest{Id: "missing"})},
		{"wallet transaction", rpcapi.RPCMethodServerWalletTransactionsGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromWalletTransactionsGetRequest, rpcapi.WalletTransactionsGetRequest{Id: "missing"})},
	} {
		t.Run(tc.name+"-not-found", func(t *testing.T) {
			resp := callRPC(t, srv, tc.name+"-not-found", tc.method, tc.params)
			requireRPCError(t, resp, rpcapi.RPCErrorCodeNotFound)
		})
	}

	authless := newTestResourceServer()
	resp := callRPC(t, authless, "acl-missing", rpcapi.RPCMethodServerModelGet, rpcParams(t, (*rpcapi.RPCRequest_Params).FromModelGetRequest, rpcapi.ModelGetRequest{Id: "model-a"}))
	requireRPCError(t, resp, rpcapi.RPCErrorCodeInternalError)
}

func peerresourceTarPayload(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, body := range files {
		data := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: modTime}); err != nil {
			t.Fatalf("WriteHeader(%s): %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close tar: %v", err)
	}
	return buf.Bytes()
}

func TestHelpers(t *testing.T) {
	if IsMethod(rpcapi.RPCMethodAllPing) {
		t.Fatal("IsMethod(all.ping) = true")
	}
	value := 7
	if got := int32Ptr(&value); got == nil || *got != 7 {
		t.Fatalf("int32Ptr() = %#v", got)
	}
	if got := int32Ptr(nil); got != nil {
		t.Fatalf("int32Ptr(nil) = %#v", got)
	}
	if got := peerListLimit(nil); got != 50 {
		t.Fatalf("peerListLimit(nil) = %d, want 50", got)
	}
	zero := 0
	if got := peerListLimit(&zero); got != 50 {
		t.Fatalf("peerListLimit(0) = %d, want 50", got)
	}
	tooHigh := 201
	if got := peerListLimit(&tooHigh); got != 200 {
		t.Fatalf("peerListLimit(201) = %d, want 200", got)
	}
	inRange := 7
	if got := peerListLimit(&inRange); got != 7 {
		t.Fatalf("peerListLimit(7) = %d", got)
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
	walletServer := &wallet.Server{DB: newTestDB(nil), Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	rewardServer := &reward.Server{
		Store:    kv.NewMemory(nil),
		Wallet:   walletServer,
		Decider:  fixedRewardDecision(rpcapi.RewardDecision{PointAmount: 8}),
		Cooldown: -1,
		Now:      func() time.Time { return time.Unix(1, 0).UTC() },
	}
	return &Server{
		Caller:      giznet.PublicKey{1},
		Workflows:   &workflow.Server{Store: workflowStore},
		Workspaces:  &workspace.Server{Store: kv.NewMemory(nil), WorkflowStore: workflowStore},
		Models:      &model.Server{Store: kv.NewMemory(nil), Now: func() time.Time { return time.Unix(1, 0).UTC() }},
		Credentials: &credential.Server{Store: kv.NewMemory(nil)},
		Voices:      &voice.Server{Store: kv.NewMemory(nil), Now: func() time.Time { return time.Unix(1, 0).UTC() }},
		Pets: &pet.Server{
			Store:           kv.NewMemory(nil),
			Wallet:          walletServer,
			SpeciesSelector: fixedSpecies("rabbit"),
			VoiceSelector:   fixedVoice("voice-a"),
			ActionDecider:   fixedPetDecision(rpcapi.PetActionDecision{LifeDelta: rpcapi.PetLifeStats{Satiety: 5}}),
			AdoptPointCost:  -1,
			Now:             func() time.Time { return time.Unix(1, 0).UTC() },
		},
		Wallets: walletServer,
		Rewards: rewardServer,
	}
}

func newTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		if t != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		panic(err)
	}
	if t != nil {
		t.Cleanup(func() {
			if err := db.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
				t.Fatalf("close sqlite: %v", err)
			}
		})
	}
	return db
}

type fixedSpecies string

func (s fixedSpecies) SelectSpecies(context.Context, string) (string, error) {
	return string(s), nil
}

type fixedVoice string

func (v fixedVoice) SelectVoice(context.Context, string) (string, error) {
	return string(v), nil
}

type fixedPetDecision rpcapi.PetActionDecision

func (d fixedPetDecision) DecidePetAction(context.Context, string, string, rpcapi.PetObject) (rpcapi.PetActionDecision, error) {
	return rpcapi.PetActionDecision(d), nil
}

type fixedRewardDecision rpcapi.RewardDecision

func (d fixedRewardDecision) DecideReward(context.Context, string, string) (rpcapi.RewardDecision, error) {
	return rpcapi.RewardDecision(d), nil
}

type fixedPeerConfigService struct {
	peer apitypes.Peer
	err  error
}

func (s fixedPeerConfigService) LoadPeer(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
	if s.err != nil {
		return apitypes.Peer{}, s.err
	}
	return s.peer, nil
}

func stringPtr(value string) *string {
	return &value
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
	spec := rpcapi.FlowcraftWorkflowSpec{"entry_agent": ""}
	return rpcapi.WorkflowDocument{
		Metadata: rpcapi.WorkflowMetadata{Name: name},
		Spec: rpcapi.WorkflowSpec{
			Driver:    rpcapi.WorkflowDriverFlowcraft,
			Flowcraft: &spec,
		},
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
		Body:     testRPCOpenAICredentialBody(key),
	}
}

func testVoiceUpsert(id string) adminservice.VoiceUpsert {
	return adminservice.VoiceUpsert{
		Id:     id,
		Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKindOpenaiTenant,
			Name: "global",
		},
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

type listingAuthorizer struct {
	*ruleAuthorizer
	bindings     []apitypes.ACLPolicyBinding
	listRequests []acl.ListPolicyBindingsRequest
}

func newListingAuthorizer() *listingAuthorizer {
	return &listingAuthorizer{ruleAuthorizer: newRuleAuthorizer()}
}

func (a *listingAuthorizer) ListPolicyBindings(_ context.Context, request acl.ListPolicyBindingsRequest) ([]apitypes.ACLPolicyBinding, bool, *string, error) {
	a.listRequests = append(a.listRequests, request)
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	}
	cursorPassed := request.Cursor == ""
	filtered := make([]apitypes.ACLPolicyBinding, 0, len(a.bindings))
	for _, binding := range a.bindings {
		if !cursorPassed {
			if binding.Id == request.Cursor {
				cursorPassed = true
			}
			continue
		}
		if request.SubjectKind != "" && binding.Policy.Subject.Kind != request.SubjectKind {
			continue
		}
		if request.SubjectID != "" && binding.Policy.Subject.Id != request.SubjectID {
			continue
		}
		if request.ResourceKind != "" && binding.Policy.Resource.Kind != request.ResourceKind {
			continue
		}
		if request.ResourceIDPrefix != "" && !strings.HasPrefix(binding.Policy.Resource.Id, request.ResourceIDPrefix) {
			continue
		}
		filtered = append(filtered, binding)
	}
	if len(filtered) <= limit {
		return filtered, false, nil, nil
	}
	nextCursor := filtered[limit-1].Id
	return filtered[:limit], true, &nextCursor, nil
}
