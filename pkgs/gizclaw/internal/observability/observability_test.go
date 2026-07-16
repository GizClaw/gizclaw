package observability

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *captureHandler) WithGroup(string) slog.Handler            { return h }
func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, record.Clone())
	h.mu.Unlock()
	return nil
}

func TestOutcomeLogsBoundedScalarContract(t *testing.T) {
	handler := &captureHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })

	outcome := NewOutcome(TransportRPC, SurfacePeerRPC, "server.workspace.create")
	ctx := WithOutcome(context.Background(), outcome)
	outcome.SetRequestID("request-1")
	outcome.SetPeer("peer-key", "client")
	Annotate(ctx, AnnotationWorkspaceName, "workspace-1")
	Annotate(ctx, AnnotationWorkflowName, "workflow-1")
	Annotate(ctx, AnnotationKey("authorization"), "secret")
	SetErrorCode(ctx, "INVALID_WORKSPACE")
	outcome.SetRPC(400, ResultClientError)
	Log(ctx, outcome)

	if len(handler.records) != 1 {
		t.Fatalf("records = %d, want 1", len(handler.records))
	}
	record := handler.records[0]
	if record.Message != CompletionMessage || record.Level != slog.LevelWarn {
		t.Fatalf("record = (%q, %v), want completion WARN", record.Message, record.Level)
	}
	attrs := recordAttrs(record)
	for key, want := range map[string]any{
		"transport": "rpc", "surface": "peer-rpc", "operation": "server.workspace.create",
		"result": "client_error", "rpc_code": int64(400), "request_id": "request-1",
		"peer_public_key": "peer-key", "peer_role": "client", "error_code": "INVALID_WORKSPACE",
		"workspace_name": "workspace-1", "workflow_name": "workflow-1",
	} {
		if got := attrs[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	if _, ok := attrs["authorization"]; ok {
		t.Fatal("non-allowlisted annotation was logged")
	}
}

func TestOutcomeRejectsUnsafeValuesAndMapsLevels(t *testing.T) {
	if got := boundedRoute("/users/private?token=secret"); got != "unknown" {
		t.Fatalf("boundedRoute() = %q, want unknown", got)
	}
	if got := boundedOperation("/users/private"); got != "unknown" {
		t.Fatalf("boundedOperation() = %q, want unknown", got)
	}
	if got := boundedHTTPMethod("SECRET-METHOD"); got != "OTHER" {
		t.Fatalf("boundedHTTPMethod() = %q, want OTHER", got)
	}
	if got := levelFor(ResultSuccess, 204, 0); got != slog.LevelInfo {
		t.Fatalf("success level = %v", got)
	}
	if got := levelFor(ResultCanceled, 0, 0); got != slog.LevelWarn {
		t.Fatalf("canceled level = %v", got)
	}
	if got := levelFor(ResultTransportError, 0, 0); got != slog.LevelError {
		t.Fatalf("transport level = %v", got)
	}
	if got := levelFor(ResultClientError, 0, 500); got != slog.LevelError {
		t.Fatalf("RPC 500 level = %v", got)
	}
	outcome := NewOutcome(TransportRPC, SurfacePeerRPC, "method")
	outcome.SetRequestID("unsafe/request/id")
	outcome.SetErrorCode("unsafe/error/code")
	_, attrs := outcome.logRecord()
	for _, attr := range attrs {
		if attr.Key == "request_id" || attr.Key == "error_code" {
			t.Fatalf("unsafe %s was logged", attr.Key)
		}
	}
}

func TestOutcomeOmitsAbsentRPCCodeAndMapsHTTPStyleCodeClass(t *testing.T) {
	outcome := NewOutcome(TransportRPC, SurfacePeerRPC, "server.workspace.get")
	outcome.SetRPC(0, ResultSuccess)
	_, attrs := outcome.logRecord()
	for _, attr := range attrs {
		if attr.Key == "rpc_code" {
			t.Fatal("zero RPC code was logged")
		}
	}

	outcome.SetRPC(404, ResultClientError)
	_, attrs = outcome.logRecord()
	got := make(map[string]any)
	for _, attr := range attrs {
		got[attr.Key] = attr.Value.Any()
	}
	if got["rpc_code"] != int64(404) || got["status_class"] != "4xx" {
		t.Fatalf("attrs = %#v", got)
	}
}

func TestOutcomeSupportsConcurrentAnnotationsAndCompletionReads(t *testing.T) {
	outcome := NewOutcome(TransportHTTP, SurfaceServerPublic, "GetStatus")
	var group sync.WaitGroup
	for index := range 16 {
		group.Go(func() {
			outcome.SetHTTP("GET", "/status", 200+index%2, ResultSuccess)
			outcome.SetRequestID("request-1")
			outcome.SetPeer("peer-key", "client")
			outcome.SetErrorCode("SAFE_CODE")
			outcome.Annotate(AnnotationWorkspaceName, "workspace-a")
			_, _ = outcome.logRecord()
		})
	}
	group.Wait()
}

func recordAttrs(record slog.Record) map[string]any {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	return attrs
}
