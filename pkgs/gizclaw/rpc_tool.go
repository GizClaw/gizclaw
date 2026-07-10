package gizclaw

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func (m *Manager) ToolPeerAvailable(peerID string) bool {
	publicKey, err := parseToolPeerID(peerID)
	if err != nil {
		return false
	}
	_, ok := m.Peer(publicKey)
	return ok
}

func (m *Manager) InvokePeerTool(ctx context.Context, peerID string, request rpcapi.ToolInvokeRequest) (rpcapi.ToolInvokeResponse, error) {
	publicKey, err := parseToolPeerID(peerID)
	if err != nil {
		return rpcapi.ToolInvokeResponse{}, err
	}
	if strings.TrimSpace(request.CallId) == "" {
		request.CallId = request.ToolId
	}
	response, err := callPeerRPC(m, ctx, publicKey, func(client *rpcClient, conn net.Conn) (*rpcapi.ToolInvokeResponse, error) {
		return client.InvokeTool(ctx, conn, request.CallId, request)
	})
	if errors.Is(err, ErrDeviceOffline) {
		return rpcapi.ToolInvokeResponse{}, toolkit.ErrExecutorUnavailable
	}
	if err != nil {
		return rpcapi.ToolInvokeResponse{}, err
	}
	return *response, nil
}

func (c *rpcClient) InvokeTool(ctx context.Context, conn net.Conn, id string, request rpcapi.ToolInvokeRequest) (*rpcapi.ToolInvokeResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromToolInvokeRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientToolInvoke, params), rpcapi.RPCPayload.AsToolInvokeResponse)
	if err != nil {
		return nil, wrapRPCResultError("tool invocation", err)
	}
	return result, nil
}

func parseToolPeerID(value string) (giznet.PublicKey, error) {
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
		return giznet.PublicKey{}, err
	}
	return publicKey, nil
}
