package gizwebrtc

import (
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

func TestConnDiagnosticsDuringRemoteClose(t *testing.T) {
	client, server := nativeChannelTestPair(t)
	if got := client.Diagnostics(); !got.ICECounters {
		t.Fatalf("connected Peer has no selected ICE counters: %+v", got)
	}
	started := make(chan struct{})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		for {
			_ = client.Diagnostics()
			_ = client.peerConnectionStateDetails()
			_ = client.peerConnectionCloseError(webrtc.PeerConnectionStateFailed)
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	<-started
	closeErr := server.Close()
	<-client.closeCh
	clientCloseErr := client.Close()
	close(stop)
	<-done
	if closeErr != nil || clientCloseErr != nil {
		t.Fatalf("close errors: server=%v client=%v", closeErr, clientCloseErr)
	}
	if _, err := client.Dial(1234); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("Dial after remote close = %v, want connection closed", err)
	}
}

func TestConnDiagnosticsString(t *testing.T) {
	diagnostics := ConnDiagnostics{
		PeerConnectionState: "connected",
		ICEConnectionState:  "connected",
		SCTPTransportState:  "connected",
		SCTPBytesSent:       101,
		SCTPBytesReceived:   202,
		ICECounters:         true,
		ICEPacketsSent:      3,
		ICEPacketsReceived:  4,
		ICEBytesSent:        303,
		ICEBytesReceived:    404,
		ICEDiscardedPackets: 5,
		ICEDiscardedBytes:   505,
	}
	want := "peer_state=connected ice_state=connected sctp_state=connected " +
		"sctp_tx_bytes=101 sctp_rx_bytes=202 ice_counters=true ice_tx_packets=3 " +
		"ice_rx_packets=4 ice_tx_bytes=303 ice_rx_bytes=404 ice_discarded_packets=5 " +
		"ice_discarded_bytes=505"
	if got := diagnostics.String(); got != want {
		t.Fatalf("ConnDiagnostics.String() = %q, want %q", got, want)
	}
}

func TestNilConnDiagnostics(t *testing.T) {
	var conn *Conn
	got := conn.Diagnostics()
	if got.PeerConnectionState != "unknown" || got.ICEConnectionState != "unknown" ||
		got.SCTPTransportState != "unknown" {
		t.Fatalf("nil Conn diagnostics = %+v", got)
	}
}
