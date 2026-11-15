package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCacheRefreshStopsSchedulingAfterFailure(t *testing.T) {
	for _, failure := range []string{"read error", "cache budget"} {
		t.Run(failure, func(t *testing.T) {
			const resourceCount = 64
			entered := make(chan struct{})
			release := make(chan struct{})
			releaseReads := sync.OnceFunc(func() { close(release) })
			defer releaseReads()
			var reads atomic.Int32
			server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			for index := range resourceCount {
				uri := fmt.Sprintf("fixture:///%03d", index)
				server.AddResource(&mcp.Resource{URI: uri, Name: fmt.Sprintf("resource-%03d", index)}, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
					if reads.Add(1) == 8 {
						close(entered)
					}
					if request.Params.URI == "fixture:///000" {
						// Keep the first batch occupied so the scheduler is waiting
						// for a slot when this resource causes cancellation.
						select {
						case <-entered:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
						if failure == "read error" {
							return nil, errors.New("read rejected")
						}
						return textResource(request.Params.URI, strings.Repeat("x", 14<<10)), nil
					}
					select {
					case <-release:
						return textResource(request.Params.URI, "released"), nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				})
			}
			client := dialTest(t, Config{Endpoint: testServer(t, server, nil)})
			cache := testCache(t, client, CacheOptions{MaxBytes: 16 << 10})
			done := make(chan error, 1)
			go func() { done <- cache.Refresh(t.Context()) }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("refresh succeeded")
				}
				if failure == "cache budget" && !errors.Is(err, ErrCacheFull) {
					t.Fatalf("budget error = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("failed refresh did not stop")
			}
			// DoChan registers a flight synchronously. Joining a flight for the
			// final URI proves that canceled refresh work was wrongly admitted,
			// even if its network goroutine has not reached the server yet.
			probe := cache.loads.DoChan("fixture:///063", func() (any, error) { return "not scheduled", nil })
			releaseReads()
			select {
			case result := <-probe:
				if result.Shared || result.Val != "not scheduled" {
					t.Fatal("refresh started a shared load after cancellation")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("load probe did not finish")
			}
			if reads.Load() != 8 {
				t.Fatalf("refresh started %d reads, want only the initial eight", reads.Load())
			}
			if cache.Snapshot().Version() != 0 {
				t.Fatal("failed refresh published partial state")
			}
		})
	}
}
