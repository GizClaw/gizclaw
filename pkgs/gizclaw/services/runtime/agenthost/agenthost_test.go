package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
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

func TestTransformerRegistryRegisterAndGet(t *testing.T) {
	registry := NewTransformerRegistry()
	factory := TransformerFactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return passthroughTransformer{}, nil
	})
	if err := registry.Register("chatroom", factory); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, ok := registry.Get("chatroom"); !ok {
		t.Fatal("Get() did not return registered transformer")
	}
	if err := registry.Register("chatroom", factory); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate Register() error = %v", err)
	}
	if err := registry.Register("", factory); err == nil || !strings.Contains(err.Error(), "transformer type is required") {
		t.Fatalf("empty Register() error = %v", err)
	}
	if err := registry.Register("bad", nil); err == nil || !strings.Contains(err.Error(), "transformer factory is required") {
		t.Fatalf("nil Register() error = %v", err)
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
			"demo": {
				Name:         "demo",
				Parameters:   &params,
				WorkflowName: "workflow-1",
			},
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
			"demo": {Name: "demo", WorkflowName: "workflow-1"},
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

func TestServiceResolverRejectsWorkspaceAgentTypeWorkflowDriverMismatch(t *testing.T) {
	var params apitypes.WorkspaceParameters
	if err := params.FromPetWorkspaceParameters(apitypes.PetWorkspaceParameters{
		AgentType: apitypes.PetWorkspaceParametersAgentTypePet,
		Voice:     apitypes.PetVoiceParameters{VoiceId: "voice"},
	}); err != nil {
		t.Fatalf("FromPetWorkspaceParameters() error = %v", err)
	}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {Name: "demo", WorkflowName: "workflow-1", Parameters: &params},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": mustWorkflow(t, "workflow-1"),
		}},
	}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "does not match workflow driver") {
		t.Fatalf("Resolve() mismatch error = %v", err)
	}
}

func TestServiceResolverResolvesToolkitPolicy(t *testing.T) {
	workflowToolIDs := []string{"system.mode.switch", "system.music.play"}
	workspaceToolIDs := []string{"system.music.play"}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {
				Name:         "demo",
				WorkflowName: "workflow-1",
				Toolkit:      &apitypes.ToolkitPolicy{ToolIds: &workspaceToolIDs},
			},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": {
				Name: "workflow-1",
				Spec: apitypes.WorkflowSpec{
					Driver:  apitypes.WorkflowDriverFlowcraft,
					Toolkit: &apitypes.ToolkitPolicy{ToolIds: &workflowToolIDs},
				},
			},
		}},
		ToolBuilder:   &toolkit.Builder{},
		ToolExecutors: toolkit.NewExecutorRegistry(),
	}

	spec, err := resolver.Resolve(WithACLSubject(context.Background(), acl.PublicKeySubject("peer-a")), "demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Toolkit == nil {
		t.Fatal("Toolkit = nil")
	}
	build := spec.Toolkit.BuildRequest
	if build.Subject != acl.PublicKeySubject("peer-a") {
		t.Fatalf("Toolkit subject = %#v", build.Subject)
	}
	if !build.RestrictToolIDs || len(build.AllowedToolIDs) != 1 || build.AllowedToolIDs[0] != "system.music.play" {
		t.Fatalf("Toolkit build request = %#v, want only system.music.play", build)
	}
}

func TestServiceResolverWorkspaceToolkitCanExposeNoTools(t *testing.T) {
	emptyToolIDs := []string{}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {
				Name:         "demo",
				WorkflowName: "workflow-1",
				Toolkit:      &apitypes.ToolkitPolicy{ToolIds: &emptyToolIDs},
			},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": mustWorkflow(t, "workflow-1"),
		}},
		ToolBuilder:   &toolkit.Builder{},
		ToolExecutors: toolkit.NewExecutorRegistry(),
	}

	spec, err := resolver.Resolve(WithACLSubject(context.Background(), acl.PublicKeySubject("peer-a")), "demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Toolkit == nil || !spec.Toolkit.BuildRequest.RestrictToolIDs || len(spec.Toolkit.BuildRequest.AllowedToolIDs) != 0 {
		t.Fatalf("Toolkit build request = %#v, want explicit empty allowlist", spec.Toolkit)
	}
}

func TestServiceResolverUsesToolkitAuthorizerFromContext(t *testing.T) {
	workflowToolIDs := []string{"system.music.play"}
	rawAuthorizer := &testToolkitAuthorizer{}
	overrideAuthorizer := &testToolkitAuthorizer{}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {
				Name:         "demo",
				WorkflowName: "workflow-1",
			},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": {
				Name: "workflow-1",
				Spec: apitypes.WorkflowSpec{
					Driver:  apitypes.WorkflowDriverFlowcraft,
					Toolkit: &apitypes.ToolkitPolicy{ToolIds: &workflowToolIDs},
				},
			},
		}},
		ToolBuilder:   &toolkit.Builder{Authorizer: rawAuthorizer},
		ToolExecutors: toolkit.NewExecutorRegistry(),
	}

	ctx := WithACLSubject(context.Background(), acl.PublicKeySubject("peer-a"))
	spec, err := resolver.Resolve(WithToolkitAuthorizer(ctx, overrideAuthorizer), "demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Toolkit == nil {
		t.Fatal("Toolkit = nil")
	}
	if spec.Toolkit.Builder.Authorizer != overrideAuthorizer {
		t.Fatalf("Toolkit authorizer = %#v, want context override", spec.Toolkit.Builder.Authorizer)
	}
	if resolver.ToolBuilder.Authorizer != rawAuthorizer {
		t.Fatalf("resolver builder authorizer mutated to %#v", resolver.ToolBuilder.Authorizer)
	}
}

func TestToolkitContextInvokeUsesCurrentContextSubject(t *testing.T) {
	ctx := context.Background()
	toolName := "echo"
	store := &toolkit.Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, toolkit.Tool{
		ID:          "system.toolkit.echo",
		Name:        &toolName,
		Source:      toolkit.ToolSourceBuiltin,
		Enabled:     true,
		InputSchema: jsonschema.Schema{Type: "object"},
		Executor: toolkit.ToolExecutor{
			Kind: toolkit.ToolExecutorKindBuiltin,
			Name: &toolName,
		},
	}); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	authorizer := &recordingToolkitAuthorizer{}
	executors := toolkit.NewExecutorRegistry()
	var callSubject string
	if err := executors.Register(toolName, toolkit.ExecutorFunc(func(_ context.Context, call toolkit.Call) (toolkit.Result, error) {
		callSubject = call.SubjectID
		return toolkit.Result{Data: json.RawMessage(`{"ok":true}`)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	tools := &ToolkitContext{
		Builder: &toolkit.Builder{
			Tools:      store,
			Authorizer: authorizer,
		},
		Executors: executors,
		BuildRequest: toolkit.BuildRequest{
			Subject:        acl.PublicKeySubject("peer-a"),
			AllowedToolIDs: []string{"system.toolkit.echo"},
		},
	}

	if _, err := tools.Invoke(WithACLSubject(ctx, acl.PublicKeySubject("peer-b")), "call-1", toolName, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(authorizer.subjects) != 1 || authorizer.subjects[0] != "peer-b" {
		t.Fatalf("authorized subjects = %#v, want peer-b", authorizer.subjects)
	}
	if callSubject != "peer-b" {
		t.Fatalf("call subject = %q, want peer-b", callSubject)
	}
}

func TestToolkitContextBuildAgentToolkitInvokesAndClones(t *testing.T) {
	ctx := context.Background()
	toolName := "echo"
	description := "Echo input"
	store := &toolkit.Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, toolkit.Tool{
		ID:          "system.toolkit.echo",
		Name:        &toolName,
		Description: &description,
		Source:      toolkit.ToolSourceBuiltin,
		Enabled:     true,
		InputSchema: jsonschema.Schema{Type: "object"},
		Executor: toolkit.ToolExecutor{
			Kind: toolkit.ToolExecutorKindBuiltin,
			Name: &toolName,
		},
	}); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	executors := toolkit.NewExecutorRegistry()
	if err := executors.Register(toolName, toolkit.ExecutorFunc(func(_ context.Context, call toolkit.Call) (toolkit.Result, error) {
		return toolkit.Result{Data: append(json.RawMessage(nil), call.Args...)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	resolved, err := (&ToolkitContext{
		Builder:   &toolkit.Builder{Tools: store},
		Executors: executors,
	}).BuildAgentToolkit(ctx)
	if err != nil {
		t.Fatalf("BuildAgentToolkit() error = %v", err)
	}
	tools := resolved.Tools()
	if len(tools) != 1 || tools[0].ID != "system.toolkit.echo" || tools[0].Name != toolName || tools[0].Description != description {
		t.Fatalf("Tools() = %#v", tools)
	}
	tools[0].InputSchema.Type = "string"
	if got := resolved.Tools()[0].InputSchema.Type; got != "object" {
		t.Fatalf("Tools() schema = %q after caller mutation", got)
	}
	result, err := resolved.Invoke(ctx, commonagent.ToolCall{
		ID:        "call-1",
		Name:      toolName,
		Arguments: json.RawMessage(`{"value":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.ID != "call-1" || string(result.Content) != `{"value":"hello"}` || result.IsError {
		t.Fatalf("Invoke() result = %+v", result)
	}
	undeclared, err := resolved.Invoke(ctx, commonagent.ToolCall{ID: "call-2", Name: "new-after-build", Arguments: json.RawMessage(`{}`)})
	if err != nil || !undeclared.IsError || !strings.Contains(string(undeclared.Content), "tool_not_found") {
		t.Fatalf("Invoke(undeclared) = %+v, %v", undeclared, err)
	}
}

func TestAgentToolkitMapsBusinessFailureToStructuredResult(t *testing.T) {
	resolved := agentToolkit{context: &ToolkitContext{}, tools: []commonagent.Tool{{Name: "missing"}}}
	result, err := resolved.Invoke(t.Context(), commonagent.ToolCall{ID: "call-1", Name: "missing", Arguments: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !result.IsError || result.ID != "call-1" || !json.Valid(result.Content) || !strings.Contains(string(result.Content), "tool_error") {
		t.Fatalf("Invoke() result = %+v", result)
	}
}

func TestServiceResolverRequiresSubjectForToolkit(t *testing.T) {
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {
				Name:         "demo",
				WorkflowName: "workflow-1",
				Toolkit:      &apitypes.ToolkitPolicy{},
			},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"workflow-1": mustWorkflow(t, "workflow-1"),
		}},
		ToolBuilder:   &toolkit.Builder{},
		ToolExecutors: toolkit.NewExecutorRegistry(),
	}

	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "authenticated subject") {
		t.Fatalf("Resolve() error = %v, want authenticated subject error", err)
	}
}

func TestAgentTypeFromWorkflowDriver(t *testing.T) {
	for _, tc := range []struct {
		driver string
		want   string
	}{
		{driver: "flowcraft", want: "flowcraft"},
		{driver: "chatroom", want: "chatroom"},
		{driver: "doubao-realtime", want: "doubao-realtime"},
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
		"demo": {Name: "demo", WorkflowName: "missing"},
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
		"demo": {Name: "demo", Parameters: &params, WorkflowName: "bad-agent-type"},
	}}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "agent_type") {
		t.Fatalf("bad agent_type error = %v", err)
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

func TestHostTransformDoesNotReuseToolkitRuntimeAcrossSubjects(t *testing.T) {
	host := New(subjectToolkitResolver{})
	var subjects []string
	if err := host.Register("echo", FactoryFunc(func(_ context.Context, spec Spec) (genx.Transformer, error) {
		subjects = append(subjects, spec.Toolkit.BuildRequest.Subject.Id)
		return fixedTransformer{text: "ok"}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	first, err := host.Transform(WithACLSubject(context.Background(), acl.PublicKeySubject("peer-a")), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("Transform(peer-a) error = %v", err)
	}

	if _, err := host.Transform(WithACLSubject(context.Background(), acl.PublicKeySubject("peer-b")), "demo", emptyStream{}); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("Transform(peer-b while peer-a active) error = %v, want %v", err, ErrWorkspaceBusy)
	}
	if len(subjects) != 1 || subjects[0] != "peer-a" {
		t.Fatalf("factory subjects while busy = %#v, want only peer-a", subjects)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close(peer-a) error = %v", err)
	}

	second, err := host.Transform(WithACLSubject(context.Background(), acl.PublicKeySubject("peer-b")), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("Transform(peer-b after release) error = %v", err)
	}
	defer second.Close()

	if len(subjects) != 2 || subjects[0] != "peer-a" || subjects[1] != "peer-b" {
		t.Fatalf("factory subjects = %#v, want distinct peer subjects after release", subjects)
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

type fakeResolver struct {
	spec Spec
	err  error
}

func (r fakeResolver) Resolve(context.Context, string) (Spec, error) {
	if r.err != nil {
		return Spec{}, r.err
	}
	return r.spec, nil
}

type fakeWorkspaceService struct {
	workspace.WorkspaceAdminService
	items   map[string]apitypes.Workspace
	runtime workspace.Runtime
}

func (s fakeWorkspaceService) GetWorkspace(_ context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	item, ok := s.items[string(request.Name)]
	if !ok {
		return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkspace200JSONResponse(item), nil
}

func (s fakeWorkspaceService) GetWorkspaceRuntime(context.Context, string) (workspace.Runtime, error) {
	return s.runtime, nil
}

type fakeWorkflowService struct {
	workflow.WorkflowAdminService
	items map[string]apitypes.Workflow
}

type subjectToolkitResolver struct{}

func (subjectToolkitResolver) Resolve(ctx context.Context, _ string) (Spec, error) {
	subject, ok := aclSubjectFromContext(ctx)
	if !ok {
		return Spec{}, errors.New("missing subject")
	}
	return Spec{
		Workspace: apitypes.Workspace{Name: "demo"},
		AgentType: "echo",
		Toolkit: &ToolkitContext{
			BuildRequest: toolkit.BuildRequest{Subject: subject},
		},
	}, nil
}

type testToolkitAuthorizer struct{}

func (*testToolkitAuthorizer) Authorize(context.Context, acl.AuthorizeRequest) error {
	return nil
}

type recordingToolkitAuthorizer struct {
	subjects []string
}

func (a *recordingToolkitAuthorizer) Authorize(_ context.Context, req acl.AuthorizeRequest) error {
	a.subjects = append(a.subjects, req.Subject.Id)
	return nil
}

func (s fakeWorkflowService) GetWorkflow(_ context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	item, ok := s.items[string(request.Name)]
	if !ok {
		return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkflow200JSONResponse(item), nil
}

func mustWorkflow(t *testing.T, name string) apitypes.Workflow {
	t.Helper()

	return apitypes.Workflow{
		Name: name,
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft,
		},
	}
}

func rawWorkflow(t *testing.T, driver string) apitypes.Workflow {
	t.Helper()
	return apitypes.Workflow{
		Name: "workflow",
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriver(driver),
		},
	}
}

type passthroughTransformer struct{}

func (passthroughTransformer) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return input, nil
}

type fixedTransformer struct {
	text string
}

func (t fixedTransformer) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return &fixedStream{chunks: []*genx.MessageChunk{{Part: genx.Text(t.text)}}}, nil
}

type nilStreamTransformer struct{}

func (nilStreamTransformer) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
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
