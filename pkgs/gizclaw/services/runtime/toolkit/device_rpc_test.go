package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestDeviceRPCExecutorAvailabilityInvokeAndNoRetry(t *testing.T) {
	client := &fakeDeviceRPCClient{online: true, response: rpcapi.ToolInvokeResponse{DataJson: `{"playing":true}`}}
	executor := &DeviceRPCExecutor{Client: client}
	tool := testDeviceTool("peer.peer-a.music.play", "peer-a")

	available, err := executor.ToolAvailable(context.Background(), tool)
	if err != nil || !available {
		t.Fatalf("ToolAvailable() = %v, %v", available, err)
	}
	result, err := executor.Invoke(context.Background(), Call{ID: "call-1", Tool: tool, Args: json.RawMessage(`{"query":"song"}`)})
	if err != nil || string(result.Data) != `{"playing":true}` {
		t.Fatalf("Invoke() = %s, %v", result.Data, err)
	}
	if client.calls != 1 || client.request.CallId != "call-1" || client.request.ToolId != tool.ID || client.request.Method != "music.play" || client.request.Args["query"] != "song" {
		t.Fatalf("device request = %#v calls=%d", client.request, client.calls)
	}

	client.online = false
	if available, err := executor.ToolAvailable(context.Background(), tool); err != nil || available {
		t.Fatalf("ToolAvailable(offline) = %v, %v", available, err)
	}
	if _, err := executor.Invoke(context.Background(), Call{Tool: tool}); !errors.Is(err, ErrExecutorUnavailable) {
		t.Fatalf("Invoke(offline) error = %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("offline Invoke calls = %d, want no retry/call", client.calls)
	}
}

func TestExecutorRegistryDispatchesDeviceAndBuilderTracksReconnect(t *testing.T) {
	ctx := context.Background()
	client := &fakeDeviceRPCClient{response: rpcapi.ToolInvokeResponse{DataJson: `null`}}
	device := &DeviceRPCExecutor{Client: client}
	registry := NewExecutorRegistry()
	if err := registry.RegisterDevice(device, device); err != nil {
		t.Fatalf("RegisterDevice() error = %v", err)
	}
	store := &Server{Store: kv.NewMemory(nil)}
	if _, err := store.PutTool(ctx, testDeviceTool("peer.peer-a.music.play", "peer-a")); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	builder := &Builder{Tools: store, Availability: registry}

	kit, err := builder.Build(ctx, BuildRequest{OwnerPublicKey: "peer-a"})
	if err != nil || len(kit.Tools) != 0 {
		t.Fatalf("Build(offline) = %#v, %v", kit, err)
	}
	client.online = true
	kit, err = builder.Build(ctx, BuildRequest{OwnerPublicKey: "peer-a"})
	if err != nil || len(kit.Tools) != 1 {
		t.Fatalf("Build(reconnected) = %#v, %v", kit, err)
	}
	if _, err := builder.Invoke(ctx, registry, InvokeRequest{Build: BuildRequest{OwnerPublicKey: "peer-a"}, Name: kit.Tools[0].ID}); err != nil {
		t.Fatalf("Invoke(device) error = %v", err)
	}
}

func TestDeviceRPCExecutorPropagatesContextCancellationWithoutRetry(t *testing.T) {
	client := &fakeDeviceRPCClient{online: true}
	executor := &DeviceRPCExecutor{Client: client}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := executor.Invoke(ctx, Call{Tool: testDeviceTool("peer.peer-a.music.play", "peer-a")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Invoke(canceled) error = %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("Invoke(canceled) calls = %d, want exactly one attempt", client.calls)
	}
}

type fakeDeviceRPCClient struct {
	online   bool
	response rpcapi.ToolInvokeResponse
	err      error
	calls    int
	request  rpcapi.ToolInvokeRequest
}

func (f *fakeDeviceRPCClient) ToolPeerAvailable(string) bool { return f.online }

func (f *fakeDeviceRPCClient) InvokePeerTool(ctx context.Context, _ string, request rpcapi.ToolInvokeRequest) (rpcapi.ToolInvokeResponse, error) {
	f.calls++
	f.request = request
	if err := ctx.Err(); err != nil {
		return rpcapi.ToolInvokeResponse{}, err
	}
	return f.response, f.err
}
