package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workspace"
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
	params := testFlowcraftWorkspaceParameters()
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {
				Name:         "demo",
				Parameters:   params,
				WorkflowName: "workflow-1",
			},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.WorkflowDocument{
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
}

func TestServiceResolverUsesWorkflowAPIVersionAsAgentType(t *testing.T) {
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {Name: "demo", WorkflowName: "workflow-1"},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.WorkflowDocument{
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

func TestAgentTypeFromWorkflowAPIVersion(t *testing.T) {
	for _, tc := range []struct {
		apiVersion string
		want       string
	}{
		{apiVersion: "gizclaw.flowcraft/v1alpha1", want: "flowcraft"},
		{apiVersion: "gizclaw.intercom/v1alpha1", want: "intercom"},
	} {
		doc := rawWorkflow(t, tc.apiVersion)
		got, err := agentTypeFromWorkflow(doc)
		if err != nil {
			t.Fatalf("agentTypeFromWorkflow(%q) error = %v", tc.apiVersion, err)
		}
		if got != tc.want {
			t.Fatalf("agentTypeFromWorkflow(%q) = %q, want %q", tc.apiVersion, got, tc.want)
		}
	}

	if _, err := agentTypeFromWorkflow(rawWorkflow(t, "bad")); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported apiVersion error = %v", err)
	}
	if _, err := agentTypeFromWorkflow(rawWorkflow(t, "")); err == nil || !strings.Contains(err.Error(), "apiVersion is required") {
		t.Fatalf("empty apiVersion error = %v", err)
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
	resolver.Workflows = fakeWorkflowService{items: map[string]apitypes.WorkflowDocument{
		"bad-agent-type": mustWorkflow(t, "bad-agent-type"),
	}}
	resolver.Workspaces = fakeWorkspaceService{items: map[string]apitypes.Workspace{
		"demo": {Name: "demo", Parameters: &params, WorkflowName: "bad-agent-type"},
	}}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "workspace parameters") {
		t.Fatalf("bad agent_type error = %v", err)
	}
}

func testFlowcraftWorkspaceParameters() *apitypes.WorkspaceParameters {
	var params apitypes.WorkspaceParameters
	if err := params.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{
		AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
	}); err != nil {
		panic(err)
	}
	return &params
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

func TestHostTransformPreparesWorkspaceRuntime(t *testing.T) {
	host := New(fakeResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, AgentType: "echo"}})
	host.WorkspaceStore = fakeWorkspaceStore{runtime: WorkspaceRuntime{
		ObjectPrefix: "workspaces/demo",
		LocalDir:     "/tmp/gizclaw-agenthost/workspaces/demo",
	}}
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

func TestHostTransformRejectsConcurrentSameWorkspace(t *testing.T) {
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
	defer stream.Close()

	_, err = host.Transform(context.Background(), "demo", emptyStream{})
	if !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("second Transform() error = %v, want %v", err, ErrWorkspaceBusy)
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

type fakeWorkspaceStore struct {
	runtime WorkspaceRuntime
	err     error
}

func (s fakeWorkspaceStore) PrepareWorkspace(context.Context, string) (WorkspaceRuntime, error) {
	return s.runtime, s.err
}

type fakeWorkspaceService struct {
	workspace.WorkspaceAdminService
	items map[string]apitypes.Workspace
}

func (s fakeWorkspaceService) GetWorkspace(_ context.Context, request adminservice.GetWorkspaceRequestObject) (adminservice.GetWorkspaceResponseObject, error) {
	item, ok := s.items[string(request.Name)]
	if !ok {
		return adminservice.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
	}
	return adminservice.GetWorkspace200JSONResponse(item), nil
}

type fakeWorkflowService struct {
	workflow.WorkflowAdminService
	items map[string]apitypes.WorkflowDocument
}

func (s fakeWorkflowService) GetWorkflow(_ context.Context, request adminservice.GetWorkflowRequestObject) (adminservice.GetWorkflowResponseObject, error) {
	item, ok := s.items[string(request.Name)]
	if !ok {
		return adminservice.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "not found")), nil
	}
	return adminservice.GetWorkflow200JSONResponse(item), nil
}

func mustWorkflow(t *testing.T, name string) apitypes.WorkflowDocument {
	t.Helper()

	return apitypes.WorkflowDocument{
		ApiVersion: apitypes.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
		Kind:       apitypes.FlowcraftWorkflowKindFlowcraftWorkflow,
		Metadata:   apitypes.WorkflowMetadata{Name: name},
		Spec:       map[string]interface{}{"nodes": []interface{}{}},
	}
}

func rawWorkflow(t *testing.T, apiVersion string) apitypes.WorkflowDocument {
	t.Helper()
	data := []byte(`{"apiVersion":"` + apiVersion + `","kind":"FlowcraftWorkflow","metadata":{"name":"workflow"},"spec":{}}`)
	var doc apitypes.WorkflowDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal raw workflow: %v", err)
	}
	return doc
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
