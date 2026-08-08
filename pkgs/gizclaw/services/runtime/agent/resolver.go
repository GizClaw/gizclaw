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
	workflow, err := r.getWorkflow(ctx, workspace.WorkflowId)
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
	if after, ok := strings.CutPrefix(pattern, "workspaces/"); ok {
		pattern = after
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
	resolver, ok := r.Workspaces.(interface {
		GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error)
	})
	if !ok {
		return apitypes.Workspace{}, fmt.Errorf("agent: workspace service does not support Peer name lookup")
	}
	return resolver.GetWorkspaceByName(ctx, name)
}

func (r ServiceResolver) getWorkflow(ctx context.Context, id string) (apitypes.Workflow, error) {
	response, err := r.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: id})
	if err != nil {
		return apitypes.Workflow{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		return apitypes.Workflow(response), nil
	case adminhttp.GetWorkflow404JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("agent: workflow %q not found", id)
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("agent: get workflow %q failed: %s", id, response.Error.Message)
	default:
		return apitypes.Workflow{}, fmt.Errorf("agent: unexpected GetWorkflow response %T", response)
	}
}
