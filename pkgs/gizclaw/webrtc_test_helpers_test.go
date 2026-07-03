package gizclaw

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

func newTestWebRTCConnPair(t *testing.T, serverKey, clientKey *giznet.KeyPair, serverPolicy, clientPolicy giznet.SecurityPolicy) (giznet.Conn, giznet.Conn) {
	t.Helper()
	serverListener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: serverPolicy,
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("gizwebrtc Listen(server) error = %v", err)
	}
	t.Cleanup(func() { _ = serverListener.Close() })
	signalingServer := httptest.NewServer(serverListener.SignalingHandler())
	t.Cleanup(signalingServer.Close)

	accepted := make(chan giznet.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clientListener, clientConn, err := gizwebrtc.Dial(ctx, clientKey, serverKey.Public, gizwebrtc.DialConfig{
		SignalingURL:   signalingServer.URL + gizwebrtc.SignalingPath,
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: clientPolicy,
	})
	if err != nil {
		t.Fatalf("gizwebrtc Dial error = %v", err)
	}
	t.Cleanup(func() { _ = clientListener.Close() })

	select {
	case serverConn := <-accepted:
		return clientConn, serverConn
	case err := <-acceptErr:
		_ = clientConn.Close()
		t.Fatalf("gizwebrtc Accept error = %v", err)
	case <-time.After(5 * time.Second):
		_ = clientConn.Close()
		t.Fatal("gizwebrtc Accept timeout")
	}
	return nil, nil
}
