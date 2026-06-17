package cmdhttp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
)

type stringAddr string

func (a stringAddr) Network() string { return "tcp" }
func (a stringAddr) String() string  { return string(a) }

func TestUIAPIProxyReusesHealthyClient(t *testing.T) {
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		connects.Add(1)
		return fake, nil
	}, time.Second)
	defer proxy.Close()

	for i := range 2 {
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("ServeHTTP(%d) status = %d", i, rec.Code)
		}
	}
	if got := connects.Load(); got != 1 {
		t.Fatalf("connects = %d, want 1", got)
	}
	if fake.closed.Load() {
		t.Fatal("healthy client was closed")
	}
}

func TestUIAPIProxyInvalidatesTimedOutClient(t *testing.T) {
	first := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			http.Error(w, "deadline", http.StatusGatewayTimeout)
		}),
	}
	second := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		switch connects.Add(1) {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			t.Fatal("unexpected reconnect")
			return nil, errors.New("unexpected reconnect")
		}
	}, 10*time.Millisecond)
	defer proxy.Close()

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP status = %d", rec.Code)
	}
	if !first.closed.Load() {
		t.Fatal("timed out client was not closed")
	}
	if got := connects.Load(); got != 2 {
		t.Fatalf("connects = %d, want 2", got)
	}
}

func TestUIAPIProxyRetriesBadGatewayClient(t *testing.T) {
	first := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}),
	}
	second := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		switch connects.Add(1) {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			t.Fatal("unexpected reconnect")
			return nil, errors.New("unexpected reconnect")
		}
	}, time.Second)
	defer proxy.Close()

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/minimax-tenants", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP status = %d", rec.Code)
	}
	if !first.closed.Load() {
		t.Fatal("bad gateway client was not closed")
	}
	if got := connects.Load(); got != 2 {
		t.Fatalf("connects = %d, want 2", got)
	}
}

func TestUIAPIProxyDoesNotRetryNonRetryableFailure(t *testing.T) {
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		connects.Add(1)
		return fake, nil
	}, time.Second)
	defer proxy.Close()

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/credentials", strings.NewReader("{}")))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("ServeHTTP status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := connects.Load(); got != 1 {
		t.Fatalf("connects = %d, want 1", got)
	}
	if fake.closed.Load() {
		t.Fatal("non-retryable failure should not invalidate client")
	}
}

func TestUIAPIProxyStreamsNonRetryableRequests(t *testing.T) {
	releaseSecond := make(chan struct{})
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: first\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-releaseSecond
			_, _ = w.Write([]byte("data: second\n\n"))
		}),
	}
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		return fake, nil
	}, time.Second)
	defer proxy.Close()
	server := httptest.NewServer(proxy)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := server.Client().Do(req)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	var resp *http.Response
	select {
	case resp = <-respCh:
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(500 * time.Millisecond):
		close(releaseSecond)
		t.Fatal("streaming POST was buffered before response headers")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		close(releaseSecond)
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	lineCh := make(chan string, 1)
	readErrCh := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(resp.Body).ReadString('\n')
		if err != nil {
			readErrCh <- err
			return
		}
		lineCh <- line
	}()
	select {
	case line := <-lineCh:
		if line != "data: first\n" {
			close(releaseSecond)
			t.Fatalf("first line = %q", line)
		}
	case err := <-readErrCh:
		close(releaseSecond)
		t.Fatal(err)
	case <-time.After(500 * time.Millisecond):
		close(releaseSecond)
		t.Fatal("streaming POST did not flush first chunk")
	}
	close(releaseSecond)
}

func TestUIAPIProxyDirectRequestsUseCallerContext(t *testing.T) {
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte("ok"))
		}),
	}
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		return fake, nil
	}, 10*time.Millisecond)
	defer proxy.Close()

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", strings.NewReader("{}")))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("ServeHTTP status/body = %d/%q, want 200/ok", rec.Code, rec.Body.String())
	}
}

func TestUIAPIProxySetUsesExistingClient(t *testing.T) {
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		t.Fatal("proxy should reuse the preset client")
		return nil, errors.New("unexpected connect")
	}, time.Second)
	proxy.set(fake)
	defer proxy.Close()

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("ServeHTTP status/body = %d/%q", rec.Code, rec.Body.String())
	}
}

func TestUIAPIProxyInvalidatesCanceledClient(t *testing.T) {
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			http.Error(w, "canceled", http.StatusGatewayTimeout)
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		connects.Add(1)
		return fake, nil
	}, time.Second)
	defer proxy.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req.WithContext(ctx))
	if !fake.closed.Load() {
		t.Fatal("canceled client was not closed")
	}
}

func TestUIAPIProxyConnectErrorReturnsUnavailable(t *testing.T) {
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		return nil, errors.New("dial failed")
	}, time.Second)

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ServeHTTP status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("direct ServeHTTP status = %d", rec.Code)
	}
}

func TestUIAPIProxyGizCLIClientRejectsUnexpectedClient(t *testing.T) {
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		return &fakeUIAPIProxyClient{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, nil
	}, time.Second)
	defer proxy.Close()
	_, err := proxy.gizCLIClient()
	if err == nil || !strings.Contains(err.Error(), "unexpected ui client") {
		t.Fatalf("gizCLIClient error = %v", err)
	}
}

func TestUIAPIProxyInvalidateNilGizCLIClient(t *testing.T) {
	proxy := newUIAPIProxy(nil, time.Second)
	proxy.invalidateGizCLIClient(nil)
}

func TestUIAPIProxyRetryHelpers(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if !isUIAPIProxyRetryable(httptest.NewRequest(method, "/", nil)) {
			t.Fatalf("%s should be retryable", method)
		}
	}
	if isUIAPIProxyRetryable(httptest.NewRequest(http.MethodPost, "/", nil)) {
		t.Fatal("POST should not be retryable")
	}
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		if !isUIAPIProxyFailure(status) {
			t.Fatalf("%d should be retryable failure status", status)
		}
	}
	if isUIAPIProxyFailure(http.StatusInternalServerError) {
		t.Fatal("500 should not be a retryable failure status")
	}
}

func TestBufferedHTTPResponseDefaultsAndIgnoresSecondStatus(t *testing.T) {
	resp := newBufferedHTTPResponse()
	if got := resp.statusCode(); got != http.StatusOK {
		t.Fatalf("default status = %d", got)
	}
	resp.WriteHeader(http.StatusCreated)
	resp.WriteHeader(http.StatusTeapot)
	if got := resp.statusCode(); got != http.StatusCreated {
		t.Fatalf("status = %d", got)
	}
}

func TestAdminUIRedirectsLegacyWorkspaceTemplateRoutes(t *testing.T) {
	mux := http.NewServeMux()
	registerAdminUIRoutes(mux)

	for _, path := range []string{"/workspace-templates", "/workspace-templates/demo", "/ai/workspace-templates", "/ai/workspace-templates/demo"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusFound {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusFound)
		}
		if got := rec.Header().Get("Location"); got != "/ai/workflows" {
			t.Fatalf("GET %s Location = %q, want /ai/workflows", path, got)
		}
	}
}

func TestAdminUIAPIProxyRoutesIncludeAdminAndOpenAIPaths(t *testing.T) {
	mux := http.NewServeMux()
	registerAdminUIAPIProxyRoutes(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))

	for _, path := range []string{"/api/admin/peers", "/v1/models"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, rec.Code)
		}
		if got := rec.Body.String(); got != path {
			t.Fatalf("GET %s body = %q", path, got)
		}
	}
}

func TestPlayUIAPIProxyRoutesOnlyIncludeOpenAIPath(t *testing.T) {
	mux := http.NewServeMux()
	registerPlayUIAPIProxyRoutes(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/v1/models", want: http.StatusOK},
		{path: "/api/admin/peers", want: http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.want {
			t.Fatalf("GET %s status = %d, want %d", tc.path, rec.Code, tc.want)
		}
	}
}

func TestPlayUIResourceCatalogDoesNotDialClient(t *testing.T) {
	mux := http.NewServeMux()
	registerPlayUIRoutes(mux, func() (*gizcli.Client, error) {
		t.Fatal("resource catalog should not dial client")
		return nil, errors.New("unexpected dial")
	}, nil)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/peer-resources", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /peer-resources status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, resource := range []string{"workspaces", "workflows", "models", "credentials", "voices", "pets", "wallet", "wallet-transactions", "rewards"} {
		if !strings.Contains(body, resource) {
			t.Fatalf("GET /peer-resources body missing %q: %s", resource, body)
		}
	}
}

func TestPlayUIUnknownResourceDoesNotDialClient(t *testing.T) {
	mux := http.NewServeMux()
	registerPlayUIRoutes(mux, func() (*gizcli.Client, error) {
		t.Fatal("unknown resource should not dial client")
		return nil, errors.New("unexpected dial")
	}, nil)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/peer-resources/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /peer-resources/missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestPlayUIGeneratedRoutesRejectWrongMethod(t *testing.T) {
	mux := http.NewServeMux()
	registerPlayUIRoutes(mux, func() (*gizcli.Client, error) {
		t.Fatal("method mismatch should not dial client")
		return nil, errors.New("unexpected dial")
	}, nil)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/peer-resources/workspaces", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /peer-resources/workspaces status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestPlayUISpeechRouteRejectsWrongMethod(t *testing.T) {
	mux := http.NewServeMux()
	registerPlayUIRoutes(mux, func() (*gizcli.Client, error) {
		t.Fatal("method mismatch should not dial client")
		return nil, errors.New("unexpected dial")
	}, nil)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/audio/speech", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /v1/audio/speech status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}

func TestPlayUISpeechRouteReportsClientError(t *testing.T) {
	mux := http.NewServeMux()
	registerPlayUIRoutes(mux, func() (*gizcli.Client, error) {
		return nil, errors.New("offline")
	}, nil)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hi"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST /v1/audio/speech status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "offline") {
		t.Fatalf("body = %q, want offline error", rec.Body.String())
	}
}

func TestListenAndServeUIRejectsBadInputs(t *testing.T) {
	err := listenAndServeUI("", "", "test", http.NotFoundHandler(), io.Discard, nil, registerPlayUIAPIProxyRoutes, nil)
	if err == nil || !strings.Contains(err.Error(), "empty listen addr") {
		t.Fatalf("empty addr error = %v", err)
	}

	err = listenAndServeUI("", "127.0.0.1:bad-port", "test", http.NotFoundHandler(), io.Discard, nil, registerPlayUIAPIProxyRoutes, nil)
	if err == nil || !strings.Contains(err.Error(), "listen ui") {
		t.Fatalf("bad listen addr error = %v", err)
	}
}

func TestPlayOpenAIBufferedProxyRejectsUnreadableBody(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	req.Body = errReadCloser{err: errors.New("read failed")}
	playOpenAIBufferedProxy(func() (*gizcli.Client, error) {
		t.Fatal("unreadable request body should not dial client")
		return nil, errors.New("unexpected dial")
	}, nil, rec, req, "/v1/audio/speech")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "read failed") {
		t.Fatalf("body = %q, want read failed error", rec.Body.String())
	}
}

func TestPlayOpenAIBufferedProxyRetriesTransportError(t *testing.T) {
	var attempts atomic.Int32
	var invalidates atomic.Int32
	resetPlayOpenAIHTTPClient(t, func(*http.Request) (*http.Response, error) {
		switch attempts.Add(1) {
		case 1:
			return nil, errors.New("transport failed")
		case 2:
			return httpStringResponse(http.StatusOK, "audio", nil), nil
		default:
			t.Fatal("unexpected retry")
			return nil, errors.New("unexpected retry")
		}
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hi"}`))
	playOpenAIBufferedProxy(func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}, func(*gizcli.Client) {
		invalidates.Add(1)
	}, rec, req, "/v1/audio/speech")
	if rec.Code != http.StatusOK || rec.Body.String() != "audio" {
		t.Fatalf("status/body = %d/%q, want 200/audio", rec.Code, rec.Body.String())
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if got := invalidates.Load(); got != 1 {
		t.Fatalf("invalidates = %d, want 1", got)
	}
}

func TestPlayOpenAIBufferedProxyReportsRepeatedBadGateway(t *testing.T) {
	resetPlayOpenAIHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return httpStringResponse(http.StatusBadGateway, "upstream failed", nil), nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"hi"}`))
	playOpenAIBufferedProxy(func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}, nil, rec, req, "/v1/audio/speech")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestPlayOpenAIBufferedProxyForwardsBufferedSpeech(t *testing.T) {
	resetPlayOpenAIHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("path = %q, want /v1/audio/speech", r.URL.Path)
		}
		if r.URL.RawQuery != "format=mp3" {
			t.Fatalf("query = %q, want format=mp3", r.URL.RawQuery)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"input":"hi"}` {
			t.Fatalf("body = %q", body)
		}
		return httpStringResponse(http.StatusCreated, "audio-bytes", http.Header{"X-Test": []string{"ok"}}), nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech?format=mp3", strings.NewReader(`{"input":"hi"}`))
	playOpenAIBufferedProxy(func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}, nil, rec, req, "/v1/audio/speech")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len("audio-bytes")) {
		t.Fatalf("Content-Length = %q", got)
	}
	if got := rec.Header().Get("X-Test"); got != "ok" {
		t.Fatalf("X-Test = %q", got)
	}
	if rec.Body.String() != "audio-bytes" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestPlayOpenAIStreamingProxyReportsClientError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"stream_format":"sse"}`))
	playOpenAIStreamingProxy(func() (*gizcli.Client, error) {
		return nil, errors.New("offline")
	}, nil, rec, req, "/v1/audio/speech", []byte(`{"stream_format":"sse"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestPlayOpenAIStreamingProxyReportsRepeatedTransportErrors(t *testing.T) {
	resetPlayOpenAIHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	})

	var invalidates atomic.Int32
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"stream_format":"sse"}`))
	playOpenAIStreamingProxy(func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}, func(*gizcli.Client) {
		invalidates.Add(1)
	}, rec, req, "/v1/audio/speech", []byte(`{"stream_format":"sse"}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := invalidates.Load(); got != 2 {
		t.Fatalf("invalidates = %d, want 2", got)
	}
}

func TestPlayOpenAIStreamingProxyFlushesSpeechChunks(t *testing.T) {
	releaseSecond := make(chan struct{})
	resetPlayOpenAIHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !isOpenAIStreamingSpeechRequest(body) {
			t.Fatalf("request was not streaming speech: %q", body)
		}
		pr, pw := io.Pipe()
		go func() {
			_, _ = pw.Write([]byte("event: audio.delta\n\n"))
			<-releaseSecond
			_, _ = pw.Write([]byte("event: done\n\n"))
			_ = pw.Close()
		}()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       pr,
		}, nil
	})

	mux := http.NewServeMux()
	registerPlayUIRoutes(mux, func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}, nil)
	server := httptest.NewServer(mux)
	defer server.Close()

	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := server.Client().Post(server.URL+"/v1/audio/speech", "application/json", strings.NewReader(`{"input":"hi","stream_format":"sse"}`))
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	var resp *http.Response
	select {
	case resp = <-respCh:
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(500 * time.Millisecond):
		close(releaseSecond)
		t.Fatal("streaming speech response was buffered before headers")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		close(releaseSecond)
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	lineCh := make(chan string, 1)
	readErrCh := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(resp.Body).ReadString('\n')
		if err != nil {
			readErrCh <- err
			return
		}
		lineCh <- line
	}()
	select {
	case line := <-lineCh:
		if line != "event: audio.delta\n" {
			close(releaseSecond)
			t.Fatalf("first line = %q", line)
		}
	case err := <-readErrCh:
		close(releaseSecond)
		t.Fatal(err)
	case <-time.After(500 * time.Millisecond):
		close(releaseSecond)
		t.Fatal("streaming speech did not flush first chunk")
	}
	close(releaseSecond)
}

func TestPlayOpenAIStreamingProxyRetriesBadGatewayBeforeStreaming(t *testing.T) {
	var attempts atomic.Int32
	var invalidates atomic.Int32
	resetPlayOpenAIHTTPClient(t, func(*http.Request) (*http.Response, error) {
		switch attempts.Add(1) {
		case 1:
			return httpStringResponse(http.StatusBadGateway, "bad gateway", nil), nil
		case 2:
			return httpStringResponse(http.StatusOK, "ok", nil), nil
		default:
			t.Fatal("unexpected retry")
			return nil, errors.New("unexpected retry")
		}
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"stream_format":"sse"}`))
	playOpenAIBufferedProxy(func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}, func(*gizcli.Client) {
		invalidates.Add(1)
	}, rec, req, "/v1/audio/speech")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("status/body = %d/%q, want 200/ok", rec.Code, rec.Body.String())
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if got := invalidates.Load(); got != 1 {
		t.Fatalf("invalidates = %d, want 1", got)
	}
}

func TestIsOpenAIStreamingSpeechRequest(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"stream_format":"sse"}`),
		[]byte(`{"stream_format":" SSE "}`),
	} {
		if !isOpenAIStreamingSpeechRequest(body) {
			t.Fatalf("body %s should be streaming speech", body)
		}
	}
	for _, body := range [][]byte{
		[]byte(`{"stream_format":"audio"}`),
		[]byte(`{"stream":true}`),
		[]byte(`not-json`),
	} {
		if isOpenAIStreamingSpeechRequest(body) {
			t.Fatalf("body %s should not be streaming speech", body)
		}
	}
}

func TestCopyHTTPHeaders(t *testing.T) {
	dst := http.Header{}
	src := http.Header{"X-Test": []string{"a", "b"}}
	copyHTTPHeaders(dst, src)
	if got := dst.Values("X-Test"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("headers = %v", got)
	}
}

func TestNormalizeListenAddr(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: " 8080 ", want: ":8080"},
		{in: "127.0.0.1:8080", want: "127.0.0.1:8080"},
	} {
		if got := normalizeListenAddr(tc.in); got != tc.want {
			t.Fatalf("normalizeListenAddr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDisplayURL(t *testing.T) {
	if got := displayURL(nil); got != "" {
		t.Fatalf("displayURL(nil) = %q", got)
	}
	tcpAddr := &net.TCPAddr{IP: net.IPv4zero, Port: 8080}
	if got := displayURL(tcpAddr); got != "http://127.0.0.1:8080" {
		t.Fatalf("displayURL(0.0.0.0) = %q", got)
	}
	if got := displayURL(stringAddr("bad addr")); got != "http://bad addr" {
		t.Fatalf("displayURL(bad) = %q", got)
	}
}

type fakeUIAPIProxyClient struct {
	handler http.Handler
	closed  atomic.Bool
}

func (c *fakeUIAPIProxyClient) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *fakeUIAPIProxyClient) ProxyHandler() http.Handler {
	return c.handler
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read([]byte) (int, error) {
	return 0, r.err
}

func (r errReadCloser) Close() error {
	return nil
}

func resetPlayOpenAIHTTPClient(t *testing.T, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	orig := playOpenAIHTTPClient
	playOpenAIHTTPClient = func(*gizcli.Client) *http.Client {
		return &http.Client{Transport: roundTripFunc(fn)}
	}
	t.Cleanup(func() {
		playOpenAIHTTPClient = orig
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func httpStringResponse(statusCode int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
