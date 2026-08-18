//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestAdminAPIWorkspacesListGetPaginationAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)
	peer := env.h.ConnectClientFromContext("admin-api-peer")
	defer peer.Close()
	registerAdminHistoryPeers(t, env, peer)

	names := []string{mutationName("workspace-a"), mutationName("workspace-b")}
	created := make([]*rpcapi.Workspace, 0, len(names))
	for _, name := range names {
		workspace, err := peer.CreateWorkspace(env.ctx, "admin.workspace.create."+name, rpcapi.WorkspaceCreateRequest{
			Name:         name,
			Collection:   "social",
			WorkflowName: "direct",
		})
		if err != nil {
			t.Fatalf("peer create workspace %q: %v", name, err)
		}
		created = append(created, workspace)
	}
	t.Cleanup(func() {
		for _, workspace := range created {
			_, _ = peer.DeleteWorkspace(env.ctx, "admin.workspace.cleanup."+workspace.Name, rpcapi.WorkspaceDeleteRequest{Name: workspace.Name})
		}
	})

	all := collectAdminPages(t, 1, func(cursor *string, limit int32) ([]apitypes.Workspace, bool, *string) {
		resp, err := env.api.ListWorkspacesWithResponse(env.ctx, &adminhttp.ListWorkspacesParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list workspaces: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list workspaces missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	seed := requireName(t, all, names[0], func(item apitypes.Workspace) string { return item.Name })
	second := requireName(t, all, names[1], func(item apitypes.Workspace) string { return item.Name })

	get, err := env.api.GetWorkspaceWithResponse(env.ctx, seed.Id)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != seed.Id || get.JSON200.Name != seed.Name || get.JSON200.WorkflowId == "" {
		t.Fatalf("get workspace = %#v", get.JSON200)
	}

	updated, err := env.api.PutWorkspaceWithResponse(env.ctx, seed.Id, adminhttp.WorkspaceUpsert{
		Id:         seed.Id,
		Name:       seed.Name,
		WorkflowId: seed.WorkflowId,
		Parameters: seed.Parameters,
	})
	if err != nil {
		t.Fatalf("update workspace: %v", err)
	}
	requireStatusOK(t, updated, updated.Body)
	if updated.JSON200 == nil || updated.JSON200.Id != seed.Id || updated.JSON200.Name != seed.Name {
		t.Fatalf("updated workspace = %#v", updated.JSON200)
	}
	deleted, err := env.api.DeleteWorkspaceWithResponse(env.ctx, second.Id)
	if err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
