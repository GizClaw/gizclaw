package gizclaw

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
)

func TestRPCFirmwareMetadataReadsPersistedBoundEntry(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedBoundFirmware(t, f, "bound-firmware")
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	server := &rpcServer{
		callerPublicKey: f.owner,
		serverResources: &peerresource.Server{Caller: f.owner, Peers: f.peers, Firmwares: f.firmware},
	}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Handle(serverSide) }()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	response, err := callRPC(ctx, clientSide, &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: "metadata", Method: rpcapi.RPCMethodServerFirmwareMetadataGet,
		Params: mustRPCParams(&rpcpb.FirmwareMetadataGetRequest{Key: "modem"}, (*rpcapi.RPCPayload).FromFirmwareMetadataGetRequest),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	entry, err := response.Result.AsFirmwareMetadataGetResponse()
	if err != nil || entry.Key != "modem" || entry.Value != `{"version":"vendor-2026.10","urls":["https://firmware.example.com/modem/ap.bin","https://firmware.example.com/modem/cp.bin"]}` {
		t.Fatalf("metadata = %v, %v", entry, err)
	}
	if err := clientSide.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
