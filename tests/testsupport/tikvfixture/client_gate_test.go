package tikvfixture

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

type gateTransport struct {
	tikv.Client
	response *tikvrpc.Response
	err      error
	requests chan *tikvrpc.Request
	closed   atomic.Int64
}

func (c *gateTransport) Close() error { c.closed.Add(1); return nil }

func (c *gateTransport) SendRequest(_ context.Context, _ string, req *tikvrpc.Request, _ time.Duration) (*tikvrpc.Response, error) {
	c.requests <- req
	return c.response, c.err
}

func (c *gateTransport) SendRequestAsync(ctx context.Context, addr string, req *tikvrpc.Request, cb async.Callback[*tikvrpc.Response]) {
	resp, err := c.SendRequest(ctx, addr, req, time.Second)
	cb.Invoke(resp, err)
}

func TestClientGateForwardsNativeRequestResponseAndError(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "async"}[asynchronous], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			native := &gateTransport{response: &tikvrpc.Response{Resp: &kvrpcpb.BatchGetResponse{}}, err: errors.New("native transport error"), requests: make(chan *tikvrpc.Request, 1)}
			gate, err := StartClientGate(native, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = gate.Close() })
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/arm", ClientGateArm{Key: []byte("target"), StartTS: 7}, nil); err != nil {
				t.Fatal(err)
			}
			request := tikvrpc.NewRequest(tikvrpc.CmdBatchGet, &kvrpcpb.BatchGetRequest{Keys: [][]byte{[]byte("target"), []byte("target")}, Version: 11})
			done := make(chan struct{})
			var response *tikvrpc.Response
			var returned error
			go func() {
				if asynchronous {
					gate.SendRequestAsync(ctx, "native:20160", request, async.NewCallback(nil, func(r *tikvrpc.Response, err error) { response, returned = r, err; close(done) }))
				} else {
					response, returned = gate.SendRequest(ctx, "native:20160", request, time.Second)
					close(done)
				}
			}()
			waitGateRead(t, ctx, gate.URL())
			if len(native.requests) != 0 {
				t.Fatal("native request bypassed the armed rendezvous")
			}
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/release/reads", nil, nil); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if response != native.response || returned != native.err || <-native.requests != request || len(request.BatchGet().Keys) != 2 || request.BatchGet().Version != 11 {
				t.Fatal("observer changed the native request, response or error")
			}
			var state ClientGateState
			if err := HTTP(ctx, http.MethodGet, gate.URL()+"/status", nil, &state); err != nil || state.Reads != 1 || state.DuplicateReads != 1 || state.InFlight != 0 {
				t.Fatalf("completed native call = %+v, %v", state, err)
			}
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/reset", nil, nil); err != nil {
				t.Fatal(err)
			}
			if err := HTTP(ctx, http.MethodGet, gate.URL()+"/status", nil, &state); err != nil || state.Armed {
				t.Fatalf("reset = %+v, %v", state, err)
			}
		})
	}
}

func TestClientGateCancellationAndCloseReleaseBlockedRequests(t *testing.T) {
	for _, closeGate := range []bool{false, true} {
		t.Run(map[bool]string{false: "caller-cancel", true: "close"}[closeGate], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			native := &gateTransport{requests: make(chan *tikvrpc.Request, 1)}
			gate, err := StartClientGate(native, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = gate.Close() })
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/arm", ClientGateArm{Key: []byte("target"), StartTS: 7}, nil); err != nil {
				t.Fatal(err)
			}
			requestCtx, stop := context.WithCancel(ctx)
			defer stop()
			done := make(chan error, 1)
			go func() {
				_, err := gate.SendRequest(requestCtx, "native:20160", tikvrpc.NewRequest(tikvrpc.CmdBatchGet, &kvrpcpb.BatchGetRequest{Keys: [][]byte{[]byte("target")}}), time.Second)
				done <- err
			}()
			waitGateRead(t, ctx, gate.URL())
			if closeGate {
				if err := gate.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				stop()
			}
			select {
			case err := <-done:
				if err == nil || (!closeGate && !errors.Is(err, context.Canceled)) {
					t.Fatalf("blocked request cancellation: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := gate.Close(); err != nil || native.closed.Load() != 1 || len(native.requests) != 0 {
				t.Fatalf("close = %v; native closes=%d requests=%d", err, native.closed.Load(), len(native.requests))
			}
		})
	}
}

func waitGateRead(t *testing.T, ctx context.Context, url string) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var state ClientGateState
		if err := HTTP(ctx, http.MethodGet, url+"/status", nil, &state); err != nil {
			t.Fatal(err)
		}
		if state.Reads > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}
