package gizclaw

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
)

type slogCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *slogCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h *slogCapture) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *slogCapture) WithGroup(string) slog.Handler            { return h }
func (h *slogCapture) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, record.Clone())
	h.mu.Unlock()
	return nil
}

func captureSlog(t *testing.T) *slogCapture {
	t.Helper()
	handler := &slogCapture{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return handler
}

func TestObserveHTTPHandlerLogsSafeDomainErrorAndRequestID(t *testing.T) {
	capture := captureSlog(t)
	handler := observeHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outcome := observability.FromContext(r.Context())
		outcome.SetOperation("CreateWorkspace")
		outcome.SetRoute("/workspaces")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"INVALID_WORKSPACE","message":"secret provider text"}}`))
	}), httpObservationOptions{surface: observability.SurfaceAdminHTTP, peerPublicKey: "peer-key", peerRole: "admin"})

	req := httptest.NewRequest(http.MethodPost, "/workspaces?token=secret", strings.NewReader("prompt-secret"))
	req.Header.Set(requestIDHeader, "request-1")
	req.Header.Set("Authorization", "Bearer credential-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || rec.Header().Get(requestIDHeader) != "request-1" {
		t.Fatalf("response = (%d, %q)", rec.Code, rec.Header().Get(requestIDHeader))
	}
	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level != slog.LevelWarn || record.Message != observability.CompletionMessage {
		t.Fatalf("record = (%v, %q)", record.Level, record.Message)
	}
	for key, want := range map[string]any{
		"transport": "http", "surface": "admin-http", "operation": "CreateWorkspace",
		"route": "/workspaces", "method": "POST", "status": int64(400), "status_class": "4xx",
		"result": "client_error", "error_code": "INVALID_WORKSPACE", "request_id": "request-1",
		"peer_public_key": "peer-key", "peer_role": "admin",
	} {
		if got := attrs[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	var text strings.Builder
	text.WriteString(record.Message)
	for key, value := range attrs {
		text.WriteString(key)
		text.WriteByte('=')
		text.WriteString(fmt.Sprint(value))
	}
	for _, secret := range []string{"credential-secret", "provider text", "prompt-secret", "token=secret"} {
		if strings.Contains(text.String(), secret) {
			t.Errorf("record contains sensitive value %q", secret)
		}
	}
}

func TestObserveHTTPHandlerReplacesInvalidRequestID(t *testing.T) {
	capture := captureSlog(t)
	handler := observeHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(requestIDHeader, "attacker-selected-id")
		w.WriteHeader(http.StatusNoContent)
	}), httpObservationOptions{surface: observability.SurfaceServerPublic})
	req := httptest.NewRequest(http.MethodGet, "/private/raw/path", nil)
	req.Header.Set(requestIDHeader, "bad request id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	id := rec.Header().Get(requestIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("generated request ID = %q", id)
	}
	_, attrs := onlyCapturedRecord(t, capture)
	if attrs["operation"] != "unknown" || attrs["route"] != "unknown" || attrs["request_id"] != id {
		t.Fatalf("attrs = %#v", attrs)
	}
}

func TestRequestIDGenerationIsConcurrentAndUnique(t *testing.T) {
	ids := make(chan string, 64)
	var group sync.WaitGroup
	for range 64 {
		group.Go(func() {
			id, err := validOrNewRequestID("", rand.Reader)
			if err != nil {
				t.Errorf("validOrNewRequestID() error = %v", err)
				return
			}
			ids <- id
		})
	}
	group.Wait()
	close(ids)
	unique := make(map[string]struct{}, 64)
	for id := range ids {
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
			t.Errorf("generated request ID = %q", id)
		}
		unique[id] = struct{}{}
	}
	if len(unique) != 64 {
		t.Fatalf("unique IDs = %d, want 64", len(unique))
	}
}

func TestPublicCORSAllowsAndExposesRequestID(t *testing.T) {
	header := make(http.Header)
	setPublicHTTPCORSHeaders(header)
	if !strings.Contains(header.Get("Access-Control-Allow-Headers"), requestIDHeader) {
		t.Fatalf("allow headers = %q", header.Get("Access-Control-Allow-Headers"))
	}
	if !strings.Contains(header.Get("Access-Control-Expose-Headers"), requestIDHeader) {
		t.Fatalf("expose headers = %q", header.Get("Access-Control-Expose-Headers"))
	}
}

func TestObserveHTTPHandlerOmitsRequestIDWhenEntropyFails(t *testing.T) {
	previousWarningAt := requestIDWarningAt.Load()
	requestIDWarningAt.Store(0)
	t.Cleanup(func() { requestIDWarningAt.Store(previousWarningAt) })
	capture := captureSlog(t)
	handler := observeHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), httpObservationOptions{
		surface: observability.SurfaceServerPublic,
		entropy: failingEntropyReader{},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get(requestIDHeader); got != "" {
		t.Fatalf("request ID = %q, want omitted", got)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.records) != 2 {
		t.Fatalf("records = %d, want warning and completion", len(capture.records))
	}
	if capture.records[0].Message != "gizclaw: request id generation failed" || capture.records[0].Level != slog.LevelWarn {
		t.Fatalf("warning = (%q, %s)", capture.records[0].Message, capture.records[0].Level)
	}
	attrs := make(map[string]any)
	capture.records[1].Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	if _, ok := attrs["request_id"]; ok {
		t.Fatalf("completion attrs = %#v", attrs)
	}
}

type failingEntropyReader struct{}

func (failingEntropyReader) Read([]byte) (int, error) {
	return 0, errors.New("secret entropy backend failure")
}

func TestObserveHTTPHandlerLogsRethrownPanic(t *testing.T) {
	capture := captureSlog(t)
	handler := observeHTTPHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret panic value")
	}), httpObservationOptions{surface: observability.SurfaceServerPublic})

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("handler did not rethrow panic")
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/raw/private", nil))
	}()

	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level != slog.LevelError || attrs["result"] != "panic" || attrs["status_class"] != "unknown" || attrs["status"] != int64(0) {
		t.Fatalf("record = (%s, %#v)", record.Level, attrs)
	}
	if attrs["route"] != "unknown" || attrs["operation"] != "unknown" {
		t.Fatalf("raw path leaked into attrs = %#v", attrs)
	}
	if strings.Contains(fmt.Sprint(attrs), "secret panic value") {
		t.Fatalf("panic value leaked into attrs = %#v", attrs)
	}
}

func TestObserveHTTPHandlerLogsCanceledRequest(t *testing.T) {
	capture := captureSlog(t)
	handler := observeHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), httpObservationOptions{surface: observability.SurfaceServerPublic})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))

	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level != slog.LevelWarn || attrs["result"] != "canceled" || attrs["status_class"] != "2xx" {
		t.Fatalf("record = (%s, %#v)", record.Level, attrs)
	}
}

func TestObserveFiberRouteUsesRegisteredTemplateAndName(t *testing.T) {
	capture := captureSlog(t)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(observeFiberRoute)
	app.Get("/users/:id", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(http.StatusNoContent)
	}).Name("GetUser")
	handler := observeHTTPHandler(fiberHTTPHandler(app), httpObservationOptions{surface: observability.SurfaceAdminHTTP})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/private-user?token=secret", nil))

	_, attrs := onlyCapturedRecord(t, capture)
	if attrs["route"] != "/users/:id" || attrs["operation"] != "GetUser" {
		t.Fatalf("attrs = %#v", attrs)
	}
}

func TestObserveFiberRouteLeavesUnknownRouteUnknown(t *testing.T) {
	capture := captureSlog(t)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(observeFiberRoute)
	handler := observeHTTPHandler(fiberHTTPHandler(app), httpObservationOptions{surface: observability.SurfacePeerHTTP})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/private/raw/path", nil))

	_, attrs := onlyCapturedRecord(t, capture)
	if attrs["route"] != "unknown" || attrs["operation"] != "unknown" {
		t.Fatalf("unknown route attrs = %#v", attrs)
	}
}

func TestObserveHTTPHandlerUsesRegisteredMuxFallback(t *testing.T) {
	capture := captureSlog(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/openai/v1/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	})
	handler := observeHTTPHandler(mux, httpObservationOptions{surface: observability.SurfaceServerPublic})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil))

	_, attrs := onlyCapturedRecord(t, capture)
	if attrs["route"] != "/openai/v1/" || attrs["operation"] != "OpenAIProxy" {
		t.Fatalf("attrs = %#v", attrs)
	}
}

func TestRegisteredHTTPOperationRejectsWrongMethod(t *testing.T) {
	if got := registeredHTTPOperation(http.MethodDelete, "/me"); got != "" {
		t.Fatalf("registeredHTTPOperation() = %q, want empty", got)
	}
}

func TestObservePeerHTTPAuthAndPreflightUseAllowlistedFallback(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		wantStatus    int
		wantOperation string
	}{
		{name: "auth rejection", method: http.MethodGet, path: "/me", wantStatus: http.StatusUnauthorized, wantOperation: "GetMe"},
		{name: "preflight", method: http.MethodOptions, path: "/me/status", wantStatus: http.StatusNoContent, wantOperation: "CORSPreflight"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := captureSlog(t)
			handler := observeHTTPHandler((&PeerService{}).publicHTTPHandler(nil), httpObservationOptions{surface: observability.SurfacePeerHTTP})
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			_, attrs := onlyCapturedRecord(t, capture)
			if attrs["route"] != test.path || attrs["operation"] != test.wantOperation {
				t.Fatalf("attrs = %#v", attrs)
			}
		})
	}
}

func TestObserveHTTPHandlerMapsRepresentativeStatuses(t *testing.T) {
	tests := []struct {
		status      int
		wantLevel   slog.Level
		wantResult  string
		wantClass   string
		wantErrCode string
	}{
		{status: http.StatusOK, wantLevel: slog.LevelInfo, wantResult: "success", wantClass: "2xx"},
		{status: http.StatusNoContent, wantLevel: slog.LevelInfo, wantResult: "success", wantClass: "2xx"},
		{status: http.StatusBadRequest, wantLevel: slog.LevelWarn, wantResult: "client_error", wantClass: "4xx", wantErrCode: "HTTP_CLIENT_ERROR"},
		{status: http.StatusForbidden, wantLevel: slog.LevelWarn, wantResult: "client_error", wantClass: "4xx", wantErrCode: "HTTP_CLIENT_ERROR"},
		{status: http.StatusNotFound, wantLevel: slog.LevelWarn, wantResult: "client_error", wantClass: "4xx", wantErrCode: "HTTP_CLIENT_ERROR"},
		{status: http.StatusConflict, wantLevel: slog.LevelWarn, wantResult: "client_error", wantClass: "4xx", wantErrCode: "HTTP_CLIENT_ERROR"},
		{status: http.StatusInternalServerError, wantLevel: slog.LevelError, wantResult: "server_error", wantClass: "5xx", wantErrCode: "HTTP_SERVER_ERROR"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			capture := captureSlog(t)
			handler := observeHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}), httpObservationOptions{surface: observability.SurfaceServerPublic})
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			record, attrs := onlyCapturedRecord(t, capture)
			if record.Level != tc.wantLevel || attrs["result"] != tc.wantResult || attrs["status_class"] != tc.wantClass || attrs["status"] != int64(tc.status) {
				t.Fatalf("record = (%s, %#v)", record.Level, attrs)
			}
			if tc.wantErrCode == "" {
				if _, ok := attrs["error_code"]; ok {
					t.Fatalf("unexpected error_code in %#v", attrs)
				}
			} else if attrs["error_code"] != tc.wantErrCode {
				t.Fatalf("error_code = %#v, want %q", attrs["error_code"], tc.wantErrCode)
			}
		})
	}
}

func TestObserveHTTPHandlerLeavesMalformedAndOversizedErrorsUnchanged(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"error":`},
		{name: "oversized", body: `{"error":{"code":"SECRET_CODE","message":"` + strings.Repeat("x", maxObservedResponseBytes) + `"}}`},
		{name: "unsafe code", body: `{"error":{"code":"SECRET/code","message":"private"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := captureSlog(t)
			handler := observeHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}), httpObservationOptions{surface: observability.SurfaceServerPublic})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if got := rec.Body.String(); got != tc.body {
				t.Fatalf("body changed: got %q want %q", got, tc.body)
			}
			_, attrs := onlyCapturedRecord(t, capture)
			if attrs["error_code"] != "HTTP_CLIENT_ERROR" {
				t.Fatalf("attrs = %#v", attrs)
			}
			for _, secret := range []string{"SECRET_CODE", "SECRET/code", "private"} {
				if strings.Contains(fmt.Sprint(attrs), secret) {
					t.Fatalf("attrs contain %q: %#v", secret, attrs)
				}
			}
		})
	}
}

func TestObserveHTTPHandlerPreservesOptionalWriterInterfaces(t *testing.T) {
	capture := captureSlog(t)
	handler := observeHTTPHandler(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		for name, ok := range map[string]bool{
			"flusher":    implementsWriter[http.Flusher](writer),
			"hijacker":   implementsWriter[http.Hijacker](writer),
			"readerFrom": implementsWriter[io.ReaderFrom](writer),
			"pusher":     implementsWriter[http.Pusher](writer),
		} {
			if !ok {
				t.Errorf("wrapped writer lost %s", name)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.Copy(writer, strings.NewReader(`{"error":{"code":"INVALID_REQUEST","message":"private"}}`))
	}), httpObservationOptions{surface: observability.SurfaceServerPublic})
	all := newObservabilityAllWriter()
	handler.ServeHTTP(all, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := all.body.String(); got != `{"error":{"code":"INVALID_REQUEST","message":"private"}}` {
		t.Fatalf("response body = %q", got)
	}
	_, attrs := onlyCapturedRecord(t, capture)
	if attrs["error_code"] != "INVALID_REQUEST" {
		t.Fatalf("attrs = %#v", attrs)
	}

	minimal := &observabilityMinimalWriter{header: make(http.Header)}
	observeHTTPHandler(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if implementsWriter[http.Flusher](writer) || implementsWriter[http.Hijacker](writer) || implementsWriter[io.ReaderFrom](writer) || implementsWriter[http.Pusher](writer) {
			t.Error("wrapped minimal writer gained an optional interface")
		}
	}), httpObservationOptions{surface: observability.SurfaceServerPublic}).ServeHTTP(minimal, httptest.NewRequest(http.MethodGet, "/", nil))
}

func implementsWriter[T any](value any) bool {
	_, ok := value.(T)
	return ok
}

type observabilityMinimalWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *observabilityMinimalWriter) Header() http.Header { return w.header }
func (w *observabilityMinimalWriter) WriteHeader(status int) {
	w.status = status
}
func (w *observabilityMinimalWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

type observabilityAllWriter struct {
	*observabilityMinimalWriter
}

func newObservabilityAllWriter() *observabilityAllWriter {
	return &observabilityAllWriter{observabilityMinimalWriter: &observabilityMinimalWriter{header: make(http.Header)}}
}

func (*observabilityAllWriter) Flush() {}
func (*observabilityAllWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not connected")
}
func (w *observabilityAllWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.ReadFrom(reader)
}
func (*observabilityAllWriter) Push(string, *http.PushOptions) error { return nil }

func onlyCapturedRecord(t *testing.T, capture *slogCapture) (slog.Record, map[string]any) {
	t.Helper()
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.records) != 1 {
		t.Fatalf("records = %d, want 1", len(capture.records))
	}
	record := capture.records[0]
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	return record, attrs
}
