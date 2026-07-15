package agent

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

type Resolver interface {
	Resolve(context.Context, string) (Spec, error)
}

type ServiceResolver struct {
	Workspaces workspace.WorkspaceAdminService
	Workflows  workflow.WorkflowAdminService
}

func (r ServiceResolver) Resolve(ctx context.Context, pattern string) (Spec, error) {
	workspaceName, err := ParseWorkspacePattern(pattern)
	if err != nil {
		return Spec{}, err
	}
	if r.Workspaces == nil {
		return Spec{}, fmt.Errorf("agent: workspace service is required")
	}
	if r.Workflows == nil {
		return Spec{}, fmt.Errorf("agent: workflow service is required")
	}

	workspace, err := r.getWorkspace(ctx, workspaceName)
	if err != nil {
		return Spec{}, err
	}
	workflow, err := r.getWorkflow(ctx, string(workspace.WorkflowName))
	if err != nil {
		return Spec{}, err
	}
	workflowType, err := resolveWorkflowType(workflow)
	if err != nil {
		return Spec{}, err
	}
	return Spec{
		Workspace:    workspace,
		Workflow:     workflow,
		WorkflowType: workflowType,
	}, nil
}

func ParseWorkspacePattern(pattern string) (string, error) {
	pattern = strings.Trim(strings.TrimSpace(pattern), "/")
	if pattern == "" {
		return "", fmt.Errorf("agent: workspace pattern is required")
	}
	if pattern == "workspaces" {
		return "", fmt.Errorf("agent: workspace pattern is required")
	}
	if strings.HasPrefix(pattern, "workspaces/") {
		pattern = strings.TrimPrefix(pattern, "workspaces/")
	}
	if strings.Contains(pattern, "/") {
		return "", fmt.Errorf("agent: workspace pattern %q must identify one workspace", pattern)
	}
	name, err := url.PathUnescape(pattern)
	if err != nil {
		return "", fmt.Errorf("agent: invalid workspace pattern %q: %w", pattern, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("agent: workspace pattern is required")
	}
	return name, nil
}

func (r ServiceResolver) getWorkspace(ctx context.Context, name string) (apitypes.Workspace, error) {
	response, err := r.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: string(name)})
	if err != nil {
		return apitypes.Workspace{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkspace200JSONResponse:
		return apitypes.Workspace(response), nil
	case adminhttp.GetWorkspace404JSONResponse:
		return apitypes.Workspace{}, fmt.Errorf("agent: workspace %q not found", name)
	case adminhttp.GetWorkspace500JSONResponse:
		return apitypes.Workspace{}, fmt.Errorf("agent: get workspace %q failed: %s", name, response.Error.Message)
	default:
		return apitypes.Workspace{}, fmt.Errorf("agent: unexpected GetWorkspace response %T", response)
	}
}

func (r ServiceResolver) getWorkflow(ctx context.Context, name string) (apitypes.Workflow, error) {
	response, err := r.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: string(name)})
	if err != nil {
		return apitypes.Workflow{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		return apitypes.Workflow(response), nil
	case adminhttp.GetWorkflow404JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("agent: workflow %q not found", name)
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("agent: get workflow %q failed: %s", name, response.Error.Message)
	default:
		return apitypes.Workflow{}, fmt.Errorf("agent: unexpected GetWorkflow response %T", response)
	}
}
