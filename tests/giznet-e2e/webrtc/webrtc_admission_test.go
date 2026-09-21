//go:build giznet_e2e

package webrtc_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type credentialPolicy struct{ allowAllPolicy }

func (credentialPolicy) AllowPeer(_ context.Context, admission giznet.PeerAdmission) bool {
	c := admission.Credential
	return c != nil && c.Version == 7 && c.Type == "e2e-custom" && c.Value == "opaque-value"
}

func TestWebRTCStructuredAdmissionCredential(t *testing.T) {
	key := mustKeyPair(t)
	server := startWebRTCServerWithConfig(t, key, gizwebrtc.ListenConfig{SecurityPolicy: credentialPolicy{}})
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	for _, accepted := range []bool{true, false} {
		credential := &giznetpb.AdmissionCredential{Version: 7, Type: "e2e-custom", Value: "opaque-value"}
		if !accepted {
			credential.Value = "wrong-value"
		}
		listener, conn, err := gizwebrtc.Dial(ctx, mustKeyPair(t), key.Public, gizwebrtc.DialConfig{SignalingURL: server.signalingURL, Credential: credential, SecurityPolicy: allowAllPolicy{}})
		if !accepted {
			if err == nil {
				_ = conn.Close()
				_ = listener.Close()
				t.Fatal("wrong credential accepted")
			}
			if !strings.Contains(err.Error(), "peer_forbidden") {
				t.Fatalf("unexpected rejection: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		peer := acceptConn(t, server.listener)
		roundTripPacket(t, conn, peer, 0x42, []byte("admitted"))
		_ = peer.Close()
		_ = conn.Close()
		_ = listener.Close()
	}
}
