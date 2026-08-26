package workspace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestCreatePeerWorkspaceRequiresOwnerAndRollsBackInitializerFailure(t *testing.T) {
	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-1")
	runtimes := &recordingRuntimeStore{}
	srv.RuntimeStore = runtimes

	_, err := srv.CreatePeerWorkspace(t.Context(), PeerWorkspaceCreateRequest{Name: "unauthenticated", WorkflowID: "workflow-1"})
	var createErr *PeerWorkspaceCreateError
	if !errors.As(err, &createErr) || createErr.Kind != PeerWorkspaceCreateInvalid {
		t.Fatalf("CreatePeerWorkspace(no owner) error = %#v", err)
	}
	if len(runtimes.prepared) != 0 {
		t.Fatalf("unauthenticated create prepared runtimes: %#v", runtimes.prepared)
	}

	ctx := ownership.WithOwner(t.Context(), "peer-owner")
	_, err = srv.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{
		Name: "initializer-fails", WorkflowID: "workflow-1",
		Initialize: func(context.Context, Runtime) error { return errors.New("injected initializer failure") },
	})
	if !errors.As(err, &createErr) || createErr.Kind != PeerWorkspaceCreateInternal {
		t.Fatalf("CreatePeerWorkspace(initializer failure) error = %#v", err)
	}
	if len(runtimes.prepared) != 1 || len(runtimes.deleted) != 1 || runtimes.prepared[0] != runtimes.deleted[0] {
		t.Fatalf("runtime rollback prepared=%#v deleted=%#v", runtimes.prepared, runtimes.deleted)
	}
	if _, err := srv.GetWorkspaceByName(ctx, "initializer-fails"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("failed initializer Workspace lookup error = %v, want not found", err)
	}
}

func TestCreatePeerWorkspaceDoesNotBlockIndependentOwner(t *testing.T) {
	server := newTestServer(t)
	seedWorkflow(t, server, "workflow-1")
	server.RuntimeStore = concurrentRuntimeStore{}
	ownerA := ownership.WithOwner(t.Context(), "peer-a")
	ownerB := ownership.WithOwner(t.Context(), "peer-b")
	firstEntered := make(chan struct{})
	firstRelease := make(chan struct{})

	type createResult struct {
		workspace apitypes.Workspace
		err       error
	}
	firstDone := make(chan createResult, 1)
	go func() {
		workspace, err := server.createPeerWorkspace(ownerA, PeerWorkspaceCreateRequest{
			Name: "first-workspace", WorkflowID: "workflow-1",
			Initialize: func(context.Context, Runtime) error {
				close(firstEntered)
				<-firstRelease
				return nil
			},
		}, "workspace-a")
		firstDone <- createResult{workspace: workspace, err: err}
	}()
	<-firstEntered

	snapshotDone := make(chan struct {
		snapshot PeerRetirementSnapshot
		err      error
	}, 1)
	go func() {
		snapshot, err := server.SnapshotPeerWorkspaces(ownerA, "peer-a", nil)
		snapshotDone <- struct {
			snapshot PeerRetirementSnapshot
			err      error
		}{snapshot: snapshot, err: err}
	}()
	secondDone := make(chan createResult, 1)
	go func() {
		workspace, err := server.createPeerWorkspace(ownerB, PeerWorkspaceCreateRequest{
			Name: "second-workspace", WorkflowID: "workflow-1",
		}, "workspace-b")
		secondDone <- createResult{workspace: workspace, err: err}
	}()

	select {
	case got := <-secondDone:
		if got.err != nil || got.workspace.Id != "workspace-b" {
			t.Fatalf("independent create = %#v, %v", got.workspace, got.err)
		}
	case <-time.After(time.Second):
		close(firstRelease)
		t.Fatal("independent owner could not create a Workspace while first owner initializer was blocked")
	}
	select {
	case got := <-snapshotDone:
		close(firstRelease)
		t.Fatalf("same-owner retirement snapshot completed before admitted create: %#v", got)
	default:
	}

	close(firstRelease)
	first := <-firstDone
	if first.err != nil || first.workspace.Id != "workspace-a" {
		t.Fatalf("first create = %#v, %v", first.workspace, first.err)
	}
	snapshot := <-snapshotDone
	if snapshot.err != nil || len(snapshot.snapshot.Workspaces) != 1 || snapshot.snapshot.Workspaces[0].ID != "workspace-a" {
		t.Fatalf("retirement snapshot = %#v, %v", snapshot.snapshot, snapshot.err)
	}
}

type concurrentRuntimeStore struct{}

func (concurrentRuntimeStore) PrepareWorkspace(_ context.Context, workspace string) (Runtime, error) {
	return Runtime{ObjectPrefix: ObjectPrefix(workspace)}, nil
}

func (concurrentRuntimeStore) GetWorkspaceRuntime(_ context.Context, workspace string) (Runtime, error) {
	return Runtime{ObjectPrefix: ObjectPrefix(workspace)}, nil
}

func (concurrentRuntimeStore) DeleteWorkspaceRuntime(context.Context, string) error { return nil }

type createWorkspaceRequestObject struct {
	Body *adminhttp.WorkspaceUpsert
}

type createWorkspace200JSONResponse apitypes.Workspace
type createWorkspace400JSONResponse apitypes.ErrorResponse
type createWorkspace409JSONResponse apitypes.ErrorResponse
type createWorkspace500JSONResponse apitypes.ErrorResponse

func createWorkspaceForTest(s *Server, ctx context.Context, request createWorkspaceRequestObject) (any, error) {
	if request.Body == nil {
		return createWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	body := *request.Body
	store, err := s.store()
	if err != nil {
		return createWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	normalized, err := normalizeWorkspaceUpsert(body, "")
	if err != nil {
		return createWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	err = s.validateReferences(ctx, normalized, true)
	var created apitypes.Workspace
	if err == nil {
		created, err = s.createWorkspaceRecord(ctx, store, normalized, false, nil)
	}
	if err == nil {
		return createWorkspace200JSONResponse(created), nil
	}
	if isInvalidWorkspaceReference(err) {
		return createWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	if errors.Is(err, errWorkspaceIDExists) || errors.Is(err, errWorkspaceNameExists) {
		return createWorkspace409JSONResponse(apitypes.NewErrorResponse("WORKSPACE_ALREADY_EXISTS", err.Error())), nil
	}
	return createWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
}
