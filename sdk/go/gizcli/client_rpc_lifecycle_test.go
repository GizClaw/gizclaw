package gizcli

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type inboundRPCTestConn struct {
	freshPingPeerConn
	listener *inboundRPCTestListener
}

func (c *inboundRPCTestConn) ListenService(uint64) giznet.ServiceListener { return c.listener }

type inboundRPCTestListener struct {
	streams chan net.Conn
	done    chan struct{}
}

func (l *inboundRPCTestListener) Accept() (net.Conn, error) {
	select {
	case stream := <-l.streams:
		return stream, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *inboundRPCTestListener) Close() error   { return nil }
func (l *inboundRPCTestListener) Addr() net.Addr { return nil }

func TestClientServeRPCClosesCompletedStreams(t *testing.T) {
	listener := &inboundRPCTestListener{streams: make(chan net.Conn), done: make(chan struct{})}
	client := &Client{conn: &inboundRPCTestConn{listener: listener}}
	served := make(chan error, 1)
	go func() { served <- client.serveRPC() }()
	t.Cleanup(func() {
		close(listener.done)
		if err := <-served; err != nil {
			t.Errorf("serveRPC: %v", err)
		}
	})

	for range 2 {
		deviceSide, callerSide := net.Pipe()
		t.Cleanup(func() { _ = deviceSide.Close(); _ = callerSide.Close() })
		listener.streams <- deviceSide
		if err := callerSide.SetDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		params, err := newRPCPingRequestParams(rpcapi.PingRequest{})
		if err != nil {
			t.Fatal(err)
		}
		request := &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: "lifecycle", Method: rpcapi.RPCMethodAllPing, Params: params}
		if err := rpcapi.WriteRequest(callerSide, request); err != nil {
			t.Fatal(err)
		}
		if err := rpcapi.WriteEOS(callerSide); err != nil {
			t.Fatal(err)
		}
		response, err := rpcapi.ReadResponseForMethod(callerSide, request.Method)
		if err != nil || response.Error != nil || response.Result == nil {
			t.Fatalf("read complete response: response=%v error=%v", response, err)
		}
		if err := rpcapi.ReadEOS(callerSide); err != nil {
			t.Fatal(err)
		}
		if _, err := callerSide.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("completed RPC transport remains open: %v", err)
		}
	}
}
