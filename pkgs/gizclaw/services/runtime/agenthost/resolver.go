package agenthost

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

type Resolver interface {
	Resolve(context.Context, string) (Spec, error)
}

type ServiceResolver struct {
	Workspaces    workspace.WorkspaceAdminService
	Workflows     workflow.WorkflowAdminService
	ToolBuilder   *toolkit.Builder
	ToolExecutors *toolkit.ExecutorRegistry
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
	workflowName, err := resolveWorkspaceWorkflowName(ctx, ws)
	if err != nil {
		return Spec{}, err
	}
	workflow, err := r.getWorkflow(ctx, workflowName)
	if err != nil {
		return Spec{}, err
	}
	if ws.WorkflowSource != nil && *ws.WorkflowSource == apitypes.WorkspaceWorkflowSourceOwned {
		if ws.OwnerPublicKey == nil || workflow.OwnerPublicKey == nil || *ws.OwnerPublicKey != *workflow.OwnerPublicKey {
			return Spec{}, fmt.Errorf("agenthost: owned workflow %q is not owned by workspace owner", ws.WorkflowName)
		}
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
	tools, err := r.resolveToolkit(ctx, ws, workflow)
	if err != nil {
		return Spec{}, err
	}
	ownerPublicKey := ""
	if access, ok := resourceAccessFromContext(ctx); ok {
		ownerPublicKey = access.ownerPublicKey
	}
	return Spec{
		Workspace:      ws,
		Workflow:       workflow,
		AgentType:      agentType,
		OwnerPublicKey: ownerPublicKey,
		Runtime:        runtime,
		Toolkit:        tools,
	}, nil
}

func resolveWorkspaceWorkflowName(ctx context.Context, ws apitypes.Workspace) (string, error) {
	if ws.WorkflowSource == nil {
		if ws.System == nil || !*ws.System {
			return "", fmt.Errorf("agenthost: direct workflow reference requires a system workspace")
		}
		return string(ws.WorkflowName), nil
	}
	switch *ws.WorkflowSource {
	case apitypes.WorkspaceWorkflowSourceRuntime:
		access, ok := resourceAccessFromContext(ctx)
		if !ok {
			return "", fmt.Errorf("agenthost: resource access context is required for runtime workflow %q", ws.WorkflowName)
		}
		name := strings.TrimSpace(access.profileWorkflowBindings[string(ws.WorkflowName)])
		if name == "" {
			return "", fmt.Errorf("agenthost: runtime workflow alias %q not found", ws.WorkflowName)
		}
		return name, nil
	case apitypes.WorkspaceWorkflowSourceOwned:
		return string(ws.WorkflowName), nil
	default:
		return "", fmt.Errorf("agenthost: unsupported workflow source %q", *ws.WorkflowSource)
	}
}

func (r ServiceResolver) resolveToolkit(ctx context.Context, ws apitypes.Workspace, workflow apitypes.Workflow) (*ToolkitContext, error) {
	if ws.Toolkit == nil && workflow.Spec.Toolkit == nil {
		return nil, nil
	}
	if r.ToolBuilder == nil || r.ToolExecutors == nil {
		return nil, fmt.Errorf("agenthost: toolkit services are required")
	}
	access, ok := resourceAccessFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("agenthost: resource access context is required for toolkit")
	}
	workflowIDs, workflowRestrict, err := policyToolIDs(workflow.Spec.Toolkit)
	if err != nil {
		return nil, fmt.Errorf("agenthost: workflow toolkit policy: %w", err)
	}
	workspaceIDs, workspaceRestrict, err := policyToolIDs(ws.Toolkit)
	if err != nil {
		return nil, fmt.Errorf("agenthost: workspace toolkit policy: %w", err)
	}
	restrict := workflowRestrict || workspaceRestrict
	ids := workflowIDs
	switch {
	case workflowRestrict && workspaceRestrict:
		ids = intersectToolIDs(workflowIDs, workspaceIDs)
	case workspaceRestrict:
		ids = workspaceIDs
	}
	return &ToolkitContext{
		Builder:   r.ToolBuilder,
		Executors: r.ToolExecutors,
		BuildRequest: toolkit.BuildRequest{
			OwnerPublicKey:  access.ownerPublicKey,
			ProfileToolIDs:  append([]string(nil), access.profileToolIDs...),
			AllowedToolIDs:  ids,
			RestrictToolIDs: restrict,
		},
	}, nil
}

func policyToolIDs(policy *apitypes.ToolkitPolicy) ([]string, bool, error) {
	if policy == nil || policy.ToolIds == nil {
		return nil, false, nil
	}
	normalized, err := toolkit.NormalizePolicy(policy)
	if err != nil {
		return nil, false, err
	}
	return append([]string(nil), (*normalized.ToolIds)...), true, nil
}

func intersectToolIDs(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return []string{}
	}
	rightSet := make(map[string]bool, len(right))
	for _, id := range right {
		rightSet[id] = true
	}
	out := make([]string, 0, min(len(left), len(right)))
	for _, id := range left {
		if rightSet[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
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

func (r ServiceResolver) getWorkflow(ctx context.Context, name string) (apitypes.Workflow, error) {
	response, err := r.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Name: string(name)})
	if err != nil {
		return apitypes.Workflow{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		return apitypes.Workflow(response), nil
	case adminhttp.GetWorkflow404JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("agenthost: workflow %q not found", name)
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("agenthost: get workflow %q failed: %s", name, response.Error.Message)
	default:
		return apitypes.Workflow{}, fmt.Errorf("agenthost: unexpected GetWorkflow response %T", response)
	}
}
