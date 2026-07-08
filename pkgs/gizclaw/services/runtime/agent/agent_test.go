package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
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
	if err := registry.Register("", factory); err == nil || !strings.Contains(err.Error(), "workflow type is required") {
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
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{
			"demo": {
				Name:         "demo",
				WorkflowName: "workflow-1",
			},
		}},
		Workflows: fakeWorkflowService{items: map[string]apitypes.WorkflowDocument{
			"workflow-1": mustWorkflow("workflow-1"),
		}},
	}

	spec, err := resolver.Resolve(context.Background(), "/workspaces/demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.Workspace.Name != "demo" {
		t.Fatalf("unexpected workspace spec: %#v", spec)
	}
	if spec.WorkflowType != "flowcraft" {
		t.Fatalf("WorkflowType = %q, want flowcraft", spec.WorkflowType)
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
}

func TestWorkflowTypeFromDriver(t *testing.T) {
	got, err := resolveWorkflowType(rawWorkflow(t, apitypes.WorkflowDriverFlowcraft))
	if err != nil {
		t.Fatalf("resolveWorkflowType() error = %v", err)
	}
	if got != "flowcraft" {
		t.Fatalf("resolveWorkflowType() = %q, want flowcraft", got)
	}
	if _, err := resolveWorkflowType(rawWorkflow(t, apitypes.WorkflowDriver("bad"))); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported driver error = %v", err)
	}
	if _, err := resolveWorkflowType(rawWorkflow(t, "")); err == nil || !strings.Contains(err.Error(), "spec.driver is required") {
		t.Fatalf("empty driver error = %v", err)
	}
}

func TestHostTransformRunsWorkflow(t *testing.T) {
	host := NewHost(staticResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, WorkflowType: "echo"}})
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
}

func TestHostTransformAllowsSameWorkspacePerConnection(t *testing.T) {
	host := NewHost(staticResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, WorkflowType: "echo"}})
	if err := host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return fixedTransformer{text: "ok"}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	first, err := host.Transform(context.Background(), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("first Transform() error = %v", err)
	}
	defer first.Close()
	second, err := host.Transform(context.Background(), "demo", emptyStream{})
	if err != nil {
		t.Fatalf("second Transform() error = %v", err)
	}
	defer second.Close()
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

	host = NewHost(staticResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, WorkflowType: "missing"}})
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err == nil || !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("missing factory Transform() error = %v", err)
	}

	host = NewHost(staticResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, WorkflowType: "nil-agent"}})
	if err := host.Register("nil-agent", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return nil, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := host.Transform(context.Background(), "demo", emptyStream{}); err == nil || !strings.Contains(err.Error(), "nil agent") {
		t.Fatalf("nil agent Transform() error = %v", err)
	}
}

func TestServiceReloadReplacesPerConnectionRuntime(t *testing.T) {
	host := NewHost(staticResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, WorkflowType: "echo"}})
	if err := host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return passthroughTransformer{}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	source := &recordingSource{}
	consumer := &recordingConsumer{}
	service := &Service{
		Host:     host,
		Pattern:  PatternSourceFunc(func(context.Context) (string, error) { return "demo", nil }),
		Source:   source,
		Consumer: consumer,
	}
	if err := service.Reload(context.Background()); err != nil {
		t.Fatalf("first Reload() error = %v", err)
	}
	first := source.streams[0]
	if err := service.Reload(context.Background()); err != nil {
		t.Fatalf("second Reload() error = %v", err)
	}
	select {
	case <-first.closed:
	case <-time.After(time.Second):
		t.Fatal("first runtime was not closed by reload")
	}
	if got := source.count(); got != 2 {
		t.Fatalf("source count = %d, want 2", got)
	}
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestServiceValidationAndFuncAdapters(t *testing.T) {
	if err := (*Service)(nil).Reload(context.Background()); !errors.Is(err, ErrNilService) {
		t.Fatalf("nil Reload() error = %v, want %v", err, ErrNilService)
	}
	if err := (&Service{}).Reload(context.Background()); !errors.Is(err, ErrMissingHost) {
		t.Fatalf("missing host error = %v", err)
	}
	if err := (&Service{Host: &Host{}}).Reload(context.Background()); !errors.Is(err, ErrMissingPattern) {
		t.Fatalf("missing pattern error = %v", err)
	}
	if err := (&Service{Host: &Host{}, Pattern: PatternSourceFunc(func(context.Context) (string, error) {
		return "demo", nil
	})}).Reload(context.Background()); !errors.Is(err, ErrMissingSource) {
		t.Fatalf("missing source error = %v", err)
	}
	if err := (&Service{
		Host:    &Host{},
		Pattern: PatternSourceFunc(func(context.Context) (string, error) { return "demo", nil }),
		Source:  StreamSourceFunc(func(context.Context) (genx.Stream, error) { return emptyStream{}, nil }),
	}).Reload(context.Background()); !errors.Is(err, ErrMissingConsumer) {
		t.Fatalf("missing consumer error = %v", err)
	}

	service := &Service{
		Host: NewHost(staticResolver{spec: Spec{
			Workspace:    apitypes.Workspace{Name: "demo"},
			WorkflowType: "echo",
		}}),
		Pattern: PatternSourceFunc(func(context.Context) (string, error) { return "demo", nil }),
		Source:  StreamSourceFunc(func(context.Context) (genx.Stream, error) { return emptyStream{}, nil }),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error {
			return nil
		}),
	}
	if err := service.Host.Register("echo", FactoryFunc(func(context.Context, Spec) (genx.Transformer, error) {
		return passthroughTransformer{}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := service.Reload(nil); err != nil {
		t.Fatalf("Reload(nil) error = %v", err)
	}
	if err := service.Stop(nil); err != nil {
		t.Fatalf("Stop(nil) error = %v", err)
	}
}

func TestServiceReloadClosesInputOnTransformError(t *testing.T) {
	input := newBlockingStream()
	service := &Service{
		Host:     NewHost(staticResolver{spec: Spec{Workspace: apitypes.Workspace{Name: "demo"}, WorkflowType: "missing"}}),
		Pattern:  PatternSourceFunc(func(context.Context) (string, error) { return "demo", nil }),
		Source:   StreamSourceFunc(func(context.Context) (genx.Stream, error) { return input, nil }),
		Consumer: StreamConsumerFunc(func(context.Context, genx.Stream) error { return nil }),
	}
	if err := service.Reload(context.Background()); err == nil || !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("Reload() error = %v", err)
	}
	select {
	case <-input.closed:
	case <-time.After(time.Second):
		t.Fatal("input was not closed after transform error")
	}
}

func TestServiceResolverResponseErrors(t *testing.T) {
	resolver := ServiceResolver{
		Workspaces: responseWorkspaceService{response: adminhttp.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed"))},
		Workflows:  fakeWorkflowService{},
	}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("workspace 500 error = %v", err)
	}

	resolver.Workspaces = responseWorkspaceService{response: adminhttp.GetWorkspace200JSONResponse(apitypes.Workspace{Name: "demo", WorkflowName: "workflow"})}
	resolver.Workflows = responseWorkflowService{response: adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed"))}
	if _, err := resolver.Resolve(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("workflow 500 error = %v", err)
	}
}

type staticResolver struct {
	spec Spec
	err  error
}

func (r staticResolver) Resolve(context.Context, string) (Spec, error) {
	return r.spec, r.err
}

type passthroughTransformer struct{}

func (passthroughTransformer) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return input, nil
}

type fixedTransformer struct {
	text string
}

func (t fixedTransformer) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleModel, Part: genx.Text(t.text)}}}, nil
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (emptyStream) Close() error                      { return nil }
func (emptyStream) CloseWithError(error) error        { return nil }

type sliceStream struct {
	chunks []*genx.MessageChunk
	idx    int
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if s.idx >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.idx]
	s.idx++
	return chunk, nil
}

func (s *sliceStream) Close() error               { return nil }
func (s *sliceStream) CloseWithError(error) error { return nil }

type blockingStream struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingStream() *blockingStream {
	return &blockingStream{closed: make(chan struct{})}
}

func (s *blockingStream) Next() (*genx.MessageChunk, error) {
	<-s.closed
	return nil, io.EOF
}

func (s *blockingStream) Close() error {
	s.once.Do(func() {
		close(s.closed)
	})
	return nil
}

func (s *blockingStream) CloseWithError(error) error {
	return s.Close()
}

type recordingSource struct {
	mu      sync.Mutex
	streams []*blockingStream
}

func (s *recordingSource) OpenAgentInput(context.Context) (genx.Stream, error) {
	stream := newBlockingStream()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams = append(s.streams, stream)
	return stream, nil
}

func (s *recordingSource) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.streams)
}

type recordingConsumer struct{}

func (c *recordingConsumer) ConsumeAgentOutput(_ context.Context, stream genx.Stream) error {
	for {
		_, err := stream.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type fakeWorkspaceService struct {
	workspace.WorkspaceAdminService
	items map[string]apitypes.Workspace
}

func (s fakeWorkspaceService) GetWorkspace(_ context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	item, ok := s.items[string(request.Name)]
	if !ok {
		return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkspace200JSONResponse(item), nil
}

type fakeWorkflowService struct {
	workflow.WorkflowAdminService
	items map[string]apitypes.WorkflowDocument
}

func (s fakeWorkflowService) GetWorkflow(_ context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	item, ok := s.items[string(request.Name)]
	if !ok {
		return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkflow200JSONResponse(item), nil
}

type responseWorkspaceService struct {
	workspace.WorkspaceAdminService
	response adminhttp.GetWorkspaceResponseObject
}

func (s responseWorkspaceService) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return s.response, nil
}

type responseWorkflowService struct {
	workflow.WorkflowAdminService
	response adminhttp.GetWorkflowResponseObject
}

func (s responseWorkflowService) GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	return s.response, nil
}

func mustWorkflow(name string) apitypes.WorkflowDocument {
	return apitypes.WorkflowDocument{
		Metadata: apitypes.WorkflowMetadata{Name: name},
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft,
		},
	}
}

func rawWorkflow(t *testing.T, driver apitypes.WorkflowDriver) apitypes.WorkflowDocument {
	t.Helper()
	spec := apitypes.FlowcraftWorkflowSpec{}
	doc := apitypes.WorkflowDocument{
		Metadata: apitypes.WorkflowMetadata{Name: "workflow"},
		Spec: apitypes.WorkflowSpec{
			Driver: driver,
		},
	}
	if driver == apitypes.WorkflowDriverFlowcraft {
		doc.Spec.Flowcraft = &spec
	}
	return doc
}
