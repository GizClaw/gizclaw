package tikvfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/tikvrpc"
)

func TestUsageHTTPFixedFixtureOrigins(t *testing.T) {
	var client *usageHTTP
	for _, fixture := range []struct {
		origin string
		path   string
	}{
		{"http://server-a:9791", "/invoke"},
		{"http://server-b:9791", "/after"},
		{"http://edge-a:9821", "/gizclaw/v1/friend-groups/test-group/invite-token"},
		{"http://edge-b:9821", "/gizclaw/v1/api-keys/self"},
	} {
		t.Run(fixture.origin, func(t *testing.T) {
			req, err := client.request(t.Context(), fixture.origin, http.MethodPost, fixture.path, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			if req.URL.Scheme+"://"+req.URL.Host != fixture.origin || req.URL.Path != fixture.path || req.URL.RawQuery != "" || req.URL.User != nil {
				t.Fatalf("trusted origin/path changed: %s", req.URL)
			}
		})
	}
	for _, origin := range []string{
		"http://127.0.0.1:9791", "http://localhost:9791", "http://server-a:9820", "http://tikv-pd:2379",
		"http://169.254.169.254", "http://server-a:9791.evil", "http://server-a:9791/", "http://server-a:9791?ignored",
		"http://server-a:9791#ignored", "http://user@server-a:9791", "http://server-a:9791@evil", "http://server-a.:9791",
		"http://SERVER-A:9791", "https://server-a:9791", "//server-a:9791", "http://server-a:09791", "http://server-a%3a9791",
		"http://server-a:9791\\@evil", "http://server-a:9791%2f@evil", "http://[::1]:9791", "http://2130706433:9791",
	} {
		t.Run(origin, func(t *testing.T) {
			if _, err := client.request(t.Context(), origin, http.MethodPost, "/invoke", nil, false); err == nil {
				t.Fatal("untrusted fixture origin was accepted")
			}
		})
	}
}

func TestUsageHTTPRejectsUntrustedDestinationsBeforeIO(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		usageJSON(w, map[string]bool{"actual": true})
	}))
	defer upstream.Close()
	client := &usageHTTP{edges: []string{upstream.URL}}
	for _, path := range []string{
		"", "gizclaw/v1/friends", "//evil/gizclaw/v1/friends", "http://evil/gizclaw/v1/friends",
		"/gizclaw/v1/friends?url=http://evil", "/gizclaw/v1/friends#fragment", "/gizclaw/v1/friends/../api-keys",
		"/gizclaw/v1/friends/.", "/gizclaw/v1/friends//evil", "/gizclaw/v1/friends/", "/gizclaw/v1/friends\\evil",
		"/gizclaw/v1/friends/%2f%2fevil", "/gizclaw/v1/friends/%5Cevil", "/gizclaw/v1/friends/%2e%2e",
		"/gizclaw/v1/friends/%252e%252e", "/gizclaw/v1/friends/name%3fnext=http://evil", "/gizclaw/v1/friends/name%23fragment",
		"/gizclaw/v1/friends/name\r\nHost: evil", "/gizclaw/v1/friends/name\x00", "/gizclaw/v1/friends/name with space",
		"/invoke", "/server-info", "/gizclaw/v1/other", "/gizclaw/v1/friend-groups/name/other",
	} {
		t.Run(path, func(t *testing.T) {
			if _, err := client.forward(ctx, UsageAction{Endpoint: upstream.URL, Method: http.MethodGet, Path: path}); err == nil {
				t.Fatal("untrusted product path was forwarded")
			}
		})
	}
	for _, endpoint := range []string{upstream.URL + "/", upstream.URL + "?", upstream.URL + "#", "http://example.invalid", "http://169.254.169.254"} {
		if _, err := client.forward(ctx, UsageAction{Endpoint: endpoint, Method: http.MethodGet, Path: "/gizclaw/v1/friends"}); err == nil {
			t.Errorf("untrusted origin %q was forwarded", endpoint)
		}
	}
	if err := client.control(ctx, upstream.URL, "/invoke", map[string]bool{}, new(any)); err == nil {
		t.Fatal("control request reached a product origin")
	}
	client = &usageHTTP{controls: []string{upstream.URL}}
	for _, path := range []string{"/invoke/", "//evil", "/invoke?path=/release", "/invoke#fragment", "/%69nvoke", "/invoke%2f..%2frelease", "/gizclaw/v1/friends", "/reset"} {
		if err := client.control(ctx, upstream.URL, path, map[string]bool{}, new(any)); err == nil {
			t.Errorf("untrusted control path %q was forwarded", path)
		}
	}
	if _, err := client.forward(ctx, UsageAction{Endpoint: upstream.URL, Method: http.MethodGet, Path: "/invoke"}); err == nil {
		t.Fatal("non-POST control request was forwarded")
	}
	if err := client.control(ctx, upstream.URL+"/", "/invoke", map[string]bool{}, new(any)); err == nil {
		t.Fatal("untrusted control origin was forwarded")
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected requests performed %d upstream calls", calls.Load())
	}
}

func TestUsageHTTPPreservesRedirectResponsesWithoutFollowing(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var escaped atomic.Int64
			forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				escaped.Add(1)
				usageJSON(w, map[string]bool{"escaped": true})
			}))
			defer forbidden.Close()
			var location atomic.Value
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/escaped" {
					escaped.Add(1)
					return
				}
				w.Header().Set("Location", location.Load().(string))
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"actual":"redirect"}`))
			}))
			defer upstream.Close()
			for _, target := range []string{forbidden.URL + "/escaped", "//" + strings.TrimPrefix(forbidden.URL, "http://") + "/escaped", "/escaped"} {
				location.Store(target)
				client := &usageHTTP{edges: []string{upstream.URL}}
				value, err := client.forward(ctx, UsageAction{Endpoint: upstream.URL, Method: http.MethodPost, Path: "/gizclaw/v1/friends", Body: map[string]bool{"actual": true}})
				if err != nil {
					t.Fatal(err)
				}
				result := value.(map[string]any)
				if result["status"] != status || result["body"].(map[string]any)["actual"] != "redirect" {
					t.Fatalf("actual redirect changed: %#v", result)
				}
				client = &usageHTTP{controls: []string{upstream.URL}}
				err = client.control(ctx, upstream.URL, "/invoke", map[string]bool{"actual": true}, new(any))
				if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("status %d", status)) {
					t.Fatalf("actual control redirect status lost: %v", err)
				}
			}
			if escaped.Load() != 0 {
				t.Fatalf("redirect escape performed %d requests", escaped.Load())
			}
		})
	}
}

func TestUsageHTTPPropagatesCancellation(t *testing.T) {
	for _, control := range []bool{false, true} {
		t.Run(fmt.Sprint(control), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			arrived := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(arrived)
				select {
				case <-r.Context().Done():
				case <-ctx.Done():
				}
			}))
			defer upstream.Close()
			request, stop := context.WithCancel(ctx)
			defer stop()
			done := make(chan error, 1)
			go func() {
				if control {
					client := &usageHTTP{controls: []string{upstream.URL}}
					done <- client.control(request, upstream.URL, "/wait", map[string]string{"id": "held"}, new(any))
				} else {
					client := &usageHTTP{edges: []string{upstream.URL}}
					_, err := client.forward(request, UsageAction{Endpoint: upstream.URL, Method: http.MethodGet, Path: "/gizclaw/v1/friends"})
					done <- err
				}
			}()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			stop()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("request cancellation lost: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

func TestUsageCoordinationRejectsUntrustedOriginsBeforeIO(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		usageJSON(w, map[string]bool{"actual": true})
	}))
	defer upstream.Close()
	gate := &UsageGate{}
	gate.services.Store(&UsageServices{http: &usageHTTP{controls: []string{upstream.URL}}})
	for _, coordination := range []struct {
		path   string
		input  any
		status int
	}{
		{"/race", map[string]any{"branches": map[string]usageHTTPBranch{
			"trusted": {Endpoint: upstream.URL, Method: http.MethodPost, Path: "/invoke"},
			"escape":  {Endpoint: upstream.URL + "/", Method: http.MethodPost, Path: "/invoke"},
		}}, http.StatusBadGateway},
		{"/after", usageAfter{WaitEndpoint: upstream.URL, ReleaseEndpoint: upstream.URL + "/", Gate: "held", Actions: []UsageAction{{Operation: "identity.new"}}}, http.StatusGatewayTimeout},
		{"/after", usageAfter{WaitEndpoint: upstream.URL + "/", ReleaseEndpoint: upstream.URL, Gate: "held", Actions: []UsageAction{{Operation: "identity.new"}}}, http.StatusGatewayTimeout},
	} {
		raw, err := json.Marshal(coordination.input)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequestWithContext(ctx, http.MethodPost, coordination.path, bytes.NewReader(raw))
		response := httptest.NewRecorder()
		gate.control(response, request)
		if response.Code != coordination.status {
			t.Fatalf("%s status = %d, want %d: %s", coordination.path, response.Code, coordination.status, response.Body)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid coordination performed %d upstream calls", calls.Load())
	}
}

func TestUsageAfterReleasesActualNativeResponseWithHTTPResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/gizclaw/v1/friends/fixture-peer" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("actual product HTTP request changed")
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"ACTUAL_CONFLICT"}}`))
	}))
	defer upstream.Close()
	native := &gateTransport{response: &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{Value: []byte("actual")}}, requests: make(chan *tikvrpc.Request, 1)}
	waitGate, err := StartUsageGate(native, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer waitGate.Close()
	actionGate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer actionGate.Close()
	client := &usageHTTP{controls: []string{waitGate.URL(), actionGate.URL()}, edges: []string{upstream.URL}}
	waitGate.services.Store(&UsageServices{http: client})
	actionGate.services.Store(&UsageServices{http: client})
	if err := HTTP(ctx, http.MethodPost, waitGate.URL()+"/arm", UsageRule{ID: "held", Key: []string{"held"}, Commands: []string{"Get"}, Stage: "response"}, nil); err != nil {
		t.Fatal(err)
	}
	request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Key: []byte("namespace:held"), Version: 97})
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, err := waitGate.SendRequest(ctx, "center:20160", request, time.Second)
		if err != nil || response != native.response {
			t.Errorf("actual native response changed: %p, %v", response, err)
		}
	}()
	var output struct {
		Boundary UsageObservation    `json:"boundary"`
		Actions  []usageActionResult `json:"actions"`
	}
	err = client.control(ctx, actionGate.URL(), "/after", usageAfter{
		WaitEndpoint: waitGate.URL(), Gate: "held",
		Actions: []UsageAction{{Operation: "fixture.invoke", Endpoint: waitGate.URL(), Body: UsageAction{
			Operation: "http", Endpoint: upstream.URL, Method: http.MethodDelete, Path: "/gizclaw/v1/friends/fixture-peer", Headers: map[string]string{"Authorization": "Bearer fixture-key"},
		}}},
	}, &output)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if output.Boundary.Completed != 1 || output.Boundary.LastReadVersion != 97 || !waitGate.rule("held").observation().Released || len(native.requests) != 1 || <-native.requests != request {
		t.Fatalf("native coordination evidence changed: %+v", output.Boundary)
	}
	if len(output.Actions) != 1 {
		t.Fatalf("actual HTTP result missing: %+v", output.Actions)
	}
	actual := output.Actions[0].Value.(map[string]any)
	value := actual["value"].(map[string]any)
	if actual["diagnostic"] != false || value["status"] != float64(http.StatusConflict) || value["body"].(map[string]any)["error"].(map[string]any)["code"] != "ACTUAL_CONFLICT" {
		t.Fatalf("actual product HTTP response changed: %#v", actual)
	}
}
