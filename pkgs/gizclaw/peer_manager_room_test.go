package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// TestPeerRunningWorkspaceFollowsRuntime covers the in_room source
// server.friend_group.members.list reads: a Peer counts only while its active
// connection runs exactly the asked Workspace.
func TestPeerRunningWorkspaceFollowsRuntime(t *testing.T) {
	ctx := context.Background()
	key := giznet.PublicKey{6}
	runs := peerruntest.New(t)
	if _, err := runs.SetRunAgent(ctx, key, apitypes.AgentSelection{WorkspaceName: "team"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: newPeerConnBlockingStream()},
		PeerRun:   runs,
		PublicKey: key,
		Source: agenthost.StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return agenthost.NewInputStream(1), nil
		}),
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	manager := &Manager{PeerRun: runs}
	conn := &testGiznetConn{publicKey: key}
	running := func(workspace string) bool {
		return manager.PeerRunningWorkspace(ctx, key.String(), workspace)
	}

	if err := manager.SetPeerRunStatus(key, conn, runtime.Status); !errors.Is(err, ErrPeerConnNotActive) {
		t.Fatalf("SetPeerRunStatus before activation = %v", err)
	}
	manager.SetPeerUp(key, conn)
	if running("team") {
		t.Fatal("online Peer without a registered runtime counted as in the Room")
	}
	if err := manager.SetPeerRunStatus(key, conn, runtime.Status); err != nil {
		t.Fatalf("SetPeerRunStatus() error = %v", err)
	}
	if running("team") {
		t.Fatal("Peer with a stopped runtime counted as in the Room")
	}

	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !running("team") || !running(" team ") {
		t.Fatal("Peer running the Workspace is not in the Room")
	}
	if running("other") || running("") || manager.PeerRunningWorkspace(ctx, "not-a-key", "team") ||
		manager.PeerRunningWorkspace(ctx, giznet.PublicKey{7}.String(), "team") {
		t.Fatal("another Workspace, malformed key or unknown Peer counted as in the Room")
	}

	if _, err := runtime.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if running("team") {
		t.Fatal("Peer stayed in the Room after server.run.stop")
	}

	// A replacement connection starts without the previous generation's
	// runtime, and a disconnected Peer is never in the Room.
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	t.Cleanup(func() { _, _ = runtime.Shutdown(context.Background()) })
	next := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, next)
	if running("team") {
		t.Fatal("replacement connection inherited the previous runtime")
	}
	if err := manager.SetPeerRunStatus(key, conn, runtime.Status); !errors.Is(err, ErrPeerConnNotActive) {
		t.Fatalf("SetPeerRunStatus for a replaced connection = %v", err)
	}
	if err := manager.SetPeerRunStatus(key, next, runtime.Status); err != nil {
		t.Fatalf("SetPeerRunStatus() error = %v", err)
	}
	if !running("team") {
		t.Fatal("Peer running the Workspace on the replacement connection is not in the Room")
	}
	manager.SetPeerDown(key, next)
	if running("team") {
		t.Fatal("offline Peer counted as in the Room")
	}
}
