package gizwebrtc

import (
	"fmt"
)

// ConnDiagnostics is a bounded, address-free snapshot of one parent WebRTC
// connection. It is intended for failure attribution, not steady-state
// telemetry.
type ConnDiagnostics struct {
	PeerConnectionState string
	ICEConnectionState  string
	SCTPTransportState  string
	SCTPBytesSent       uint64
	SCTPBytesReceived   uint64
	ICECounters         bool
	ICEPacketsSent      uint32
	ICEPacketsReceived  uint32
	ICEBytesSent        uint64
	ICEBytesReceived    uint64
	ICEDiscardedPackets uint32
	ICEDiscardedBytes   uint32
}

func (d ConnDiagnostics) String() string {
	return fmt.Sprintf(
		"peer_state=%s ice_state=%s sctp_state=%s sctp_tx_bytes=%d sctp_rx_bytes=%d "+
			"ice_counters=%t ice_tx_packets=%d ice_rx_packets=%d ice_tx_bytes=%d ice_rx_bytes=%d "+
			"ice_discarded_packets=%d ice_discarded_bytes=%d",
		d.PeerConnectionState,
		d.ICEConnectionState,
		d.SCTPTransportState,
		d.SCTPBytesSent,
		d.SCTPBytesReceived,
		d.ICECounters,
		d.ICEPacketsSent,
		d.ICEPacketsReceived,
		d.ICEBytesSent,
		d.ICEBytesReceived,
		d.ICEDiscardedPackets,
		d.ICEDiscardedBytes,
	)
}

// Diagnostics captures parent ICE and SCTP counters without exposing network
// addresses, candidate identifiers, credentials, or unbounded values.
func (c *Conn) Diagnostics() ConnDiagnostics {
	if c == nil || c.pc == nil {
		return ConnDiagnostics{
			PeerConnectionState: "unknown",
			ICEConnectionState:  "unknown",
			SCTPTransportState:  "unknown",
		}
	}
	diagnostics := ConnDiagnostics{
		PeerConnectionState: c.pc.ConnectionState().String(),
		ICEConnectionState:  c.pc.ICEConnectionState().String(),
		SCTPTransportState:  "unknown",
	}
	if transport := c.pc.SCTP(); transport != nil {
		diagnostics.SCTPTransportState = transport.State().String()
		stats := transport.Stats()
		diagnostics.SCTPBytesSent = stats.BytesSent
		diagnostics.SCTPBytesReceived = stats.BytesReceived
	}
	if observation := selectedICEObservation(c.pc); observation != nil {
		diagnostics.ICECounters = observation.CountersSupported
		diagnostics.ICEPacketsSent = observation.PacketsSent
		diagnostics.ICEPacketsReceived = observation.PacketsReceived
		diagnostics.ICEBytesSent = observation.BytesSent
		diagnostics.ICEBytesReceived = observation.BytesReceived
		diagnostics.ICEDiscardedPackets = observation.PacketsDiscardedOnSend
		diagnostics.ICEDiscardedBytes = observation.BytesDiscardedOnSend
	}
	return diagnostics
}

// DiagnosticString exposes the bounded parent snapshot to callers without
// requiring them to depend on the concrete WebRTC transport type.
func (c *Conn) DiagnosticString() string {
	return c.Diagnostics().String()
}

var _ interface{ DiagnosticString() string } = (*Conn)(nil)
var _ fmt.Stringer = ConnDiagnostics{}
