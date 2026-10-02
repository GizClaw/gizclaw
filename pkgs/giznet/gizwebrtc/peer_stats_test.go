package gizwebrtc

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestPeerStatsShutdownWaitsForReadersAndIsolatesPeers(t *testing.T) {
	conn := &Conn{}
	if !conn.beginStats() || !conn.beginStats() {
		t.Fatal("open Peer rejected stats readers")
	}
	done := conn.stopStats()
	if conn.beginStats() {
		t.Fatal("closing Peer admitted a new stats reader")
	}
	conn.endStats()
	select {
	case <-done:
		t.Fatal("shutdown completed with an active stats reader")
	default:
	}

	other, _ := nativeChannelTestPair(t)
	if len(other.collectStats()) == 0 {
		t.Fatal("another Peer's snapshots stopped during shutdown")
	}
	conn.endStats()
	<-done
	if other.stopStats() == nil {
		t.Fatal("completed snapshot has no completion signal")
	}
	if got := other.collectStats(); got != nil {
		t.Fatal("closed stats collector still read Pion state")
	}
}

func TestPeerStatsDuringNativePeerClose(t *testing.T) {
	conn, _ := nativeChannelTestPair(t)
	report := conn.collectStats()
	pair, ok := selectedICECandidatePair(report)
	if !ok {
		t.Fatal("connected Peer has no native ICE pair counters")
	}
	for _, stat := range report {
		switch stat.(type) {
		case webrtc.ICECandidatePairStats, webrtc.ICECandidateStats:
		default:
			t.Fatalf("ICE diagnostics collected non-ICE stats: %T", stat)
		}
	}
	if pair.LocalCandidateID == "" || pair.RemoteCandidateID == "" {
		t.Fatal("native ICE pair candidate identifiers missing")
	}
	started, stop, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		for {
			_ = conn.Diagnostics()
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	<-started
	// Bypass Conn.close to exercise the native destruction path from the CI race.
	closeErr := conn.pc.Close()
	close(stop)
	<-done
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
