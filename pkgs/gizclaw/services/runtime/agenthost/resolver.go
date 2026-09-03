package agenthost

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/memorylayout"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
)

type Resolver interface {
	Resolve(context.Context, string) (Spec, error)
}

type ServiceResolver struct {
	Workspaces             workspace.WorkspaceAdminService
	Workflows              workflow.WorkflowAdminService
	MemoryLayouts          memorylayout.MemoryLayoutAdminService
	RuntimeProfileForOwner func(context.Context, string) (apitypes.RuntimeProfile, error)
	ToolBuilder            *toolkit.Builder
	ToolCredentials        toolCredentialResolver
	HTTPTools              giztools.HTTPExecutor
	ClientToolTimeout      time.Duration
}

type workspaceRuntimeProvider interface {
	GetWorkspaceRuntimeByID(context.Context, string) (workspace.Runtime, error)
}

type availableWorkspaceProvider interface {
	GetAvailableWorkspaceByID(context.Context, string) (apitypes.Workspace, error)
}

// ResolveMemory resolves only the Workspace, outer Workflow, and owner
// RuntimeProfile Memory binding. Background product services use this path
// without constructing an Agent toolkit or runtime.
func (r ServiceResolver) ResolveMemory(ctx context.Context, pattern string) (Spec, error) {
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
	return r.resolveWorkspaceMemory(ctx, ws)
}

// ResolveMemoryByID resolves a Workspace Memory binding through the canonical
// Admin identity. Background services must use this path so a mutable or
// owner-scoped Peer name never becomes a durable foreign key.
func (r ServiceResolver) ResolveMemoryByID(ctx context.Context, workspaceID string) (Spec, error) {
	if r.Workspaces == nil {
		return Spec{}, fmt.Errorf("agenthost: workspace service is required")
	}
	if r.Workflows == nil {
		return Spec{}, fmt.Errorf("agenthost: workflow service is required")
	}
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return Spec{}, fmt.Errorf("agenthost: invalid workspace id: %w", err)
	}
	ws, err := r.getAvailableWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return Spec{}, err
	}
	return r.resolveWorkspaceMemory(ctx, ws)
}

func (r ServiceResolver) resolveWorkspaceMemory(ctx context.Context, ws apitypes.Workspace) (Spec, error) {
	workflowName, err := resolveWorkspaceWorkflowName(ctx, ws)
	if err != nil {
		return Spec{}, err
	}
	resolvedWorkflow, err := r.getWorkflow(ctx, workflowName)
	if err != nil {
		return Spec{}, err
	}
	if workflowIsSFU(resolvedWorkflow) {
		return Spec{Workspace: ws, Workflow: resolvedWorkflow}, nil
	}
	resolutionCtx, err := r.ownerRuntimeContext(ctx, ws)
	if err != nil {
		return Spec{}, err
	}
	memoryName, memoryBinding, memoryLayout, err := r.resolveMemory(resolutionCtx, resolvedWorkflow)
	if err != nil {
		return Spec{}, err
	}
	var memoryProfileID, memoryProfileRevision string
	if memoryBinding != nil {
		profile := resolutionCtx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
		memoryProfileID = profile.Id
		memoryProfileRevision = profile.Revision
	}
	return Spec{
		Workspace: ws, Workflow: resolvedWorkflow,
		MemoryName: memoryName, MemoryBinding: memoryBinding, MemoryLayout: memoryLayout,
		MemoryProfileID: memoryProfileID, MemoryProfileRevision: memoryProfileRevision,
	}, nil
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
	return r.resolveWorkspace(ctx, ws)
}

// ResolveByID loads a Workspace through its canonical Admin identity. Peer
// runtimes use this only after their name-based access check has resolved the
// selected Workspace, so shared system Workspaces never depend on one
// participant's name index.
func (r ServiceResolver) ResolveByID(ctx context.Context, workspaceID string) (Spec, error) {
	if r.Workspaces == nil {
		return Spec{}, fmt.Errorf("agenthost: workspace service is required")
	}
	if r.Workflows == nil {
		return Spec{}, fmt.Errorf("agenthost: workflow service is required")
	}
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return Spec{}, fmt.Errorf("agenthost: invalid workspace id: %w", err)
	}
	ws, err := r.getWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return Spec{}, err
	}
	return r.resolveWorkspace(ctx, ws)
}

// resolveWorkspace builds the Spec for one Workspace. An SFU Workspace is an
// empty runtime entry: it binds the Peer to the Social Room through the shared
// Social KV and the local driver alone, so its Spec carries no owner
// RuntimeProfile, toolkit, or Memory and any Server can activate it without
// the owner's local catalog.
func (r ServiceResolver) resolveWorkspace(ctx context.Context, ws apitypes.Workspace) (Spec, error) {
	workflowName, err := resolveWorkspaceWorkflowName(ctx, ws)
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
	if workflowIsSFU(workflow) {
		return Spec{Workspace: ws, Workflow: workflow, AgentType: agentType}, nil
	}
	resolutionCtx, err := r.ownerRuntimeContext(ctx, ws)
	if err != nil {
		return Spec{}, err
	}
	var runtime workspace.Runtime
	if provider, ok := r.Workspaces.(workspaceRuntimeProvider); ok {
		runtime, err = provider.GetWorkspaceRuntimeByID(ctx, ws.Id)
		if err != nil {
			return Spec{}, err
		}
	}
	tools, err := r.resolveToolkit(resolutionCtx, ws, workflow)
	if err != nil {
		return Spec{}, err
	}
	memoryName, memoryBinding, memoryLayout, err := r.resolveMemory(resolutionCtx, workflow)
	if err != nil {
		return Spec{}, err
	}
	var memoryProfileID, memoryProfileRevision string
	if memoryBinding != nil {
		profile := resolutionCtx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
		memoryProfileID = profile.Id
		memoryProfileRevision = profile.Revision
	}
	return Spec{
		Workspace:             ws,
		Workflow:              workflow,
		AgentType:             agentType,
		Runtime:               runtime,
		ToolInvoker:           tools,
		MemoryName:            memoryName,
		MemoryProfileID:       memoryProfileID,
		MemoryProfileRevision: memoryProfileRevision,
		MemoryBinding:         memoryBinding,
		MemoryLayout:          memoryLayout,
	}, nil
}

// workflowIsSFU reports whether the Workflow uses the provider-neutral SFU
// driver.
func workflowIsSFU(workflow apitypes.Workflow) bool {
	return strings.TrimSpace(string(workflow.Spec.Driver)) == string(apitypes.WorkflowDriverSfu)
}

type runtimeProfileContextKey struct{}

func withRuntimeProfile(ctx context.Context, profile apitypes.RuntimeProfile) context.Context {
	return context.WithValue(ctx, runtimeProfileContextKey{}, profile)
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
	resolved := WithResourceAccess(
		ctx,
		owner,
		runtimeProfileToolBindings(profile.Spec.Resources.Tools),
		runtimeProfileWorkflowBindings(profile),
		runtimeProfileFingerprint(profile),
	)
	return withRuntimeProfile(resolved, profile), nil
}

func (r ServiceResolver) resolveMemory(ctx context.Context, workflow apitypes.Workflow) (string, *apitypes.RuntimeProfileMemoryBinding, *apitypes.MemoryLayout, error) {
	if workflow.Spec.Memory == nil {
		return "", nil, nil, nil
	}
	alias := strings.TrimSpace(string(*workflow.Spec.Memory))
	if alias == "" {
		return "", nil, nil, fmt.Errorf("agenthost: workflow memory alias is empty")
	}
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	if !ok {
		return "", nil, nil, fmt.Errorf("agenthost: workflow memory alias %q requires an owner RuntimeProfile", alias)
	}
	if profile.Spec.Resources.Memories == nil {
		return "", nil, nil, fmt.Errorf("agenthost: runtime memory alias %q not found", alias)
	}
	binding, ok := (*profile.Spec.Resources.Memories)[alias]
	if !ok {
		return "", nil, nil, fmt.Errorf("agenthost: runtime memory alias %q not found", alias)
	}
	if r.MemoryLayouts == nil {
		return "", nil, nil, fmt.Errorf("agenthost: memory layout service is required")
	}
	response, err := r.MemoryLayouts.GetMemoryLayout(ctx, adminhttp.GetMemoryLayoutRequestObject{Id: binding.LayoutId})
	if err != nil {
		return "", nil, nil, err
	}
	var layout apitypes.MemoryLayout
	switch response := response.(type) {
	case adminhttp.GetMemoryLayout200JSONResponse:
		layout = apitypes.MemoryLayout(response)
	case adminhttp.GetMemoryLayout404JSONResponse:
		return "", nil, nil, fmt.Errorf("agenthost: memory layout %q not found", binding.LayoutId)
	case adminhttp.GetMemoryLayout500JSONResponse:
		return "", nil, nil, fmt.Errorf("agenthost: get memory layout %q failed: %s", binding.LayoutId, response.Error.Message)
	default:
		return "", nil, nil, fmt.Errorf("agenthost: unexpected GetMemoryLayout response %T", response)
	}
	return alias, &binding, &layout, nil
}

func resolveWorkspaceWorkflowName(ctx context.Context, ws apitypes.Workspace) (string, error) {
	_ = ctx
	id := ws.WorkflowId
	if err := customid.ValidateResourceID(id); err != nil {
		return "", fmt.Errorf("agenthost: workspace %q has an invalid workflow id: %w", ws.Name, err)
	}
	return id, nil
}

func (r ServiceResolver) resolveToolkit(_ context.Context, ws apitypes.Workspace, workflow apitypes.Workflow) (*ToolkitInvoker, error) {
	workflowPolicies := workflowToolkitPolicies(workflow.Spec)
	if r.ToolBuilder == nil {
		if ws.Toolkit == nil && len(workflowPolicies) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("agenthost: toolkit services are required")
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
	restrict := workflowRestrict || workspaceRestrict
	ids := workflowIDs
	switch {
	case workflowRestrict && workspaceRestrict:
		ids = intersectToolIDs(workflowIDs, workspaceIDs)
	case workspaceRestrict:
		ids = workspaceIDs
	}
	return &ToolkitInvoker{
		Builder:       r.ToolBuilder,
		Credentials:   r.ToolCredentials,
		HTTP:          r.HTTPTools,
		ClientTimeout: r.ClientToolTimeout,
		Request: toolkit.BuildRequest{
			AllowedTools:  ids,
			RestrictTools: restrict,
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
	if resolver, ok := r.Workspaces.(interface {
		GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error)
	}); ok {
		return resolver.GetWorkspaceByName(ctx, name)
	}
	return apitypes.Workspace{}, fmt.Errorf("agenthost: workspace name resolver is required")
}

func (r ServiceResolver) getWorkspaceByID(ctx context.Context, id string) (apitypes.Workspace, error) {
	response, err := r.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: id})
	if err != nil {
		return apitypes.Workspace{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkspace200JSONResponse:
		return apitypes.Workspace(response), nil
	case adminhttp.GetWorkspace404JSONResponse:
		return apitypes.Workspace{}, fmt.Errorf("agenthost: workspace %q not found", id)
	case adminhttp.GetWorkspace500JSONResponse:
		return apitypes.Workspace{}, fmt.Errorf("agenthost: get workspace %q failed: %s", id, response.Error.Message)
	default:
		return apitypes.Workspace{}, fmt.Errorf("agenthost: unexpected GetWorkspace response %T", response)
	}
}

func (r ServiceResolver) getAvailableWorkspaceByID(ctx context.Context, id string) (apitypes.Workspace, error) {
	provider, ok := r.Workspaces.(availableWorkspaceProvider)
	if !ok {
		return apitypes.Workspace{}, errors.New("agenthost: Workspace availability resolver is required")
	}
	value, err := provider.GetAvailableWorkspaceByID(ctx, id)
	if err != nil {
		return apitypes.Workspace{}, fmt.Errorf("agenthost: get available workspace %q: %w", id, err)
	}
	return value, nil
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
		return apitypes.Workflow{}, fmt.Errorf("agenthost: workflow %q not found", id)
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("agenthost: get workflow %q failed: %s", id, response.Error.Message)
	default:
		return apitypes.Workflow{}, fmt.Errorf("agenthost: unexpected GetWorkflow response %T", response)
	}
}
