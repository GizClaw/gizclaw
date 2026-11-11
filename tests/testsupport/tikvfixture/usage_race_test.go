package tikvfixture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestUsageRaceRequiresBothRealRequestsAndReturnsTheirBodies(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var arrived atomic.Int64
	both := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if arrived.Add(1) == 2 {
			close(both)
		}
		select {
		case <-both:
			usageJSON(w, map[string]string{"actual_path": r.URL.Path})
		case <-r.Context().Done():
			t.Error("concurrent HTTP branch was never started")
		}
	}))
	defer upstream.Close()
	gate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	var output map[string]map[string]string
	err = usageControl(ctx, gate.URL(), "/race", map[string]any{"branches": map[string]usageHTTPBranch{
		"first":  {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/first"},
		"second": {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/second"},
	}}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if arrived.Load() != 2 || output["first"]["actual_path"] != "/first" || output["second"]["actual_path"] != "/second" {
		t.Fatalf("real HTTP branches were replaced: %#v", output)
	}
}

func TestUsageRaceDoesNotReplaceFailedBranchWithSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			http.Error(w, "actual upstream failure", http.StatusServiceUnavailable)
			return
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()
	gate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	err = usageControl(ctx, gate.URL(), "/race", map[string]any{"branches": map[string]usageHTTPBranch{
		"fail":    {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/fail"},
		"blocked": {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/blocked"},
	}}, new(any))
	if err == nil {
		t.Fatal("HTTP failure was silently converted to success")
	}
	if ctx.Err() != nil {
		t.Fatal("failed branch did not cancel and join its sibling")
	}
}
