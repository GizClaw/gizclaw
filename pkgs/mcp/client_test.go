package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServer(t *testing.T, server *mcp.Server, wrap func(http.Handler) http.Handler) string {
	t.Helper()
	handler := http.Handler(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true, PropagateRequestCancellation: true}))
	if wrap != nil {
		handler = wrap(handler)
	}
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	return httpServer.URL
}

func dialTest(t *testing.T, config Config) *Client {
	t.Helper()
	client, err := Dial(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	return client
}

func testCache(t *testing.T, client *Client, options CacheOptions) *ResourceCache {
	t.Helper()
	cache, err := NewResourceCache(client, options)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func textResource(uri, text string) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "text/plain", Text: text}}}
}

func TestClientToolsAndResources(t *testing.T) {
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{PageSize: 1, SupportedProtocolVersions: []string{protocol}})
			var calls atomic.Int32
			for _, name := range []string{"echo", "other"} {
				server.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					calls.Add(1)
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(request.Params.Arguments)}}, StructuredContent: map[string]any{"ok": true}}, nil
				})
			}
			server.AddResource(&mcp.Resource{URI: "fixture:///text", Name: "text"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return textResource(request.Params.URI, "hello"), nil
			})
			server.AddResource(&mcp.Resource{URI: "fixture:///binary", Name: "binary"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, MIMEType: "application/octet-stream", Blob: []byte{0, 1, 255}}}}, nil
			})
			for _, template := range []string{"dynamic:///{name}", "other:///{name}"} {
				server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: template, Name: template}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
					return textResource(request.Params.URI, "dynamic"), nil
				})
			}
			headers := http.Header{"Authorization": {"Bearer fixture-token"}}
			endpoint := testServer(t, server, func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer fixture-token" {
						http.Error(w, "unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
				})
			})
			client := dialTest(t, Config{Endpoint: endpoint, Headers: headers})
			headers.Set("Authorization", "changed-by-caller")
			tools, err := client.ListTools(t.Context())
			if err != nil || len(tools) != 2 {
				t.Fatalf("tools = %v, %v", tools, err)
			}
			for range 2 {
				result, err := client.CallTool(t.Context(), "echo", json.RawMessage(`{"text":"hi"}`))
				if err != nil || result.IsError || result.Content[0].(*mcp.TextContent).Text != `{"text":"hi"}` {
					t.Fatalf("tool result = %v, %v", result, err)
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("tool calls cached or repeated: %d", calls.Load())
			}
			resources, err := client.ListResources(t.Context())
			if err != nil || len(resources) != 2 {
				t.Fatalf("resources = %v, %v", resources, err)
			}
			templates, err := client.ListResourceTemplates(t.Context())
			if err != nil || len(templates) != 2 {
				t.Fatalf("templates = %v, %v", templates, err)
			}
			result, err := client.ReadResource(t.Context(), "fixture:///binary")
			if err != nil || string(result.Contents[0].Blob) != string([]byte{0, 1, 255}) {
				t.Fatalf("binary result = %v, %v", result, err)
			}
			result, err = client.ReadResource(t.Context(), "dynamic:///chosen")
			if err != nil || result.Contents[0].Text != "dynamic" {
				t.Fatalf("template result = %v, %v", result, err)
			}
			if _, err := client.ReadResource(t.Context(), "fixture:///missing"); err == nil {
				t.Fatal("missing resource succeeded")
			}
			for _, arguments := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`1`), json.RawMessage(`{`)} {
				if _, err := client.CallTool(t.Context(), "echo", arguments); err == nil {
					t.Fatalf("accepted arguments %s", arguments)
				}
			}
			if calls.Load() != 2 {
				t.Fatal("invalid arguments reached server")
			}
		})
	}
}

func TestClientFailureSemantics(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "reject", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "rejected"}}}, nil
	})
	client := dialTest(t, Config{Endpoint: testServer(t, server, nil)})
	result, err := client.CallTool(t.Context(), "reject", nil)
	if err != nil || !result.IsError {
		t.Fatalf("tool error lost: %v, %v", result, err)
	}
	if _, err := client.ListResources(t.Context()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported resources: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListTools(t.Context()); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed client: %v", err)
	}
}

func TestClientCancellationAndClose(t *testing.T) {
	for _, action := range []string{"cancel", "timeout", "close"} {
		t.Run(action, func(t *testing.T) {
			entered := make(chan struct{})
			server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			server.AddTool(&mcp.Tool{Name: "block", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				close(entered)
				<-ctx.Done()
				return nil, ctx.Err()
			})
			config := Config{Endpoint: testServer(t, server, nil)}
			if action == "timeout" {
				config.RequestTimeout = 300 * time.Millisecond
			}
			client := dialTest(t, config)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := client.CallTool(ctx, "block", nil); done <- err }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("tool never entered")
			}
			switch action {
			case "cancel":
				cancel()
			case "close":
				if err := client.Close(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("blocked operation succeeded")
				}
				if action == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel error = %v", err)
				}
				if action == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("timeout error = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("operation did not stop")
			}
		})
	}
}

func TestDialContextDoesNotOwnSession(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}})
	endpoint := testServer(t, server, nil)
	ctx, cancel := context.WithCancel(t.Context())
	client, err := Dial(ctx, Config{Endpoint: endpoint})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	cancel()
	if _, err := client.ListTools(t.Context()); err != nil {
		t.Fatalf("dial context closed session: %v", err)
	}
}

func TestClientResponseLimits(t *testing.T) {
	for _, sse := range []bool{false, true} {
		t.Run(fmt.Sprintf("sse=%v", sse), func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			server.AddResource(&mcp.Resource{URI: "fixture:///large", Name: "large"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return textResource(request.Params.URI, strings.Repeat("x", 4096)), nil
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: !sse})
			httpServer := httptest.NewServer(handler)
			t.Cleanup(httpServer.Close)
			client := dialTest(t, Config{Endpoint: httpServer.URL, MaxResponseBytes: 2048})
			_, err := client.ReadResource(t.Context(), "fixture:///large")
			if err == nil {
				t.Fatal("oversized resource succeeded")
			}
			if !sse {
				if !errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("size error lost: %v", err)
				}
				if _, err := client.ListResources(t.Context()); err != nil {
					t.Fatalf("oversized response poisoned session: %v", err)
				}
			}
		})
	}
}

func TestClientPaginationGuards(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{PageSize: 1})
	for _, uri := range []string{"fixture:///a", "fixture:///b"} {
		server.AddResource(&mcp.Resource{URI: uri, Name: uri}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return textResource(request.Params.URI, "ok"), nil
		})
	}
	endpoint := testServer(t, server, nil)
	client := dialTest(t, Config{Endpoint: endpoint, MaxPages: 1})
	if _, err := client.ListResources(t.Context()); err == nil || !strings.Contains(err.Error(), "page limit") {
		t.Fatalf("page bound: %v", err)
	}
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			if method == "resources/list" {
				return &mcp.ListResourcesResult{Resources: []*mcp.Resource{{URI: "fixture:///a", Name: "a"}}, NextCursor: "loop"}, nil
			}
			return next(ctx, method, request)
		}
	})
	client = dialTest(t, Config{Endpoint: endpoint})
	if _, err := client.ListResources(t.Context()); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("cursor loop: %v", err)
	}
}

func TestConfigAndTransportBoundaries(t *testing.T) {
	for _, config := range []Config{
		{Endpoint: "file:///socket"}, {Endpoint: "http://user:secret@example.com"}, {Endpoint: "http://example.com/#fragment"},
		{Endpoint: "http://example.com", RequestTimeout: -1}, {Endpoint: "http://example.com", Headers: http.Header{"Mcp-Session-Id": {"override"}}},
		{Endpoint: "http://example.com", Headers: http.Header{"Authorization": {"line\nbreak"}}},
	} {
		if client, err := Dial(t.Context(), config); err == nil {
			_ = client.Close()
			t.Fatal("invalid config accepted")
		}
	}
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	if client, err := Dial(t.Context(), Config{Endpoint: redirect.URL}); err == nil {
		_ = client.Close()
		t.Fatal("redirect succeeded")
	}
	if targetRequests.Load() != 0 {
		t.Fatal("redirect followed")
	}
}

func TestCacheSnapshotsAndOfflineReads(t *testing.T) {
	var version atomic.Int32
	version.Store(1)
	var reads atomic.Int32
	var fail atomic.Bool
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	handler := func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		reads.Add(1)
		if fail.Load() {
			return nil, errors.New("source unavailable")
		}
		return textResource(request.Params.URI, fmt.Sprintf("v%d", version.Load())), nil
	}
	server.AddResource(&mcp.Resource{URI: "fixture:///one", Name: "one"}, handler)
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "dynamic:///{name}", Name: "dynamic"}, handler)
	client := dialTest(t, Config{Endpoint: testServer(t, server, nil)})
	cache := testCache(t, client, CacheOptions{})
	if cache.Snapshot().Version() != 0 {
		t.Fatal("new cache already published")
	}
	if _, err := cache.Snapshot().ListResources(); !errors.Is(err, ErrNotCached) {
		t.Fatalf("cold catalog: %v", err)
	}
	if _, err := cache.ListResources(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadResource(t.Context(), "dynamic:///chosen"); err != nil {
		t.Fatal(err)
	}
	old := cache.Snapshot()
	for range 10 {
		result, err := cache.ReadResource(t.Context(), "fixture:///one")
		if err != nil || result.Contents[0].Text != "v1" {
			t.Fatalf("local read = %v, %v", result, err)
		}
		result.Contents[0].Text = "caller mutation"
	}
	if reads.Load() != 2 {
		t.Fatalf("cache hit contacted source: %d", reads.Load())
	}
	catalog, err := old.ListResources()
	if err != nil {
		t.Fatal(err)
	}
	catalog[0].Name = "caller mutation"
	catalog, _ = old.ListResources()
	if catalog[0].Name != "one" {
		t.Fatal("catalog mutated shared snapshot")
	}
	version.Store(2)
	if err := cache.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{"fixture:///one", "dynamic:///chosen"} {
		before, _ := old.ReadResource(uri)
		after, err := cache.Snapshot().ReadResource(uri)
		if err != nil || before.Contents[0].Text != "v1" || after.Contents[0].Text != "v2" {
			t.Fatalf("snapshot versions: %v, %v, %v", before, after, err)
		}
	}
	if cache.Snapshot().Version() <= old.Version() || cache.Snapshot().RefreshedAt().IsZero() {
		t.Fatal("refresh metadata did not advance")
	}
	stable := cache.Snapshot()
	fail.Store(true)
	if err := cache.Refresh(t.Context()); err == nil {
		t.Fatal("refresh unexpectedly succeeded")
	}
	if cache.Snapshot().Version() != stable.Version() {
		t.Fatal("failed refresh published partial state")
	}
	fail.Store(false)
	server.RemoveResources("fixture:///one")
	if err := cache.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Snapshot().ReadResource("fixture:///one"); !errors.Is(err, ErrNotCached) {
		t.Fatalf("removed resource remained: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := cache.ReadResource(t.Context(), "dynamic:///chosen")
	if err != nil || result.Contents[0].Text != "v2" {
		t.Fatalf("offline read: %v, %v", result, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := cache.ReadResource(ctx, "dynamic:///chosen"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled hit: %v", err)
	}
}

func TestCacheSharedMissAndCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var reads atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddResource(&mcp.Resource{URI: "fixture:///one", Name: "one"}, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if reads.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			return textResource(request.Params.URI, "shared"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	client := dialTest(t, Config{Endpoint: testServer(t, server, nil)})
	cache := testCache(t, client, CacheOptions{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceled := make(chan error, 1)
	go func() { _, err := cache.ReadResource(ctx, "fixture:///one"); canceled <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("read never entered")
	}
	cancel()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter cancellation: %v", err)
	}
	var waiters sync.WaitGroup
	errorsCh := make(chan error, 32)
	for range 32 {
		waiters.Go(func() {
			result, err := cache.ReadResource(t.Context(), "fixture:///one")
			if err == nil && result.Contents[0].Text != "shared" {
				err = errors.New("wrong resource")
			}
			errorsCh <- err
		})
	}
	close(release)
	waiters.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if reads.Load() != 1 {
		t.Fatalf("duplicate remote loads: %d", reads.Load())
	}
}

func TestCacheLimitsAndAuthorizationIsolation(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	for _, uri := range []string{"fixture:///one", "fixture:///two"} {
		server.AddResource(&mcp.Resource{URI: uri, Name: uri}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return textResource(request.Params.URI, "body"), nil
		})
	}
	endpoint := testServer(t, server, nil)
	client := dialTest(t, Config{Endpoint: endpoint})
	for _, options := range []CacheOptions{{MaxEntries: 1}, {MaxBytes: 8}} {
		cache := testCache(t, client, options)
		if err := cache.Refresh(t.Context()); !errors.Is(err, ErrCacheFull) {
			t.Fatalf("cache bound = %v", err)
		}
		if cache.Snapshot().Version() != 0 {
			t.Fatal("over-budget refresh was published")
		}
	}
	cache := testCache(t, client, CacheOptions{MaxEntries: 1})
	if _, err := cache.ReadResource(t.Context(), "fixture:///one"); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.ReadResource(t.Context(), "fixture:///two"); !errors.Is(err, ErrCacheFull) {
		t.Fatalf("lazy bound: %v", err)
	}
	other := testCache(t, dialTest(t, Config{Endpoint: endpoint, Headers: http.Header{"Authorization": {"different-context"}}}), CacheOptions{})
	if _, err := other.Snapshot().ReadResource("fixture:///one"); !errors.Is(err, ErrNotCached) {
		t.Fatalf("cache crossed clients: %v", err)
	}
}

func TestCacheRunLifecycle(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddResource(&mcp.Resource{URI: "fixture:///one", Name: "one"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return nil, errors.New("offline")
	})
	client := dialTest(t, Config{Endpoint: testServer(t, server, nil)})
	cache := testCache(t, client, CacheOptions{})
	if err := cache.Run(t.Context(), 0, nil); err == nil {
		t.Fatal("zero interval accepted")
	}
	if err := cache.Run(t.Context(), time.Second, nil); err == nil {
		t.Fatal("refresh failure swallowed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reported := make(chan error, 2)
	done := make(chan error, 1)
	go func() {
		done <- cache.Run(ctx, 10*time.Millisecond, func(err error) {
			select {
			case reported <- err:
			default:
			}
		})
	}()
	for range 2 {
		select {
		case <-reported:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not retry")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("worker stopped: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}
