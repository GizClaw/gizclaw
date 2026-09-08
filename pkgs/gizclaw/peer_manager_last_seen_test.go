package gizclaw

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// TestPeerRuntimeLastSeenSurvivesDisconnect covers the two contract points of
// last_seen_at: an online Peer reports the activity observed on its
// connection, and an offline Peer keeps the last activity recorded when that
// connection went down instead of a zero timestamp.
func TestPeerRuntimeLastSeenSurvivesDisconnect(t *testing.T) {
	manager := &Manager{PeerRun: peerruntest.New(t)}
	key := giznet.PublicKey{1}
	activity := time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)
	conn := &testGiznetConn{publicKey: key, peerInfo: &giznet.PeerInfo{PublicKey: key, LastSeen: activity}}
	manager.SetPeerUp(key, conn)

	runtime := manager.PeerRuntime(context.Background(), key)
	if !runtime.Online || !runtime.LastSeenAt.Equal(activity) {
		t.Fatalf("online runtime = %+v, want last seen %v", runtime, activity)
	}

	// Traffic advances the connection's own clock, and the runtime follows it
	// rather than reporting the query time.
	later := activity.Add(30 * time.Second)
	conn.peerInfo.LastSeen = later
	runtime = manager.PeerRuntime(context.Background(), key)
	if !runtime.Online || !runtime.LastSeenAt.Equal(later) {
		t.Fatalf("runtime after traffic = %+v, want last seen %v", runtime, later)
	}

	manager.SetPeerDown(key, conn)
	runtime = manager.PeerRuntime(context.Background(), key)
	if runtime.Online {
		t.Fatalf("runtime after disconnect = %+v, want offline", runtime)
	}
	if !runtime.LastSeenAt.Equal(later) {
		t.Fatalf("offline last seen = %v, want %v", runtime.LastSeenAt, later)
	}
}

func TestPeerRuntimeLastSeenRecordedOnForcePeerDown(t *testing.T) {
	manager := &Manager{PeerRun: peerruntest.New(t)}
	key := giznet.PublicKey{2}
	activity := time.Date(2026, 9, 7, 11, 30, 0, 0, time.UTC)
	conn := &testGiznetConn{publicKey: key, peerInfo: &giznet.PeerInfo{PublicKey: key, LastSeen: activity}}
	manager.SetPeerUp(key, conn)

	manager.ForcePeerDown(key)
	runtime := manager.PeerRuntime(context.Background(), key)
	if runtime.Online || !runtime.LastSeenAt.Equal(activity) {
		t.Fatalf("runtime after ForcePeerDown = %+v, want offline at %v", runtime, activity)
	}
}

func TestPeerRuntimeLastSeenZeroForUnknownPeer(t *testing.T) {
	manager := &Manager{PeerRun: peerruntest.New(t)}
	runtime := manager.PeerRuntime(context.Background(), giznet.PublicKey{3})
	if runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("never-seen runtime = %+v", runtime)
	}
}
