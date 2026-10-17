package monitor

import (
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
)

func (s *nodeServer) deviceMetrics() *monitorapi.NodeDeviceMetrics {
	result := &monitorapi.NodeDeviceMetrics{Transports: make([]monitorapi.NodePeerTransport, 0, 4)}
	if s.node.OnlineDevices != nil {
		if count := s.node.OnlineDevices(); count >= 0 {
			result.OnlineDevices = new(uint64(count))
		}
	}
	role := "application"
	if s.node.Role == "edge" {
		role = "edge"
	}
	for _, item := range gizwebrtc.ReadTrafficSnapshot(role) {
		result.Transports = append(result.Transports, monitorapi.NodePeerTransport{
			PeerKind:  monitorapi.NodePeerTransportPeerKind(item.PeerKind),
			RtpTracks: monitorapi.MonitorDirections{Rx: uint64(max(0, item.RTPRXTracks)), Tx: uint64(max(0, item.RTPTXTracks))},
			DataChannels: monitorapi.MonitorDataChannels{
				Packet: uint64(max(0, item.PacketChannels)), Service: uint64(max(0, item.ServiceChannels)), Native: uint64(max(0, item.NativeChannels)),
			},
			RtpPacketsTotal:          monitorapi.MonitorDirections{Rx: item.RTPRXPackets, Tx: item.RTPTXPackets},
			DataChannelMessagesTotal: monitorapi.MonitorDirections{Rx: item.DataChannelRXMessages, Tx: item.DataChannelTXMessages},
		})
	}
	return result
}
