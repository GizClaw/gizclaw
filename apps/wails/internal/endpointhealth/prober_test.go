package endpointhealth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestProbeValidatesGizClawServerInfo(t *testing.T) {
	kp, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server-info" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprintf(w, `{"build_commit":"test","endpoint":"127.0.0.1:9820","ice":{"tcp":false,"udp":true},"protocol":"gizclaw-webrtc","public_key":%q,"server_time":1,"signaling_path":"/webrtc/v1/offer"}`, kp.Public.String())
	}))
	defer server.Close()
	prober := New()
	result := prober.Probe(context.Background(), strings.TrimPrefix(server.URL, "http://"))
	if result.State != Reachable || result.CheckedAt == "" || result.PublicKey != kp.Public.String() {
		t.Fatalf("result = %+v", result)
	}
}

func TestProbeAllBoundsConcurrency(t *testing.T) {
	kp, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	var active, maximum int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			seen := atomic.LoadInt32(&maximum)
			if current <= seen || atomic.CompareAndSwapInt32(&maximum, seen, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = fmt.Fprintf(w, `{"build_commit":"test","endpoint":"127.0.0.1:9820","ice":{"tcp":false,"udp":true},"protocol":"gizclaw-webrtc","public_key":%q,"server_time":1,"signaling_path":"/webrtc/v1/offer"}`, kp.Public.String())
	}))
	defer server.Close()
	endpoint := strings.TrimPrefix(server.URL, "http://")
	prober := New()
	prober.Concurrency = 2
	results := prober.ProbeAll(context.Background(), []string{endpoint, endpoint, endpoint, endpoint, endpoint, endpoint})
	if len(results) != 6 || atomic.LoadInt32(&maximum) > 2 {
		t.Fatalf("results/max = %d/%d", len(results), maximum)
	}
}

func TestProbeRejectsNonGizClawResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ok":true}`)) }))
	defer server.Close()
	result := New().Probe(context.Background(), strings.TrimPrefix(server.URL, "http://"))
	if result.State != InvalidResponse {
		t.Fatalf("result = %+v", result)
	}
}

func TestProbeRejectsZeroPublicKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"endpoint":"127.0.0.1:9820","protocol":"gizclaw-webrtc","public_key":"11111111111111111111111111111111","server_time":1,"signaling_path":"/webrtc/v1/offer"}`)
	}))
	defer server.Close()
	result := New().Probe(context.Background(), strings.TrimPrefix(server.URL, "http://"))
	if result.State != InvalidResponse {
		t.Fatalf("result = %+v", result)
	}
}

func TestProbeAllHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	prober := New()
	endpoint := strings.TrimPrefix(server.URL, "http://")
	results := prober.ProbeAll(ctx, []string{endpoint})
	if len(results) != 1 || results[0].State != Unreachable {
		t.Fatalf("results = %+v", results)
	}
	if cached := prober.Get(endpoint); cached.State != Unreachable || cached.CheckedAt == "" {
		t.Fatalf("cached result = %+v", cached)
	}
}
