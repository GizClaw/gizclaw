//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIWorkspacesListGetPaginationAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 25, func(cursor *string, limit int32) ([]apitypes.Workspace, bool, *string) {
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
	seed := requireName(t, all, "support-desk-workspace", func(item apitypes.Workspace) string { return item.Name })
	requirePrefixCount(t, all, "workspace-scenario-", 100, func(item apitypes.Workspace) string { return item.Name })

	get, err := env.api.GetWorkspaceWithResponse(env.ctx, seed.Id)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != seed.Id || get.JSON200.Name != seed.Name || get.JSON200.WorkflowId == "" {
		t.Fatalf("get workspace = %#v", get.JSON200)
	}

	name := mutationName("workspace")
	id := mutationName("workspace-id")
	created, err := env.api.CreateWorkspaceWithResponse(env.ctx, adminhttp.WorkspaceUpsert{
		Id:         id,
		Name:       name,
		WorkflowId: seed.WorkflowId,
		Parameters: flowcraftWorkspaceParameters(t, apitypes.WorkspaceInputModePushToTalk),
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Id != id || created.JSON200.Name != name {
		t.Fatalf("created workspace = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteWorkspaceWithResponse(env.ctx, created.JSON200.Id)
	if err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
