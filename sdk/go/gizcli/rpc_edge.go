package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func (c *rpcClient) EdgePeerLookup(ctx context.Context, conn net.Conn, id string, request rpcapi.EdgePeerLookupRequest) (*rpcapi.EdgePeerLookupResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodEdgePeerLookup, request, (*rpcapi.RPCPayload).FromEdgePeerLookupRequest, rpcapi.RPCPayload.AsEdgePeerLookupResponse, "edge peer lookup")
}

func (c *rpcClient) EdgePeerAssign(ctx context.Context, conn net.Conn, id string, request rpcapi.EdgePeerAssignRequest) (*rpcapi.EdgePeerAssignResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodEdgePeerAssign, request, (*rpcapi.RPCPayload).FromEdgePeerAssignRequest, rpcapi.RPCPayload.AsEdgePeerAssignResponse, "edge peer assign")
}

func (c *rpcClient) EdgeRouteResolve(ctx context.Context, conn net.Conn, id string, request rpcapi.EdgeRouteResolveRequest) (*rpcapi.EdgeRouteResolveResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodEdgeRouteResolve, request, (*rpcapi.RPCPayload).FromEdgeRouteResolveRequest, rpcapi.RPCPayload.AsEdgeRouteResolveResponse, "edge route resolve")
}
