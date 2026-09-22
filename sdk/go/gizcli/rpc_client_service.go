package gizcli

import (
	"context"
	"errors"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

// ObserveClientRPC installs an optional observer for valid Server-to-Client
// RPC dispatch. The observer must return quickly and must not call Client
// lifecycle methods.
func (c *Client) ObserveClientRPC(observer func(rpcapi.RPCMethod)) error {
	if c == nil {
		return errors.New("gizclaw: nil client")
	}
	c.clientRPCMu.Lock()
	defer c.clientRPCMu.Unlock()
	c.clientRPCObserver = observer
	return nil
}

func (c *Client) observeClientRPC(method rpcapi.RPCMethod) {
	if c == nil {
		return
	}
	c.clientRPCMu.RLock()
	observer := c.clientRPCObserver
	c.clientRPCMu.RUnlock()
	if observer != nil {
		observer(method)
	}
}

func (c *rpcClient) GetClientInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientGetInfoRequest{}, (*rpcapi.RPCPayload).FromClientGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_INFO_GET, params, rpcapi.RPCPayload.AsClientGetInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("device info", err)
	}
	return result, nil
}

func (c *rpcClient) GetClientIdentifiers(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientGetIdentifiersResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientGetIdentifiersRequest{}, (*rpcapi.RPCPayload).FromClientGetIdentifiersRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_IDENTIFIERS_GET, params, rpcapi.RPCPayload.AsClientGetIdentifiersResponse)
	if err != nil {
		return nil, wrapRPCResultError("device identifiers", err)
	}
	return result, nil
}

func (c *rpcClient) handleGetClientInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientGetInfoRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeInternal, Message: "peer client not configured"}.RPCResponse(), nil
	}
	c.peer.observeClientRPC(req.Method)
	result, err := convertRPCType[rpcapi.ClientGetInfoResponse](peerDeviceToPeerRefreshInfo(c.peer.Device))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientGetInfoResponse)
}

func (c *rpcClient) handleGetClientIdentifiers(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientGetIdentifiersRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeInternal, Message: "peer client not configured"}.RPCResponse(), nil
	}
	c.peer.observeClientRPC(req.Method)
	result, err := convertRPCType[rpcapi.ClientGetIdentifiersResponse](peerDeviceToPeerRefreshIdentifiers(c.peer.Device))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientGetIdentifiersResponse)
}
