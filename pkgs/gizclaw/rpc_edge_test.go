package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestEdgeRPCAssignLookupResolve(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	serverKey := giznet.PublicKey{2}
	peers := edgeTestPeers{items: map[giznet.PublicKey]apitypes.Peer{
		peerKey: {
			PublicKey:     peerKey.String(),
			Role:          apitypes.PeerRoleClient,
			Status:        apitypes.PeerRegistrationStatusActive,
			Device:        apitypes.DeviceInfo{},
			Configuration: apitypes.Configuration{},
		},
	}}
	server := &edgeRPCServer{routes: &peerroute.Server{
		Store:           kv.NewMemory(nil),
		Peers:           peers,
		ServerPublicKey: serverKey,
		ServerEndpoint:  "server:9820",
	}}

	assignResp := edgeDispatch(t, server, "assign", rpcapi.RPCMethodServerPeerAssign, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerAssignRequest, rpcapi.ServerPeerAssignRequest{PeerPublicKey: peerKey.String()}))
	if assignResp.Error != nil || assignResp.Result == nil {
		t.Fatalf("assign response = %+v", assignResp)
	}
	assigned, err := assignResp.Result.AsServerPeerAssignResponse()
	if err != nil {
		t.Fatalf("AsServerPeerAssignResponse error = %v", err)
	}
	if assigned.Assignment.PeerPublicKey != peerKey.String() || assigned.Assignment.ServerEndpoint != "server:9820" || assigned.Assignment.Version != 1 {
		t.Fatalf("assigned = %+v", assigned.Assignment)
	}

	lookupResp := edgeDispatch(t, server, "lookup", rpcapi.RPCMethodServerPeerLookup, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerLookupRequest, rpcapi.ServerPeerLookupRequest{PeerPublicKey: peerKey.String()}))
	lookedUp, err := lookupResp.Result.AsServerPeerLookupResponse()
	if err != nil {
		t.Fatalf("AsServerPeerLookupResponse error = %v", err)
	}
	if lookedUp.Assignment.GetPeerPublicKey() != assigned.Assignment.GetPeerPublicKey() || lookedUp.Assignment.GetVersion() != assigned.Assignment.GetVersion() {
		t.Fatalf("lookup assignment = %+v, want %+v", lookedUp.Assignment, assigned.Assignment)
	}

	resolveResp := edgeDispatch(t, server, "resolve", rpcapi.RPCMethodServerRouteResolve, edgeParams(t, (*rpcapi.RPCPayload).FromServerRouteResolveRequest, rpcapi.ServerRouteResolveRequest{TargetPeerPublicKey: peerKey.String()}))
	resolved, err := resolveResp.Result.AsServerRouteResolveResponse()
	if err != nil {
		t.Fatalf("AsServerRouteResolveResponse error = %v", err)
	}
	if resolved.Assignment.GetPeerPublicKey() != assigned.Assignment.GetPeerPublicKey() || resolved.Assignment.GetVersion() != assigned.Assignment.GetVersion() {
		t.Fatalf("resolve assignment = %+v, want %+v", resolved.Assignment, assigned.Assignment)
	}
}

func TestEdgeRPCRejectsMismatchedPayload(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	server := &edgeRPCServer{routes: &peerroute.Server{Store: kv.NewMemory(nil)}}
	resp := edgeDispatch(t, server, "lookup", rpcapi.RPCMethodServerPeerLookup, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerAssignRequest, rpcapi.ServerPeerAssignRequest{PeerPublicKey: peerKey.String()}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("mismatched payload response = %+v", resp)
	}
}

func TestEdgeRPCMapsMissingAssignmentToNotFound(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	server := &edgeRPCServer{routes: &peerroute.Server{Store: kv.NewMemory(nil)}}
	resp := edgeDispatch(t, server, "lookup", rpcapi.RPCMethodServerPeerLookup, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerLookupRequest, rpcapi.ServerPeerLookupRequest{PeerPublicKey: peerKey.String()}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("missing assignment response = %+v", resp)
	}
}

func TestEdgeRPCMapsMissingPeerToNotFound(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	server := &edgeRPCServer{routes: &peerroute.Server{
		Store:           kv.NewMemory(nil),
		Peers:           edgeTestPeers{err: peer.ErrPeerNotFound},
		ServerPublicKey: giznet.PublicKey{2},
		ServerEndpoint:  "server:9820",
	}}
	resp := edgeDispatch(t, server, "assign", rpcapi.RPCMethodServerPeerAssign, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerAssignRequest, rpcapi.ServerPeerAssignRequest{PeerPublicKey: peerKey.String()}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("missing peer response = %+v", resp)
	}
}

func TestEdgeRPCRejectsNonClientAssignment(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	server := &edgeRPCServer{routes: &peerroute.Server{
		Store: kv.NewMemory(nil),
		Peers: edgeTestPeers{items: map[giznet.PublicKey]apitypes.Peer{
			peerKey: {
				PublicKey:     peerKey.String(),
				Role:          apitypes.PeerRoleServer,
				Status:        apitypes.PeerRegistrationStatusActive,
				Device:        apitypes.DeviceInfo{},
				Configuration: apitypes.Configuration{},
			},
		}},
		ServerPublicKey: giznet.PublicKey{2},
		ServerEndpoint:  "server:9820",
	}}
	resp := edgeDispatch(t, server, "assign", rpcapi.RPCMethodServerPeerAssign, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerAssignRequest, rpcapi.ServerPeerAssignRequest{PeerPublicKey: peerKey.String()}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("server peer assign response = %+v", resp)
	}
}

type edgeTestPeers struct {
	items map[giznet.PublicKey]apitypes.Peer
	err   error
}

func (p edgeTestPeers) LoadPeer(_ context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	if p.err != nil {
		return apitypes.Peer{}, p.err
	}
	peer, ok := p.items[publicKey]
	if !ok {
		return apitypes.Peer{}, kv.ErrNotFound
	}
	return peer, nil
}

func edgeDispatch(t *testing.T, server *edgeRPCServer, id string, method rpcapi.RPCMethod, params *rpcapi.RPCPayload) *rpcapi.RPCResponse {
	t.Helper()
	resp, err := server.dispatch(context.Background(), &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Method: method,
		Params: params,
	})
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	if resp == nil {
		t.Fatal("dispatch returned nil response")
	}
	return resp
}

func edgeParams[T any](t *testing.T, encode func(*rpcapi.RPCPayload, T) error, value T) *rpcapi.RPCPayload {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := encode(&payload, value); err != nil {
		t.Fatalf("encode params error = %v", err)
	}
	return &payload
}
