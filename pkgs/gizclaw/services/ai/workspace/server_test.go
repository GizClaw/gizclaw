package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerWorkspacesCRUD(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	deletionFencer := &recordingWorkspaceDeletionFencer{}
	srv.DeletionFencer = deletionFencer
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	createBody := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_id": "workflow-1",
		"parameters": {"mode": "demo"}
	}`)

	createResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResp)
	}
	if created.Name != "alpha001" || created.WorkflowId != "workflow-1" {
		t.Fatalf("CreateWorkspace() workspace = %#v", created)
	}
	if created.System == nil || *created.System {
		t.Fatalf("CreateWorkspace() system = %#v, want false", created.System)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() || created.LastActiveAt.IsZero() {
		t.Fatalf("CreateWorkspace() timestamps = %#v", created)
	}
	if !created.LastActiveAt.Equal(created.CreatedAt) {
		t.Fatalf("CreateWorkspace() last_active_at = %s, want created_at %s", created.LastActiveAt, created.CreatedAt)
	}
	workspaceID := string(created.Id)
	if workspaceID != createBody.Id {
		t.Fatalf("CreateWorkspace() id = %q, want caller ID %q", workspaceID, createBody.Id)
	}
	if len(runtime.prepared) != 1 || runtime.prepared[0] != workspaceID {
		t.Fatalf("runtime prepared after create = %#v", runtime.prepared)
	}

	listResp, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.Items[0].Name != "alpha001" || listed.HasNext {
		t.Fatalf("ListWorkspaces() = %#v", listed)
	}

	getResp, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: workspaceID})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	got, ok := getResp.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("GetWorkspace() response = %#v", getResp)
	}
	if got.Name != "alpha001" {
		t.Fatalf("GetWorkspace() = %#v", got)
	}

	updateBody := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_id": "workflow-1",
		"parameters": {"mode": "updated"}
	}`)
	putResp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{
		Id:   workspaceID,
		Body: &updateBody,
	})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	updated, ok := putResp.(adminhttp.PutWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("PutWorkspace() response = %#v", putResp)
	}
	if updated.CreatedAt.IsZero() || updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Fatalf("PutWorkspace() timestamps = %#v", updated)
	}
	if !updated.LastActiveAt.Equal(created.LastActiveAt) {
		t.Fatalf("PutWorkspace() last_active_at = %s, want unchanged %s", updated.LastActiveAt, created.LastActiveAt)
	}
	if len(runtime.prepared) != 2 || runtime.prepared[1] != workspaceID {
		t.Fatalf("runtime prepared after put = %#v", runtime.prepared)
	}

	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: workspaceID})
	if err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() response = %#v", deleteResp)
	}
	if len(deletionFencer.workspaceIDs) != 1 || deletionFencer.workspaceIDs[0] != workspaceID || !deletionFencer.callbackInvoked {
		t.Fatalf("Workspace deletion fence = %#v", deletionFencer)
	}
	if len(runtime.deleted) != 0 {
		t.Fatalf("runtime deleted during pending-deletion request = %#v", runtime.deleted)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, srv.Store, pendingdeletion.KindWorkspace, workspaceID); err != nil || !pending {
		t.Fatalf("workspace pending deletion = %v, error = %v", pending, err)
	}

	getAfterDelete, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: workspaceID})
	if err != nil {
		t.Fatalf("GetWorkspace() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminhttp.GetWorkspace200JSONResponse); !ok {
		t.Fatalf("GetWorkspace() after delete response = %#v", getAfterDelete)
	}
	listAfterDelete, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkspaces() after delete error = %v", err)
	}
	listedAfterDelete, ok := listAfterDelete.(adminhttp.ListWorkspaces200JSONResponse)
	if !ok || len(listedAfterDelete.Items) != 1 || listedAfterDelete.Items[0].Name != "alpha001" {
		t.Fatalf("ListWorkspaces() after delete = %#v", listAfterDelete)
	}
	if _, err := srv.GetAvailableWorkspaceByID(ctx, workspaceID); !errors.Is(err, ErrWorkspacePendingDeletion) {
		t.Fatalf("GetAvailableWorkspaceByID() after delete error = %v, want %v", err, ErrWorkspacePendingDeletion)
	}
	srv.RuntimeStore = nil
	if _, err := srv.GetWorkspaceRuntimeByID(ctx, workspaceID); !errors.Is(err, ErrWorkspacePendingDeletion) {
		t.Fatalf("GetWorkspaceRuntimeByID() without RuntimeStore error = %v, want %v", err, ErrWorkspacePendingDeletion)
	}
	createAfterDelete, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateWorkspace() while pending error = %v", err)
	}
	if _, ok := createAfterDelete.(adminhttp.CreateWorkspace409JSONResponse); !ok {
		t.Fatalf("CreateWorkspace() while retained response = %#v", createAfterDelete)
	}
	putAfterDelete, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: workspaceID, Body: &updateBody})
	if err != nil {
		t.Fatalf("PutWorkspace() while pending error = %v", err)
	}
	if response, ok := putAfterDelete.(adminhttp.PutWorkspace409JSONResponse); !ok || response.Error.Code != WorkspacePendingDeletionCode {
		t.Fatalf("PutWorkspace() while marked response = %#v", putAfterDelete)
	}
	invalidPutBody := updateBody
	invalidPutBody.Name = "other-workspace"
	invalidPutAfterDelete, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: workspaceID, Body: &invalidPutBody})
	if err != nil {
		t.Fatalf("PutWorkspace() invalid while pending error = %v", err)
	}
	if response, ok := invalidPutAfterDelete.(adminhttp.PutWorkspace409JSONResponse); !ok || response.Error.Code != WorkspacePendingDeletionCode {
		t.Fatalf("PutWorkspace() invalid while pending response = %#v, want pending conflict", invalidPutAfterDelete)
	}
}

func TestGetAvailableWorkspaceByIDFencesPendingOwnerButAdminRetainsRecord(t *testing.T) {
	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-owner-fence")
	ctx := ownership.WithOwner(t.Context(), "peer-a")
	body := adminhttp.WorkspaceUpsert{
		Id: "workspace-owner-fence", Name: "workspace-owner-fence", WorkflowId: "workflow-owner-fence",
	}
	response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateWorkspace200JSONResponse); !ok {
		t.Fatalf("CreateWorkspace() response = %#v", response)
	}
	srv.PeerAvailability = func(context.Context, string) error { return ErrPeerPendingDeletion }
	adminResponse, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: body.Id})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if _, ok := adminResponse.(adminhttp.GetWorkspace200JSONResponse); !ok {
		t.Fatalf("GetWorkspace() response = %#v, want retained record", adminResponse)
	}
	if _, err := srv.GetAvailableWorkspaceByID(ctx, body.Id); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("GetAvailableWorkspaceByID() error = %v, want %v", err, ErrPeerPendingDeletion)
	}
}

func TestGetAvailableWorkspaceByIDClassifiesOnlyCanonicalPhysicalAbsence(t *testing.T) {
	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-physical-delete")
	ctx := ownership.WithOwner(t.Context(), "peer-a")
	body := adminhttp.WorkspaceUpsert{
		Id: "workspace-physical-delete", Name: "workspace-physical-delete", WorkflowId: "workflow-physical-delete",
	}
	response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateWorkspace200JSONResponse); !ok {
		t.Fatalf("CreateWorkspace() response = %#v", response)
	}
	if err := srv.Store.Delete(ctx, workspaceKey(body.Id)); err != nil {
		t.Fatalf("delete canonical Workspace record: %v", err)
	}
	_, err = srv.GetAvailableWorkspaceByID(ctx, body.Id)
	if !errors.Is(err, ErrWorkspaceDeleted) {
		t.Fatalf("GetAvailableWorkspaceByID() error = %v, want %v", err, ErrWorkspaceDeleted)
	}
	if errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetAvailableWorkspaceByID() exposed Store not-found identity: %v", err)
	}
	if _, err := srv.GetAvailableWorkspaceByID(ctx, " not-valid "); errors.Is(err, ErrWorkspaceDeleted) {
		t.Fatalf("invalid Workspace ID classified as deleted: %v", err)
	}
}

func TestCreateWorkspaceRecordAtomicallyClaimsCallerID(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	servers := []*Server{
		srv,
		{Store: srv.Store, RuntimeStore: srv.RuntimeStore},
	}
	inputs := []adminhttp.WorkspaceUpsert{
		{Id: "shared-workspace-id", Name: "workspace-alpha", WorkflowId: "workflow-1"},
		{Id: "shared-workspace-id", Name: "workspace-beta", WorkflowId: "workflow-1"},
	}
	type result struct {
		workspace apitypes.Workspace
		err       error
	}
	start := make(chan struct{})
	results := make(chan result, len(inputs))
	for i, input := range inputs {
		go func(server *Server, body adminhttp.WorkspaceUpsert) {
			<-start
			workspace, err := server.createWorkspaceRecord(t.Context(), server.Store, body, false)
			results <- result{workspace: workspace, err: err}
		}(servers[i], input)
	}
	close(start)

	var winner apitypes.Workspace
	created := 0
	collisions := 0
	for range inputs {
		result := <-results
		switch {
		case result.err == nil:
			winner = result.workspace
			created++
		case errors.Is(result.err, errWorkspaceIDExists):
			collisions++
		default:
			t.Fatalf("createWorkspaceRecord() error = %v", result.err)
		}
	}
	if created != 1 || collisions != 1 {
		t.Fatalf("create results = %d created, %d collisions; want 1 each", created, collisions)
	}
	stored, err := getWorkspaceByID(t.Context(), srv.Store, "shared-workspace-id")
	if err != nil {
		t.Fatalf("getWorkspaceByID() error = %v", err)
	}
	if stored.Name != winner.Name {
		t.Fatalf("stored Workspace name = %q, winner = %q", stored.Name, winner.Name)
	}
	for _, input := range inputs {
		_, err := getWorkspace(t.Context(), srv.Store, input.Name)
		if input.Name == winner.Name && err != nil {
			t.Fatalf("winner name %q lookup error = %v", input.Name, err)
		}
		if input.Name != winner.Name && !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("loser name %q lookup error = %v, want not found", input.Name, err)
		}
	}
}

func TestCreateWorkspaceRecordAtomicallyClaimsScopedName(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	servers := []*Server{
		srv,
		{Store: srv.Store, RuntimeStore: srv.RuntimeStore},
	}
	inputs := []adminhttp.WorkspaceUpsert{
		{Id: "workspace-alpha-id", Name: "shared-workspace-name", WorkflowId: "workflow-1"},
		{Id: "workspace-beta-id", Name: "shared-workspace-name", WorkflowId: "workflow-1"},
	}
	type result struct {
		workspace apitypes.Workspace
		err       error
	}
	start := make(chan struct{})
	results := make(chan result, len(inputs))
	for i, input := range inputs {
		go func(server *Server, body adminhttp.WorkspaceUpsert) {
			<-start
			workspace, err := server.createWorkspaceRecord(t.Context(), server.Store, body, false)
			results <- result{workspace: workspace, err: err}
		}(servers[i], input)
	}
	close(start)

	var winner apitypes.Workspace
	created := 0
	conflicts := 0
	for range inputs {
		result := <-results
		switch {
		case result.err == nil:
			winner = result.workspace
			created++
		case errors.Is(result.err, errWorkspaceNameExists):
			conflicts++
		default:
			t.Fatalf("createWorkspaceRecord() error = %v", result.err)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("create results = %d created, %d conflicts; want 1 each", created, conflicts)
	}
	stored, err := getWorkspace(t.Context(), srv.Store, "shared-workspace-name")
	if err != nil {
		t.Fatalf("getWorkspace() error = %v", err)
	}
	if stored.Id != winner.Id {
		t.Fatalf("stored Workspace ID = %q, winner = %q", stored.Id, winner.Id)
	}
	for _, input := range inputs {
		_, err := getWorkspaceByID(t.Context(), srv.Store, input.Id)
		if input.Id == winner.Id && err != nil {
			t.Fatalf("winner ID %q lookup error = %v", input.Id, err)
		}
		if input.Id != winner.Id && !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("loser ID %q lookup error = %v, want not found", input.Id, err)
		}
	}
}

func TestServerSystemWorkspaceLifecycle(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := ownership.WithOwner(context.Background(), " peer-a ")
	seedWorkflow(t, srv, "chatroom")
	directMode := apitypes.ChatRoomModeDirect
	pushToTalk := apitypes.WorkspaceInputModePushToTalk
	parameters := apitypes.WorkspaceParameters{}
	if err := parameters.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Mode:  &directMode,
		Input: &pushToTalk,
	}); err != nil {
		t.Fatalf("encode initial chatroom parameters: %v", err)
	}
	body := adminhttp.WorkspaceUpsert{Name: "friend-chat", WorkflowId: "chatroom", Parameters: &parameters}

	created, wasCreated, err := srv.CreateSystemWorkspace(ctx, body)
	if err != nil {
		t.Fatalf("CreateSystemWorkspace() error = %v", err)
	}
	if !wasCreated || created.System == nil || !*created.System || created.OwnerPublicKey == nil || *created.OwnerPublicKey != "peer-a" {
		t.Fatalf("CreateSystemWorkspace() = %#v, created=%v", created, wasCreated)
	}
	existing, wasCreated, err := srv.CreateSystemWorkspace(ctx, body)
	if err != nil {
		t.Fatalf("CreateSystemWorkspace(existing) error = %v", err)
	}
	if wasCreated || existing.System == nil || !*existing.System {
		t.Fatalf("CreateSystemWorkspace(existing) = %#v, created=%v", existing, wasCreated)
	}
	realtime := apitypes.WorkspaceInputModeRealtime
	updatedParameters := apitypes.WorkspaceParameters{}
	if err := updatedParameters.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Mode:  &directMode,
		Input: &realtime,
	}); err != nil {
		t.Fatalf("encode updated chatroom parameters: %v", err)
	}
	putBody := adminhttp.WorkspaceUpsert{
		Id:         created.Id,
		Name:       "friend-chat",
		WorkflowId: "chatroom",
		Parameters: &updatedParameters,
	}
	putCtx := WithRuntimeWorkflowBindings(ctx, map[string]string{"personal": "chatroom"})
	putResp, err := srv.PutWorkspace(putCtx, adminhttp.PutWorkspaceRequestObject{Id: created.Id, Body: &putBody})
	if err != nil {
		t.Fatalf("PutWorkspace(system) error = %v", err)
	}
	updated, ok := putResp.(adminhttp.PutWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("PutWorkspace(system parameters) response = %#v", putResp)
	}
	chatroom, err := updated.Parameters.AsChatRoomWorkspaceParameters()
	if err != nil || chatroom.Input == nil || *chatroom.Input != realtime {
		t.Fatalf("PutWorkspace(system parameters) = %#v, error = %v", updated, err)
	}
	labels := map[string]string{"domain": "changed"}
	conflictingLabelsBody := putBody
	conflictingLabelsBody.Labels = &labels
	conflictingLabels, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: created.Id, Body: &conflictingLabelsBody})
	if err != nil {
		t.Fatalf("PutWorkspace(system conflicting labels) error = %v", err)
	}
	blockedUpdate, ok := conflictingLabels.(adminhttp.PutWorkspace409JSONResponse)
	if !ok || blockedUpdate.Error.Code != SystemWorkspaceUpdateForbiddenCode {
		t.Fatalf("PutWorkspace(system conflicting labels) response = %#v", conflictingLabels)
	}
	toolIDs := []string{"tool-a"}
	conflictingToolkitBody := putBody
	conflictingToolkitBody.Toolkit = &apitypes.ToolkitPolicy{ToolIds: &toolIDs}
	conflictingToolkit, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: created.Id, Body: &conflictingToolkitBody})
	if err != nil {
		t.Fatalf("PutWorkspace(system conflicting toolkit) error = %v", err)
	}
	blockedUpdate, ok = conflictingToolkit.(adminhttp.PutWorkspace409JSONResponse)
	if !ok || blockedUpdate.Error.Code != SystemWorkspaceUpdateForbiddenCode {
		t.Fatalf("PutWorkspace(system conflicting toolkit) response = %#v", conflictingToolkit)
	}
	transcriptEnabled := true
	conflictingTranscriptParameters := apitypes.WorkspaceParameters{}
	if err := conflictingTranscriptParameters.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Mode:       &directMode,
		Input:      &realtime,
		Transcript: &apitypes.ChatRoomWorkspaceTranscriptParameters{Enabled: &transcriptEnabled},
	}); err != nil {
		t.Fatalf("encode conflicting chatroom transcript parameters: %v", err)
	}
	conflictingTranscriptBody := putBody
	conflictingTranscriptBody.Parameters = &conflictingTranscriptParameters
	conflictingTranscript, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: created.Id, Body: &conflictingTranscriptBody})
	if err != nil {
		t.Fatalf("PutWorkspace(system conflicting transcript) error = %v", err)
	}
	blockedUpdate, ok = conflictingTranscript.(adminhttp.PutWorkspace409JSONResponse)
	if !ok || blockedUpdate.Error.Code != SystemWorkspaceUpdateForbiddenCode {
		t.Fatalf("PutWorkspace(system conflicting transcript) response = %#v", conflictingTranscript)
	}
	groupMode := apitypes.ChatRoomModeGroup
	conflictingParameters := apitypes.WorkspaceParameters{}
	if err := conflictingParameters.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Mode:  &groupMode,
		Input: &realtime,
	}); err != nil {
		t.Fatalf("encode conflicting chatroom parameters: %v", err)
	}
	conflictingPutBody := putBody
	conflictingPutBody.Parameters = &conflictingParameters
	conflictingPut, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: created.Id, Body: &conflictingPutBody})
	if err != nil {
		t.Fatalf("PutWorkspace(system conflicting mode) error = %v", err)
	}
	blockedUpdate, ok = conflictingPut.(adminhttp.PutWorkspace409JSONResponse)
	if !ok || blockedUpdate.Error.Code != SystemWorkspaceUpdateForbiddenCode {
		t.Fatalf("PutWorkspace(system conflicting mode) response = %#v", conflictingPut)
	}
	conflictingWorkflowBody := putBody
	conflictingWorkflowBody.WorkflowId = "other-workflow"
	conflictingWorkflow, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: created.Id, Body: &conflictingWorkflowBody})
	if err != nil {
		t.Fatalf("PutWorkspace(system conflicting Workflow) error = %v", err)
	}
	blockedUpdate, ok = conflictingWorkflow.(adminhttp.PutWorkspace409JSONResponse)
	if !ok || blockedUpdate.Error.Code != SystemWorkspaceUpdateForbiddenCode {
		t.Fatalf("PutWorkspace(system conflicting Workflow) response = %#v", conflictingWorkflow)
	}

	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteWorkspace(system) error = %v", err)
	}
	blocked, ok := deleteResp.(adminhttp.DeleteWorkspace409JSONResponse)
	if !ok || blocked.Error.Code != SystemWorkspaceDeleteForbiddenCode {
		t.Fatalf("DeleteWorkspace(system) response = %#v", deleteResp)
	}
	if len(runtime.deleted) != 0 {
		t.Fatalf("runtime deleted after rejected generic delete = %#v", runtime.deleted)
	}
	if _, err := getWorkspace(ctx, srv.Store, "friend-chat"); err != nil {
		t.Fatalf("system workspace after rejected generic delete: %v", err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, srv.Store, pendingdeletion.KindWorkspace, created.Id); err != nil || pending {
		t.Fatalf("system workspace pending deletion = %v, error = %v", pending, err)
	}

	deleted, err := srv.DeleteSystemWorkspace(ctx, "friend-chat")
	if err != nil {
		t.Fatalf("DeleteSystemWorkspace() error = %v", err)
	}
	if deleted.System == nil || !*deleted.System {
		t.Fatalf("DeleteSystemWorkspace() = %#v", deleted)
	}
	if len(runtime.deleted) != 1 || runtime.deleted[0] != created.Id {
		t.Fatalf("runtime deleted after system delete = %#v", runtime.deleted)
	}
	if _, err := srv.DeleteSystemWorkspace(ctx, "friend-chat"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("DeleteSystemWorkspace(missing) error = %v, want kv.ErrNotFound", err)
	}
	if len(runtime.deleted) != 1 {
		t.Fatalf("runtime deleted after missing system delete = %#v, want no name-based cleanup", runtime.deleted)
	}
}

func TestWorkspaceDeleteSerializesWithPut(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")
	body := adminhttp.WorkspaceUpsert{Id: "concurrent-id", Name: "concurrent", WorkflowId: "workflow-1"}
	response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, ok := response.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkspace response = %#v", response)
	}
	workspaceID := created.Id

	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		response, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: workspaceID})
		if err == nil {
			if _, ok := response.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
				err = fmt.Errorf("DeleteWorkspace response = %#v", response)
			}
		}
		errs <- err
	}()
	go func() {
		<-start
		response, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: workspaceID, Body: &body})
		if err == nil {
			switch response := response.(type) {
			case adminhttp.PutWorkspace200JSONResponse:
			case adminhttp.PutWorkspace409JSONResponse:
				if response.Error.Code != WorkspacePendingDeletionCode {
					err = fmt.Errorf("PutWorkspace conflict = %#v", response)
				}
			default:
				err = fmt.Errorf("PutWorkspace response = %#v", response)
			}
		}
		errs <- err
	}()
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := getWorkspaceByID(ctx, srv.Store, workspaceID); err != nil {
		t.Fatalf("workspace after concurrent delete/put error = %v", err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, srv.Store, pendingdeletion.KindWorkspace, workspaceID); err != nil || !pending {
		t.Fatalf("workspace pending deletion = %v, error = %v", pending, err)
	}
}

func TestCreateSystemWorkspaceRejectsRetiringWorkspace(t *testing.T) {
	srv := newTestServer(t)
	ctx := ownership.WithOwner(context.Background(), "peer-a")
	seedWorkflow(t, srv, "chatroom")
	directMode := apitypes.ChatRoomModeDirect
	parameters := apitypes.WorkspaceParameters{}
	if err := parameters.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Mode: &directMode,
	}); err != nil {
		t.Fatalf("encode Chatroom parameters: %v", err)
	}
	body := adminhttp.WorkspaceUpsert{
		Name:       "friend-chat-retiring",
		WorkflowId: "chatroom",
		Parameters: &parameters,
	}
	_, _, err := srv.CreateSystemWorkspace(ctx, body)
	if err != nil {
		t.Fatalf("CreateSystemWorkspace() error = %v", err)
	}
	if _, err := srv.RetireSystemWorkspace(ctx, body.Name, directMode, "peer-a:peer-b"); err != nil {
		t.Fatalf("RetireSystemWorkspace() error = %v", err)
	}
	if _, _, err := srv.CreateSystemWorkspace(ctx, body); err == nil ||
		!strings.Contains(err.Error(), "pending deletion") {
		t.Fatalf("CreateSystemWorkspace(retiring) error = %v, want pending deletion conflict", err)
	}
}

func TestCreateSystemWorkspaceRechecksOwnerInsideCreateLock(t *testing.T) {
	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-create-fence")
	calls := 0
	srv.PeerAvailability = func(context.Context, string) error {
		calls++
		if calls == 2 {
			return ErrPeerPendingDeletion
		}
		return nil
	}
	ctx := ownership.WithOwner(t.Context(), "peer-a")
	body := adminhttp.WorkspaceUpsert{
		Id: "workspace-create-fence", Name: "workspace-create-fence", WorkflowId: "workflow-create-fence",
	}
	if _, _, err := srv.CreateSystemWorkspace(ctx, body); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("CreateSystemWorkspace() error = %v, want pending deletion", err)
	}
	if calls != 2 {
		t.Fatalf("PeerAvailability calls = %d, want pre-lock and in-lock checks", calls)
	}
	if _, err := getWorkspaceByID(t.Context(), srv.Store, body.Id); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("rejected Workspace error = %v, want not found", err)
	}
}

func TestCreateSystemWorkspaceRejectsRetiringWorkspaceAfterRecordRemoval(t *testing.T) {
	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := ownership.WithOwner(context.Background(), "peer-a")
	seedWorkflow(t, srv, "chatroom")
	directMode := apitypes.ChatRoomModeDirect
	parameters := apitypes.WorkspaceParameters{}
	if err := parameters.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Mode: &directMode,
	}); err != nil {
		t.Fatalf("encode Chatroom parameters: %v", err)
	}
	body := adminhttp.WorkspaceUpsert{
		Name:       "friend-chat-partially-cleaned",
		WorkflowId: "chatroom",
		Parameters: &parameters,
	}
	created, _, err := srv.CreateSystemWorkspace(ctx, body)
	if err != nil {
		t.Fatalf("CreateSystemWorkspace() error = %v", err)
	}
	if _, err := srv.RetireSystemWorkspace(ctx, body.Name, directMode, "peer-a:peer-b"); err != nil {
		t.Fatalf("RetireSystemWorkspace() error = %v", err)
	}
	if err := srv.Store.Delete(ctx, workspaceKey(created.Id)); err != nil {
		t.Fatalf("remove active Workspace record: %v", err)
	}
	preparedBefore := len(runtime.prepared)

	if _, _, err := srv.CreateSystemWorkspace(ctx, body); err == nil ||
		!strings.Contains(err.Error(), "pending deletion") {
		t.Fatalf("CreateSystemWorkspace(partially cleaned) error = %v, want pending deletion conflict", err)
	}
	if len(runtime.prepared) != preparedBefore {
		t.Fatalf("runtime prepared after pending conflict = %#v, want no new preparation", runtime.prepared)
	}
	retired, err := srv.RetireSystemWorkspace(ctx, body.Name, directMode, "peer-a:peer-b")
	if err != nil {
		t.Fatalf("RetireSystemWorkspace(retry after cleanup) error = %v", err)
	}
	if retired.Name != body.Name {
		t.Fatalf("RetireSystemWorkspace(retry after cleanup) name = %q, want %q", retired.Name, body.Name)
	}
	if retired.OwnerPublicKey == nil || *retired.OwnerPublicKey != "peer-a" {
		t.Fatalf("RetireSystemWorkspace(retry after cleanup) owner = %#v, want peer-a", retired.OwnerPublicKey)
	}
	if _, err := srv.RetireSystemWorkspace(ctx, body.Name, directMode, "peer-a:peer-c"); err == nil {
		t.Fatal("RetireSystemWorkspace(mismatched completed retry) error = nil")
	}
}

func TestServerSystemWorkspaceClassificationComesFromCreationPath(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = &recordingRuntimeStore{}
	ctx := context.Background()
	seedWorkflow(t, srv, "chatroom")
	body := adminhttp.WorkspaceUpsert{Id: "friend-user-created-id", Name: "friend-user-created", WorkflowId: "chatroom"}

	createResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok || created.System == nil || *created.System {
		t.Fatalf("CreateWorkspace() response = %#v, want user Workspace", createResp)
	}
	if _, _, err := srv.CreateSystemWorkspace(ctx, body); err == nil {
		t.Fatal("CreateSystemWorkspace(user Workspace) error = nil, want classification conflict")
	}
	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() response = %#v", deleteResp)
	}
}

func TestServerCreateWorkspaceDeletesPreparedRuntimeWhenRecordCreationFails(t *testing.T) {
	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	seedWorkflow(t, srv, "workflow-1")
	srv.Store = failingCreateIfAbsentStore{Store: srv.Store, err: errors.New("injected create failure")}
	body := adminhttp.WorkspaceUpsert{Id: "workspace-failed-create", Name: "failed-create", WorkflowId: "workflow-1"}

	response, err := srv.CreateWorkspace(t.Context(), adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	internal, ok := response.(adminhttp.CreateWorkspace500JSONResponse)
	if !ok || !strings.Contains(internal.Error.Message, "injected create failure") {
		t.Fatalf("CreateWorkspace() response = %#v, want injected 500", response)
	}
	if len(runtime.prepared) != 1 || runtime.prepared[0] != "workspace-failed-create" {
		t.Fatalf("prepared runtimes = %#v", runtime.prepared)
	}
	if len(runtime.deleted) != 1 || runtime.deleted[0] != "workspace-failed-create" {
		t.Fatalf("deleted runtimes = %#v, want prepared runtime cleanup", runtime.deleted)
	}
}

func TestServerRejectsWorkspaceRecordsOutsideFinalSchema(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	incomplete := map[string]any{
		"id":          "legacy-id",
		"name":        "legacy",
		"workflow_id": "workflow-1",
		"created_at":  createdAt.Format(time.RFC3339Nano),
		"updated_at":  updatedAt.Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := srv.Store.BatchSet(ctx, []kv.Entry{
		{Key: workspaceKey("legacy-id"), Value: data},
		{Key: workspaceScopeNameKey(nil, "legacy"), Value: []byte("legacy-id")},
	}); err != nil {
		t.Fatalf("seed legacy workspace: %v", err)
	}

	if _, err := getWorkspace(ctx, srv.Store, "legacy"); err == nil || !strings.Contains(err.Error(), "requires system") {
		t.Fatalf("getWorkspace() error = %v", err)
	}
	if _, _, _, err := listWorkspacePage(ctx, srv.Store, workspacesRoot, "", 10, nil); err == nil || !strings.Contains(err.Error(), "requires system") {
		t.Fatalf("listWorkspacePage() error = %v", err)
	}
}

func TestServerListWorkspacesPagination(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	for _, name := range []string{"alpha001", "beta0001", "gamma001"} {
		body := adminhttp.WorkspaceUpsert{
			Id:         name + "-id",
			Name:       string(name),
			WorkflowId: "workflow-1",
		}
		if _, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateWorkspace(%q) error = %v", name, err)
		}
	}

	limit := int32(1)
	firstResp, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{
		Params: adminhttp.ListWorkspacesParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListWorkspaces(first page) error = %v", err)
	}
	first, ok := firstResp.(adminhttp.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkspaces(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{
		Params: adminhttp.ListWorkspacesParams{
			Cursor: &cursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListWorkspaces(second page) error = %v", err)
	}
	second, ok := secondResp.(adminhttp.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 || second.Items[0].Name == first.Items[0].Name {
		t.Fatalf("ListWorkspaces(second page) = %#v", second)
	}
}

func TestServerWorkspaceLabelsRoundTripPreserveAndClear(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-1")
	ctx := context.Background()
	inputLabels := map[string]string{"collection": "raids", "tier": "Gold"}
	body := adminhttp.WorkspaceUpsert{Id: "labels01-id", Name: "labels01", WorkflowId: "workflow-1", Labels: &inputLabels}
	response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := response.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok || created.Labels == nil || (*created.Labels)["collection"] != "raids" {
		t.Fatalf("CreateWorkspace() = %#v", response)
	}
	inputLabels["collection"] = "mutated"
	(*created.Labels)["collection"] = "also-mutated"

	workspaceID := created.Id
	getResponse, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: workspaceID})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	stored := getResponse.(adminhttp.GetWorkspace200JSONResponse)
	if stored.Labels == nil || (*stored.Labels)["collection"] != "raids" {
		t.Fatalf("stored labels = %#v", stored.Labels)
	}

	preserve := adminhttp.WorkspaceUpsert{Id: workspaceID, Name: "labels01", WorkflowId: "workflow-1"}
	putResponse, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: workspaceID, Body: &preserve})
	if err != nil {
		t.Fatalf("PutWorkspace(preserve) error = %v", err)
	}
	preserved := putResponse.(adminhttp.PutWorkspace200JSONResponse)
	if preserved.Labels == nil || (*preserved.Labels)["collection"] != "raids" {
		t.Fatalf("preserved labels = %#v", preserved.Labels)
	}

	empty := map[string]string{}
	clear := adminhttp.WorkspaceUpsert{Id: workspaceID, Name: "labels01", WorkflowId: "workflow-1", Labels: &empty}
	putResponse, err = srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: workspaceID, Body: &clear})
	if err != nil {
		t.Fatalf("PutWorkspace(clear) error = %v", err)
	}
	cleared := putResponse.(adminhttp.PutWorkspace200JSONResponse)
	if cleared.Labels == nil || len(*cleared.Labels) != 0 {
		t.Fatalf("cleared labels = %#v", cleared.Labels)
	}
}

func TestServerWorkspaceLabelValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels map[string]string
	}{
		{name: "empty key", labels: map[string]string{"": "value"}},
		{name: "uppercase key", labels: map[string]string{"Collection": "value"}},
		{name: "invalid key character", labels: map[string]string{"collection/x": "value"}},
		{name: "invalid key end", labels: map[string]string{"collection-": "value"}},
		{name: "empty value", labels: map[string]string{"collection": ""}},
		{name: "leading whitespace", labels: map[string]string{"collection": " raids"}},
		{name: "control character", labels: map[string]string{"collection": "raid\n"}},
		{name: "invalid utf8", labels: map[string]string{"collection": string([]byte{0xff})}},
		{name: "oversized value", labels: map[string]string{"collection": strings.Repeat("x", maxWorkspaceLabelValueBytes+1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv := newTestServer(t)
			seedWorkflow(t, srv, "workflow-1")
			body := adminhttp.WorkspaceUpsert{Id: "invalid1-id", Name: "invalid1", WorkflowId: "workflow-1", Labels: &test.labels}
			response, err := srv.CreateWorkspace(context.Background(), adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatalf("CreateWorkspace() error = %v", err)
			}
			if _, ok := response.(adminhttp.CreateWorkspace400JSONResponse); !ok {
				t.Fatalf("CreateWorkspace() response = %#v, want 400", response)
			}
			if _, err := getWorkspace(context.Background(), srv.Store, "invalid1"); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("invalid Workspace write error = %v, want kv.ErrNotFound", err)
			}
		})
	}
}

func TestServerWorkspaceLabelFilteringBeforePagination(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ids := []string{"workspace-01", "workspace-02", "workspace-03", "workspace-04"}
	seedWorkflow(t, srv, "workflow-1")
	ctx := ownership.WithOwner(context.Background(), "peer-a")
	fixtures := []struct {
		name       string
		collection string
		tier       string
	}{
		{name: "alpha001", collection: "raids", tier: "gold"},
		{name: "beta0001", collection: "assistants", tier: "gold"},
		{name: "gamma001", collection: "raids", tier: "silver"},
		{name: "omega001", collection: "raids", tier: "gold"},
	}
	for _, fixture := range fixtures {
		labels := map[string]string{"collection": fixture.collection, "tier": fixture.tier}
		body := adminhttp.WorkspaceUpsert{Id: ids[0], Name: fixture.name, WorkflowId: "workflow-1", Labels: &labels}
		ids = ids[1:]
		if _, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateWorkspace(%q) error = %v", fixture.name, err)
		}
	}

	selectors := []string{"collection=raids"}
	limit := int32(2)
	firstResponse, err := srv.ListWorkspaces(context.Background(), adminhttp.ListWorkspacesRequestObject{Params: adminhttp.ListWorkspacesParams{Label: &selectors, Limit: &limit}})
	if err != nil {
		t.Fatalf("ListWorkspaces(first) error = %v", err)
	}
	first := firstResponse.(adminhttp.ListWorkspaces200JSONResponse)
	if len(first.Items) != 2 || first.Items[0].Name != "alpha001" || first.Items[1].Name != "gamma001" || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkspaces(first) = %#v", first)
	}
	cursor := *first.NextCursor
	secondResponse, err := srv.ListWorkspaces(context.Background(), adminhttp.ListWorkspacesRequestObject{Params: adminhttp.ListWorkspacesParams{Cursor: &cursor, Label: &selectors, Limit: &limit}})
	if err != nil {
		t.Fatalf("ListWorkspaces(second) error = %v", err)
	}
	second := secondResponse.(adminhttp.ListWorkspaces200JSONResponse)
	if len(second.Items) != 1 || second.Items[0].Name != "omega001" || second.HasNext {
		t.Fatalf("ListWorkspaces(second) = %#v", second)
	}

	owned, err := srv.ListWorkspacesByOwnerAndLabels(context.Background(), "peer-a", map[string]string{"collection": "raids", "tier": "gold"})
	if err != nil {
		t.Fatalf("ListWorkspacesByOwnerAndLabels() error = %v", err)
	}
	if len(owned) != 2 || owned[0].Name != "alpha001" || owned[1].Name != "omega001" {
		t.Fatalf("ListWorkspacesByOwnerAndLabels() = %#v", owned)
	}

	invalid := []string{"collection"}
	invalidResponse, err := srv.ListWorkspaces(context.Background(), adminhttp.ListWorkspacesRequestObject{Params: adminhttp.ListWorkspacesParams{Label: &invalid}})
	if err != nil {
		t.Fatalf("ListWorkspaces(invalid) error = %v", err)
	}
	if _, ok := invalidResponse.(adminhttp.ListWorkspaces400JSONResponse); !ok {
		t.Fatalf("ListWorkspaces(invalid) = %#v, want 400", invalidResponse)
	}
}

func TestServerRejectsInvalidWorkspaceReferences(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	missingWorkflow := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_id": "missing-workflow"
	}`)
	resp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &missingWorkflow})
	if err != nil {
		t.Fatalf("CreateWorkspace(missing workflow) error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(missing workflow) response = %#v", resp)
	}

	nilCreateResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{})
	if err != nil {
		t.Fatalf("CreateWorkspace(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(nil body) response = %#v", nilCreateResp)
	}

	missingID := adminhttp.WorkspaceUpsert{Name: "alpha001", WorkflowId: "workflow-1"}
	missingIDResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &missingID})
	if err != nil {
		t.Fatalf("CreateWorkspace(missing id) error = %v", err)
	}
	invalidMissingID, ok := missingIDResp.(adminhttp.CreateWorkspace400JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkspace(missing id) response = %#v, want 400", missingIDResp)
	}
	if !strings.Contains(invalidMissingID.Error.Message, "id is required") {
		t.Fatalf("CreateWorkspace(missing id) message = %q", invalidMissingID.Error.Message)
	}

	missingName := mustWorkspaceUpsert(t, `{
		"name": " ",
		"workflow_id": "workflow-1"
	}`)
	missingNameResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &missingName})
	if err != nil {
		t.Fatalf("CreateWorkspace(missing name) error = %v", err)
	}
	if _, ok := missingNameResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(missing name) response = %#v", missingNameResp)
	}

	invalidWorkflowId := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_id": "Bad_Name"
	}`)
	invalidWorkflowResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &invalidWorkflowId})
	if err != nil {
		t.Fatalf("CreateWorkspace(invalid workflow name) error = %v", err)
	}
	if _, ok := invalidWorkflowResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(invalid workflow name) response = %#v", invalidWorkflowResp)
	}
}

func TestNormalizeWorkspaceUpsertAcceptsOpaqueWorkflowIDs(t *testing.T) {
	t.Parallel()

	for _, workflowID := range []string{"2fa-chat", "Opaque_Workflow_ID", strings.Repeat("a", 80)} {
		got, err := normalizeWorkspaceUpsert(adminhttp.WorkspaceUpsert{
			Id:         "runtime-workspace-id",
			Name:       "runtime-workspace",
			WorkflowId: workflowID,
		}, "")
		if err != nil {
			t.Fatalf("normalizeWorkspaceUpsert(%q) error = %v", workflowID, err)
		}
		if got.WorkflowId != workflowID {
			t.Fatalf("normalizeWorkspaceUpsert() workflow_id = %q, want %q", got.WorkflowId, workflowID)
		}
	}
}

func TestNormalizeWorkspaceUpsertRejectsInvalidWorkflowID(t *testing.T) {
	t.Parallel()

	_, err := normalizeWorkspaceUpsert(adminhttp.WorkspaceUpsert{
		Id:         "runtime-workspace-id",
		Name:       "runtime-workspace",
		WorkflowId: "  0123456789abcdef  ",
	}, "")
	if err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("normalizeWorkspaceUpsert(whitespace workflow_id) error = %v", err)
	}

	_, err = normalizeWorkspaceUpsert(adminhttp.WorkspaceUpsert{
		Id:         "runtime-workspace-id",
		Name:       "runtime-workspace",
		WorkflowId: "  ",
	}, "")
	if err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("normalizeWorkspaceUpsert(empty workflow_id) error = %v", err)
	}
}

func TestServerRejectsInvalidToolkitPolicy(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")
	toolIDs := []string{""}
	body := adminhttp.WorkspaceUpsert{
		Id:         "alpha001-id",
		Name:       "alpha001",
		WorkflowId: "workflow-1",
		Toolkit:    &apitypes.ToolkitPolicy{ToolIds: &toolIDs},
	}

	createResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, ok := createResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResp)
	}

	putResp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: "alpha001", Body: &body})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	if _, ok := putResp.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace() response = %#v", putResp)
	}
}

func TestServerPutRejectsPathNameMismatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	createdBody := mustWorkspaceUpsert(t, `{
		"name": "expected1",
		"workflow_id": "workflow-1"
	}`)
	createResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &createdBody})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResp)
	}

	body := mustWorkspaceUpsert(t, `{
		"name": "other001",
		"workflow_id": "workflow-1"
	}`)
	resp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{
		Id:   created.Id,
		Body: &body,
	})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	if _, ok := resp.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace() response = %#v", resp)
	}

	nilPutResp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("PutWorkspace(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace(nil body) response = %#v", nilPutResp)
	}
}

func TestServerWorkspaceConflictAndMissingDelete(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	body := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_id": "workflow-1"
	}`)
	if _, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateWorkspace(seed) error = %v", err)
	}
	duplicateResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace(duplicate) error = %v", err)
	}
	if _, ok := duplicateResp.(adminhttp.CreateWorkspace409JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(duplicate) response = %#v", duplicateResp)
	}

	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeleteWorkspace(missing) error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkspace404JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace(missing) response = %#v", deleteResp)
	}
	if len(runtime.deleted) != 0 {
		t.Fatalf("runtime deleted for missing workspace = %#v", runtime.deleted)
	}
}

func TestServerDefersDirectFlowcraftAliasesToOwnerRuntimeProfile(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedFlowcraftWorkflow(t, srv, "model-service-missing", "chat-alias")
	srv.Models = nil
	body := mustWorkspaceUpsert(t, `{"name":"model-service-missing","workflow_id":"model-service-missing","parameters":{"agent_type":"flowcraft"}}`)
	resp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace(model service missing) error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkspace200JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(model service missing) response = %#v, want 200", resp)
	}
}

func TestServerValidatesRuntimeFlowcraftModelAliases(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedFlowcraftWorkflow(t, srv, "flowcraft-chat", "generate-model")
	seedModel(t, srv, "chat-model", apitypes.ModelKindLlm)
	ctx := WithRuntimeModelBindings(
		WithRuntimeWorkflowBindings(context.Background(), map[string]string{"2fa-chat": "flowcraft-chat"}),
		map[string]string{
			"generate-model": "chat-model",
		},
	)

	tests := []struct {
		name     string
		bindings map[string]string
		want     string
		ok       bool
	}{
		{
			name:     "valid aliases",
			bindings: map[string]string{"generate-model": "chat-model"},
			ok:       true,
		},
		{
			name:     "missing graph alias",
			bindings: map[string]string{},
			want:     `references missing runtime Model alias "generate-model"`,
		},
	}
	var validWorkspaceID string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCtx := WithRuntimeModelBindings(WithRuntimeWorkflowBindings(context.Background(), map[string]string{"2fa-chat": "flowcraft-chat"}), tt.bindings)
			body := mustWorkspaceUpsert(t, fmt.Sprintf(`{"name":%q,"workflow_id":"flowcraft-chat","parameters":{"agent_type":"flowcraft"}}`, "runtime-"+strings.ReplaceAll(tt.name, " ", "-")))
			resp, err := srv.CreateWorkspace(testCtx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatalf("CreateWorkspace() error = %v", err)
			}
			if tt.ok {
				created, ok := resp.(adminhttp.CreateWorkspace200JSONResponse)
				if !ok {
					t.Fatalf("CreateWorkspace() response = %#v", resp)
				}
				validWorkspaceID = created.Id
				return
			}
			invalid, ok := resp.(adminhttp.CreateWorkspace400JSONResponse)
			if !ok {
				t.Fatalf("CreateWorkspace() response = %#v, want 400", resp)
			}
			if !strings.Contains(invalid.Error.Message, tt.want) {
				t.Fatalf("CreateWorkspace() message = %q, want substring %q", invalid.Error.Message, tt.want)
			}
			if strings.Contains(invalid.Error.Message, "chat-model") {
				t.Fatalf("CreateWorkspace() message exposes canonical runtime Model: %q", invalid.Error.Message)
			}
		})
	}
	getResp, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: validWorkspaceID})
	if err != nil {
		t.Fatalf("GetWorkspace(runtime-valid) error = %v", err)
	}
	stored, ok := getResp.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("GetWorkspace(runtime-valid) response = %#v", getResp)
	}
	parameters, err := stored.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatalf("stored Workspace parameters: %v", err)
	}
	if parameters.AgentType != apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft {
		t.Fatalf("stored Workspace parameters = %#v", parameters)
	}
}

func TestValidateDoubaoRealtimeOverridesRejectsTools(t *testing.T) {
	tools := []apitypes.DoubaoRealtimeFunctionTool{{
		Type: apitypes.DoubaoRealtimeFunctionToolTypeFunction,
		Name: "get_weather",
	}}
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromDoubaoRealtimeWorkspaceParameters(apitypes.DoubaoRealtimeWorkspaceParameters{
		AgentType: apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime,
		Tools:     &tools,
	}); err != nil {
		t.Fatal(err)
	}
	if err := validateDoubaoRealtimeOverrides(&parameters); err == nil || !strings.Contains(err.Error(), "tools are unsupported") {
		t.Fatalf("validateDoubaoRealtimeOverrides() error = %v", err)
	}
}

func TestValidateRealtimeOverridesRejectInvalidOptions(t *testing.T) {
	temperature := float32(3)
	var dashParameters apitypes.WorkspaceParameters
	if err := dashParameters.FromDashScopeRealtimeWorkspaceParameters(apitypes.DashScopeRealtimeWorkspaceParameters{
		AgentType:   apitypes.DashScopeRealtimeWorkspaceParametersAgentTypeDashscopeRealtime,
		Temperature: &temperature,
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t)
	if err := srv.validateDashScopeRealtimeOverrides(
		context.Background(), nil, &dashParameters, false,
	); err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Fatalf("validateDashScopeRealtimeOverrides() error = %v", err)
	}

	sampleRate := apitypes.DoubaoRealtimeDuplexWorkspaceParametersSampleRate(16000)
	var duplexParameters apitypes.WorkspaceParameters
	if err := duplexParameters.FromDoubaoRealtimeDuplexWorkspaceParameters(apitypes.DoubaoRealtimeDuplexWorkspaceParameters{
		AgentType:  apitypes.DoubaoRealtimeDuplexWorkspaceParametersAgentTypeDoubaoRealtimeDuplex,
		SampleRate: &sampleRate,
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.validateDoubaoRealtimeDuplexOverrides(
		context.Background(), nil, &duplexParameters, false,
	); err == nil || !strings.Contains(err.Error(), "sample_rate") {
		t.Fatalf("validateDoubaoRealtimeDuplexOverrides() error = %v", err)
	}
}

func TestServerValidatesRuntimeASTTranslateAliases(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	store := testWorkflowStore(t, srv)
	if err := store.Set(context.Background(), workflowReferenceKey("ast-workflow"), []byte(`{"name":"ast-workflow","spec":{"driver":"ast-translate","ast_translate":{"translation_model":"translate-model","lang_pair":"auto"}}}`)); err != nil {
		t.Fatal(err)
	}
	seedModel(t, srv, "translation-resource", apitypes.ModelKindTranslation)
	seedModel(t, srv, "llm-resource", apitypes.ModelKindLlm)
	ctx := WithRuntimeVoiceBindings(
		WithRuntimeModelBindings(
			WithRuntimeWorkflowBindings(context.Background(), map[string]string{"translate": "ast-workflow"}),
			map[string]string{"translate-model": "translation-resource", "wrong-model": "llm-resource"},
		),
		map[string]string{"translator": "voice-resource"},
	)
	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{name: "valid", body: `{"name":"ast-valid","workflow_id":"ast-workflow","parameters":{"agent_type":"ast-translate","translation_model":"translate-model","voice":{"tts_voice":"translator"}}}`, ok: true},
		{name: "missing model", body: `{"name":"ast-missing-model","workflow_id":"ast-workflow","parameters":{"agent_type":"ast-translate","translation_model":"missing"}}`, want: `missing runtime Model alias "missing"`},
		{name: "wrong model kind", body: `{"name":"ast-wrong-model","workflow_id":"ast-workflow","parameters":{"agent_type":"ast-translate","translation_model":"wrong-model"}}`, want: `has kind "llm", want "translation"`},
		{name: "missing voice", body: `{"name":"ast-missing-voice","workflow_id":"ast-workflow","parameters":{"agent_type":"ast-translate","voice":{"tts_voice":"missing"}}}`, want: `missing runtime Voice alias "missing"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := mustWorkspaceUpsert(t, test.body)
			response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if test.ok {
				if _, ok := response.(adminhttp.CreateWorkspace200JSONResponse); !ok {
					t.Fatalf("CreateWorkspace() = %#v, want 200", response)
				}
				return
			}
			invalid, ok := response.(adminhttp.CreateWorkspace400JSONResponse)
			if !ok || !strings.Contains(invalid.Error.Message, test.want) {
				t.Fatalf("CreateWorkspace() = %#v, want %q", response, test.want)
			}
			if strings.Contains(invalid.Error.Message, "translation-resource") || strings.Contains(invalid.Error.Message, "llm-resource") {
				t.Fatalf("CreateWorkspace() exposes canonical Model ID: %q", invalid.Error.Message)
			}
		})
	}
}

func TestServerValidatesNewRealtimeWorkspaceOverrides(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	store := testWorkflowStore(t, srv)
	for name, body := range map[string]string{
		"dash-workflow":   `{"name":"dash-workflow","spec":{"driver":"dashscope-realtime","dashscope_realtime":{"model":"dash-default","voice":"dash-default-voice"}}}`,
		"duplex-workflow": `{"name":"duplex-workflow","spec":{"driver":"doubao-realtime-duplex","doubao_realtime_duplex":{"model":"duplex-default","voice":"duplex-default-voice"}}}`,
	} {
		if err := store.Set(t.Context(), workflowReferenceKey(name), []byte(body)); err != nil {
			t.Fatalf("seed workflow %q: %v", name, err)
		}
	}
	seedProviderModel(t, srv, "dash-resource", apitypes.ModelKindRealtime, apitypes.ModelProviderKindDashscopeTenant, "dash-tenant")
	seedProviderModel(t, srv, "duplex-resource", apitypes.ModelKindRealtimeDuplex, apitypes.ModelProviderKindVolcTenant, "volc-tenant")
	seedProviderModel(t, srv, "wrong-kind-resource", apitypes.ModelKindLlm, apitypes.ModelProviderKindDashscopeTenant, "dash-tenant")
	seedProviderVoice(t, srv, "dash-voice-resource", apitypes.VoiceProviderKindDashscopeTenant, "dash-tenant")
	seedProviderVoice(t, srv, "duplex-voice-resource", apitypes.VoiceProviderKindVolcTenant, "volc-tenant")
	seedProviderVoice(t, srv, "wrong-voice-resource", apitypes.VoiceProviderKindVolcTenant, "other-tenant")

	baseContext := WithRuntimeVoiceBindings(
		WithRuntimeModelBindings(
			WithRuntimeWorkflowBindings(t.Context(), map[string]string{
				"dash": "dash-workflow", "duplex": "duplex-workflow",
			}),
			map[string]string{
				"dash-default":      "dash-resource",
				"dash-override":     "dash-resource",
				"duplex-default":    "duplex-resource",
				"duplex-override":   "duplex-resource",
				"wrong-kind":        "wrong-kind-resource",
				"wrong-kind-duplex": "wrong-kind-resource",
			},
		),
		map[string]string{
			"dash-default-voice":    "dash-voice-resource",
			"dash-override-voice":   "dash-voice-resource",
			"duplex-default-voice":  "duplex-voice-resource",
			"duplex-override-voice": "duplex-voice-resource",
			"wrong-voice":           "wrong-voice-resource",
		},
	)
	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{
			name: "dash valid",
			body: `{"name":"dash-valid","workflow_id":"dash-workflow","parameters":{"agent_type":"dashscope-realtime","model":"dash-override","voice":"dash-override-voice"}}`,
			ok:   true,
		},
		{
			name: "dash wrong parameter variant",
			body: `{"name":"dash-wrong-variant","workflow_id":"dash-workflow","parameters":{"agent_type":"eino"}}`,
			want: "dashscope_realtime parameters are required",
		},
		{
			name: "dash missing model",
			body: `{"name":"dash-missing-model","workflow_id":"dash-workflow","parameters":{"agent_type":"dashscope-realtime","model":"missing"}}`,
			want: `missing runtime Model alias "missing"`,
		},
		{
			name: "dash wrong model kind",
			body: `{"name":"dash-wrong-kind","workflow_id":"dash-workflow","parameters":{"agent_type":"dashscope-realtime","model":"wrong-kind"}}`,
			want: `has kind "llm", want "realtime"`,
		},
		{
			name: "dash missing voice",
			body: `{"name":"dash-missing-voice","workflow_id":"dash-workflow","parameters":{"agent_type":"dashscope-realtime","voice":"missing"}}`,
			want: `missing runtime Voice alias "missing"`,
		},
		{
			name: "dash incompatible voice",
			body: `{"name":"dash-wrong-voice","workflow_id":"dash-workflow","parameters":{"agent_type":"dashscope-realtime","voice":"wrong-voice"}}`,
			want: "uses provider",
		},
		{
			name: "duplex valid",
			body: `{"name":"duplex-valid","workflow_id":"duplex-workflow","parameters":{"agent_type":"doubao-realtime-duplex","model":"duplex-override","voice":"duplex-override-voice"}}`,
			ok:   true,
		},
		{
			name: "duplex wrong parameter variant",
			body: `{"name":"duplex-wrong-variant","workflow_id":"duplex-workflow","parameters":{"agent_type":"eino"}}`,
			want: "doubao_realtime_duplex parameters are required",
		},
		{
			name: "duplex missing model",
			body: `{"name":"duplex-missing-model","workflow_id":"duplex-workflow","parameters":{"agent_type":"doubao-realtime-duplex","model":"missing"}}`,
			want: `missing runtime Model alias "missing"`,
		},
		{
			name: "duplex wrong model kind",
			body: `{"name":"duplex-wrong-kind","workflow_id":"duplex-workflow","parameters":{"agent_type":"doubao-realtime-duplex","model":"wrong-kind-duplex"}}`,
			want: `has kind "llm", want "realtime-duplex"`,
		},
		{
			name: "duplex missing voice",
			body: `{"name":"duplex-missing-voice","workflow_id":"duplex-workflow","parameters":{"agent_type":"doubao-realtime-duplex","voice":"missing"}}`,
			want: `missing runtime Voice alias "missing"`,
		},
		{
			name: "duplex incompatible voice",
			body: `{"name":"duplex-wrong-voice","workflow_id":"duplex-workflow","parameters":{"agent_type":"doubao-realtime-duplex","voice":"wrong-voice"}}`,
			want: "uses provider",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := mustWorkspaceUpsert(t, test.body)
			response, err := srv.CreateWorkspace(baseContext, adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if test.ok {
				if _, ok := response.(adminhttp.CreateWorkspace200JSONResponse); !ok {
					t.Fatalf("CreateWorkspace() = %#v, want 200", response)
				}
				return
			}
			invalid, ok := response.(adminhttp.CreateWorkspace400JSONResponse)
			if !ok || !strings.Contains(invalid.Error.Message, test.want) {
				t.Fatalf("CreateWorkspace() = %#v, want %q", response, test.want)
			}
			for _, hidden := range []string{
				"dash-resource", "duplex-resource", "wrong-kind-resource",
				"dash-voice-resource", "duplex-voice-resource", "wrong-voice-resource",
			} {
				if strings.Contains(invalid.Error.Message, hidden) {
					t.Fatalf("CreateWorkspace() exposes canonical resource ID %q: %q", hidden, invalid.Error.Message)
				}
			}
		})
	}
}

func TestServerRejectsWrongEinoWorkspaceParameterVariant(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	store := testWorkflowStore(t, srv)
	if err := store.Set(t.Context(), workflowReferenceKey("eino-workflow"), []byte(
		`{"name":"eino-workflow","spec":{"driver":"eino","eino":{"graph":{"name":"eino","compile":{"node_trigger_mode":"any_predecessor"},"state":{"fields":[]},"nodes":[],"edges":[],"branches":[],"outputs":[]}}}}`,
	)); err != nil {
		t.Fatal(err)
	}
	ctx := WithRuntimeWorkflowBindings(t.Context(), map[string]string{"eino": "eino-workflow"})
	body := mustWorkspaceUpsert(t, `{"name":"eino-wrong-variant","workflow_id":"eino-workflow","parameters":{"agent_type":"dashscope-realtime"}}`)
	response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	invalid, ok := response.(adminhttp.CreateWorkspace400JSONResponse)
	if !ok || !strings.Contains(invalid.Error.Message, "eino parameters are required") {
		t.Fatalf("CreateWorkspace() = %#v", response)
	}
}

func TestServerValidatesEinoWorkspaceInputMode(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	store := testWorkflowStore(t, srv)
	if err := store.Set(t.Context(), workflowReferenceKey("eino-workflow"), []byte(
		`{"name":"eino-workflow","spec":{"driver":"eino","eino":{"graph":{"name":"eino","compile":{"node_trigger_mode":"any_predecessor"},"state":{"fields":[]},"nodes":[],"edges":[],"branches":[],"outputs":[]}}}}`,
	)); err != nil {
		t.Fatal(err)
	}
	ctx := WithRuntimeWorkflowBindings(t.Context(), map[string]string{"eino": "eino-workflow"})
	for _, testCase := range []struct {
		name       string
		parameters string
		valid      bool
	}{
		{name: "omitted", valid: true},
		{name: "push-to-talk", parameters: `,"parameters":{"agent_type":"eino","input":"push-to-talk"}`, valid: true},
		{name: "realtime", parameters: `,"parameters":{"agent_type":"eino","input":"realtime"}`, valid: true},
		{name: "invalid", parameters: `,"parameters":{"agent_type":"eino","input":"invalid"}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := mustWorkspaceUpsert(t, fmt.Sprintf(`{"name":"eino-%s","workflow_id":"eino-workflow"%s}`, testCase.name, testCase.parameters))
			response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if testCase.valid {
				if _, ok := response.(adminhttp.CreateWorkspace200JSONResponse); !ok {
					t.Fatalf("CreateWorkspace() = %#v, want 200", response)
				}
				return
			}
			invalid, ok := response.(adminhttp.CreateWorkspace400JSONResponse)
			if !ok || !strings.Contains(invalid.Error.Message, "input \"invalid\" is unsupported") {
				t.Fatalf("CreateWorkspace() = %#v, want invalid input rejection", response)
			}
		})
	}
}

func TestServerStoreHelpers(t *testing.T) {
	t.Parallel()

	var nilServer *Server
	if _, err := nilServer.store(); err == nil {
		t.Fatal("nil server store() error = nil")
	}
	if _, err := nilServer.getWorkflow(t.Context(), "missing"); err == nil {
		t.Fatal("nil server getWorkflow() error = nil")
	}
	if _, err := (&Server{}).getWorkflow(t.Context(), "missing"); err == nil {
		t.Fatal("empty server getWorkflow() error = nil")
	}

	base := kv.NewMemory(nil)
	srv := &Server{Store: base}
	if _, err := srv.getWorkflow(t.Context(), "missing"); err == nil {
		t.Fatal("getWorkflow missing service error = nil")
	}

	workflows := kv.NewMemory(nil)
	srv.Workflows = testWorkflowService{store: workflows}
	if _, err := srv.getWorkflow(t.Context(), "missing"); !isInvalidWorkspaceReference(err) {
		t.Fatalf("getWorkflow missing resource error = %v", err)
	}
}

type testWorkflowService struct {
	store kv.Store
}

func (s testWorkflowService) GetWorkflow(ctx context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	data, err := s.store.Get(ctx, workflowReferenceKey(request.Id))
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "workflow not found")), nil
	}
	if err != nil {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	var value apitypes.Workflow
	if err := json.Unmarshal(data, &value); err != nil {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetWorkflow200JSONResponse(value), nil
}

func testWorkflowStore(t *testing.T, srv *Server) kv.Store {
	t.Helper()
	service, ok := srv.Workflows.(testWorkflowService)
	if !ok {
		t.Fatalf("Workflows = %T", srv.Workflows)
	}
	return service.store
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Server{
		Store:     kv.Prefixed(store, kv.Key{"workspaces"}),
		Workflows: testWorkflowService{store: kv.Prefixed(store, kv.Key{"workflows"})},
		Models:    &model.Server{Store: kv.Prefixed(store, kv.Key{"models"})},
		Voices:    &voice.Server{Store: kv.Prefixed(store, kv.Key{"voices"})},
	}
}

func seedWorkflow(t *testing.T, srv *Server, name string) {
	t.Helper()

	store := testWorkflowStore(t, srv)
	if err := store.Set(context.Background(), workflowReferenceKey(name), []byte(`{}`)); err != nil {
		t.Fatalf("seed workflow %q: %v", name, err)
	}
}

func seedFlowcraftWorkflow(t *testing.T, srv *Server, name, generateModel string) {
	t.Helper()

	store := testWorkflowStore(t, srv)
	body := fmt.Appendf(nil, `{"name":%q,"spec":{"driver":"flowcraft","flowcraft":{"graph":{"name":"Assistant","entry":"answer","nodes":[{"id":"answer","type":"llm","publish":true,"config":{"model":%q}}]}}}}`, name, generateModel)
	if err := store.Set(context.Background(), workflowReferenceKey(name), body); err != nil {
		t.Fatalf("seed flowcraft workflow %q: %v", name, err)
	}
}

func seedModel(t *testing.T, srv *Server, id string, kind apitypes.ModelKind) {
	t.Helper()

	modelServer, ok := srv.Models.(*model.Server)
	if !ok {
		t.Fatalf("Models = %T", srv.Models)
	}
	data, err := json.Marshal(apitypes.Model{Id: id, Kind: kind})
	if err != nil {
		t.Fatalf("json.Marshal(model) error = %v", err)
	}
	if err := modelServer.Store.Set(context.Background(), kv.Key{"by-id", id}, data); err != nil {
		t.Fatalf("seed model %q: %v", id, err)
	}
}

func seedProviderModel(
	t *testing.T,
	srv *Server,
	id string,
	kind apitypes.ModelKind,
	providerKind apitypes.ModelProviderKind,
	providerName string,
) {
	t.Helper()
	var providerData apitypes.ModelProviderData
	switch providerKind {
	case apitypes.ModelProviderKindDashscopeTenant:
		mode := apitypes.DashScopeTenantModelProviderDataApiModeRealtime
		if err := providerData.FromDashScopeTenantModelProviderData(
			apitypes.DashScopeTenantModelProviderData{ApiMode: &mode},
		); err != nil {
			t.Fatal(err)
		}
	case apitypes.ModelProviderKindVolcTenant:
		if err := providerData.FromVolcTenantModelProviderData(
			apitypes.VolcTenantModelProviderData{
				ApiMode: apitypes.VolcTenantModelProviderDataApiModeRealtimeDuplex,
			},
		); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported test Model provider %q", providerKind)
	}
	modelServer, ok := srv.Models.(*model.Server)
	if !ok {
		t.Fatalf("Models = %T", srv.Models)
	}
	data, err := json.Marshal(apitypes.Model{
		Id: id, Kind: kind,
		Provider:     apitypes.ModelProvider{Kind: providerKind, Id: providerName},
		ProviderData: providerData,
		Source:       apitypes.ModelSourceManual,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := modelServer.Store.Set(t.Context(), kv.Key{"by-id", id}, data); err != nil {
		t.Fatal(err)
	}
}

func seedProviderVoice(
	t *testing.T,
	srv *Server,
	id string,
	providerKind apitypes.VoiceProviderKind,
	providerName string,
) {
	t.Helper()
	voiceServer, ok := srv.Voices.(*voice.Server)
	if !ok {
		t.Fatalf("Voices = %T", srv.Voices)
	}
	if err := voice.Write(t.Context(), voiceServer.Store, apitypes.Voice{
		Id: id,
		Provider: apitypes.VoiceProvider{
			Kind: providerKind,
			Id:   providerName,
		},
		Source: apitypes.VoiceSourceManual,
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func mustWorkspaceUpsert(t *testing.T, raw string) adminhttp.WorkspaceUpsert {
	t.Helper()

	var upsert adminhttp.WorkspaceUpsert
	if err := json.Unmarshal([]byte(raw), &upsert); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if upsert.Id == "" {
		upsert.Id = upsert.Name + "-id"
	}
	return upsert
}

type recordingRuntimeStore struct {
	prepared []string
	deleted  []string
}

type recordingWorkspaceDeletionFencer struct {
	workspaceIDs    []string
	callbackInvoked bool
}

func (f *recordingWorkspaceDeletionFencer) WithWorkspaceDeletionFence(
	ctx context.Context,
	workspaceID string,
	createMarker func(context.Context) error,
) error {
	f.workspaceIDs = append(f.workspaceIDs, workspaceID)
	if err := createMarker(ctx); err != nil {
		return err
	}
	f.callbackInvoked = true
	return nil
}

type failingCreateIfAbsentStore struct {
	kv.Store
	err error
}

func (s failingCreateIfAbsentStore) CreateIfAbsent(context.Context, kv.Entry, []kv.Entry) ([]byte, bool, error) {
	return nil, false, s.err
}

func (s failingCreateIfAbsentStore) CreateIfAllAbsent(context.Context, []kv.Entry, []kv.Entry) (kv.Key, []byte, bool, error) {
	return nil, nil, false, s.err
}

func (s *recordingRuntimeStore) PrepareWorkspace(_ context.Context, workspace string) (Runtime, error) {
	s.prepared = append(s.prepared, workspace)
	return Runtime{ObjectPrefix: ObjectPrefix(workspace), LocalDir: "/tmp/" + workspace}, nil
}

func (s *recordingRuntimeStore) GetWorkspaceRuntime(_ context.Context, workspace string) (Runtime, error) {
	return Runtime{ObjectPrefix: ObjectPrefix(workspace), LocalDir: "/tmp/" + workspace}, nil
}

func (s *recordingRuntimeStore) DeleteWorkspaceRuntime(_ context.Context, workspace string) error {
	s.deleted = append(s.deleted, workspace)
	return nil
}
