package gizcli

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCClientHandleDeviceInfoMethods(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	name := "main"
	device := &Client{Device: apitypes.DeviceInfo{
		Hardware: &apitypes.HardwareInfo{
			Manufacturer: new("Acme"),
			Model:        new("M1"),
		},
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn: new("sn-1"),
			Imeis: &[]apitypes.PeerIMEI{{
				Name:   &name,
				Tac:    "12345678",
				Serial: "0000001",
			}},
		},
	}}

	errCh := make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{peer: device}).Handle(clientSide)
	}()

	caller := &rpcClient{}
	info, err := caller.GetClientInfo(context.Background(), serverSide, "device-info")
	if err != nil {
		t.Fatalf("GetClientInfo() error = %v", err)
	}
	if info.Manufacturer == nil || *info.Manufacturer != "Acme" {
		t.Fatalf("GetClientInfo() = %+v", info)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle(info) error = %v", err)
	}

	serverSide, clientSide = net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	errCh = make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{peer: device}).Handle(clientSide)
	}()

	identifiers, err := caller.GetClientIdentifiers(context.Background(), serverSide, "device-identifiers")
	if err != nil {
		t.Fatalf("GetClientIdentifiers() error = %v", err)
	}
	if identifiers.Sn == nil || *identifiers.Sn != "sn-1" || identifiers.Imeis == nil || len(*identifiers.Imeis) != 1 {
		t.Fatalf("GetClientIdentifiers() = %+v", identifiers)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle(identifiers) error = %v", err)
	}
}

func TestRPCClientHandleToolInvoke(t *testing.T) {
	device := &Client{}
	if err := device.HandleTool("music_play", func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		var request map[string]any
		if err := json.Unmarshal(args, &request); err != nil {
			t.Fatal(err)
		}
		if request["query"] != "song" {
			t.Fatalf("Tool handler args = %#v", request)
		}
		return json.RawMessage(`{"playing":true}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	var params rpcapi.RPCPayload
	if err := params.FromToolInvokeRequest(rpcapi.ToolInvokeRequest{InvokeName: "music_play", Args: map[string]any{"query": "song"}}); err != nil {
		t.Fatalf("FromToolInvokeRequest() error = %v", err)
	}
	resp, err := (&rpcClient{peer: device}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "invoke", Method: rpcapi.RPCMethodClientToolInvoke, Params: &params})
	if err != nil || resp.Error != nil || resp.Result == nil {
		t.Fatalf("dispatch() = %#v, %v", resp, err)
	}
	result, err := resp.Result.AsToolInvokeResponse()
	if err != nil || string(result.DataJson) != `{"playing":true}` {
		t.Fatalf("tool result = %s, %v", result.DataJson, err)
	}

	resp, err = (&rpcClient{peer: &Client{}}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "invoke", Method: rpcapi.RPCMethodClientToolInvoke, Params: &params})
	if err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeMethodNotFound {
		t.Fatalf("dispatch(no handler) = %#v, %v", resp, err)
	}
}

func TestClientHandleToolValidatesRegistration(t *testing.T) {
	client := &Client{}
	for _, name := range []string{"", "bad.name", "1bad", "音量", strings.Repeat("a", 65)} {
		if err := client.HandleTool(name, func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, nil
		}); err == nil {
			t.Fatalf("HandleTool(%q) succeeded", name)
		}
	}
	if err := client.HandleTool("volume_set", nil); err == nil {
		t.Fatal("HandleTool(nil) succeeded")
	}
}

func TestRPCClientToolHandlerErrorsAreBounded(t *testing.T) {
	tests := []struct {
		name    string
		handler ToolHandler
	}{
		{
			name: "handler error",
			handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return nil, errors.New("secret internal failure")
			},
		},
		{
			name: "invalid JSON",
			handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`{`), nil
			},
		},
		{
			name: "oversized result",
			handler: func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(`"` + strings.Repeat("x", maxClientToolResultBytes) + `"`), nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{}
			if err := client.HandleTool("volume_set", test.handler); err != nil {
				t.Fatal(err)
			}
			response := dispatchClientTool(t, client, "volume_set", map[string]any{"level": 3})
			if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError ||
				strings.Contains(response.Error.Message, "secret") {
				t.Fatalf("dispatch() response = %#v", response)
			}
		})
	}
}

func TestRPCClientToolHandlerCanBeInvokedRepeatedly(t *testing.T) {
	client := &Client{}
	var calls atomic.Int32
	if err := client.HandleTool("battery_get", func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		calls.Add(1)
		return json.RawMessage(`{"level":82}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		response := dispatchClientTool(t, client, "battery_get", map[string]any{})
		if response.Error != nil {
			t.Fatalf("dispatch() error = %#v", response.Error)
		}
	}
	if calls.Load() != 20 {
		t.Fatalf("handler calls = %d", calls.Load())
	}
}

func dispatchClientTool(t *testing.T, client *Client, name string, args map[string]any) *rpcapi.RPCResponse {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := params.FromToolInvokeRequest(rpcapi.ToolInvokeRequest{InvokeName: name, Args: args}); err != nil {
		t.Fatal(err)
	}
	response, err := (&rpcClient{peer: client}).dispatch(t.Context(), &rpcapi.RPCRequest{
		Id: "invoke", Method: rpcapi.RPCMethodClientToolInvoke, Params: &params,
	})
	if err != nil {
		t.Fatal(err)
	}
	return response
}
