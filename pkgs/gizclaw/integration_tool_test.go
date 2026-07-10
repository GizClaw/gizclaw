package gizclaw_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestIntegrationDeviceToolInvokeOnlineAndOffline(t *testing.T) {
	ts := startTestServer(t)
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	requests := make(chan rpcapi.ToolInvokeRequest, 1)
	client := &gizcli.Client{
		KeyPair:       keyPair,
		DialTransport: testWebRTCDialTransport(ts.cipherMode),
		ToolInvoker: func(_ context.Context, request rpcapi.ToolInvokeRequest) (rpcapi.ToolInvokeResponse, error) {
			requests <- request
			return rpcapi.ToolInvokeResponse{DataJson: `{"echo":"hello"}`}, nil
		},
	}
	startTestClient(t, client, ts.server.PublicKey(), ts.addr)
	t.Cleanup(func() { _ = client.Close() })

	peerID := keyPair.Public.String()
	method := "echo"
	tool := toolkit.Tool{
		ID:          "peer." + peerID + ".integration.echo",
		Source:      toolkit.ToolSourceDevice,
		Enabled:     true,
		OwnerPeer:   &peerID,
		InputSchema: jsonschema.Schema{Type: "object"},
		Executor: toolkit.ToolExecutor{
			Kind:   toolkit.ToolExecutorKindDeviceRPC,
			Method: &method,
			PeerID: &peerID,
		},
	}
	if _, err := ts.server.Manager().Tools.PutTool(context.Background(), tool); err != nil {
		t.Fatalf("PutTool(): %v", err)
	}

	if err := waitUntil(testReadyTimeout, func() error {
		if !ts.server.Manager().ToolPeerAvailable(peerID) {
			return errors.New("device Tool peer is not online")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	result, err := ts.server.Manager().ToolExecutors.Invoke(ctx, toolkit.Call{Tool: tool, Args: json.RawMessage(`{"text":"hello"}`)})
	cancel()
	if err != nil || string(result.Data) != `{"echo":"hello"}` {
		t.Fatalf("device Tool Invoke() = %s, %v", result.Data, err)
	}
	select {
	case request := <-requests:
		if request.CallId != tool.ID || request.ToolId != tool.ID || request.Method != method || request.Args["text"] != "hello" {
			t.Fatalf("client.tool.invoke request = %#v", request)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client.tool.invoke request not received")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("client Close(): %v", err)
	}
	if err := waitUntil(testReadyTimeout, func() error {
		if ts.server.Manager().ToolPeerAvailable(peerID) {
			return errors.New("device Tool peer is still online")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.server.Manager().ToolExecutors.Invoke(context.Background(), toolkit.Call{Tool: tool}); !errors.Is(err, toolkit.ErrExecutorUnavailable) {
		t.Fatalf("offline device Tool Invoke() error = %v", err)
	}
}
