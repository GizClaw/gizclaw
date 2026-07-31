//go:build gizclaw_e2e

package rpc_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestConcurrentServiceStreams(t *testing.T) {
	h := clitest.NewSetupHarness(t, "go-concurrent-service-streams")
	registerSetupPeer(t, h, "peer-a", "go-concurrent-service-streams", true)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })

	peerConn := peer.PeerConn()
	if peerConn == nil {
		t.Fatal("peer connection is nil")
	}

	eventStream, err := peer.DialPeerEventStream()
	if err != nil {
		t.Fatalf("open mandatory Event stream: %v", err)
	}
	defer eventStream.Close()
	setStreamDeadline(t, eventStream)

	firstRPC := dialServiceStream(t, peerConn.Dial, gizcli.ServicePeerRPC)
	defer firstRPC.Close()
	secondRPC := dialServiceStream(t, peerConn.Dial, gizcli.ServicePeerRPC)
	defer secondRPC.Close()
	peerHTTP := dialServiceStream(t, peerConn.Dial, gizcli.ServicePeerHTTP)
	defer peerHTTP.Close()

	if firstRPC == secondRPC || firstRPC == peerHTTP || secondRPC == peerHTTP {
		t.Fatal("service Dial reused a live channel identity")
	}
	if peer.PeerConn() != peerConn {
		t.Fatal("opening service streams replaced the Peer connection")
	}

	writePingRequest(t, firstRPC, "concurrent-services-first")
	writePingRequest(t, secondRPC, "concurrent-services-second")
	httpRequest, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://gizclaw/server-info",
		nil,
	)
	if err != nil {
		t.Fatalf("create Peer HTTP request: %v", err)
	}
	if err := httpRequest.Write(peerHTTP); err != nil {
		t.Fatalf("write Peer HTTP request: %v", err)
	}

	requirePingResponse(t, firstRPC, "concurrent-services-first")
	if err := firstRPC.Close(); err != nil {
		t.Fatalf("close first RPC stream: %v", err)
	}

	requirePingResponse(t, secondRPC, "concurrent-services-second")
	httpResponse, err := http.ReadResponse(bufio.NewReader(peerHTTP), httpRequest)
	if err != nil {
		t.Fatalf("read Peer HTTP response after closing sibling RPC: %v", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResponse.Body)
		t.Fatalf("Peer HTTP status = %d, want 200: %s", httpResponse.StatusCode, body)
	}
	if peer.PeerConn() != peerConn {
		t.Fatal("closing one service stream replaced or closed the Peer connection")
	}
}

func dialServiceStream(
	t *testing.T,
	dial func(uint64) (net.Conn, error),
	service uint64,
) net.Conn {
	t.Helper()
	stream, err := dial(service)
	if err != nil {
		t.Fatalf("dial service %d: %v", service, err)
	}
	setStreamDeadline(t, stream)
	return stream
}

func setStreamDeadline(t *testing.T, stream net.Conn) {
	t.Helper()
	if err := stream.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatalf("set stream deadline: %v", err)
	}
}

func writePingRequest(t *testing.T, stream io.Writer, id string) {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()}); err != nil {
		t.Fatalf("encode ping params: %v", err)
	}
	request := &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Method: rpcapi.RPCMethodAllPing,
		Params: &params,
	}
	if err := rpcapi.WriteRequest(stream, request); err != nil {
		t.Fatalf("write %s: %v", id, err)
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		t.Fatalf("write %s EOS: %v", id, err)
	}
}

func requirePingResponse(t *testing.T, stream io.Reader, id string) {
	t.Helper()
	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodAllPing)
	if err != nil {
		t.Fatalf("read %s response: %v", id, err)
	}
	if response.Id != id || response.Error != nil || response.Result == nil {
		t.Fatalf("unexpected %s response: %+v", id, response)
	}
	ping, err := response.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("decode %s response: %v", id, err)
	}
	if ping.ServerTime <= 0 {
		t.Fatalf("%s server_time = %d, want positive", id, ping.ServerTime)
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		t.Fatalf("read %s EOS: %v", id, err)
	}
}
