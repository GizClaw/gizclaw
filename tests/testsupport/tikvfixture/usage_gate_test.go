package tikvfixture

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

func TestUsageGateRetainsOriginalResponseAndTimestamp(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "async"}[asynchronous], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			native := &gateTransport{response: &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{NotFound: true}}, requests: make(chan *tikvrpc.Request, 2)}
			gate, err := StartUsageGate(native, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = gate.Close() })
			arm := UsageRule{ID: "retained", Key: []string{"groups", "a:b"}, Commands: []string{"Get"}, Source: "learner", Stage: "response"}
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/arm", arm, nil); err != nil {
				t.Fatal(err)
			}
			request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Key: []byte("namespace:groups:a%3Ab"), Version: 97}, kvrpcpb.Context{Peer: &metapb.Peer{Id: 1, StoreId: 2, Role: metapb.PeerRole_Learner}, StaleRead: true})
			// A diagnostic cannot steal the application gate or increment its hits.
			diagnostic := context.WithValue(ctx, usageDiagnosticKey{}, true)
			if response, err := gate.SendRequest(diagnostic, "learner:20160", request, time.Second); err != nil || response != native.response {
				t.Fatalf("diagnostic forwarding: %p, %v", response, err)
			}
			var returned *tikvrpc.Response
			var returnedErr error
			done := make(chan struct{})
			go func() {
				if asynchronous {
					gate.SendRequestAsync(ctx, "learner:20160", request, async.NewCallback(nil, func(response *tikvrpc.Response, err error) { returned, returnedErr = response, err; close(done) }))
				} else {
					returned, returnedErr = gate.SendRequest(ctx, "learner:20160", request, time.Second)
					close(done)
				}
			}()
			var state UsageObservation
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/wait", map[string]string{"id": "retained"}, &state); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
				t.Fatal("real response escaped its response gate")
			default:
			}
			if state.Hits != 1 || state.Completed != 1 || state.ApplicationReads != 1 || state.DiagnosticReads != 1 || state.LearnerReads != 2 || state.CenterReads != 0 || state.LastReadVersion != 97 || state.MissingResponses != 1 {
				t.Fatalf("native evidence: %+v", state)
			}
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/release", map[string]string{"id": "retained"}, nil); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if returned != native.response || returnedErr != nil || request.Get().Version != 97 || !request.StaleRead || len(native.requests) != 2 {
				t.Fatal("probe changed request, timestamp, response or call count")
			}
			for range 2 {
				if <-native.requests != request {
					t.Fatal("probe cloned or replaced the native request")
				}
			}
		})
	}
}

func TestUsageGateCenterPauseCancellationAndReset(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	native := &gateTransport{response: &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, requests: make(chan *tikvrpc.Request, 1)}
	gate, err := StartUsageGate(native, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gate.Close() })
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/block-center", map[string]bool{"blocked": true}, nil); err != nil {
		t.Fatal(err)
	}
	request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Key: []byte("target")})
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := gate.SendRequest(cancelled, "center:20160", request, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("paused center cancellation: %v", err)
	}
	if len(native.requests) != 0 {
		t.Fatal("paused request reached center")
	}
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/reset", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if response, err := gate.SendRequest(ctx, "center:20160", request, time.Second); err != nil || response != native.response {
		t.Fatalf("resume forwarding: %p, %v", response, err)
	}
}

func TestUsageHTTPEnvelopeKeepsActualFailureBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/group" || r.Header.Get("Authorization") != "Bearer generated-fixture-key" {
			t.Error("forwarded request changed")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["display_name"] != "changed" {
			t.Errorf("request body: %v, %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"ACTUAL_CONFLICT"}}`))
	}))
	defer upstream.Close()
	value, err := usageForward(t.Context(), UsageAction{Endpoint: upstream.URL, Method: http.MethodPut, Path: "/group", Headers: map[string]string{"Authorization": "Bearer generated-fixture-key"}, Body: map[string]any{"display_name": "changed"}})
	if err != nil {
		t.Fatal(err)
	}
	result := value.(map[string]any)
	if result["status"] != http.StatusConflict || result["body"].(map[string]any)["error"].(map[string]any)["code"] != "ACTUAL_CONFLICT" {
		t.Fatalf("actual HTTP response changed: %#v", result)
	}
	upstream.Close()
	if _, err := usageForward(t.Context(), UsageAction{Endpoint: upstream.URL, Method: http.MethodGet, Path: "/group"}); err == nil {
		t.Fatal("transport failure became a successful envelope")
	}
}
