package gizwebrtc

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestPeerConnectionStateIsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		state webrtc.PeerConnectionState
		want  bool
	}{
		{name: "new", state: webrtc.PeerConnectionStateNew},
		{name: "connecting", state: webrtc.PeerConnectionStateConnecting},
		{name: "connected", state: webrtc.PeerConnectionStateConnected},
		{name: "disconnected can recover", state: webrtc.PeerConnectionStateDisconnected},
		{name: "failed", state: webrtc.PeerConnectionStateFailed, want: true},
		{name: "closed", state: webrtc.PeerConnectionStateClosed, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := peerConnectionStateIsTerminal(test.state); got != test.want {
				t.Fatalf("peerConnectionStateIsTerminal(%s) = %t, want %t", test.state, got, test.want)
			}
		})
	}
}
