package workspace_test

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"

	_ "modernc.org/sqlite"
)

type sfuWorkflowService struct{}

func (sfuWorkflowService) GetWorkflow(_ context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	return adminhttp.GetWorkflow200JSONResponse{Id: request.Id}, nil
}

// TestRetireSystemWorkspaceDoesNotTakeRewardFenceOnSharedSQLite verifies Social
// retirement when Workspace and gameplay share a single SQL connection.
func TestRetireSystemWorkspaceDoesNotTakeRewardFenceOnSharedSQLite(t *testing.T) {
	db, err := sqlx.Open("sqlite", "file:"+t.TempDir()+"/shared.sqlite")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := workspace.Initialize(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	fencer := &gameplay.Runtime{DB: db}
	if err := fencer.Migration(t.Context()); err != nil {
		t.Fatalf("gameplay Migration: %v", err)
	}
	server := &workspace.Server{DB: db, Workflows: sfuWorkflowService{}, DeletionFencer: fencer}
	owner := "peer-owner"
	item, created, err := server.CreateSystemWorkspace(ownership.WithOwner(t.Context(), owner), adminhttp.WorkspaceUpsert{
		Id: "ws-social", Name: "social-direct-1", WorkflowId: socialutil.SFUWorkflowID,
	})
	if err != nil || !created {
		t.Fatalf("CreateSystemWorkspace() = %#v, %v, %v", item, created, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := server.RetireSystemWorkspaceByID(ctx, item.Id, socialutil.SFUWorkspaceKindFriend, "relation-1")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RetireSystemWorkspaceByID: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RetireSystemWorkspaceByID blocked on the shared SQLite handle")
	}
	pending, err := workspace.NewPendingDeletionSource(db).HasLocator(t.Context(), pendingdeletion.Locator{Kind: pendingdeletion.KindWorkspace, ResourceID: item.Id})
	if err != nil || !pending {
		t.Fatalf("pending deletion marker = %v, %v; want present", pending, err)
	}
	retired, err := server.GetRetiredSystemWorkspace(ownership.WithOwner(t.Context(), owner), item.Name, socialutil.SFUWorkspaceKindFriend, "relation-1")
	if err != nil || retired.Id != item.Id {
		t.Fatalf("GetRetiredSystemWorkspace() = %#v, %v", retired, err)
	}
}

func TestWorkspaceDeletionSharesRewardTransactionOnSingleConnection(t *testing.T) {
	server := workspacetest.New(t)
	fencer := &gameplay.Runtime{DB: server.DB}
	if err := fencer.Migration(t.Context()); err != nil {
		t.Fatal(err)
	}
	server.DeletionFencer = fencer
	now := time.Now().UTC()
	workspacetest.Seed(t, server, apitypes.Workspace{
		Id: "workspace-ordinary", Name: "ordinary", WorkflowId: "workflow",
		CreatedAt: now, UpdatedAt: now, LastActiveAt: now, System: new(false),
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	response, err := server.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: "workspace-ordinary"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() = %#v", response)
	}
	pending, err := workspace.NewPendingDeletionSource(server.DB).HasLocator(ctx, pendingdeletion.Locator{
		Kind: pendingdeletion.KindWorkspace, ResourceID: "workspace-ordinary",
	})
	if err != nil || !pending {
		t.Fatalf("pending deletion = %v, %v", pending, err)
	}
}
