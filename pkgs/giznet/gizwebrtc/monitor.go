package gizwebrtc

import "sync/atomic"

var monitorRX atomic.Uint64
var monitorTX atomic.Uint64

// MonitorSnapshot reports this process's WebRTC associations and service payload bytes.
// Byte counters are monotonic for the process lifetime and exclude ICE/DTLS overhead.
type MonitorSnapshot struct {
	Connections int    `json:"connections"`
	Services    int    `json:"services"`
	RXBytes     uint64 `json:"rx_bytes"`
	TXBytes     uint64 `json:"tx_bytes"`
}

// ReadMonitorSnapshot takes a short in-memory snapshot without network I/O.
func ReadMonitorSnapshot() MonitorSnapshot {
	activeMetrics.Lock()
	connections, services := 0, 0
	for _, n := range activeMetrics.connections {
		connections += n
	}
	for _, n := range activeMetrics.services {
		services += n
	}
	activeMetrics.Unlock()
	return MonitorSnapshot{Connections: connections, Services: services, RXBytes: monitorRX.Load(), TXBytes: monitorTX.Load()}
}
