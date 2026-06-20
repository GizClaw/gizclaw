package adminworkspaces_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestAdminWorkspacesUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "508-admin-workspaces")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	seedWorkspace(t, h)

	list := h.RunCLI("admin", "workspaces", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"name":"workspace-a"`) {
		t.Fatalf("workspaces list missing created item:\n%s", list.Stdout)
	}

	get := h.RunCLI("admin", "workspaces", "get", "workspace-a", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"workflow_name":"demo-assistant"`) {
		t.Fatalf("workspaces get missing workflow name:\n%s", get.Stdout)
	}
}

func seedWorkspace(t *testing.T, h *clitest.Harness) {
	t.Helper()

	c := h.ConnectClientFromContext("admin-a")
	defer c.Close()
	api, err := c.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workflowResp, err := api.CreateWorkflowWithResponse(ctx, workflowDocument(t, "demo-assistant"))
	if err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if workflowResp.JSON200 == nil {
		t.Fatalf("seed workflow got status %d: %s", workflowResp.StatusCode(), strings.TrimSpace(string(workflowResp.Body)))
	}
	var params apitypes.WorkspaceParameters
	if err := params.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{
		AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		E2e:       ptr(true),
	}); err != nil {
		t.Fatalf("build workspace parameters: %v", err)
	}
	workspaceResp, err := api.CreateWorkspaceWithResponse(ctx, adminservice.WorkspaceUpsert{
		Name:         "workspace-a",
		WorkflowName: "demo-assistant",
		Parameters:   &params,
	})
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if workspaceResp.JSON200 == nil {
		t.Fatalf("seed workspace got status %d: %s", workspaceResp.StatusCode(), strings.TrimSpace(string(workspaceResp.Body)))
	}
}

func workflowDocument(t *testing.T, name string) apitypes.WorkflowDocument {
	t.Helper()

	return apitypes.WorkflowDocument{
		ApiVersion: apitypes.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
		Kind:       apitypes.FlowcraftWorkflowKindFlowcraftWorkflow,
		Metadata: apitypes.WorkflowMetadata{
			Name: name,
		},
		Spec: apitypes.FlowcraftWorkflowSpec{},
	}
}

func ptr[T any](v T) *T {
	return &v
}
