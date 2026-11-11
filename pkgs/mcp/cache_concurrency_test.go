package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCacheRefreshDoesNotBlockLocalReadsOrLoseNewResources(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var block atomic.Bool
	var reads atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddResource(&mcp.Resource{URI: "fixture:///one", Name: "one"}, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		reads.Add(1)
		if block.Load() {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return textResource(request.Params.URI, "new"), nil
		}
		return textResource(request.Params.URI, "old"), nil
	})
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "dynamic:///{name}", Name: "dynamic"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return textResource(request.Params.URI, "concurrent"), nil
	})
	client := dialTest(t, Config{Endpoint: testServer(t, server, nil)})
	cache := testCache(t, client, CacheOptions{})
	if err := cache.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	old := cache.Snapshot()
	block.Store(true)
	done := make(chan error, 1)
	go func() { done <- cache.Refresh(t.Context()) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not reach server")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := cache.ReadResource(ctx, "fixture:///one")
	if err != nil || result.Contents[0].Text != "old" {
		t.Fatalf("local read blocked on refresh: %v, %v", result, err)
	}
	if _, err := cache.ReadResource(ctx, "dynamic:///added"); err != nil {
		t.Fatalf("unrelated URI blocked: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	result, err = cache.Snapshot().ReadResource("dynamic:///added")
	if err != nil || result.Contents[0].Text != "concurrent" {
		t.Fatalf("refresh lost new resource: %v, %v", result, err)
	}
	before, _ := old.ReadResource("fixture:///one")
	after, _ := cache.Snapshot().ReadResource("fixture:///one")
	if before.Contents[0].Text != "old" || after.Contents[0].Text != "new" || reads.Load() != 2 {
		t.Fatal("refresh did not preserve snapshot boundary")
	}
}

func TestCacheConcurrentRefreshAndClientClose(t *testing.T) {
	entered := make(chan struct{})
	var reads atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddResource(&mcp.Resource{URI: "fixture:///one", Name: "one"}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if reads.Add(1) == 1 {
			close(entered)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	client := dialTest(t, Config{Endpoint: testServer(t, server, nil)})
	cache := testCache(t, client, CacheOptions{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := make(chan error, 1)
	go func() { first <- cache.Refresh(ctx) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh never started")
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("refresh waiter cancellation: %v", err)
	}
	second := make(chan error, 1)
	go func() { second <- cache.Refresh(t.Context()) }()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-second:
		if err == nil {
			t.Fatal("refresh succeeded after close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close did not stop refresh")
	}
	if cache.Snapshot().Version() != 0 {
		t.Fatal("closed refresh published data")
	}
	if reads.Load() != 1 {
		t.Fatalf("parallel refresh made %d reads", reads.Load())
	}
}

func TestClientStatefulLegacyHTTP(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{SupportedProtocolVersions: []string{"2025-11-25"}, Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	client := dialTest(t, Config{Endpoint: httpServer.URL})
	if _, err := client.ListTools(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCachesSeparateAuthorizationContexts(t *testing.T) {
	servers := make(map[string]*mcp.Server)
	for _, identity := range []string{"alpha", "bravo"} {
		server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
		server.AddResource(&mcp.Resource{URI: "fixture:///private", Name: "private"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return textResource(request.Params.URI, identity), nil
		})
		servers[identity] = server
	}
	handler := mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server { return servers[request.Header.Get("X-Identity")] }, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	for _, identity := range []string{"alpha", "bravo"} {
		client := dialTest(t, Config{Endpoint: httpServer.URL, Headers: http.Header{"X-Identity": {identity}}})
		cache := testCache(t, client, CacheOptions{})
		if err := cache.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		result, err := cache.ReadResource(t.Context(), "fixture:///private")
		if err != nil || result.Contents[0].Text != identity {
			t.Fatalf("wrong authorization context: %v, %v", result, err)
		}
	}
}
