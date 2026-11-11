package tikvfixture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	client := &usageHTTP{edges: []string{upstream.URL}, controls: []string{gate.URL()}}
	gate.services.Store(&UsageServices{http: client})
	var output struct {
		First      map[string]string `json:"first"`
		Second     map[string]string `json:"second"`
		HTTPStatus map[string]int    `json:"http_status"`
	}
	err = client.control(ctx, gate.URL(), "/race", map[string]any{"branches": map[string]usageHTTPBranch{
		"first":  {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/gizclaw/v1/friends/first"},
		"second": {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/gizclaw/v1/friends/second"},
	}}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if arrived.Load() != 2 || output.First["actual_path"] != "/gizclaw/v1/friends/first" || output.Second["actual_path"] != "/gizclaw/v1/friends/second" || output.HTTPStatus["first"] != http.StatusOK || output.HTTPStatus["second"] != http.StatusOK {
		t.Fatalf("real HTTP branches were replaced: %#v", output)
	}
}

func TestUsageRaceDoesNotReplaceFailedBranchWithSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gizclaw/v1/friends/fail" {
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
	client := &usageHTTP{edges: []string{upstream.URL}, controls: []string{gate.URL()}}
	gate.services.Store(&UsageServices{http: client})
	err = client.control(ctx, gate.URL(), "/race", map[string]any{"branches": map[string]usageHTTPBranch{
		"fail":    {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/gizclaw/v1/friends/fail"},
		"blocked": {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/gizclaw/v1/friends/blocked"},
	}}, new(any))
	if err == nil {
		t.Fatal("HTTP failure was silently converted to success")
	}
	if ctx.Err() != nil {
		t.Fatal("failed branch did not cancel and join its sibling")
	}
}

func TestUsageRaceRedirectRequiresExplicitStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		pass   bool
	}{
		{name: "default-redirect-fails"},
		{name: "explicit-redirect-retains-status-and-body", status: http.StatusFound, pass: true},
		{name: "explicit-status-must-match", status: http.StatusTemporaryRedirect},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var escaped atomic.Int64
			forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				escaped.Add(1)
				usageJSON(w, map[string]bool{"escaped": true})
			}))
			defer forbidden.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/redirect") {
					w.Header().Set("Location", forbidden.URL)
					w.WriteHeader(http.StatusFound)
				}
				usageJSON(w, map[string]string{"actual_path": r.URL.Path})
			}))
			defer upstream.Close()
			gate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			client := &usageHTTP{edges: []string{upstream.URL}, controls: []string{gate.URL()}}
			gate.services.Store(&UsageServices{http: client})
			var output struct {
				Redirect   map[string]string `json:"redirect"`
				HTTPStatus map[string]int    `json:"http_status"`
			}
			err = client.control(ctx, gate.URL(), "/race", map[string]any{"branches": map[string]usageHTTPBranch{
				"redirect": {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/gizclaw/v1/friends/redirect", Status: test.status},
				"success":  {Endpoint: upstream.URL, Method: http.MethodGet, Path: "/gizclaw/v1/friends/success"},
			}}, &output)
			if test.pass {
				if err != nil {
					t.Fatal(err)
				}
				if output.HTTPStatus["redirect"] != http.StatusFound || output.HTTPStatus["success"] != http.StatusOK || output.Redirect["actual_path"] != "/gizclaw/v1/friends/redirect" {
					t.Fatalf("actual HTTP status/body lost: %+v", output)
				}
			} else if err == nil || !strings.Contains(err.Error(), "actual HTTP status 302") {
				t.Fatalf("unexpected redirect accepted or actual status lost: %v", err)
			}
			if escaped.Load() != 0 {
				t.Fatal("coordination followed a redirect")
			}
		})
	}
}
