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

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	other := &Conn{pc: pc}
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
