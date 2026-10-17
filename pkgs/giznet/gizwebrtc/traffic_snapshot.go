package gizwebrtc

// PeerTrafficSnapshot groups process-local physical resources by the bounded
// peer kind. It combines dialing and accepting endpoints without enumerating
// connections. Packet/message counters are cumulative for the process lifetime.
type PeerTrafficSnapshot struct {
	PeerKind                                        string
	RTPRXTracks, RTPTXTracks                        int64
	PacketChannels, ServiceChannels, NativeChannels int64
	RTPRXPackets, RTPTXPackets                      uint64
	DataChannelRXMessages, DataChannelTXMessages    uint64
}

// ReadTrafficSnapshot reads fixed-size atomic counters for a node. nodeRole uses
// the same application/edge values as ListenConfig.MetricsNodeRole.
func ReadTrafficSnapshot(nodeRole string) []PeerTrafficSnapshot {
	node := 0
	if normalizedMetricsNodeRole(nodeRole) == metricsNodeRoleEdge {
		node = 1
	}
	result := make([]PeerTrafficSnapshot, len(metricsPeerKinds))
	for kind, name := range metricsPeerKinds {
		item := &result[kind]
		item.PeerKind = name
		for role := range trafficMetrics[node] {
			counts := &trafficMetrics[node][role][kind]
			item.RTPRXTracks += counts.rtp[0].active.Load()
			item.RTPTXTracks += counts.rtp[1].active.Load()
			item.RTPRXPackets += counts.rtp[0].packets.Load()
			item.RTPTXPackets += counts.rtp[1].packets.Load()
			item.PacketChannels += counts.channels[channelPacket].active.Load()
			item.ServiceChannels += counts.channels[channelService].active.Load()
			item.NativeChannels += counts.channels[channelNative].active.Load()
			for i := range counts.channels {
				item.DataChannelRXMessages += counts.channels[i].rx.Load()
				item.DataChannelTXMessages += counts.channels[i].tx.Load()
			}
		}
	}
	return result
}
