package gizhttp

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type testSecurityPolicy struct {
	allowService func(giznet.PublicKey, uint64) bool
}

func (p testSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (p testSecurityPolicy) AllowService(pk giznet.PublicKey, service uint64) bool {
	if p.allowService == nil {
		return service == 0
	}
	return p.allowService(pk, service)
}

type testListenerNode struct {
	listener     *gizwebrtc.Listener
	signalingURL string
}

func (n *testListenerNode) Close() error {
	if n == nil || n.listener == nil {
		return nil
	}
	return n.listener.Close()
}

// newListenerNode creates a giznet.Listener for tests using only public APIs.
func newListenerNode(t *testing.T, key *giznet.KeyPair, cfgs ...gizwebrtc.ListenConfig) *testListenerNode {
	t.Helper()

	cfg := gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: testSecurityPolicy{},
	}
	if len(cfgs) > 0 {
		if cfgs[0].SecurityPolicy != nil {
			cfg.SecurityPolicy = cfgs[0].SecurityPolicy
		}
		cfg.PeerEventHandler = cfgs[0].PeerEventHandler
	}
	l, err := (&cfg).Listen(key)
	if err != nil {
		t.Fatalf("gizwebrtc.Listen failed: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	server := httptest.NewServer(l.SignalingHandler())
	t.Cleanup(server.Close)
	return &testListenerNode{listener: l, signalingURL: server.URL + gizwebrtc.SignalingPath}
}

func connectListenerNodes(t *testing.T, _ *testListenerNode, clientKey *giznet.KeyPair, server *testListenerNode, serverKey *giznet.KeyPair) (giznet.Conn, giznet.Conn) {
	t.Helper()

	acceptCh := make(chan giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := server.listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clientListener, clientConn, err := gizwebrtc.Dial(ctx, clientKey, serverKey.Public, gizwebrtc.DialConfig{
		SignalingURL:   server.signalingURL,
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: testSecurityPolicy{},
	})
	if err != nil {
		t.Fatalf("gizwebrtc.Dial failed: %v", err)
	}
	t.Cleanup(func() { _ = clientListener.Close() })

	select {
	case serverConn := <-acceptCh:
		return clientConn, serverConn
	case err := <-errCh:
		t.Fatalf("Accept failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timeout")
	}
	return nil, nil
}
