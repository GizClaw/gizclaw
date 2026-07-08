package gizcli

import (
	"context"
	"net"
	"testing"
)

func TestRPCPeerPingClientHandle(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{}).Handle(clientSide)
	}()

	ping, err := callRPCPing(context.Background(), serverSide, "server-ping")
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if ping.ServerTime <= 0 {
		t.Fatalf("Ping() = %+v", ping)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
}
