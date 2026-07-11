package gizcli

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestClientServerRouteRPCMethodsUseEdgeService(t *testing.T) {
	client, serverConn, cleanup := connectedFirmwareTestClient(t)
	defer cleanup()

	listener := serverConn.ListenService(ServiceEdgeRPC)
	defer listener.Close()

	serverErrCh := make(chan error, 3)
	assignment := rpcapi.PeerAssignment{
		ServerPublicKey: "server-pk",
		ServerEndpoint:  "https://edge.example",
		Version:         1,
	}
	go func() {
		serveEdgeRPCResponse(t, listener, rpcapi.RPCMethodServerPeerLookup, rpcapi.ServerPeerLookupResponse{
			Assignment: &assignment,
		}, (*rpcapi.RPCPayload).FromServerPeerLookupResponse, serverErrCh)
		serveEdgeRPCResponse(t, listener, rpcapi.RPCMethodServerPeerAssign, rpcapi.ServerPeerAssignResponse{
			Assignment: &assignment,
		}, (*rpcapi.RPCPayload).FromServerPeerAssignResponse, serverErrCh)
		serveEdgeRPCResponse(t, listener, rpcapi.RPCMethodServerRouteResolve, rpcapi.ServerRouteResolveResponse{
			Assignment: &assignment,
		}, (*rpcapi.RPCPayload).FromServerRouteResolveResponse, serverErrCh)
	}()

	if _, err := client.ServerPeerLookup(context.Background(), "edge-lookup", rpcapi.ServerPeerLookupRequest{PeerPublicKey: "peer-a"}); err != nil {
		t.Fatalf("ServerPeerLookup error = %v", err)
	}
	if _, err := client.ServerPeerAssign(context.Background(), "edge-assign", rpcapi.ServerPeerAssignRequest{PeerPublicKey: "peer-a"}); err != nil {
		t.Fatalf("ServerPeerAssign error = %v", err)
	}
	if _, err := client.ServerRouteResolve(context.Background(), "edge-route-resolve", rpcapi.ServerRouteResolveRequest{TargetPeerPublicKey: "peer-a"}); err != nil {
		t.Fatalf("ServerRouteResolve error = %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := <-serverErrCh; err != nil {
			t.Fatalf("server error = %v", err)
		}
	}
}

func serveEdgeRPCResponse[T any](
	t *testing.T,
	listener giznet.ServiceListener,
	wantMethod rpcapi.RPCMethod,
	response T,
	encode func(*rpcapi.RPCPayload, T) error,
	errCh chan<- error,
) {
	t.Helper()
	stream, err := listener.Accept()
	if err != nil {
		errCh <- err
		return
	}
	req, err := readRPCRequestWithEOS(stream)
	if err != nil {
		errCh <- err
		return
	}
	if req.Method != wantMethod {
		errCh <- &unexpectedRPCMethodError{got: req.Method, want: wantMethod}
		return
	}
	errCh <- writeRPCResponseWithEOS(stream, req.Method, resourceResponse(req.Id, response, encode))
}
