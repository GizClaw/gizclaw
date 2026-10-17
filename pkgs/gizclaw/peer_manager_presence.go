package gizclaw

import (
	"context"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// publishPeerPresenceLocked queues a transition while m.mu orders generations.
// Publish only copies state and starts a worker; it performs no storage I/O.
func (m *Manager) publishPeerPresenceLocked(key giznet.PublicKey, state *activePeer, observed giznet.Conn) {
	if m.Presence == nil {
		return
	}
	snapshot := peerpresence.Snapshot{}
	if state != nil && state.conn != nil && !state.deleting {
		snapshot.Online = true
		snapshot.WorkspaceName = state.runningWorkspace
		observed = state.conn
	}
	if observed != nil {
		if info := observed.PeerInfo(); info != nil {
			snapshot.LastSeenAt = info.LastSeen
		}
	}
	if snapshot.LastSeenAt.IsZero() {
		snapshot.LastSeenAt = time.Now().UTC()
	}
	m.Presence.Publish(key, snapshot)
}

// refreshPeerRunPresence observes a runtime transition outside the Manager lock.
// A concurrent transition, disconnect or replacement invalidates the old read.
func (m *Manager) refreshPeerRunPresence(key giznet.PublicKey, conn giznet.Conn) {
	if m.Presence == nil {
		return
	}
	m.mu.Lock()
	state := m.peers[key]
	if state == nil || state.conn != conn || state.deleting || state.runStatus == nil {
		m.mu.Unlock()
		return
	}
	state.runPresenceRevision++
	revision, status := state.runPresenceRevision, state.runStatus
	m.mu.Unlock()
	run, err := status(context.Background())
	workspace := ""
	if err == nil && run.State == apitypes.PeerRunStatusStateRunning && run.WorkspaceName != nil {
		workspace = strings.TrimSpace(*run.WorkspaceName)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.peers[key]
	if current != state || current.conn != conn || current.deleting || current.runPresenceRevision != revision {
		return
	}
	if current.runningWorkspace != workspace {
		current.runningWorkspace = workspace
		m.publishPeerPresenceLocked(key, current, conn)
	}
}

// PeerRoomPresence reads online and in-room from one shared snapshot so a list
// item never combines different transition versions. In-room implies online.
func (m *Manager) PeerRoomPresence(ctx context.Context, publicKey, workspace string) (bool, time.Time, bool) {
	if m.Presence == nil {
		online, seen := m.PeerPresence(ctx, publicKey)
		return online, seen, online && m.PeerRunningWorkspace(ctx, publicKey, workspace)
	}
	key, err := parseSocialPingPeer(publicKey)
	if err != nil {
		return false, time.Time{}, false
	}
	snapshot, err := m.Presence.Read(ctx, key)
	if err != nil {
		return false, time.Time{}, false
	}
	workspace = strings.TrimSpace(workspace)
	return snapshot.Online, snapshot.LastSeenAt, snapshot.Online && workspace != "" && snapshot.WorkspaceName == workspace
}
