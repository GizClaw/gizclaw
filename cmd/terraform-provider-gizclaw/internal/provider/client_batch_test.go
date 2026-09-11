package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

func TestFlushReadsGroupsRequestsAndSkipsCanceledCaller(t *testing.T) {
	server := newFakeServer()
	server.put(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"a"},"spec":{}}`)
	server.put(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"b"},"spec":{}}`)
	client, connector := newTestClient(server)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	first := make(chan resourceReadResult, 1)
	second := make(chan resourceReadResult, 1)
	missing := make(chan resourceReadResult, 1)
	invalid := make(chan resourceReadResult, 1)
	client.pendingReads = []resourceRead{
		{ctx: canceled, kind: "Model", id: "skip", result: make(chan resourceReadResult, 1)},
		{ctx: context.Background(), kind: "Model", id: "a", result: first},
		{ctx: context.Background(), kind: "Unknown", id: "x", result: invalid},
		{ctx: context.Background(), kind: "Model", id: "missing", result: missing},
		{ctx: context.Background(), kind: "Model", id: "b", result: second},
	}
	client.flushReads()
	for id, ch := range map[string]chan resourceReadResult{"a": first, "b": second} {
		r := <-ch
		if r.err != nil || r.resource.Metadata.ID != id {
			t.Fatalf("read %s = %+v", id, r)
		}
	}
	if r := <-missing; !adminresource.IsNotFound(r.err) {
		t.Fatalf("missing read = %+v", r)
	}
	if r := <-invalid; r.err == nil || adminresource.IsNotFound(r.err) || !strings.Contains(r.err.Error(), "unknown resource kind") {
		t.Fatalf("invalid read = %+v", r)
	}
	if server.gets.Load() != 3 || connector.connects.Load() != 1 {
		t.Fatalf("gets=%d connects=%d", server.gets.Load(), connector.connects.Load())
	}
	if len(client.pendingReads) != 0 {
		t.Fatal("pending queue not drained")
	}
}

func TestRefreshCoalescesConcurrentReads(t *testing.T) {
	server := newFakeServer()
	server.put(t, modelManifest)
	client, connector := newTestClient(server)
	results := make(chan error, 10)
	for range 10 {
		go func() {
			_, err := client.refresh(context.Background(), "Model", "example-model")
			results <- err
		}()
	}
	for range 10 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if connector.connects.Load() != 1 {
		t.Fatalf("connects = %d", connector.connects.Load())
	}
}

func TestRefreshAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, connector := newTestClient(newFakeServer())
	if _, err := client.refresh(ctx, "Model", "a"); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("got %v", err)
	}
	if connector.connects.Load() != 0 {
		t.Fatal("canceled refresh opened a connection")
	}
}
