package gizclaw

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"net"
)

func callClientToolResult[T any](ctx context.Context, conn net.Conn, id string, tool rpcpb.ClientTool, params *rpcapi.RPCPayload, decode func(rpcapi.RPCPayload) (T, error)) (*T, error) {
	wrapped, err := rpcapi.EncodeClientToolRequest(tool, params)
	if err != nil {
		return nil, err
	}
	return callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientToolV0Invoke, wrapped), func(payload rpcapi.RPCPayload) (T, error) {
		result, err := rpcapi.DecodeClientToolResponse(tool, &payload)
		if err != nil {
			var zero T
			return zero, err
		}
		return decode(*result)
	})
}
