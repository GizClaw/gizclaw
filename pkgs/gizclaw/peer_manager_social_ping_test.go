package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestManagerDeliversSocialPingOverTheActiveConnection(t *testing.T) {
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	target := giznet.PublicKey{7}
	var received rpcapi.ClientSocialPingRequest
	var reject bool
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if req.Method != rpcapi.RPCMethodClientSocialPing {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unexpected"}.RPCResponse(), nil
		}
		if reject {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "no ping handler"}.RPCResponse(), nil
		}
		params, err := req.Params.AsClientSocialPingRequest()
		if err != nil {
			return nil, err
		}
		received = params
		return newRPCResultResponse(req.Id, rpcapi.ClientSocialPingResponse{}, (*rpcapi.RPCPayload).FromClientSocialPingResponse)
	})

	if manager.PeerOnline(target.String()) {
		t.Fatal("a Peer without a connection is online")
	}
	if err := manager.DeliverSocialPing(t.Context(), target.String(), rpcapi.ClientSocialPingRequest{FromPeerPublicKey: "from"}); !errors.Is(err, ErrDeviceOffline) {
		t.Fatalf("offline delivery error = %v, want %v", err, ErrDeviceOffline)
	}
	manager.SetPeerUp(target, device)
	if !manager.PeerOnline(target.String()) || manager.PeerOnline("not-a-key") {
		t.Fatal("PeerOnline does not follow the active connection")
	}
	group := "team"
	if err := manager.DeliverSocialPing(t.Context(), target.String(), rpcapi.ClientSocialPingRequest{FromPeerPublicKey: "from", FriendGroupName: &group}); err != nil {
		t.Fatal(err)
	}
	if received.FromPeerPublicKey != "from" || received.FriendGroupName == nil || *received.FriendGroupName != "team" {
		t.Fatalf("device received %+v", received)
	}
	reject = true
	if err := manager.DeliverSocialPing(t.Context(), target.String(), rpcapi.ClientSocialPingRequest{FromPeerPublicKey: "from"}); err == nil {
		t.Fatal("a device error counted as delivered")
	}
}
