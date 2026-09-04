package workspace

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPendingDeletionSourceOwnsOnlyWorkspaces(t *testing.T) {
	source := NewPendingDeletionSource(kv.NewMemory(nil))
	if err := source.Validate(); err != nil {
		t.Fatal(err)
	}
	if source.Name() != pendingDeletionSourceName {
		t.Fatalf("Name() = %q", source.Name())
	}
	kinds := source.Kinds()
	if len(kinds) != 1 || kinds[0] != pendingdeletion.KindWorkspace {
		t.Fatalf("Kinds() = %#v", kinds)
	}
}

func TestListWorkspacesByOwnerSkipsPendingDeletion(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-1")
	ctx := ownership.WithOwner(context.Background(), "peer-a")
	labels := map[string]string{"collection": "raids"}
	for _, fixture := range []struct{ id, name string }{
		{id: "workspace-01", name: "alpha001"},
		{id: "workspace-02", name: "beta0001"},
	} {
		body := adminhttp.WorkspaceUpsert{
			Id: fixture.id, Name: fixture.name, WorkflowId: "workflow-1", Labels: &labels,
		}
		if _, err := createWorkspaceForTest(srv, ctx, createWorkspaceRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateWorkspace(%q) error = %v", fixture.name, err)
		}
	}

	deleteResponse, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: "workspace-01"})
	if err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, ok := deleteResponse.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() response = %#v", deleteResponse)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, srv.Store, pendingdeletion.KindWorkspace, "workspace-01"); err != nil || !pending {
		t.Fatalf("workspace pending deletion = %v, error = %v", pending, err)
	}

	owned, err := srv.ListWorkspacesByOwner(ctx, "peer-a")
	if err != nil {
		t.Fatalf("ListWorkspacesByOwner() error = %v", err)
	}
	if len(owned) != 1 || owned[0].Name != "beta0001" {
		t.Fatalf("ListWorkspacesByOwner() = %#v, want only beta0001", owned)
	}
	labeled, err := srv.ListWorkspacesByOwnerAndLabels(ctx, "peer-a", labels)
	if err != nil {
		t.Fatalf("ListWorkspacesByOwnerAndLabels() error = %v", err)
	}
	if len(labeled) != 1 || labeled[0].Name != "beta0001" {
		t.Fatalf("ListWorkspacesByOwnerAndLabels() = %#v, want only beta0001", labeled)
	}
}
