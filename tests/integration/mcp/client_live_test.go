package mcp_test

import (
	"context"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/mcp"
)

type countingTransport struct{ requests atomic.Int64 }

func (c *countingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	c.requests.Add(1)
	return http.DefaultTransport.RoundTrip(request)
}

// TestLiveResourceCache is opt-in because it talks to a deployed MCP service.
// It verifies actual client interoperability and absence of network I/O on hits.
func TestLiveResourceCache(t *testing.T) {
	endpoint := os.Getenv("GIZCLAW_MCP_LIVE_ENDPOINT")
	if endpoint == "" {
		t.Skip("set GIZCLAW_MCP_LIVE_ENDPOINT to qualify a deployed MCP service")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	transport := &countingTransport{}
	client, err := mcp.Dial(ctx, mcp.Config{Endpoint: endpoint, HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.ListTools(ctx)
	if err != nil || len(tools) == 0 {
		t.Fatalf("tool discovery: count=%d error=%v", len(tools), err)
	}
	cache, err := mcp.NewResourceCache(client, mcp.CacheOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot := cache.Snapshot()
	resources, err := snapshot.ListResources()
	if err != nil || len(resources) == 0 {
		t.Fatalf("resource discovery: count=%d error=%v", len(resources), err)
	}
	templates, err := snapshot.ListResourceTemplates()
	if err != nil {
		t.Fatal(err)
	}
	before := transport.requests.Load()
	for range 3 {
		for _, resource := range resources {
			contents, err := snapshot.ReadResource(resource.URI)
			if err != nil || len(contents.Contents) == 0 {
				t.Fatalf("cached resource %s: %v", resource.URI, err)
			}
		}
	}
	if after := transport.requests.Load(); after != before {
		t.Fatalf("local reads made %d remote requests", after-before)
	}
	t.Logf("tools=%d resources=%d templates=%d version=%d local_reads=%d network_requests_on_hits=0", len(tools), len(resources), len(templates), snapshot.Version(), 3*len(resources))
}
