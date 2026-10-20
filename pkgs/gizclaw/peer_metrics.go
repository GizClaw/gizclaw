package gizclaw

import (
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

// DeviceMetricsSnapshot reads the Server's activated device count without
// enumerating peers or reading storage. Pass it to gizmetrics.WithCollector to
// report a fresh value even while no connections change. Edge/admin identities
// and activation reservations are excluded; replacements count once.
func (s *Server) DeviceMetricsSnapshot(at time.Time) []metrics.Sample {
	return []metrics.Sample{{Name: "gizclaw_devices_online", Timestamp: at, Value: float64(s.deviceCount.Load())}}
}

// OnlineDevices reads the current distinct active Client count without storage
// access. It is the same value sampled by DeviceMetricsSnapshot.
func (s *Server) OnlineDevices() int64 { return s.deviceCount.Load() }

// Call only while holding m.mu, after changing the connection, role, or fence.
func (m *Manager) updateDeviceCountLocked(state *activePeer) {
	online := state.conn != nil && !state.deleting && state.role == apitypes.PeerRoleClient
	if state.countedDevice == online {
		return
	}
	if m.deviceCount == nil {
		m.deviceCount = new(atomic.Int64)
	}
	state.countedDevice = online
	if online {
		m.deviceCount.Add(1)
	} else {
		m.deviceCount.Add(-1)
	}
}

func (m *Manager) removePeerLocked(publicKey giznet.PublicKey) {
	if state := m.peers[publicKey]; state != nil && state.countedDevice {
		state.countedDevice = false
		m.deviceCount.Add(-1)
	}
	delete(m.peers, publicKey)
}
