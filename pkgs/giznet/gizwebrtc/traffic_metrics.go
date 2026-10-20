package gizwebrtc

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/pion/datachannel"
)

const (
	metricDataChannelsActive  = "giz_webrtc_datachannels_active"
	metricDataChannelMessages = "giz_webrtc_datachannel_messages_total"
	metricRTPTracksActive     = "giz_webrtc_rtp_tracks_active"
	metricRTPPackets          = "giz_webrtc_rtp_packets_total"
)

const (
	channelPacket = iota
	channelService
	channelNative
)

type channelMetrics struct {
	active atomic.Int64
	rx     atomic.Uint64
	tx     atomic.Uint64
}

type transportMetrics struct {
	used     atomic.Bool
	channels [3]channelMetrics
	rtp      [2]rtpMetrics
}

type rtpMetrics struct {
	active  atomic.Int64
	packets atomic.Uint64
}

// Dimensions are fixed process/connection roles, never peer or channel IDs.
var trafficMetrics [2][3][4]transportMetrics

var metricsPeerKinds = [...]string{"device", "upstream", "admin", "unknown"}

func metricsPeerKindIndex(kind string) int {
	for i, name := range metricsPeerKinds {
		if name == kind {
			return i
		}
	}
	return len(metricsPeerKinds) - 1
}

func (c *Conn) trafficMetrics() *transportMetrics {
	node, role := 0, 2
	if normalizedMetricsNodeRole(c.nodeRole) == metricsNodeRoleEdge {
		node = 1
	}
	switch c.metricsRole() {
	case "client":
		role = 0
	case "server":
		role = 1
	}
	counts := &trafficMetrics[node][role][metricsPeerKindIndex(c.metricsPeerKind)]
	counts.used.Store(true)
	return counts
}

// MetricsSnapshot reads fixed-size process-local transport counters. Hosts pass
// this to gizmetrics.WithCollector for periodic samples, including unchanged
// gauges and idle counters. nodeRole follows ListenConfig.MetricsNodeRole.
// Counts cover physical WebRTC resources; tunneled audio is DataChannel traffic.
func MetricsSnapshot(at time.Time, nodeRole string) []metrics.Sample {
	nodeRole = normalizedMetricsNodeRole(nodeRole)
	node := 0
	if nodeRole == metricsNodeRoleEdge {
		node = 1
	}
	var samples []metrics.Sample
	for i, role := range []string{"client", "server", "unknown"} {
		for kindIndex, peerKind := range metricsPeerKinds {
			counts := &trafficMetrics[node][i][kindIndex]
			configured := role == "server" && peerKind == "device" ||
				nodeRole == "edge" && role == "client" && peerKind == "upstream" ||
				nodeRole == "application" && role == "server" && (peerKind == "upstream" || peerKind == "admin")
			if !configured && !counts.used.Load() {
				continue
			}
			for kind, name := range []string{"packet", "service", "native"} {
				channel := &counts.channels[kind]
				samples = append(samples, metrics.Sample{Name: metricDataChannelsActive, Timestamp: at,
					Labels: map[string]string{"node_role": nodeRole, "role": role, "peer_kind": peerKind, "channel_kind": name}, Value: float64(channel.active.Load())})
				for direction, value := range map[string]uint64{"rx": channel.rx.Load(), "tx": channel.tx.Load()} {
					samples = append(samples, metrics.Sample{Name: metricDataChannelMessages, Timestamp: at,
						Labels: map[string]string{"node_role": nodeRole, "role": role, "peer_kind": peerKind, "channel_kind": name, "direction": direction}, Value: float64(value)})
				}
			}
			for directionIndex, direction := range []string{"rx", "tx"} {
				track := &counts.rtp[directionIndex]
				samples = append(samples, metrics.Sample{Name: metricRTPTracksActive, Timestamp: at,
					Labels: map[string]string{"node_role": nodeRole, "role": role, "peer_kind": peerKind, "direction": direction}, Value: float64(track.active.Load())})
				samples = append(samples, metrics.Sample{Name: metricRTPPackets, Timestamp: at,
					Labels: map[string]string{"node_role": nodeRole, "role": role, "peer_kind": peerKind, "direction": direction}, Value: float64(track.packets.Load())})
			}
		}
	}
	return samples
}

type meteredDataChannel struct {
	datachannel.ReadWriteCloserDeadliner
	counts *channelMetrics
	once   sync.Once
}

func (c *Conn) meterDataChannel(raw datachannel.ReadWriteCloserDeadliner, kind int) *meteredDataChannel {
	counts := &c.trafficMetrics().channels[kind]
	counts.active.Add(1)
	return &meteredDataChannel{ReadWriteCloserDeadliner: raw, counts: counts}
}

func (c *meteredDataChannel) ReadDataChannel(buf []byte) (int, bool, error) {
	n, text, err := c.ReadWriteCloserDeadliner.ReadDataChannel(buf)
	if err == nil {
		c.counts.rx.Add(1)
	} else if nativeChannelErrorIsTerminal(err) {
		c.release()
	}
	return n, text, err
}

func (c *meteredDataChannel) WriteDataChannel(buf []byte, text bool) (int, error) {
	n, err := c.ReadWriteCloserDeadliner.WriteDataChannel(buf, text)
	if err == nil && n == len(buf) {
		c.counts.tx.Add(1)
	} else if nativeChannelErrorIsTerminal(err) {
		c.release()
	}
	return n, err
}

func (c *meteredDataChannel) release() {
	c.once.Do(func() { c.counts.active.Add(-1) })
}

func (c *meteredDataChannel) Close() error {
	c.release()
	return c.ReadWriteCloserDeadliner.Close()
}
