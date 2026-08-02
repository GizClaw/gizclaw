package gizwebrtc

import (
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
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

func TestConnReadReportsUnexpectedCloseCause(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection error = %v", err)
	}
	conn := &Conn{
		pc:       pc,
		services: make(map[uint64]*ServiceListener),
		streams:  make(map[uint64]map[*dataChannelConn]struct{}),
		closeCh:  make(chan struct{}),
		readCh:   make(chan directPacket),
	}
	want := errors.New("transport failed")
	if err := conn.closeWithError(want); err != nil {
		t.Fatalf("closeWithError error = %v", err)
	}
	if _, _, err := conn.Read(make([]byte, 1)); !errors.Is(err, want) {
		t.Fatalf("Read error = %v, want %v", err, want)
	}
}

func TestConnReadReportsNormalClose(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection error = %v", err)
	}
	conn := &Conn{
		pc:       pc,
		services: make(map[uint64]*ServiceListener),
		streams:  make(map[uint64]map[*dataChannelConn]struct{}),
		closeCh:  make(chan struct{}),
		readCh:   make(chan directPacket),
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if _, _, err := conn.Read(make([]byte, 1)); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("Read error = %v, want %v", err, giznet.ErrConnClosed)
	}
}

func TestSelectedICECandidatePairPrefersNominatedTraffic(t *testing.T) {
	report := webrtc.StatsReport{
		"non-nominated": webrtc.ICECandidatePairStats{
			ID:            "non-nominated",
			BytesSent:     1000,
			BytesReceived: 1000,
		},
		"nominated-low": webrtc.ICECandidatePairStats{
			ID:            "nominated-low",
			Nominated:     true,
			BytesSent:     10,
			BytesReceived: 10,
		},
		"nominated-high": webrtc.ICECandidatePairStats{
			ID:            "nominated-high",
			Nominated:     true,
			BytesSent:     20,
			BytesReceived: 20,
		},
	}
	pair, ok := selectedICECandidatePair(report)
	if !ok {
		t.Fatal("selectedICECandidatePair found no pair")
	}
	if pair.ID != "nominated-high" {
		t.Fatalf("selected pair = %q, want nominated-high", pair.ID)
	}
}
