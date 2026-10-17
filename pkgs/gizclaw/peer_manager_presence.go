package gizclaw

import (
	"context"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// publishPeerPresenceLocked queues a transition while m.mu orders generations.
// Publish only copies state and starts a worker; it performs no storage I/O.
func (m *Manager) publishPeerPresenceLocked(state *activePeer, observed giznet.Conn) {
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
	if state != nil && state.presencePublisher != nil {
		state.presencePublisher.Publish(snapshot)
	}
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
		m.publishPeerPresenceLocked(current, conn)
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

// PeerRoomPresenceBatch reads a list page through the shared presence Store's
// bulk reads. Legacy Managers without shared presence retain their local
// per-Peer behavior, with bounded concurrency and the caller's page deadline.
func (m *Manager) PeerRoomPresenceBatch(ctx context.Context, publicKeys []string, workspace string) []peerpresence.Snapshot {
	snapshots := make([]peerpresence.Snapshot, len(publicKeys))
	if m.Presence == nil {
		socialutil.ReadPresencePage(ctx, len(publicKeys), func(ctx context.Context, i int) {
			if ctx.Err() != nil {
				return
			}
			online, seen, inRoom := m.PeerRoomPresence(ctx, publicKeys[i], workspace)
			snapshots[i] = peerpresence.Snapshot{Online: online, LastSeenAt: seen}
			if inRoom {
				snapshots[i].WorkspaceName = workspace
			}
		})
		return snapshots
	}
	var keys []giznet.PublicKey
	var indices []int
	for i, publicKey := range publicKeys {
		if key, err := parseSocialPingPeer(publicKey); err == nil {
			keys = append(keys, key)
			indices = append(indices, i)
		}
	}
	for i, result := range m.Presence.ReadBatch(ctx, keys) {
		if result.Err == nil {
			snapshots[indices[i]] = result.Snapshot
		}
	}
	return snapshots
}
