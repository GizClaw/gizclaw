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
	Workspaces             workspace.WorkspaceAdminService
	Workflows              workflow.WorkflowAdminService
	RuntimeProfileForOwner func(context.Context, string) (apitypes.RuntimeProfile, error)
	ToolBuilder            *toolkit.Builder
	ToolExecutors          *toolkit.ExecutorRegistry
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
	resolutionCtx, err := r.ownerRuntimeContext(ctx, ws)
	if err != nil {
		return Spec{}, err
	}
	workflowName, err := resolveWorkspaceWorkflowName(resolutionCtx, ws)
	if err != nil {
		return Spec{}, err
	}
	workflow, err := r.getWorkflow(ctx, workflowName)
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
	tools, err := r.resolveToolkit(resolutionCtx, ws, workflow)
	if err != nil {
		return Spec{}, err
	}
	return Spec{
		Workspace: ws,
		Workflow:  workflow,
		AgentType: agentType,
		Runtime:   runtime,
		Toolkit:   tools,
	}, nil
}

func (r ServiceResolver) ownerRuntimeContext(ctx context.Context, ws apitypes.Workspace) (context.Context, error) {
	if ws.OwnerPublicKey == nil || strings.TrimSpace(*ws.OwnerPublicKey) == "" || r.RuntimeProfileForOwner == nil {
		return ctx, nil
	}
	owner := strings.TrimSpace(*ws.OwnerPublicKey)
	profile, err := r.RuntimeProfileForOwner(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("agenthost: resolve workspace %q owner runtime profile: %w", ws.Name, err)
	}
	return WithResourceAccess(ctx, owner, runtimeProfileToolBindings(profile.Spec.Resources.Tools), runtimeProfileWorkflowBindings(profile)), nil
}

func resolveWorkspaceWorkflowName(ctx context.Context, ws apitypes.Workspace) (string, error) {
	if ws.OwnerPublicKey != nil && ws.Labels != nil && strings.TrimSpace((*ws.Labels)["collection"]) != "" {
		access, ok := resourceAccessFromContext(ctx)
		if !ok {
			return "", fmt.Errorf("agenthost: resource access context is required for runtime workflow %q", ws.WorkflowName)
		}
		name := strings.TrimSpace(access.profileWorkflowBindings[string(ws.WorkflowName)])
		if name == "" {
			return "", fmt.Errorf("agenthost: runtime workflow alias %q not found", ws.WorkflowName)
		}
		return name, nil
	}
	return string(ws.WorkflowName), nil
}

func (r ServiceResolver) resolveToolkit(ctx context.Context, ws apitypes.Workspace, workflow apitypes.Workflow) (*ToolkitContext, error) {
	workflowPolicies := workflowToolkitPolicies(workflow.Spec)
	if ws.Toolkit == nil && len(workflowPolicies) == 0 {
		return nil, nil
	}
	if r.ToolBuilder == nil || r.ToolExecutors == nil {
		return nil, fmt.Errorf("agenthost: toolkit services are required")
	}
	access, ok := resourceAccessFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("agenthost: resource access context is required for toolkit")
	}
	var workflowIDs []string
	workflowRestrict := false
	for _, policy := range workflowPolicies {
		ids, restrict, err := policyToolIDs(policy)
		if err != nil {
			return nil, fmt.Errorf("agenthost: workflow toolkit policy: %w", err)
		}
		if !restrict {
			continue
		}
		ids = resolveToolAliases(ids, access.profileToolBindings)
		if workflowRestrict {
			workflowIDs = intersectToolIDs(workflowIDs, ids)
		} else {
			workflowIDs = ids
			workflowRestrict = true
		}
	}
	workspaceIDs, workspaceRestrict, err := policyToolIDs(ws.Toolkit)
	if err != nil {
		return nil, fmt.Errorf("agenthost: workspace toolkit policy: %w", err)
	}
	workspaceIDs = resolveToolAliases(workspaceIDs, access.profileToolBindings)
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
			CallerPublicKey: access.ownerPublicKey,
			ProfileToolIDs:  append([]string(nil), access.profileToolIDs...),
			AllowedToolIDs:  ids,
			RestrictToolIDs: restrict,
		},
	}, nil
}

func workflowToolkitPolicies(spec apitypes.WorkflowSpec) []*apitypes.ToolkitPolicy {
	policies := make([]*apitypes.ToolkitPolicy, 0, 2)
	if spec.Toolkit != nil {
		policies = append(policies, spec.Toolkit)
	}
	if spec.Pet != nil && spec.Pet.Toolkit != nil {
		policies = append(policies, spec.Pet.Toolkit)
	}
	return policies
}

func resolveToolAliases(ids []string, bindings map[string]string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if resourceID := strings.TrimSpace(bindings[id]); resourceID != "" {
			out = append(out, resourceID)
			continue
		}
		out = append(out, id)
	}
	return out
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
	if workspaceName, ok := strings.CutPrefix(pattern, "workspaces/"); ok {
		pattern = workspaceName
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
