package gizclaw

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRPCPeerDeleteAcknowledgesBeforeTerminalAction(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	publicKey := giznet.PublicKey{1}
	if _, err := peers.SavePeer(context.Background(), apitypes.Peer{
		PublicKey: publicKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    apitypes.DeviceInfo{},
	}); err != nil {
		t.Fatalf("SavePeer: %v", err)
	}
	terminal := make(chan struct{}, 1)
	server := &rpcServer{
		peer:            peers,
		callerPublicKey: publicKey,
		onPeerDeleted: func() {
			terminal <- struct{}{}
		},
	}
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Handle(serverSide) }()

	request := newRPCRequest(
		"delete-self",
		rpcapi.RPCMethodServerPeerDelete,
		mustRPCParams(rpcapi.ServerPeerDeleteRequest{}, (*rpcapi.RPCPayload).FromServerPeerDeleteRequest),
	)
	response, err := callRPC(context.Background(), clientSide, request)
	if err != nil {
		t.Fatalf("callRPC: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("response error = %#v", response.Error)
	}
	if _, err := response.Result.AsServerPeerDeleteResponse(); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	select {
	case <-terminal:
	case <-time.After(time.Second):
		t.Fatal("terminal action was not called after response")
	}
	if _, err := peers.LoadPeer(context.Background(), publicKey); err == nil {
		t.Fatal("Peer remains active after self-delete")
	}
	if pending, err := pendingdeletion.HasLocator(context.Background(), store, pendingdeletion.KindPeer, publicKey.String()); err != nil || !pending {
		t.Fatalf("peer pending deletion = %v, error = %v", pending, err)
	}
	_ = clientSide.Close()
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server Handle: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server Handle did not stop")
	}
}
