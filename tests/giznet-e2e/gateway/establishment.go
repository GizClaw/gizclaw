package main

import (
	"math"
	"slices"
	"time"
)

const (
	phaseKeyGeneration        = "key_generation"
	phaseClientDial           = "client_dial"
	phaseTransportDial        = "transport_dial"
	phaseHTTPSignaling        = "http_signaling"
	phaseTransportOther       = "transport_other"
	phaseMandatoryEventStream = "mandatory_event_stream"
	phaseClientPeerConnection = "client_peer_connection_construction"
	phaseClientICEGathering   = "client_ice_gathering"
	phaseDTLSSCTPDataChannel  = "dtls_sctp_datachannel"
	phaseServerPeerConnection = "server_peer_connection_construction"
	phaseServerSetRemote      = "server_set_remote_description"
	phaseServerCreateAnswer   = "server_create_answer"
	phaseServerSetLocal       = "server_set_local_description"
	phaseServerICEGathering   = "server_ice_gathering"
	phaseServerRewriteSDP     = "server_rewrite_sdp"
)

var unsupportedEstablishmentPhases = map[string]string{
	phaseClientPeerConnection: "Pion does not expose this boundary through the current gizwebrtc Dial contract",
	phaseClientICEGathering:   "Pion does not expose this boundary through the current gizwebrtc Dial contract",
	phaseDTLSSCTPDataChannel:  "DTLS, SCTP, and DataChannel readiness are combined in transport_dial",
}

var serverTimingPhases = map[string]string{
	"giz_peer_connection": phaseServerPeerConnection,
	"giz_set_remote":      phaseServerSetRemote,
	"giz_create_answer":   phaseServerCreateAnswer,
	"giz_set_local":       phaseServerSetLocal,
	"giz_ice_gathering":   phaseServerICEGathering,
	"giz_rewrite_sdp":     phaseServerRewriteSDP,
}

type establishmentSummary struct {
	StartedAt               time.Time                            `json:"started_at"`
	FinishedAt              time.Time                            `json:"finished_at"`
	Duration                time.Duration                        `json:"duration"`
	UsableSessionsPerSecond float64                              `json:"usable_sessions_per_second"`
	Dial                    latencySummary                       `json:"dial_ms"`
	Phases                  map[string]establishmentPhaseSummary `json:"phases"`
	Sessions                []establishmentSessionResult         `json:"sessions"`
}

type establishmentPhaseSummary struct {
	Supported bool           `json:"supported"`
	Reason    string         `json:"reason,omitempty"`
	Latency   latencySummary `json:"latency_ms"`
}

type establishmentSessionResult struct {
	Index        int                      `json:"index"`
	Edge         string                   `json:"edge"`
	Upstream     string                   `json:"upstream,omitempty"`
	StartedAt    time.Time                `json:"started_at"`
	Duration     time.Duration            `json:"duration"`
	DialDuration time.Duration            `json:"dial_duration"`
	Phases       map[string]time.Duration `json:"phases"`
	Error        string                   `json:"error,omitempty"`
}

func summarizeEstablishment(
	startedAt time.Time,
	finishedAt time.Time,
	attempts []establishmentSessionResult,
) establishmentSummary {
	summary := establishmentSummary{
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Phases:     make(map[string]establishmentPhaseSummary),
		Sessions:   sortedEstablishmentAttempts(attempts),
	}
	dialDurations := make([]time.Duration, 0, len(attempts))
	phaseDurations := make(map[string][]time.Duration)
	established := 0
	for _, attempt := range attempts {
		if attempt.Error == "" && attempt.DialDuration > 0 {
			established++
			dialDurations = append(dialDurations, attempt.DialDuration)
		}
		for phase, duration := range attempt.Phases {
			if duration >= 0 {
				phaseDurations[phase] = append(phaseDurations[phase], duration)
			}
		}
	}
	if summary.Duration > 0 {
		summary.UsableSessionsPerSecond = float64(established) / summary.Duration.Seconds()
	}
	summary.Dial = summarizeLatency(dialDurations)
	for phase, durations := range phaseDurations {
		summary.Phases[phase] = establishmentPhaseSummary{
			Supported: true,
			Latency:   summarizeLatency(durations),
		}
	}
	for phase, reason := range unsupportedEstablishmentPhases {
		if _, ok := summary.Phases[phase]; !ok {
			summary.Phases[phase] = establishmentPhaseSummary{
				Supported: false,
				Reason:    reason,
			}
		}
	}
	return summary
}

func establishmentWithin(summary establishmentSummary, config artifactConfig) bool {
	if config.MinEstablishmentRate > 0 &&
		(summary.UsableSessionsPerSecond < config.MinEstablishmentRate ||
			math.IsNaN(summary.UsableSessionsPerSecond)) {
		return false
	}
	if config.MaxDialP95 > 0 && durationFromMilliseconds(summary.Dial.P95) > config.MaxDialP95 {
		return false
	}
	if config.MaxDialP99 > 0 && durationFromMilliseconds(summary.Dial.P99) > config.MaxDialP99 {
		return false
	}
	return true
}

func durationFromMilliseconds(value float64) time.Duration {
	return time.Duration(math.Round(value * float64(time.Millisecond)))
}

func sortedEstablishmentAttempts(attempts []establishmentSessionResult) []establishmentSessionResult {
	result := append([]establishmentSessionResult(nil), attempts...)
	slices.SortFunc(result, func(a, b establishmentSessionResult) int {
		return a.Index - b.Index
	})
	return result
}
