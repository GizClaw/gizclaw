package gizedge

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestGatewayDeviceCountDeduplicatesReplacementAndLateRemoval(t *testing.T) {
	count := new(atomic.Int64)
	g := &Gateway{cfg: Config{deviceCount: count}, sessions: make(map[*gatewaySession]struct{})}
	old := &gatewaySession{client: &failingGiznetConn{publicKey: giznet.PublicKey{1}}}
	next := &gatewaySession{client: &failingGiznetConn{publicKey: giznet.PublicKey{1}}}
	other := &gatewaySession{client: &failingGiznetConn{publicKey: giznet.PublicKey{2}}}
	assertCount := func(want int64) {
		t.Helper()
		if got := count.Load(); got != want {
			t.Fatalf("device count = %d, want %d", got, want)
		}
	}
	g.addSession(old)
	g.addSession(next)
	g.addSession(next)
	assertCount(1)
	g.removeSession(old)
	g.removeSession(old)
	assertCount(1)
	g.addSession(other)
	assertCount(2)
	g.removeSession(next)
	g.removeSession(other)
	assertCount(0)
	if len(g.deviceSessions) != 0 {
		t.Fatal("disconnected identities retained")
	}
}

func TestGatewayDeviceCountConcurrentSessions(t *testing.T) {
	count := new(atomic.Int64)
	g := &Gateway{cfg: Config{deviceCount: count}, sessions: make(map[*gatewaySession]struct{})}
	sessions := make([]*gatewaySession, 100)
	var group sync.WaitGroup
	for i := range sessions {
		session := &gatewaySession{client: &failingGiznetConn{publicKey: giznet.PublicKey{byte(i%2 + 1)}}}
		sessions[i] = session
		group.Go(func() { g.addSession(session) })
	}
	group.Wait()
	if got := count.Load(); got != 2 {
		t.Fatalf("concurrent distinct devices = %d, want 2", got)
	}
	for _, session := range sessions {
		group.Go(func() { g.removeSession(session) })
	}
	group.Wait()
	if got := count.Load(); got != 0 {
		t.Fatalf("count after concurrent close = %d", got)
	}
}
