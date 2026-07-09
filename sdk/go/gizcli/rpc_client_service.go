package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func (c *rpcClient) GetClientInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientGetInfoRequest{}, (*rpcapi.RPCPayload).FromClientGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientInfoGet, params), rpcapi.RPCPayload.AsClientGetInfoResponse)
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
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientIdentifiersGet, params), rpcapi.RPCPayload.AsClientGetIdentifiersResponse)
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
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer client not configured"}.RPCResponse(), nil
	}
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
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer client not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ClientGetIdentifiersResponse](peerDeviceToPeerRefreshIdentifiers(c.peer.Device))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientGetIdentifiersResponse)
}
