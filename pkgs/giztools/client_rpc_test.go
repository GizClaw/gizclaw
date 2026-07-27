package giztools

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestClientRPCExecutorInvokesExactlyOnce(t *testing.T) {
	dialer := newClientRPCDialer(t, func(request *rpcapi.RPCRequest) *rpcapi.RPCResponse {
		if request.Method != rpcapi.RPCMethodClientToolInvoke {
			t.Fatalf("method = %q", request.Method)
		}
		params, err := request.Params.AsToolInvokeRequest()
		if err != nil {
			t.Fatal(err)
		}
		if params.Name != "set_volume" || params.Args["level"] != float64(7) {
			t.Fatalf("params = %#v", params)
		}
		return clientRPCResult(t, request.Id, `{"ok":true}`)
	})

	result, err := (ClientRPCExecutor{}).Invoke(
		t.Context(), dialer, "set_volume", json.RawMessage(`{"level":7}`),
	)
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("Invoke() = %s, %v", result, err)
	}
	dialer.assertCounts(t, 1, 1)
}

func TestClientRPCExecutorUnavailableIsStableAndNotRetried(t *testing.T) {
	for _, test := range []struct {
		name   string
		dialer ServiceDialer
	}{
		{name: "dial failure", dialer: &clientRPCDialer{dialErr: errors.New("offline")}},
		{
			name: "missing handler",
			dialer: newClientRPCDialer(t, func(request *rpcapi.RPCRequest) *rpcapi.RPCResponse {
				return &rpcapi.RPCResponse{
					V: rpcapi.RPCVersionV1, Id: request.Id,
					Error: &rpcapi.RPCError{
						Code: rpcapi.RPCErrorCodeMethodNotFound, Message: "private detail",
					},
				}
			}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := (ClientRPCExecutor{}).Invoke(t.Context(), test.dialer, "set_volume", json.RawMessage(`{}`))
			if !errors.Is(err, ErrClientToolUnavailable) {
				t.Fatalf("Invoke() error = %v", err)
			}
			if dialer, ok := test.dialer.(*clientRPCDialer); ok {
				dialer.mu.Lock()
				dials := dialer.dials
				dialer.mu.Unlock()
				if dials != 1 {
					t.Fatalf("Dial() calls = %d, want 1", dials)
				}
			}
		})
	}
}

func TestClientRPCExecutorHonorsDeadline(t *testing.T) {
	dialer := newClientRPCDialer(t, func(request *rpcapi.RPCRequest) *rpcapi.RPCResponse {
		time.Sleep(100 * time.Millisecond)
		return clientRPCResult(t, request.Id, `{"late":true}`)
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	_, err := (ClientRPCExecutor{}).Invoke(ctx, dialer, "set_volume", json.RawMessage(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Invoke() error = %v", err)
	}
	dialer.assertCounts(t, 1, 1)
}

func TestClientRPCExecutorRejectsUnsafeResponses(t *testing.T) {
	for _, test := range []struct {
		name   string
		result string
	}{
		{name: "invalid JSON", result: `{`},
		{name: "oversized", result: `"` + strings.Repeat("x", maxClientRPCResult) + `"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			dialer := newClientRPCDialer(t, func(request *rpcapi.RPCRequest) *rpcapi.RPCResponse {
				return clientRPCResult(t, request.Id, test.result)
			})
			_, err := (ClientRPCExecutor{}).Invoke(t.Context(), dialer, "get_status", json.RawMessage(`{}`))
			if err == nil {
				t.Fatal("Invoke() succeeded")
			}
			dialer.assertCounts(t, 1, 1)
		})
	}
}

func TestClientRPCExecutorRejectsInvalidRequestBeforeDial(t *testing.T) {
	for _, test := range []struct {
		name string
		tool string
		args json.RawMessage
	}{
		{name: "invalid name", tool: "bad.name", args: json.RawMessage(`{}`)},
		{name: "non-object arguments", tool: "get_status", args: json.RawMessage(`null`)},
		{name: "oversized arguments", tool: "get_status", args: json.RawMessage(`{"x":"` + strings.Repeat("x", maxClientRPCArguments) + `"}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			dialer := &clientRPCDialer{}
			if _, err := (ClientRPCExecutor{}).Invoke(t.Context(), dialer, test.tool, test.args); err == nil {
				t.Fatal("Invoke() succeeded")
			}
			dialer.mu.Lock()
			defer dialer.mu.Unlock()
			if dialer.dials != 0 {
				t.Fatalf("Dial() calls = %d, want 0", dialer.dials)
			}
		})
	}
}

type clientRPCDialer struct {
	mu      sync.Mutex
	handler func(*rpcapi.RPCRequest) *rpcapi.RPCResponse
	dialErr error
	dials   int
	closes  int
}

func newClientRPCDialer(
	t *testing.T,
	handler func(*rpcapi.RPCRequest) *rpcapi.RPCResponse,
) *clientRPCDialer {
	t.Helper()
	return &clientRPCDialer{handler: handler}
}

func (d *clientRPCDialer) Dial(service uint64) (net.Conn, error) {
	d.mu.Lock()
	d.dials++
	d.mu.Unlock()
	if d.dialErr != nil {
		return nil, d.dialErr
	}
	if service != PeerRPCService {
		return nil, errors.New("wrong service")
	}
	client, server := net.Pipe()
	go func() {
		defer func() {
			_ = server.Close()
			d.mu.Lock()
			d.closes++
			d.mu.Unlock()
		}()
		payload, consumedEOS, err := readRPCEnvelope(server)
		if err != nil {
			return
		}
		if !consumedEOS && rpcapi.ReadEOS(server) != nil {
			return
		}
		request, err := rpcapi.DecodeRequestFrame(rpcapi.Frame{
			Type: rpcapi.FrameTypeBinary, Payload: payload,
		})
		if err != nil {
			return
		}
		response := d.handler(request)
		frame, err := rpcapi.NewResponseFrameForMethod(request.Method, response)
		if err != nil {
			return
		}
		if err := writeRPCEnvelope(server, frame.Payload); err != nil {
			return
		}
		_ = rpcapi.WriteEOS(server)
	}()
	return client, nil
}

func (d *clientRPCDialer) assertCounts(t *testing.T, dials, closes int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		d.mu.Lock()
		gotDials, gotCloses := d.dials, d.closes
		d.mu.Unlock()
		if gotDials == dials && gotCloses == closes {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("dials/closes = %d/%d, want %d/%d", gotDials, gotCloses, dials, closes)
		}
		time.Sleep(time.Millisecond)
	}
}

func clientRPCResult(t *testing.T, id, data string) *rpcapi.RPCResponse {
	t.Helper()
	var result rpcapi.RPCPayload
	if err := result.FromToolInvokeResponse(rpcapi.ToolInvokeResponse{DataJson: data}); err != nil {
		t.Fatal(err)
	}
	return &rpcapi.RPCResponse{
		V: rpcapi.RPCVersionV1, Id: id, Result: &result,
	}
}
