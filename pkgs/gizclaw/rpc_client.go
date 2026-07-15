package gizclaw

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcClient struct{}

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
