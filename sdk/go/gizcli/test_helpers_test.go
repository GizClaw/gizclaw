package gizcli

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type testClientListener struct{}

func (testClientListener) Accept() (giznet.Conn, error) { return nil, giznet.ErrClosed }
func (testClientListener) Close() error                 { return nil }

func testWebRTCDialTransport() DialTransportFunc {
	return func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		listener, conn, err := gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
			SignalingURL:   serverAddr,
			CipherMode:     gizwebrtc.CipherModePlaintext,
			SecurityPolicy: securityPolicy,
		})
		if err != nil {
			return nil, nil, err
		}
		return listener, conn, nil
	}
}

func newTestWebRTCServer(t *testing.T, key *giznet.KeyPair, policy giznet.SecurityPolicy) (*gizwebrtc.Listener, string) {
	t.Helper()
	listener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: policy,
	}).Listen(key)
	if err != nil {
		t.Fatalf("gizwebrtc Listen(server) error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := httptest.NewServer(listener.SignalingHandler())
	t.Cleanup(server.Close)
	return listener, server.URL + gizwebrtc.SignalingPath
}
