package gizclaw

import (
	"context"
	"fmt"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

// Ping opens a fresh RPC stream, sends one ping, and closes it.
// RPC servers also accept multiple sequential requests on a single stream for
// firmware clients that keep their service data channel open.
func (h *PeerConn) Ping(ctx context.Context, id string) (*rpcapi.PingResponse, error) {
	stream, err := h.rpcConn()
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	return h.rpcServer().Ping(ctx, stream, id)
}

func (h *PeerConn) rpcConn() (net.Conn, error) {
	conn := h.Conn
	stream, err := conn.Dial(ServicePeerRPC)
	if err != nil {
		return nil, fmt.Errorf("gizclaw: dial rpc stream: %w", err)
	}
	return stream, nil
}

func (s *rpcServer) Ping(ctx context.Context, conn net.Conn, id string) (*rpcapi.PingResponse, error) {
	return callRPCPing(ctx, conn, id)
}
