package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func (c *rpcClient) ServerPeerLookup(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPeerLookupRequest) (*rpcapi.ServerPeerLookupResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPeerLookup, request, (*rpcapi.RPCPayload).FromServerPeerLookupRequest, rpcapi.RPCPayload.AsServerPeerLookupResponse, "edge peer lookup")
}

func (c *rpcClient) ServerPeerAssign(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPeerAssignRequest) (*rpcapi.ServerPeerAssignResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerPeerAssign, request, (*rpcapi.RPCPayload).FromServerPeerAssignRequest, rpcapi.RPCPayload.AsServerPeerAssignResponse, "edge peer assign")
}

func (c *rpcClient) ServerRouteResolve(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerRouteResolveRequest) (*rpcapi.ServerRouteResolveResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerRouteResolve, request, (*rpcapi.RPCPayload).FromServerRouteResolveRequest, rpcapi.RPCPayload.AsServerRouteResolveResponse, "edge route resolve")
}
