package gizwebrtc

import (
	"net"

	"github.com/pion/webrtc/v4"
)

// ICECandidateObservation is an address-free snapshot of one ICE candidate.
// It intentionally omits candidate identifiers, addresses, ports, priorities,
// foundations, URLs, and provider credentials.
type ICECandidateObservation struct {
	Type          string
	Protocol      string
	AddressFamily string
	Component     uint16
}

// ICECandidatePairObservation is an immutable, address-free snapshot of the
// selected ICE path and the bounded counters Pion exposes for that path.
type ICECandidatePairObservation struct {
	Local     ICECandidateObservation
	Remote    ICECandidateObservation
	State     string
	Nominated bool

	CountersSupported       bool
	PacketsSent             uint32
	PacketsReceived         uint32
	BytesSent               uint64
	BytesReceived           uint64
	CurrentRoundTripTime    float64
	RequestsSent            uint64
	ResponsesReceived       uint64
	RetransmissionsSent     uint64
	RetransmissionsReceived uint64
	PacketsDiscardedOnSend  uint32
	BytesDiscardedOnSend    uint32
}

func selectedICEObservation(pc *webrtc.PeerConnection) *ICECandidatePairObservation {
	if pc == nil || pc.SCTP() == nil || pc.SCTP().Transport() == nil ||
		pc.SCTP().Transport().ICETransport() == nil {
		return nil
	}
	pair, err := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Local == nil || pair.Remote == nil {
		return nil
	}
	observation := &ICECandidatePairObservation{
		Local:     sanitizeICECandidate(pair.Local),
		Remote:    sanitizeICECandidate(pair.Remote),
		State:     "selected",
		Nominated: true,
	}
	if stats, ok := selectedICECandidatePairStats(pc.GetStats(), pair); ok {
		observation.State = string(stats.State)
		observation.Nominated = observation.Nominated || stats.Nominated
		observation.CountersSupported = true
		observation.PacketsSent = stats.PacketsSent
		observation.PacketsReceived = stats.PacketsReceived
		observation.BytesSent = stats.BytesSent
		observation.BytesReceived = stats.BytesReceived
		observation.CurrentRoundTripTime = stats.CurrentRoundTripTime
		observation.RequestsSent = stats.RequestsSent
		observation.ResponsesReceived = stats.ResponsesReceived
		observation.RetransmissionsSent = stats.RetransmissionsSent
		observation.RetransmissionsReceived = stats.RetransmissionsReceived
		observation.PacketsDiscardedOnSend = stats.PacketsDiscardedOnSend
		observation.BytesDiscardedOnSend = stats.BytesDiscardedOnSend
	}
	return observation
}

func selectedICECandidatePairStats(
	report webrtc.StatsReport,
	selected *webrtc.ICECandidatePair,
) (webrtc.ICECandidatePairStats, bool) {
	if selected == nil || selected.Local == nil || selected.Remote == nil {
		return webrtc.ICECandidatePairStats{}, false
	}
	for _, stat := range report {
		pair, ok := stat.(webrtc.ICECandidatePairStats)
		if !ok {
			continue
		}
		local, localOK := report[pair.LocalCandidateID].(webrtc.ICECandidateStats)
		remote, remoteOK := report[pair.RemoteCandidateID].(webrtc.ICECandidateStats)
		if localOK && remoteOK && iceCandidateStatsMatch(local, selected.Local) &&
			iceCandidateStatsMatch(remote, selected.Remote) {
			return pair, true
		}
	}
	return webrtc.ICECandidatePairStats{}, false
}

func iceCandidateStatsMatch(stats webrtc.ICECandidateStats, candidate *webrtc.ICECandidate) bool {
	return candidate != nil && stats.IP == candidate.Address && stats.Port == int32(candidate.Port) &&
		stats.Protocol == candidate.Protocol.String() && stats.CandidateType == candidate.Typ
}

func sanitizeICECandidate(candidate *webrtc.ICECandidate) ICECandidateObservation {
	if candidate == nil {
		return ICECandidateObservation{AddressFamily: "unknown"}
	}
	return ICECandidateObservation{
		Type:          candidate.Typ.String(),
		Protocol:      candidate.Protocol.String(),
		AddressFamily: iceAddressFamily(candidate.Address),
		Component:     candidate.Component,
	}
}

func iceAddressFamily(address string) string {
	ip := net.ParseIP(address)
	if ip == nil {
		return "unknown"
	}
	if ip.To4() != nil {
		return "ipv4"
	}
	return "ipv6"
}
