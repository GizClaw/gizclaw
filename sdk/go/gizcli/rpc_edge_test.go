package gizcli

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestClientEdgeRPCMethodsUseEdgeService(t *testing.T) {
	client, serverConn, cleanup := connectedFirmwareTestClient(t)
	defer cleanup()

	listener := serverConn.ListenService(ServiceEdgeRPC)
	defer listener.Close()

	serverErrCh := make(chan error, 3)
	assignment := rpcapi.EdgePeerAssignment{
		ServerPublicKey: "server-pk",
		ServerEndpoint:  "https://edge.example",
		Version:         1,
	}
	go func() {
		serveEdgeRPCResponse(t, listener, rpcapi.RPCMethodEdgePeerLookup, rpcapi.EdgePeerLookupResponse{
			Assignment: assignment,
		}, (*rpcapi.RPCPayload).FromEdgePeerLookupResponse, serverErrCh)
		serveEdgeRPCResponse(t, listener, rpcapi.RPCMethodEdgePeerAssign, rpcapi.EdgePeerAssignResponse{
			Assignment: assignment,
		}, (*rpcapi.RPCPayload).FromEdgePeerAssignResponse, serverErrCh)
		serveEdgeRPCResponse(t, listener, rpcapi.RPCMethodEdgeRouteResolve, rpcapi.EdgeRouteResolveResponse{
			Assignment: assignment,
		}, (*rpcapi.RPCPayload).FromEdgeRouteResolveResponse, serverErrCh)
	}()

	if _, err := client.EdgePeerLookup(context.Background(), "edge-lookup", rpcapi.EdgePeerLookupRequest{PeerPublicKey: "peer-a"}); err != nil {
		t.Fatalf("EdgePeerLookup error = %v", err)
	}
	if _, err := client.EdgePeerAssign(context.Background(), "edge-assign", rpcapi.EdgePeerAssignRequest{PeerPublicKey: "peer-a"}); err != nil {
		t.Fatalf("EdgePeerAssign error = %v", err)
	}
	if _, err := client.EdgeRouteResolve(context.Background(), "edge-route-resolve", rpcapi.EdgeRouteResolveRequest{TargetPeerPublicKey: "peer-a"}); err != nil {
		t.Fatalf("EdgeRouteResolve error = %v", err)
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
