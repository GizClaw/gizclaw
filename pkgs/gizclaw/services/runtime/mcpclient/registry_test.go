package mcpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegistrySharesSourcesAndReleasesLeases(t *testing.T) {
	var reads atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddResource(&mcp.Resource{URI: "fixture:///one", Name: "one"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		reads.Add(1)
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, Text: "shared"}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	registry := NewRegistry()
	defer registry.Close()
	binding := apitypes.RuntimeProfileMCPServer{Endpoint: httpServer.URL}
	first, a, err := registry.Acquire(t.Context(), "profile", "knowledge", binding)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	second, b, err := registry.Acquire(t.Context(), "profile", "knowledge", binding)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if first != second {
		t.Fatal("Workflow sources are not shared")
	}
	if err := first.Prepare(t.Context(), true, false); err != nil {
		t.Fatal(err)
	}
	if err := second.Prepare(t.Context(), true, false); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 1 {
		t.Fatalf("source read %d times", reads.Load())
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if first.ctx.Err() != nil {
		t.Fatal("closing one lease closed another Workflow's source")
	}
	if result, err := second.Cache.Snapshot().ReadResource("fixture:///one"); err != nil || result.Contents[0].Text != "shared" {
		t.Fatalf("local read = %v, %v", result, err)
	}
	otherProfile, lease, err := registry.Acquire(t.Context(), "different-profile", "knowledge", binding)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if otherProfile == first {
		t.Fatal("source crossed profile boundary")
	}
	headers := map[string]string{"Authorization": "new-secret"}
	binding.Headers = &headers
	rotated, rotatedLease, err := registry.Acquire(t.Context(), "profile", "knowledge", binding)
	if err != nil {
		t.Fatal(err)
	}
	defer rotatedLease.Close()
	if rotated == first {
		t.Fatal("rotated credentials reused old source")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if first.ctx.Err() == nil {
		t.Fatal("last lease did not stop source")
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if otherProfile.ctx.Err() == nil || rotated.ctx.Err() == nil {
		t.Fatal("registry did not cancel all sources")
	}
	if _, _, err := registry.Acquire(t.Context(), "profile", "knowledge", binding); err == nil {
		t.Fatal("closed registry accepted acquire")
	}
}

func TestRegistryConcurrentAcquireAndFailedInitialization(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			var connects atomic.Int32
			entered := make(chan struct{})
			release := make(chan struct{})
			releaseOnce := sync.OnceFunc(func() { close(release) })
			defer releaseOnce()
			server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if connects.Add(1) == 1 {
					close(entered)
				}
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
				if fail {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				handler.ServeHTTP(w, request)
			}))
			defer httpServer.Close()
			registry := NewRegistry()
			defer registry.Close()
			binding := apitypes.RuntimeProfileMCPServer{Endpoint: httpServer.URL}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			first := make(chan error, 1)
			go func() {
				_, lease, err := registry.Acquire(ctx, "p", "a", binding)
				if lease != nil {
					_ = lease.Close()
				}
				first <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("connect did not start")
			}
			// A second reservation holds the shared source while one waiter leaves.
			second := make(chan error, 1)
			go func() {
				_, lease, err := registry.Acquire(t.Context(), "p", "a", binding)
				if lease != nil {
					_ = lease.Close()
				}
				second <- err
			}()
			releaseOnce()
			cancel()
			if err := <-first; err != nil && !errors.Is(err, context.Canceled) && !fail {
				t.Fatal(err)
			}
			if err := <-second; (err != nil) != fail {
				t.Fatalf("initialization = %v", err)
			}
		})
	}
}

func TestRegistryRefreshRetainsCachedVersionOnFailure(t *testing.T) {
	var version atomic.Int32
	version.Store(1)
	refreshed := make(chan struct{}, 4)
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddResource(&mcp.Resource{URI: "fixture:///one", Name: "one"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		value := version.Load()
		if value == 0 {
			select {
			case refreshed <- struct{}{}:
			default:
			}
			return nil, errors.New("offline")
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, Text: "version"}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	registry := NewRegistry()
	defer registry.Close()
	source, lease, err := registry.Acquire(t.Context(), "profile", "knowledge", apitypes.RuntimeProfileMCPServer{Endpoint: httpServer.URL, RefreshInterval: new("1s")})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := source.Prepare(t.Context(), true, false); err != nil {
		t.Fatal(err)
	}
	before := source.Cache.Snapshot()
	version.Store(0)
	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("background refresh did not run")
	}
	if current := source.Cache.Snapshot(); current.Version() != before.Version() {
		t.Fatal("failed refresh replaced local version")
	}
	if _, err := source.Cache.Snapshot().ReadResource("fixture:///one"); err != nil {
		t.Fatal(err)
	}
}
