package agenthost

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/memorylayout"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry()
	factory := FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return passthroughTransformer{}, nil
	})
	if err := registry.Register("flowcraft", factory); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, ok := registry.Get("flowcraft"); !ok {
		t.Fatal("Get() missing registered factory")
	}
	if err := registry.Register("flowcraft", factory); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate Register() error = %v", err)
	}
	if err := registry.Register("", factory); err == nil || !strings.Contains(err.Error(), "agent type is required") {
		t.Fatalf("empty Register() error = %v", err)
	}
	if err := registry.Register("bad", nil); err == nil || !strings.Contains(err.Error(), "factory is required") {
		t.Fatalf("nil factory Register() error = %v", err)
	}
}

func TestParseWorkspacePattern(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "demo", want: "demo"},
		{in: "/demo/", want: "demo"},
		{in: "/workspaces/demo", want: "demo"},
		{in: "workspaces/demo%201", want: "demo 1"},
	} {
		got, err := ParseWorkspacePattern(tc.in)
		if err != nil {
			t.Fatalf("ParseWorkspacePattern(%q) error = %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseWorkspacePattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, pattern := range []string{"", "/workspaces/", "/workspaces/demo/agents/default"} {
		if _, err := ParseWorkspacePattern(pattern); err == nil {
			t.Fatalf("ParseWorkspacePattern(%q) error = nil", pattern)
		}
	}
}

func TestServiceResolverResolvesWorkspaceAndWorkflow(t *testing.T) {
	workflow := mustWorkflow(t, "workflow-1")
	var params apitypes.WorkspaceParameters
	if err := params.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{}); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters() error = %v", err)
	}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": systemWorkspace("demo", "workflow-1", &params),
		}, runtime: workspace.Runtime{ObjectPrefix: "workspaces/demo", LocalDir: "/tmp/demo"}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": workflow,
		}},
	}

	spec, err := resolver.Resolve(context.Background(), "/workspaces/demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Workspace.Name != "demo" {
		t.Fatalf("unexpected workspace spec: %#v", spec)
	}
	if spec.AgentType != "flowcraft" {
		t.Fatalf("AgentType = %q, want flowcraft", spec.AgentType)
	}
	if spec.Runtime.ObjectPrefix != "workspaces/demo" {
		t.Fatalf("Runtime = %#v", spec.Runtime)
	}
}

func TestServiceResolverUsesWorkflowDriverAsAgentType(t *testing.T) {
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": systemWorkspace("demo", "workflow-1", nil),
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": mustWorkflow(t, "workflow-1"),
		}},
	}

	spec, err := resolver.Resolve(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.AgentType != "flowcraft" {
		t.Fatalf("AgentType = %q, want flowcraft", spec.AgentType)
	}
}

func TestServiceResolverUsesWorkspaceOwnerRuntimeProfile(t *testing.T) {
	owner := "owner-public-key"
	labels := map[string]string{"collection": "assistants"}
	ws := systemWorkspace("shared", "owner-workflow", nil)
	ws.OwnerPublicKey = &owner
	ws.Labels = &labels
	profile := apitypes.RuntimeProfile{
		Id:       "owner-profile",
		Revision: "revision-1",
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			Collections: apitypes.RuntimeProfileWorkflowCollections{
				"assistants": {
					"chat":               {ResourceId: "owner-workflow"},
					"unavailable-helper": {ResourceId: "missing-workflow"},
				},
			},
		}},
	}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{"shared": ws}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"owner-workflow":  mustWorkflow(t, "owner-workflow"),
			"caller-workflow": mustWorkflow(t, "caller-workflow"),
		}},
		RuntimeProfileForOwner: func(_ context.Context, gotOwner string) (apitypes.RuntimeProfile, error) {
			if gotOwner != owner {
				t.Fatalf("owner = %q, want %q", gotOwner, owner)
			}
			return profile, nil
		},
	}
	callerCtx := WithResourceAccess(context.Background(), "caller", nil, map[string]string{"chat": "caller-workflow"})
	spec, err := resolver.Resolve(callerCtx, "shared")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Workflow.Id != "owner-workflow" {
		t.Fatalf("workflow = %q, want owner-workflow", spec.Workflow.Id)
	}
}

func TestServiceResolverActivatesSFUWorkspaceWithoutOwnerRuntimeProfile(t *testing.T) {
	owner := "owner-on-another-server"
	ws := systemWorkspace("ws-shared", "system-sfu", nil)
	ws.OwnerPublicKey = &owner
	sfuWorkflow := apitypes.Workflow{Id: "system-sfu", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu}}
	profileLookups := 0
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{"ws-shared": ws, "id-ws-shared": ws}},
		Workflows:  fakeWorkflowService{items: map[string]apitypes.Workflow{"system-sfu": sfuWorkflow}},
		RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) {
			profileLookups++
			return apitypes.RuntimeProfile{}, errors.New("kv: not found")
		},
		ToolBuilder: &toolkit.Builder{},
	}
	for name, resolve := range map[string]func() (Spec, error){
		"by name": func() (Spec, error) { return resolver.Resolve(t.Context(), "ws-shared") },
		"by id":   func() (Spec, error) { return resolver.ResolveByID(t.Context(), "id-ws-shared") },
		"memory":  func() (Spec, error) { return resolver.ResolveMemoryByID(t.Context(), "id-ws-shared") },
	} {
		spec, err := resolve()
		if err != nil {
			t.Fatalf("%s: resolve SFU Workspace error = %v", name, err)
		}
		if spec.Workspace.Id != "id-ws-shared" || spec.Workflow.Id != "system-sfu" {
			t.Fatalf("%s: spec = %#v", name, spec)
		}
		if spec.ToolInvoker != nil || spec.MemoryBinding != nil || spec.MemoryName != "" || spec.MemoryProfileID != "" {
			t.Fatalf("%s: SFU spec resolved owner-profile resources: %#v", name, spec)
		}
	}
	if spec, err := resolver.ResolveByID(t.Context(), "id-ws-shared"); err != nil || spec.AgentType != "sfu" {
		t.Fatalf("ResolveByID() = %#v, %v; want sfu agent type", spec, err)
	}
	if profileLookups != 0 {
		t.Fatalf("owner RuntimeProfile was looked up %d times for an SFU Workspace", profileLookups)
	}
}

func TestServiceResolverFailsWhenSelectedWorkflowIsUnavailable(t *testing.T) {
	owner := "owner-public-key"
	ws := systemWorkspace("shared", "missing-workflow", nil)
	ws.OwnerPublicKey = &owner
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{"shared": ws}},
		Workflows:  fakeWorkflowService{items: map[string]apitypes.Workflow{}},
		RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) {
			return apitypes.RuntimeProfile{Id: "owner-profile", Revision: "revision-1"}, nil
		},
	}
	if _, err := resolver.Resolve(t.Context(), "shared"); err == nil || !strings.Contains(err.Error(), `workflow "missing-workflow" not found`) {
		t.Fatalf("Resolve() error = %v, want selected Workflow not found", err)
	}
}

func TestServiceResolverUsesCallerRuntimeProfileMemoryForUnownedWorkspace(t *testing.T) {
	alias := apitypes.WorkflowMemoryAlias("pet-memory")
	workflow := mustWorkflow(t, "workflow-1")
	workflow.Spec.Memory = &alias
	bindings := map[string]apitypes.RuntimeProfileMemoryBinding{
		"pet-memory": {
			LayoutId: "pet-layout",
			Driver:   apitypes.RuntimeProfileMemoryDriverFlowcraft,
		},
	}
	profile := apitypes.RuntimeProfile{
		Id:       "caller-profile",
		Revision: "revision-1",
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{Memories: &bindings},
		},
	}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": systemWorkspace("demo", "workflow-1", nil),
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": workflow,
		}},
		MemoryLayouts: fakeMemoryLayoutService{
			item: apitypes.MemoryLayout{Id: "pet-layout"},
		},
	}

	spec, err := resolver.Resolve(withRuntimeProfile(t.Context(), profile), "demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.MemoryName != "pet-memory" || spec.MemoryBinding == nil ||
		spec.MemoryLayout == nil || spec.MemoryProfileID != "caller-profile" ||
		spec.MemoryProfileRevision != "revision-1" {
		t.Fatalf("resolved Memory spec = %#v", spec)
	}
}

func TestServiceResolverResolveMemorySkipsToolkitConstruction(t *testing.T) {
	alias := apitypes.WorkflowMemoryAlias("pet-memory")
	resolvedWorkflow := mustWorkflow(t, "workflow-1")
	resolvedWorkflow.Spec.Memory = &alias
	toolIDs := []string{"tool-a"}
	ws := systemWorkspace("demo", "workflow-1", nil)
	ws.Toolkit = &apitypes.ToolkitPolicy{ToolIds: &toolIDs}
	bindings := map[string]apitypes.RuntimeProfileMemoryBinding{
		"pet-memory": {
			LayoutId: "pet-layout",
			Driver:   apitypes.RuntimeProfileMemoryDriverFlowcraft,
		},
	}
	profile := apitypes.RuntimeProfile{
		Id: "profile", Revision: "revision",
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{Memories: &bindings},
		},
	}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{"demo": ws}},
		Workflows:  fakeWorkflowService{items: map[string]apitypes.Workflow{"workflow-1": resolvedWorkflow}},
		MemoryLayouts: fakeMemoryLayoutService{
			item: apitypes.MemoryLayout{Id: "pet-layout"},
		},
	}
	ctx := withRuntimeProfile(t.Context(), profile)
	if _, err := resolver.Resolve(ctx, "demo"); err == nil || !strings.Contains(err.Error(), "toolkit") {
		t.Fatalf("Resolve() error = %v, want toolkit construction failure", err)
	}
	spec, err := resolver.ResolveMemory(ctx, "demo")
	if err != nil {
		t.Fatalf("ResolveMemory() error = %v", err)
	}
	if spec.MemoryName != "pet-memory" || spec.MemoryBinding == nil || spec.MemoryLayout == nil {
		t.Fatalf("ResolveMemory() = %#v", spec)
	}
	if spec.ToolInvoker != nil || spec.AgentType != "" {
		t.Fatalf("ResolveMemory() constructed unrelated agent state: %#v", spec)
	}
}

func TestServiceResolverRejectsWorkspaceAgentTypeWorkflowDriverMismatch(t *testing.T) {
	var params apitypes.WorkspaceParameters
	if err := params.FromEinoWorkspaceParameters(apitypes.EinoWorkspaceParameters{
		AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino,
	}); err != nil {
		t.Fatalf("FromEinoWorkspaceParameters() error = %v", err)
	}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": systemWorkspace("demo", "workflow-1", &params),
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": mustWorkflow(t, "workflow-1"),
		}},
	}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "does not match workflow driver") {
		t.Fatalf("Resolve() mismatch error = %v", err)
	}
}

func TestServiceResolverDefersCurrentPeerToolkitScopeUntilTransform(t *testing.T) {
	workspace := systemWorkspace("demo", "workflow-1", nil)
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": workspace,
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": mustWorkflow(t, "workflow-1"),
		}},
		ToolBuilder: &toolkit.Builder{},
	}

	resolved, err := resolver.Resolve(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ToolInvoker == nil {
		t.Fatal("Resolve() ToolInvoker = nil")
	}
}

func TestResolveToolkitAppliesNestedPetWorkflowPolicy(t *testing.T) {
	outerIDs := []string{"search", "clock"}
	nestedIDs := []string{"search"}
	workflow := apitypes.Workflow{Spec: apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverPet,
		Toolkit: &apitypes.ToolkitPolicy{
			ToolIds: &outerIDs,
		},
		Pet: &apitypes.PetWorkflowSpec{
			Driver: apitypes.ReusableWorkflowDriverFlowcraft,
			Toolkit: &apitypes.ToolkitPolicy{
				ToolIds: &nestedIDs,
			},
			Flowcraft: &apitypes.FlowcraftWorkflowSpec{},
		},
	}}
	resolver := ServiceResolver{
		ToolBuilder: &toolkit.Builder{},
	}
	resolved, err := resolver.resolveToolkit(context.Background(), apitypes.Workspace{}, workflow)
	if err != nil {
		t.Fatalf("resolveToolkit() error = %v", err)
	}
	if resolved == nil || !resolved.Request.RestrictTools {
		t.Fatalf("resolved toolkit = %#v", resolved)
	}
	if got, want := resolved.Request.AllowedTools, []string{"search"}; !slices.Equal(got, want) {
		t.Fatalf("AllowedTools = %#v, want %#v", got, want)
	}
}

func TestAgentTypeFromWorkflowDriver(t *testing.T) {
	for _, tc := range []struct {
		driver string
		want   string
	}{
		{driver: "flowcraft", want: "flowcraft"},
		{driver: "sfu", want: "sfu"},
		{driver: "doubao-realtime", want: "doubao-realtime"},
		{driver: "dashscope-realtime", want: "dashscope-realtime"},
		{driver: "doubao-realtime-duplex", want: "doubao-realtime-duplex"},
		{driver: "eino", want: "eino"},
	} {
		doc := rawWorkflow(t, tc.driver)
		got, err := agentTypeFromWorkflow(doc)
		if err != nil {
			t.Fatalf("agentTypeFromWorkflow(%q) error = %v", tc.driver, err)
		}
		if got != tc.want {
			t.Fatalf("agentTypeFromWorkflow(%q) = %q, want %q", tc.driver, got, tc.want)
		}
	}

	if _, err := agentTypeFromWorkflow(rawWorkflow(t, "")); err == nil || !strings.Contains(err.Error(), "spec.driver is required") {
		t.Fatalf("empty driver error = %v", err)
	}
}

func TestServiceResolverErrors(t *testing.T) {
	if _, err := (ServiceResolver{}).Resolve(context.Background(), "demo"); err == nil {
		t.Fatal("Resolve() with missing services error = nil")
	}
	resolver := ServiceResolver{Workspaces: fakeWorkspaceService{}, Workflows: fakeWorkflowService{}}
	if _, err := resolver.Resolve(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("missing workspace error = %v", err)
	}
	resolver.Workspaces = fakeWorkspaceService{items: map[string]apitypes.Workspace{
		"demo": systemWorkspace("demo", "missing", nil),
	}}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "workflow") {
		t.Fatalf("missing workflow error = %v", err)
	}
	var params apitypes.WorkspaceParameters
	if err := params.UnmarshalJSON([]byte(`{"agent_type":1}`)); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	resolver.Workflows = fakeWorkflowService{items: map[string]apitypes.Workflow{
		"bad-agent-type": mustWorkflow(t, "bad-agent-type"),
	}}
	resolver.Workspaces = fakeWorkspaceService{items: map[string]apitypes.Workspace{
		"demo": systemWorkspace("demo", "bad-agent-type", &params),
	}}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "agent_type") {
		t.Fatalf("bad agent_type error = %v", err)
	}
}

func TestServiceResolverResolveMemoryByIDPreservesWorkspaceLifecycleError(t *testing.T) {
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{availabilityErr: workspace.ErrWorkspacePendingDeletion},
		Workflows:  fakeWorkflowService{},
	}
	_, err := resolver.ResolveMemoryByID(context.Background(), "workspace-id")
	if !errors.Is(err, workspace.ErrWorkspacePendingDeletion) {
		t.Fatalf("ResolveMemoryByID() error = %v, want %v", err, workspace.ErrWorkspacePendingDeletion)
	}
}

func TestServiceResolverAllowsAdminWorkspaceWithCanonicalWorkflow(t *testing.T) {
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {Name: "demo", WorkflowId: "workflow-1"},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": mustWorkflow(t, "workflow-1"),
		}},
	}

	if _, err := resolver.Resolve(context.Background(), "demo"); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func systemWorkspace(name, workflowName string, parameters *apitypes.WorkspaceParameters) apitypes.Workspace {
	system := true
	return apitypes.Workspace{
		Id:         "id-" + name,
		Name:       name,
		Parameters: parameters,
		System:     &system,
		WorkflowId: workflowName,
	}
}

func TestHostTransformRunsAgentAndReleasesOnClose(t *testing.T) {
	host := New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "echo"}})
	if err := host.Register("echo", FactoryFunc(func(_ context.Context, spec Spec) (genx.Transformer, error) {
		if spec.Workspace.Name != "demo" {
			t.Fatalf("factory workspace = %q, want demo", spec.Workspace.Name)
		}
		return fixedTransformer{text: "ok"}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	stream, err := host.Transform(context.Background(), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got := string(chunk.Part.(genx.Text)); got != "ok" {
		t.Fatalf("chunk text = %q, want ok", got)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err != nil {
		t.Fatalf("Transform() after Close() error = %v", err)
	}
}

func TestHostTransformUsesResolvedWorkspaceRuntime(t *testing.T) {
	host := New(fakeResolver{spec: Spec{
		Workspace: apitypes.Workspace{Name: "demo"},
		AgentType: "echo",
		Runtime: workspace.Runtime{
			ObjectPrefix: "workspaces/demo",
			LocalDir:     "/tmp/gizclaw-agenthost/workspaces/demo",
		},
	}})
	if err := host.Register("echo", FactoryFunc(func(_ context.Context, spec Spec) (genx.Transformer, error) {
		if spec.Runtime.ObjectPrefix != "workspaces/demo" {
			t.Fatalf("runtime object prefix = %q, want workspaces/demo", spec.Runtime.ObjectPrefix)
		}
		if spec.Runtime.LocalDir == "" {
			t.Fatal("runtime local dir is empty")
		}
		return fixedTransformer{text: "ok"}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	stream, err := host.Transform(context.Background(), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer stream.Close()
}

func TestHostTransformReusesAgentForConcurrentSameWorkspace(t *testing.T) {
	host := New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "echo"}})
	createCount := 0
	if err := host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		createCount++
		return fixedTransformer{text: "ok"}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	first, err := host.Transform(context.Background(), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer first.Close()

	second, err := host.Transform(context.Background(), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("second Transform() error = %v", err)
	}
	defer second.Close()
	if createCount != 1 {
		t.Fatalf("factory calls = %d, want 1", createCount)
	}
}

func TestHostUsesCanonicalWorkspaceLeaseAcrossRuntimeProfiles(t *testing.T) {
	spec := Spec{Workspace: apitypes.Workspace{Id: "workspace-system", Name: "system"}, AgentType: "echo"}
	host := New(fakeResolver{spec: spec})
	createCount := 0
	if err := host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		createCount++
		return passthroughTransformer{}, nil
	})); err != nil {
		t.Fatal(err)
	}
	firstContext := WithResourceAccess(t.Context(), "peer-a", nil, nil, "profile-a")
	secondContext := WithResourceAccess(t.Context(), "peer-b", nil, nil, "profile-b")
	if runtimeKey(spec.Workspace.Id) != runtimeKey(spec.Workspace.Id) {
		t.Fatal("canonical Workspace identity must be stable across RuntimeProfiles")
	}
	first, release, err := host.OpenAgent(firstContext, "system")
	if err != nil {
		t.Fatalf("OpenAgent(first profile) error = %v", err)
	}
	second, releaseSecond, err := host.OpenAgent(secondContext, "system")
	if err != nil {
		t.Fatalf("OpenAgent(second profile) error = %v", err)
	}
	if first != second || createCount != 1 {
		t.Fatalf("canonical Workspace agent = (%p, %p), factory calls = %d; want one shared instance", first, second, createCount)
	}
	release()
	releaseSecond()
	_, releaseThird, err := host.OpenAgent(secondContext, "system")
	if err != nil {
		t.Fatalf("OpenAgent(second profile after release) error = %v", err)
	}
	if createCount != 2 {
		t.Fatalf("factory calls after final release = %d, want 2", createCount)
	}
	releaseThird()
}

func TestHostSeparatesSameNamedWorkspacesByCanonicalID(t *testing.T) {
	resolver := workspacePatternResolver{
		"first":  {Workspace: apitypes.Workspace{Id: "workspace-first", Name: "shared"}, AgentType: "echo"},
		"second": {Workspace: apitypes.Workspace{Id: "workspace-second", Name: "shared"}, AgentType: "echo"},
	}
	host := New(resolver)
	created := 0
	if err := host.Register("echo", agentFactoryFunc(func(context.Context, Spec) (Agent, error) {
		created++
		return &pointerTestAgent{Agent: NewTransformerAgent(passthroughTransformer{})}, nil
	})); err != nil {
		t.Fatal(err)
	}
	first, releaseFirst, err := host.OpenAgent(t.Context(), "first")
	if err != nil {
		t.Fatalf("OpenAgent(first) error = %v", err)
	}
	defer releaseFirst()
	second, releaseSecond, err := host.OpenAgent(t.Context(), "second")
	if err != nil {
		t.Fatalf("OpenAgent(second) error = %v", err)
	}
	defer releaseSecond()
	if first == second || created != 2 {
		t.Fatalf("same-named Workspace agents = (%p, %p), factory calls = %d; want isolated instances", first, second, created)
	}
	for _, id := range []string{"workspace-first", "workspace-second"} {
		if _, err := host.coordinator().Acquire(t.Context(), id); !errors.Is(err, ErrWorkspaceBusy) {
			t.Fatalf("Acquire(%q) error = %v, want %v", id, err, ErrWorkspaceBusy)
		}
	}
}

func TestHostRejectsWorkspaceWithoutCanonicalID(t *testing.T) {
	host := New(workspacePatternResolver{
		"missing": {Workspace: apitypes.Workspace{Name: "shared"}, AgentType: "echo"},
	})
	if _, _, err := host.OpenAgent(t.Context(), "missing"); err == nil || !strings.Contains(err.Error(), "workspace ID is required") {
		t.Fatalf("OpenAgent() error = %v, want canonical Workspace ID failure", err)
	}
}

func TestHostTransformReleasesWhenOutputEnds(t *testing.T) {
	host := New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "echo"}})
	if err := host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return fixedTransformer{text: "ok"}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	stream, err := host.Transform(context.Background(), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_, _ = stream.Next()
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal Next() error = %v, want EOF", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err != nil {
		t.Fatalf("Transform() after EOF error = %v", err)
	}
}

func TestHostTransformErrorsReleaseLease(t *testing.T) {
	host := New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "echo"}})
	wantErr := errors.New("new agent failed")
	if err := host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return nil, wantErr
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); !errors.Is(err, wantErr) {
		t.Fatalf("Transform() error = %v, want %v", err, wantErr)
	}

	host.Registry = NewRegistry()
	if err := host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return fixedTransformer{text: "ok"}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err != nil {
		t.Fatalf("Transform() after factory error = %v", err)
	}
}

func TestHostTransformValidationErrors(t *testing.T) {
	if _, err := (*Host)(nil).Transform(context.Background(), "demo", emptyStream{}); err == nil || !strings.Contains(err.Error(), "host is nil") {
		t.Fatalf("nil host Transform() error = %v", err)
	}
	host := &Host{}
	if _, err := host.Transform(context.Background(), "demo", nil); err == nil || !strings.Contains(err.Error(), "input stream") {
		t.Fatalf("nil input Transform() error = %v", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err == nil || !strings.Contains(err.Error(), "resolver") {
		t.Fatalf("missing resolver Transform() error = %v", err)
	}

	host = New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "missing"}})
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err == nil || !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("missing factory Transform() error = %v", err)
	}

	host = New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "nil-agent"}})
	if err := host.Register("nil-agent", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return nil, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err == nil || !strings.Contains(err.Error(), "nil agent") {
		t.Fatalf("nil agent Transform() error = %v", err)
	}

	host = New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "nil-stream"}})
	if err := host.Register("nil-stream", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return nilStreamTransformer{}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err == nil || !strings.Contains(err.Error(), "nil stream") {
		t.Fatalf("nil stream Transform() error = %v", err)
	}
}

func TestMemoryCoordinatorHonorsContextAndRelease(t *testing.T) {
	coordinator := NewMemoryCoordinator()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.Acquire(ctx, "demo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(canceled) error = %v, want context.Canceled", err)
	}
	lease, err := coordinator.Acquire(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if lease.Workspace() != "demo" || lease.Token() == "" {
		t.Fatalf("unexpected lease: workspace=%q token=%q", lease.Workspace(), lease.Token())
	}
	if _, err := coordinator.Acquire(context.Background(), "demo"); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("Acquire(busy) error = %v, want %v", err, ErrWorkspaceBusy)
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, err := coordinator.Acquire(context.Background(), "demo"); err != nil {
		t.Fatalf("Acquire(after release) error = %v", err)
	}
}

func TestRuntimeRegistrySharesAgentAndClosesOnFinalRelease(t *testing.T) {
	t.Parallel()
	spec := Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "shared"}
	host := New(fakeResolver{spec: spec})
	var mu sync.Mutex
	created := 0
	closed := 0
	if err := host.Register("shared", agentFactoryFunc(func(context.Context, Spec) (Agent, error) {
		mu.Lock()
		created++
		mu.Unlock()
		return &closeTrackingAgent{
			Agent: NewTransformerAgent(passthroughTransformer{}),
			close: func() {
				mu.Lock()
				closed++
				mu.Unlock()
			},
		}, nil
	})); err != nil {
		t.Fatal(err)
	}

	first, releaseFirst, err := host.OpenAgent(context.Background(), "demo")
	if err != nil {
		t.Fatalf("OpenAgent(first) error = %v", err)
	}
	second, releaseSecond, err := host.OpenAgent(context.Background(), "demo")
	if err != nil {
		t.Fatalf("OpenAgent(second) error = %v", err)
	}
	if first != second {
		t.Fatal("same Workspace did not share one Agent")
	}
	releaseFirst()
	mu.Lock()
	if created != 1 || closed != 0 {
		t.Fatalf("after first release created=%d closed=%d", created, closed)
	}
	mu.Unlock()
	releaseSecond()
	releaseSecond()
	mu.Lock()
	if closed != 1 {
		t.Fatalf("after final release closed=%d, want 1", closed)
	}
	mu.Unlock()

	third, releaseThird, err := host.OpenAgent(context.Background(), "demo")
	if err != nil {
		t.Fatalf("OpenAgent(after final release) error = %v", err)
	}
	if third == first {
		t.Fatal("Agent was not reconstructed after final release")
	}
	releaseThird()
	mu.Lock()
	defer mu.Unlock()
	if created != 2 || closed != 2 {
		t.Fatalf("final created=%d closed=%d, want 2/2", created, closed)
	}
}

func TestRuntimeRegistryQuiesceClosesOnlyExactWorkspace(t *testing.T) {
	t.Parallel()
	resolver := mutableWorkspaceResolver{idByPattern: map[string]string{"first": "workspace-a", "second": "workspace-b"}}
	host := New(resolver)
	var mu sync.Mutex
	closed := map[string]int{}
	if err := host.Register("shared", agentFactoryFunc(func(_ context.Context, spec Spec) (Agent, error) {
		id := spec.Workspace.Id
		return &closeTrackingAgent{
			Agent: NewTransformerAgent(passthroughTransformer{}),
			close: func() {
				mu.Lock()
				closed[id]++
				mu.Unlock()
			},
		}, nil
	})); err != nil {
		t.Fatal(err)
	}
	_, releaseFirst, err := host.OpenAgent(t.Context(), "first")
	if err != nil {
		t.Fatal(err)
	}
	_, releaseSecond, err := host.OpenAgent(t.Context(), "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.QuiesceWorkspace(t.Context(), "workspace-a"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if closed["workspace-a"] != 1 || closed["workspace-b"] != 0 {
		mu.Unlock()
		t.Fatalf("close counts after quiesce = %#v", closed)
	}
	mu.Unlock()
	releaseFirst()
	releaseSecond()
	mu.Lock()
	defer mu.Unlock()
	if closed["workspace-a"] != 1 || closed["workspace-b"] != 1 {
		t.Fatalf("final close counts = %#v", closed)
	}
}

func TestRuntimeRegistryQuiesceCancelsActiveAttachments(t *testing.T) {
	t.Parallel()
	resolver := mutableWorkspaceResolver{idByPattern: map[string]string{"first": "workspace-a"}}
	host := New(resolver)
	transformContext := make(chan context.Context, 1)
	if err := host.Register("shared", agentFactoryFunc(func(_ context.Context, _ Spec) (Agent, error) {
		return NewTransformerAgent(runtimeTransformerFunc(func(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
			transformContext <- ctx
			return genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1).Stream(), nil
		})), nil
	})); err != nil {
		t.Fatal(err)
	}
	agent, release, err := host.OpenAgent(t.Context(), "first")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	output, err := agent.Transform(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := <-transformContext
	if err := host.QuiesceWorkspace(t.Context(), "workspace-a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), errWorkspaceQuiesced) {
			t.Fatalf("Transform context cause = %v, want %v", context.Cause(ctx), errWorkspaceQuiesced)
		}
	case <-time.After(time.Second):
		t.Fatal("Workspace quiesce did not cancel the active Transform context")
	}
	if _, err := output.Next(); !errors.Is(err, errWorkspaceQuiesced) {
		t.Fatalf("quiesced Transform output error = %v, want %v", err, errWorkspaceQuiesced)
	}
}

func TestRuntimeRegistryQuiesceCancelsRetiredGenerationAttachments(t *testing.T) {
	t.Parallel()
	resolver := mutableWorkspaceResolver{idByPattern: map[string]string{"first": "workspace-a"}}
	host := New(resolver)
	transformContexts := make(chan context.Context, 2)
	if err := host.Register("shared", agentFactoryFunc(func(_ context.Context, _ Spec) (Agent, error) {
		return NewTransformerAgent(runtimeTransformerFunc(func(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
			transformContexts <- ctx
			return genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1).Stream(), nil
		})), nil
	})); err != nil {
		t.Fatal(err)
	}
	previous, releasePrevious, err := host.OpenAgent(t.Context(), "first")
	if err != nil {
		t.Fatal(err)
	}
	defer releasePrevious()
	previousOutput, err := previous.Transform(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	previousContext := <-transformContexts

	current, releaseCurrent, err := host.ReloadAgent(t.Context(), "first")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCurrent()
	currentOutput, err := current.Transform(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	currentContext := <-transformContexts

	if err := host.QuiesceWorkspace(t.Context(), "workspace-a"); err != nil {
		t.Fatal(err)
	}
	for label, state := range map[string]struct {
		ctx    context.Context
		output genx.Stream
	}{
		"retired": {ctx: previousContext, output: previousOutput},
		"current": {ctx: currentContext, output: currentOutput},
	} {
		select {
		case <-state.ctx.Done():
			if !errors.Is(context.Cause(state.ctx), errWorkspaceQuiesced) {
				t.Fatalf("%s Transform context cause = %v, want %v", label, context.Cause(state.ctx), errWorkspaceQuiesced)
			}
		case <-time.After(time.Second):
			t.Fatalf("Workspace quiesce did not cancel the %s Transform context", label)
		}
		if _, err := state.output.Next(); !errors.Is(err, errWorkspaceQuiesced) {
			t.Fatalf("%s Transform output error = %v, want %v", label, err, errWorkspaceQuiesced)
		}
	}
}

type mutableWorkspaceResolver struct {
	idByPattern map[string]string
}

func (r mutableWorkspaceResolver) Resolve(_ context.Context, pattern string) (Spec, error) {
	id := r.idByPattern[pattern]
	return Spec{Workspace: apitypes.Workspace{Id: id, Name: pattern}, AgentType: "shared"}, nil
}

func TestRuntimeRegistryReloadPreservesOldGenerationOnConstructorFailure(t *testing.T) {
	t.Parallel()
	spec := Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "reloadable"}
	host := New(fakeResolver{spec: spec})
	wantErr := errors.New("replacement failed")
	var mu sync.Mutex
	var agents []*closeTrackingAgent
	var closed []int
	calls := 0
	if err := host.Register("reloadable", agentFactoryFunc(func(context.Context, Spec) (Agent, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 2 {
			return nil, wantErr
		}
		index := len(agents)
		closed = append(closed, 0)
		agent := &closeTrackingAgent{Agent: NewTransformerAgent(passthroughTransformer{})}
		agent.close = func() {
			mu.Lock()
			closed[index]++
			mu.Unlock()
		}
		agents = append(agents, agent)
		return agent, nil
	})); err != nil {
		t.Fatal(err)
	}

	first, releaseFirst, err := host.OpenAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := host.ReloadAgent(t.Context(), "demo"); !errors.Is(err, wantErr) {
		t.Fatalf("ReloadAgent(failed) error = %v, want %v", err, wantErr)
	}
	stillCurrent, releaseCurrent, err := host.OpenAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if stillCurrent != first {
		t.Fatal("constructor failure replaced the old Workspace generation")
	}
	releaseCurrent()

	replacement, releaseReplacement, err := host.ReloadAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if replacement == first {
		t.Fatal("successful reload reused the old Agent generation")
	}
	mu.Lock()
	if closed[0] != 0 {
		mu.Unlock()
		t.Fatal("old generation closed while its stream attachment was live")
	}
	mu.Unlock()
	releaseFirst()
	releaseFirst()
	mu.Lock()
	if closed[0] != 1 {
		mu.Unlock()
		t.Fatalf("old generation close count = %d, want 1", closed[0])
	}
	mu.Unlock()
	releaseReplacement()
	mu.Lock()
	defer mu.Unlock()
	if len(agents) != 2 || closed[1] != 1 {
		t.Fatalf("replacement generations=%d close counts=%v", len(agents), closed)
	}
}

func TestRuntimeRegistryPreparedReplacementPublishesOnlyOnCommit(t *testing.T) {
	t.Parallel()
	spec := Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "reloadable"}
	host := New(fakeResolver{spec: spec})
	var mu sync.Mutex
	var agents []*closeTrackingAgent
	var closed []int
	if err := host.Register("reloadable", agentFactoryFunc(func(context.Context, Spec) (Agent, error) {
		mu.Lock()
		defer mu.Unlock()
		index := len(agents)
		closed = append(closed, 0)
		agent := &closeTrackingAgent{Agent: NewTransformerAgent(passthroughTransformer{})}
		agent.close = func() {
			mu.Lock()
			closed[index]++
			mu.Unlock()
		}
		agents = append(agents, agent)
		return agent, nil
	})); err != nil {
		t.Fatal(err)
	}

	first, releaseFirst, err := host.OpenAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := host.PrepareReloadAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Agent() == first {
		t.Fatal("prepared replacement reused the current generation")
	}
	stillCurrent, releaseCurrent, err := host.OpenAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if stillCurrent != first {
		t.Fatal("uncommitted replacement changed the current generation")
	}
	releaseCurrent()
	replacement.Release()

	afterAbort, releaseAfterAbort, err := host.OpenAgent(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if afterAbort != first {
		t.Fatal("aborted replacement changed the current generation")
	}
	releaseAfterAbort()
	mu.Lock()
	if len(agents) != 2 || closed[0] != 0 || closed[1] != 1 {
		mu.Unlock()
		t.Fatalf("prepared agents=%d close counts=%v, want current open and candidate closed", len(agents), closed)
	}
	mu.Unlock()

	releaseFirst()
	mu.Lock()
	defer mu.Unlock()
	if closed[0] != 1 {
		t.Fatalf("current generation close count = %d, want 1", closed[0])
	}
}

type fakeResolver struct {
	spec Spec
	err  error
}

type workspacePatternResolver map[string]Spec

func (r workspacePatternResolver) Resolve(_ context.Context, pattern string) (Spec, error) {
	spec, ok := r[pattern]
	if !ok {
		return Spec{}, errors.New("workspace not found")
	}
	return spec, nil
}

type agentFactoryFunc func(context.Context, Spec) (Agent, error)

func (f agentFactoryFunc) NewAgent(ctx context.Context, spec Spec) (Agent, error) {
	return f(ctx, spec)
}

type closeTrackingAgent struct {
	Agent
	once  sync.Once
	close func()
}

type pointerTestAgent struct {
	Agent
}

func (a *closeTrackingAgent) Close() error {
	a.once.Do(a.close)
	return nil
}

func (r fakeResolver) Resolve(context.Context, string) (Spec, error) {
	if r.err != nil {
		return Spec{}, r.err
	}
	if r.spec.Workspace.Id == "" && r.spec.Workspace.Name != "" {
		r.spec.Workspace.Id = "id-" + r.spec.Workspace.Name
	}
	return r.spec, nil
}

type fakeWorkspaceService struct {
	workspace.WorkspaceAdminService
	items           map[string]apitypes.Workspace
	runtime         workspace.Runtime
	availabilityErr error
}

func (s fakeWorkspaceService) GetWorkspace(_ context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	item, ok := s.items[string(request.Id)]
	if !ok {
		return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkspace200JSONResponse(item), nil
}

func (s fakeWorkspaceService) GetWorkspaceByName(_ context.Context, name string) (apitypes.Workspace, error) {
	item, ok := s.items[name]
	if !ok {
		return apitypes.Workspace{}, errors.New("workspace not found")
	}
	return item, nil
}

func (s fakeWorkspaceService) GetWorkspaceRuntimeByID(context.Context, string) (workspace.Runtime, error) {
	return s.runtime, nil
}

func (s fakeWorkspaceService) GetAvailableWorkspaceByID(ctx context.Context, id string) (apitypes.Workspace, error) {
	if s.availabilityErr != nil {
		return apitypes.Workspace{}, s.availabilityErr
	}
	response, err := s.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: id})
	if err != nil {
		return apitypes.Workspace{}, err
	}
	value, ok := response.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		return apitypes.Workspace{}, errors.New("workspace not found")
	}
	return apitypes.Workspace(value), nil
}

type fakeWorkflowService struct {
	workflow.WorkflowAdminService
	items map[string]apitypes.Workflow
}

type fakeMemoryLayoutService struct {
	memorylayout.MemoryLayoutAdminService
	item apitypes.MemoryLayout
}

func (service fakeMemoryLayoutService) GetMemoryLayout(
	context.Context,
	adminhttp.GetMemoryLayoutRequestObject,
) (adminhttp.GetMemoryLayoutResponseObject, error) {
	return adminhttp.GetMemoryLayout200JSONResponse(service.item), nil
}

type subjectToolkitResolver struct{}

func (s fakeWorkflowService) GetWorkflow(_ context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	item, ok := s.items[string(request.Id)]
	if !ok {
		return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkflow200JSONResponse(item), nil
}

func mustWorkflow(t *testing.T, name string) apitypes.Workflow {
	t.Helper()

	return apitypes.Workflow{
		Id: name,
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft,
		},
	}
}

func rawWorkflow(t *testing.T, driver string) apitypes.Workflow {
	t.Helper()
	return apitypes.Workflow{
		Id: "workflow",
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriver(driver),
		},
	}
}

type passthroughTransformer struct{}

func (passthroughTransformer) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	return input, nil
}

type fixedTransformer struct {
	text string
}

func (t fixedTransformer) Transform(context.Context, genx.Stream) (genx.Stream, error) {
	return &fixedStream{chunks: []*genx.MessageChunk{{Part: genx.Text(t.text)}}}, nil
}

type nilStreamTransformer struct{}

func (nilStreamTransformer) Transform(context.Context, genx.Stream) (genx.Stream, error) {
	return nil, nil
}

type fixedStream struct {
	chunks []*genx.MessageChunk
	closed bool
}

func (s *fixedStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *fixedStream) Close() error {
	s.closed = true
	return nil
}

func (s *fixedStream) CloseWithError(error) error {
	s.closed = true
	return nil
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) {
	return nil, io.EOF
}

func (emptyStream) Close() error {
	return nil
}

func (emptyStream) CloseWithError(error) error {
	return nil
}
