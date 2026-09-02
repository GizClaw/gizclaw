package gizclaw

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestEdgeRPCServerHandleOneRequestPerDataChannel(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- (&edgeRPCServer{}).Handle(serverSide)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := func(id string) *rpcapi.RPCRequest {
		return &rpcapi.RPCRequest{
			V:      rpcapi.RPCVersionV1,
			Id:     id,
			Method: rpcapi.RPCMethodServerPeerLookup,
		}
	}
	resp, err := callRPC(ctx, clientSide, request("req-1"))
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("first response = %#v, want RPC error", resp)
	}
	if _, err := callRPC(ctx, clientSide, request("req-2")); err == nil {
		t.Fatal("second call error = nil, want closed DataChannel")
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server Handle error = %v", err)
	}
}

func TestEdgeRPCRejectsMismatchedPayload(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	server := &edgeRPCServer{routes: &peerroute.Server{Store: kv.NewMemory(nil)}}
	resp := edgeDispatch(t, server, "lookup", rpcapi.RPCMethodServerPeerLookup, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerAssignRequest, rpcpb.ServerPeerAssignRequest{PeerPublicKey: peerKey.String()}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("mismatched payload response = %+v", resp)
	}
}

func TestEdgeRPCMapsMissingAssignmentToNotFound(t *testing.T) {
	peerKey := giznet.PublicKey{1}
	server := &edgeRPCServer{routes: &peerroute.Server{Store: kv.NewMemory(nil)}}
	resp := edgeDispatch(t, server, "lookup", rpcapi.RPCMethodServerPeerLookup, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerLookupRequest, rpcpb.ServerPeerLookupRequest{PeerPublicKey: peerKey.String()}))
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
	resp := edgeDispatch(t, server, "assign", rpcapi.RPCMethodServerPeerAssign, edgeParams(t, (*rpcapi.RPCPayload).FromServerPeerAssignRequest, rpcpb.ServerPeerAssignRequest{PeerPublicKey: peerKey.String()}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("missing peer response = %+v", resp)
	}
}

func TestEdgeRPCResolvesAPIKeyOwnerRoute(t *testing.T) {
	ctx := context.Background()
	peerKey := giznet.PublicKey{1}
	serverKey := giznet.PublicKey{2}
	apiKeys := apikey.NewServer(kv.NewMemory(nil))
	created, err := apiKeys.Create(ctx, peerKey.String(), "edge route", false)
	if err != nil {
		t.Fatal(err)
	}
	routes := &peerroute.Server{
		Store: kv.NewMemory(nil),
		Peers: edgeTestPeers{items: map[giznet.PublicKey]apitypes.Peer{
			peerKey: {PublicKey: peerKey.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive},
		}},
		ServerPublicKey: serverKey,
		ServerEndpoint:  "server.example.com:9820",
	}
	if _, err := routes.Assign(ctx, peerKey, nil); err != nil {
		t.Fatal(err)
	}
	server := &edgeRPCServer{routes: routes, apiKeys: apiKeys}
	resp := edgeDispatch(t, server, "resolve-api-key", rpcapi.RPCMethodServerAPIKeyResolve, edgeParams(t, (*rpcapi.RPCPayload).FromServerAPIKeyResolveRequest, rpcpb.ServerAPIKeyResolveRequest{ApiKey: created.Secret}))
	if resp.Error != nil || resp.Result == nil {
		t.Fatalf("resolve response = %+v", resp)
	}
	result, err := resp.Result.AsServerAPIKeyResolveResponse()
	if err != nil {
		t.Fatal(err)
	}
	if result.Assignment == nil || result.Assignment.PeerPublicKey != peerKey.String() || result.Assignment.ServerPublicKey != serverKey.String() {
		t.Fatalf("assignment = %+v", result.Assignment)
	}
}

func TestEdgeRPCRejectsInvalidAPIKeyWithoutEchoingCredential(t *testing.T) {
	secret := "gizclaw_sk_v1_0123456789012345678901234567890123456789012"
	server := &edgeRPCServer{
		routes:  &peerroute.Server{Store: kv.NewMemory(nil)},
		apiKeys: apikey.NewServer(kv.NewMemory(nil)),
	}
	resp := edgeDispatch(t, server, "resolve-api-key", rpcapi.RPCMethodServerAPIKeyResolve, edgeParams(t, (*rpcapi.RPCPayload).FromServerAPIKeyResolveRequest, rpcpb.ServerAPIKeyResolveRequest{ApiKey: secret}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeForbidden {
		t.Fatalf("invalid API key response = %+v", resp)
	}
	if strings.Contains(resp.Error.Message, secret) {
		t.Fatal("RPC error exposed API key")
	}
}

func TestEdgeRPCSanitizesAPIKeyStoreErrors(t *testing.T) {
	secret := "gizclaw_sk_v1_0123456789012345678901234567890123456789012"
	server := &edgeRPCServer{
		routes:  &peerroute.Server{Store: kv.NewMemory(nil)},
		apiKeys: apikey.NewServer(edgeAPIKeyErrorStore{Store: kv.NewMemory(nil), err: errors.New("backend failed for " + secret)}),
	}
	resp := edgeDispatch(t, server, "resolve-api-key", rpcapi.RPCMethodServerAPIKeyResolve, edgeParams(t, (*rpcapi.RPCPayload).FromServerAPIKeyResolveRequest, rpcpb.ServerAPIKeyResolveRequest{ApiKey: secret}))
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("store error response = %+v", resp)
	}
	if strings.Contains(resp.Error.Message, secret) {
		t.Fatal("RPC store error exposed API key")
	}
}

type edgeAPIKeyErrorStore struct {
	kv.Store
	err error
}

func (s edgeAPIKeyErrorStore) Get(context.Context, kv.Key) ([]byte, error) {
	return nil, s.err
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
