package gizhttp

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestListenerAddrHelpers(t *testing.T) {
	addr := listenerAddr{peerPK: "peer-pk", service: 7}
	if addr.Network() != "gizhttp" {
		t.Fatalf("Network = %q", addr.Network())
	}
	if addr.String() != "peer-pk/service/7" {
		t.Fatalf("String = %q", addr.String())
	}
	if !IsClosed(net.ErrClosed) {
		t.Fatal("IsClosed should match net.ErrClosed")
	}
	if IsClosed(errors.New("other")) {
		t.Fatal("IsClosed should not match other errors")
	}
}

func TestWrapStreamDeadlines(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := wrapStream(serverSide)
	deadline := time.Now().Add(time.Second)
	if err := conn.SetDeadline(deadline); err != nil {
		t.Fatalf("SetDeadline error: %v", err)
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatalf("SetReadDeadline error: %v", err)
	}
	if err := conn.SetWriteDeadline(deadline); err != nil {
		t.Fatalf("SetWriteDeadline error: %v", err)
	}
}
