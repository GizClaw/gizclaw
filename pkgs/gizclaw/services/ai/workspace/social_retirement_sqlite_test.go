package workspace_test

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/jmoiron/sqlx"

	_ "modernc.org/sqlite"
)

type sfuWorkflowService struct{}

func (sfuWorkflowService) GetWorkflow(_ context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	return adminhttp.GetWorkflow200JSONResponse{Id: request.Id}, nil
}

// TestRetireSystemWorkspaceDoesNotTakeRewardFenceOnSharedSQLite runs the
// Workspace KV store and the gameplay reward fence on one single-connection
// SQLite handle, the deployment shape of a Server whose catalog stores share
// the gameplay database. Retiring a Social SFU Workspace must write its
// marker without the fence: the fence holds the only connection inside a
// transaction and a KV write through it would block until the context expires.
func TestRetireSystemWorkspaceDoesNotTakeRewardFenceOnSharedSQLite(t *testing.T) {
	db, err := sqlx.Open("sqlite", "file:"+t.TempDir()+"/shared.sqlite")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := kv.NewSQLWithDB(db, "workspaces", nil)
	if err != nil {
		t.Fatalf("NewSQLWithDB: %v", err)
	}
	fencer := &gameplay.Runtime{DB: db}
	if err := fencer.Migration(t.Context()); err != nil {
		t.Fatalf("gameplay Migration: %v", err)
	}
	server := &workspace.Server{Store: store, Workflows: sfuWorkflowService{}, DeletionFencer: fencer}
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
	pending, err := pendingdeletion.HasLocator(t.Context(), store, pendingdeletion.KindWorkspace, item.Id)
	if err != nil || !pending {
		t.Fatalf("pending deletion marker = %v, %v; want present", pending, err)
	}
	retired, err := server.GetRetiredSystemWorkspace(ownership.WithOwner(t.Context(), owner), item.Name, socialutil.SFUWorkspaceKindFriend, "relation-1")
	if err != nil || retired.Id != item.Id {
		t.Fatalf("GetRetiredSystemWorkspace() = %#v, %v", retired, err)
	}
}
