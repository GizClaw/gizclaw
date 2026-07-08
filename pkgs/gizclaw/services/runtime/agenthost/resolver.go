package agenthost

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

type workspaceRuntimeProvider interface {
	GetWorkspaceRuntime(context.Context, string) (workspace.Runtime, error)
}

func (r ServiceResolver) Resolve(ctx context.Context, pattern string) (Spec, error) {
	workspaceName, err := ParseWorkspacePattern(pattern)
	if err != nil {
		return Spec{}, err
	}
	if r.Workspaces == nil {
		return Spec{}, fmt.Errorf("agenthost: workspace service is required")
	}
	if r.Workflows == nil {
		return Spec{}, fmt.Errorf("agenthost: workflow service is required")
	}

	ws, err := r.getWorkspace(ctx, workspaceName)
	if err != nil {
		return Spec{}, err
	}
	workflow, err := r.getWorkflow(ctx, string(ws.WorkflowName))
	if err != nil {
		return Spec{}, err
	}
	agentType, err := resolveAgentType(ws, workflow)
	if err != nil {
		return Spec{}, err
	}
	var runtime workspace.Runtime
	if provider, ok := r.Workspaces.(workspaceRuntimeProvider); ok {
		runtime, err = provider.GetWorkspaceRuntime(ctx, string(ws.Name))
		if err != nil {
			return Spec{}, err
		}
	}
	return Spec{
		Workspace: ws,
		Workflow:  workflow,
		AgentType: agentType,
		Runtime:   runtime,
	}, nil
}

func ParseWorkspacePattern(pattern string) (string, error) {
	pattern = strings.Trim(strings.TrimSpace(pattern), "/")
	if pattern == "" {
		return "", fmt.Errorf("agenthost: workspace pattern is required")
	}
	if pattern == "workspaces" {
		return "", fmt.Errorf("agenthost: workspace pattern is required")
	}
	if strings.HasPrefix(pattern, "workspaces/") {
		pattern = strings.TrimPrefix(pattern, "workspaces/")
	}
	if strings.Contains(pattern, "/") {
		return "", fmt.Errorf("agenthost: workspace pattern %q must identify one workspace", pattern)
	}
	name, err := url.PathUnescape(pattern)
	if err != nil {
		return "", fmt.Errorf("agenthost: invalid workspace pattern %q: %w", pattern, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("agenthost: workspace pattern is required")
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
		return apitypes.Workspace{}, fmt.Errorf("agenthost: workspace %q not found", name)
	case adminhttp.GetWorkspace500JSONResponse:
		return apitypes.Workspace{}, fmt.Errorf("agenthost: get workspace %q failed: %s", name, response.Error.Message)
	default:
		return apitypes.Workspace{}, fmt.Errorf("agenthost: unexpected GetWorkspace response %T", response)
	}
}

func (r ServiceResolver) getWorkflow(ctx context.Context, name string) (apitypes.WorkflowDocument, error) {
	response, err := r.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: string(name)})
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		return apitypes.WorkflowDocument(response), nil
	case adminhttp.GetWorkflow404JSONResponse:
		return apitypes.WorkflowDocument{}, fmt.Errorf("agenthost: workflow %q not found", name)
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.WorkflowDocument{}, fmt.Errorf("agenthost: get workflow %q failed: %s", name, response.Error.Message)
	default:
		return apitypes.WorkflowDocument{}, fmt.Errorf("agenthost: unexpected GetWorkflow response %T", response)
	}
}
