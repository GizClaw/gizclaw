package gizclaw

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestSharedPeerPresenceFollowsRuntimeAcrossManagers(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	a := peerpresence.New(store, giznet.PublicKey{1})
	b := peerpresence.New(store, giznet.PublicKey{2})
	for _, presence := range []*peerpresence.Service{a, b} {
		if err := presence.Start(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = presence.Close() })
	}
	runs := peerruntest.New(t)
	key := giznet.PublicKey{3}
	if _, err := runs.SetRunAgent(ctx, key, apitypes.AgentSelection{WorkspaceName: "room"}); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{Presence: a, PeerRun: runs}
	// SQL on the querying Server is deliberately unavailable. Shared presence
	// must neither require nor fall back to it, including while offline.
	queryRuns := peerruntest.New(t)
	if _, err := queryRuns.DB.ExecContext(ctx, `DROP TABLE peer_runs`); err != nil {
		t.Fatal(err)
	}
	query := &Manager{Presence: b, PeerRun: queryRuns}
	seen := time.Now().UTC()
	conn := &testGiznetConn{publicKey: key, peerInfo: &giznet.PeerInfo{LastSeen: seen}}
	manager.SetPeerUp(key, conn)
	runtime := &agenthost.Service{
		Host: peerConnTestHost{output: newPeerConnBlockingStream()}, PeerRun: runs, PublicKey: key,
		Source:          agenthost.StreamSourceFunc(func(context.Context) (genx.Stream, error) { return agenthost.NewInputStream(1), nil }),
		Consumer:        agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error { <-ctx.Done(); return nil }),
		OnStatusChanged: func() { manager.refreshPeerRunPresence(key, conn) },
	}
	t.Cleanup(func() { _, _ = runtime.Shutdown(context.Background()) })
	if err := manager.SetPeerRunStatus(key, conn, runtime.Status); err != nil {
		t.Fatal(err)
	}
	assertPresence := func(online, inRoom bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			gotOnline, gotSeen, gotRoom := query.PeerRoomPresence(ctx, key.String(), "room")
			if gotOnline == online && gotRoom == inRoom && gotSeen.Equal(seen) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("remote presence = %v, %v, %v; want %v, %v, %v", gotOnline, gotSeen, gotRoom, online, seen, inRoom)
			}
			time.Sleep(time.Millisecond)
		}
		gotOnline, gotSeen := query.PeerPresence(ctx, key.String())
		if gotOnline != online || !gotSeen.Equal(seen) || query.PeerRunningWorkspace(ctx, key.String(), "room") != inRoom {
			t.Fatal("separate Social presence entry points disagree with shared snapshot")
		}
		if query.PeerOnline(key.String()) {
			t.Fatal("shared presence changed the querying Server's local connection index")
		}
	}
	assertPresence(true, false)
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	assertPresence(true, true)
	if _, err := runtime.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	assertPresence(true, false)
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	assertPresence(true, true)
	next := &testGiznetConn{publicKey: key, peerInfo: &giznet.PeerInfo{LastSeen: seen}}
	manager.SetPeerUp(key, next)
	manager.SetPeerDown(key, conn)
	manager.refreshPeerRunPresence(key, conn)
	assertPresence(true, false)
	manager.SetPeerDown(key, next)
	assertPresence(false, false)
}
