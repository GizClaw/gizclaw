//go:build giznet_e2e

package reconnect_test

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

const echoService uint64 = 7001

type allowAllPolicy struct{}

func (allowAllPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (allowAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

func TestClientConnRecoversAfterServerRestart(t *testing.T) {
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)

	server := startEchoServer(t, serverKey, "127.0.0.1:0")
	client := startListener(t, clientKey, "127.0.0.1:0")
	t.Cleanup(func() { _ = client.Close() })

	clientConn, err := client.Dial(serverKey.Public, server.addr)
	if err != nil {
		t.Fatalf("initial Dial: %v", err)
	}
	if got := roundTrip(t, clientConn, []byte("before restart")); !bytes.Equal(got, []byte("before restart")) {
		t.Fatalf("initial echo = %q", got)
	}

	serverAddr := server.addr.String()
	server.Close()

	server = startEchoServer(t, serverKey, serverAddr)
	t.Cleanup(server.Close)

	if got := roundTrip(t, clientConn, []byte("after restart")); !bytes.Equal(got, []byte("after restart")) {
		t.Fatalf("post-restart echo = %q", got)
	}
}

type echoServer struct {
	listener *giznet.Listener
	addr     *net.UDPAddr
	done     chan error
}

func startEchoServer(t *testing.T, key *giznet.KeyPair, addr string) *echoServer {
	t.Helper()

	var listener *giznet.Listener
	var err error
	for attempt := range 20 {
		listener, err = startListenerMaybe(t, key, addr)
		if err == nil {
			break
		}
		if attempt == 19 {
			t.Fatalf("server listen %s: %v", addr, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	server := &echoServer{
		listener: listener,
		addr:     listener.HostInfo().Addr,
		done:     make(chan error, 1),
	}
	go server.serve()
	return server
}

func startListener(t *testing.T, key *giznet.KeyPair, addr string) *giznet.Listener {
	t.Helper()
	listener, err := startListenerMaybe(t, key, addr)
	if err != nil {
		t.Fatalf("listen %s failed: %v", addr, err)
	}
	return listener
}

func startListenerMaybe(t *testing.T, key *giznet.KeyPair, addr string) (*giznet.Listener, error) {
	t.Helper()
	listener, err := (&giznet.ListenConfig{
		Addr:           addr,
		SecurityPolicy: allowAllPolicy{},
	}).Listen(key)
	if err != nil {
		return nil, err
	}
	go drainUDP(listener.UDP())
	return listener, nil
}

func (s *echoServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		s.done <- err
		return
	}
	service := conn.ListenService(echoService)
	defer service.Close()
	for {
		stream, err := service.Accept()
		if err != nil {
			s.done <- err
			return
		}
		go func() {
			defer stream.Close()
			_, _ = io.Copy(stream, stream)
		}()
	}
}

func (s *echoServer) Close() {
	if s == nil || s.listener == nil {
		return
	}
	_ = s.listener.Close()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

func roundTrip(t *testing.T, conn *giznet.Conn, payload []byte) []byte {
	t.Helper()
	stream, err := conn.Dial(echoService)
	if err != nil {
		t.Fatalf("Dial(echoService): %v", err)
	}
	defer stream.Close()
	deadline := time.Now().Add(2 * time.Second)
	if err := stream.SetDeadline(deadline); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if _, err := stream.Write(payload); err != nil {
		t.Fatalf("stream write: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, got); err != nil {
		t.Fatalf("stream read: %v", err)
	}
	return got
}

func drainUDP(u *giznet.UDP) {
	buf := make([]byte, 65535)
	for {
		if _, _, err := u.ReadFrom(buf); err != nil {
			return
		}
	}
}

func mustKeyPair(t *testing.T) *giznet.KeyPair {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return key
}
