package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const relayComparisonVersion = 1

type icePathEvidence struct {
	Version      int                         `json:"version"`
	UpstreamPath string                      `json:"upstream_path"`
	Roles        map[string][]iceObservation `json:"roles"`
	Passed       bool                        `json:"passed"`
}

type iceObservation struct {
	Role                    string  `json:"role"`
	UpstreamKind            string  `json:"upstream_kind"`
	UpstreamID              string  `json:"upstream_id"`
	ConnectionEpoch         uint64  `json:"connection_epoch"`
	RelayMember             *int    `json:"relay_member,omitempty"`
	LocalCandidateType      string  `json:"local_candidate_type"`
	LocalProtocol           string  `json:"local_protocol"`
	LocalAddressFamily      string  `json:"local_address_family"`
	LocalComponent          uint16  `json:"local_component"`
	RemoteCandidateType     string  `json:"remote_candidate_type"`
	RemoteProtocol          string  `json:"remote_protocol"`
	RemoteAddressFamily     string  `json:"remote_address_family"`
	RemoteComponent         uint16  `json:"remote_component"`
	PairState               string  `json:"pair_state"`
	Nominated               bool    `json:"nominated"`
	CountersSupported       bool    `json:"counters_supported"`
	PacketsSent             uint64  `json:"packets_sent"`
	PacketsReceived         uint64  `json:"packets_received"`
	BytesSent               uint64  `json:"bytes_sent"`
	BytesReceived           uint64  `json:"bytes_received"`
	CurrentRTTSeconds       float64 `json:"current_rtt_seconds"`
	RetransmissionsSent     uint64  `json:"retransmissions_sent"`
	RetransmissionsReceived uint64  `json:"retransmissions_received"`
}

var slogFieldPattern = regexp.MustCompile(`([a-z_]+)=("(?:[^"\\]|\\.)*"|[^ ]+)`)

func collectICEPathEvidence(upstreamPath, rawInputs string) (icePathEvidence, error) {
	report := icePathEvidence{Version: 1, UpstreamPath: upstreamPath, Roles: make(map[string][]iceObservation)}
	for input := range strings.SplitSeq(rawInputs, ",") {
		role, path, ok := strings.Cut(strings.TrimSpace(input), "=")
		if !ok || (role != "edge" && role != "edge2") || strings.TrimSpace(path) == "" {
			return report, fmt.Errorf("invalid ICE log input %q", input)
		}
		if _, exists := report.Roles[role]; exists {
			return report, fmt.Errorf("duplicate ICE log role %q", role)
		}
		observations, err := parseICEObservationLog(role, path)
		if err != nil {
			return report, err
		}
		report.Roles[role] = observations
	}
	for _, role := range []string{"edge", "edge2"} {
		observations, ok := report.Roles[role]
		if !ok {
			return report, fmt.Errorf("missing ICE log for %s", role)
		}
		if err := validateICEObservations(upstreamPath, role, observations); err != nil {
			return report, err
		}
	}
	report.Passed = true
	return report, nil
}

func parseICEObservationLog(role, path string) ([]iceObservation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s ICE log: %w", role, err)
	}
	defer file.Close()
	var observations []iceObservation
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "edge: upstream ICE selected") {
			continue
		}
		fields := parseSlogFields(line)
		observation, err := decodeICEObservation(role, fields)
		if err != nil {
			return nil, fmt.Errorf("parse %s ICE observation: %w", role, err)
		}
		observations = append(observations, observation)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s ICE log: %w", role, err)
	}
	return observations, nil
}

func parseSlogFields(line string) map[string]string {
	fields := make(map[string]string)
	for _, match := range slogFieldPattern.FindAllStringSubmatch(line, -1) {
		value := match[2]
		if strings.HasPrefix(value, `"`) {
			if unquoted, err := strconv.Unquote(value); err == nil {
				value = unquoted
			}
		}
		fields[match[1]] = value
	}
	return fields
}

func decodeICEObservation(role string, fields map[string]string) (iceObservation, error) {
	required := func(key string) (string, error) {
		value := fields[key]
		if value == "" {
			return "", fmt.Errorf("missing %s", key)
		}
		return value, nil
	}
	parseUint := func(key string, bits int) (uint64, error) {
		value, err := required(key)
		if err != nil {
			return 0, err
		}
		parsed, err := strconv.ParseUint(value, 10, bits)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}
	var result iceObservation
	result.Role = role
	var err error
	if result.UpstreamKind, err = required("upstream_kind"); err != nil {
		return result, err
	}
	if result.UpstreamID, err = required("upstream_id"); err != nil {
		return result, err
	}
	if result.LocalCandidateType, err = required("local_candidate_type"); err != nil {
		return result, err
	}
	if result.LocalProtocol, err = required("local_protocol"); err != nil {
		return result, err
	}
	if result.LocalAddressFamily, err = required("local_address_family"); err != nil {
		return result, err
	}
	if result.RemoteCandidateType, err = required("remote_candidate_type"); err != nil {
		return result, err
	}
	if result.RemoteProtocol, err = required("remote_protocol"); err != nil {
		return result, err
	}
	if result.RemoteAddressFamily, err = required("remote_address_family"); err != nil {
		return result, err
	}
	if result.PairState, err = required("pair_state"); err != nil {
		return result, err
	}
	if result.ConnectionEpoch, err = parseUint("connection_epoch", 64); err != nil {
		return result, err
	}
	value, err := parseUint("local_component", 16)
	if err != nil {
		return result, err
	}
	result.LocalComponent = uint16(value)
	value, err = parseUint("remote_component", 16)
	if err != nil {
		return result, err
	}
	result.RemoteComponent = uint16(value)
	if result.Nominated, err = strconv.ParseBool(fields["nominated"]); err != nil {
		return result, fmt.Errorf("nominated: %w", err)
	}
	if result.CountersSupported, err = strconv.ParseBool(fields["counters_supported"]); err != nil {
		return result, fmt.Errorf("counters_supported: %w", err)
	}
	for key, target := range map[string]*uint64{
		"packets_sent": &result.PacketsSent, "packets_received": &result.PacketsReceived,
		"bytes_sent": &result.BytesSent, "bytes_received": &result.BytesReceived,
		"retransmissions_sent":     &result.RetransmissionsSent,
		"retransmissions_received": &result.RetransmissionsReceived,
	} {
		if fields[key] == "" {
			continue
		}
		*target, err = strconv.ParseUint(fields[key], 10, 64)
		if err != nil {
			return result, fmt.Errorf("%s: %w", key, err)
		}
	}
	if fields["current_rtt_seconds"] != "" {
		result.CurrentRTTSeconds, err = strconv.ParseFloat(fields["current_rtt_seconds"], 64)
		if err != nil {
			return result, fmt.Errorf("current_rtt_seconds: %w", err)
		}
	}
	if fields["relay_member"] != "" {
		member, parseErr := strconv.Atoi(fields["relay_member"])
		if parseErr != nil || member < 0 {
			return result, errors.New("invalid relay_member")
		}
		result.RelayMember = &member
	}
	return result, nil
}

func validateICEObservations(path, role string, observations []iceObservation) error {
	if len(observations) != 5 {
		return fmt.Errorf("%s has %d ICE observations, want exactly 5", role, len(observations))
	}
	seen := make(map[string]struct{}, 5)
	control, gateways := 0, 0
	for _, observation := range observations {
		key := observation.UpstreamKind + "/" + observation.UpstreamID
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s has duplicate ICE observation %s", role, key)
		}
		seen[key] = struct{}{}
		if !observation.Nominated {
			return fmt.Errorf("%s %s is not nominated", role, key)
		}
		if observation.LocalProtocol == "" || observation.RemoteProtocol == "" ||
			observation.LocalComponent == 0 || observation.RemoteComponent == 0 {
			return fmt.Errorf("%s %s has incomplete candidate metadata", role, key)
		}
		switch observation.UpstreamKind {
		case "control":
			if observation.UpstreamID != "control" {
				return fmt.Errorf("%s has invalid control id %q", role, observation.UpstreamID)
			}
			control++
		case "gateway":
			gateways++
		default:
			return fmt.Errorf("%s has invalid upstream kind %q", role, observation.UpstreamKind)
		}
		if path == "relay" {
			if observation.LocalCandidateType != "relay" || observation.RelayMember == nil {
				return fmt.Errorf("%s %s did not prove a selected relay candidate", role, key)
			}
		} else if observation.LocalCandidateType == "relay" || observation.RemoteCandidateType == "relay" || observation.RelayMember != nil {
			return fmt.Errorf("%s %s unexpectedly used relay", role, key)
		}
	}
	if control != 1 || gateways != 4 {
		return fmt.Errorf("%s observations are control=%d gateway=%d, want 1/4", role, control, gateways)
	}
	return nil
}

type relayComparisonReport struct {
	Version          int                       `json:"version"`
	RepositoryCommit string                    `json:"repository_commit"`
	Cells            map[string]comparisonCell `json:"cells"`
	Material         bool                      `json:"material"`
	MaterialReasons  []string                  `json:"material_reasons,omitempty"`
	CausalOwner      string                    `json:"causal_owner"`
	CausalEvidence   *relayCausalEvidence      `json:"causal_evidence,omitempty"`
	CausalConclusion string                    `json:"causal_conclusion"`
	Qualified        bool                      `json:"qualified"`
}

type relayCausalEvidence struct {
	Diagnostic                   string  `json:"diagnostic"`
	DirectClientToListenerMbps   float64 `json:"direct_client_to_listener_mbps"`
	RelayClientToListenerMbps    float64 `json:"relay_client_to_listener_mbps"`
	ClientToListenerRatio        float64 `json:"client_to_listener_relay_to_direct_ratio"`
	DirectListenerToClientMbps   float64 `json:"direct_listener_to_client_mbps"`
	RelayListenerToClientMbps    float64 `json:"relay_listener_to_client_mbps"`
	ListenerToClientRatio        float64 `json:"listener_to_client_relay_to_direct_ratio"`
	CoturnReceivedBytesDelta     int64   `json:"coturn_received_bytes_delta"`
	CoturnSentBytesDelta         int64   `json:"coturn_sent_bytes_delta"`
	ProductEdgeAndServerExcluded bool    `json:"product_edge_and_server_excluded"`
	DirectDialP95MS              float64 `json:"direct_dial_p95_ms"`
	RelayDialP95MS               float64 `json:"relay_dial_p95_ms"`
	DirectDialP99MS              float64 `json:"direct_dial_p99_ms"`
	RelayDialP99MS               float64 `json:"relay_dial_p99_ms"`
	DirectRTTP95MS               float64 `json:"direct_rtt_p95_ms"`
	RelayRTTP95MS                float64 `json:"relay_rtt_p95_ms"`
	DirectRTTP99MS               float64 `json:"direct_rtt_p99_ms"`
	RelayRTTP99MS                float64 `json:"relay_rtt_p99_ms"`
}

type causalRequirements struct {
	Upload        bool
	Download      bool
	DialP95       bool
	DialP99       bool
	RTTP95        bool
	RTTP99        bool
	ResourceBound bool
}

type comparisonCell struct {
	Sessions       int                           `json:"sessions"`
	Direct         comparisonPath                `json:"direct"`
	Relay          comparisonPath                `json:"relay"`
	Ratios         comparisonMetrics             `json:"relay_to_direct_ratio"`
	PhaseRatios    map[string]comparisonPhase    `json:"phase_relay_to_direct_ratio"`
	ResourceRatios map[string]comparisonResource `json:"relay_to_direct_resource_ratio"`
}

type comparisonPath struct {
	Runs            []comparisonRun               `json:"runs"`
	Median          comparisonMetrics             `json:"median"`
	PhaseMedians    map[string]comparisonPhase    `json:"phase_medians"`
	ResourceMedians map[string]comparisonResource `json:"resource_medians"`
}

type comparisonRun struct {
	Repetition int                           `json:"repetition"`
	Metrics    comparisonMetrics             `json:"metrics"`
	Phases     map[string]comparisonPhase    `json:"phases"`
	Resources  map[string]comparisonResource `json:"resources"`
}

type comparisonPhase struct {
	Supported bool    `json:"supported"`
	Reason    string  `json:"reason,omitempty"`
	P50MS     float64 `json:"p50_ms"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
}

type comparisonResource struct {
	CPUSecondsDelta float64 `json:"cpu_seconds_delta"`
	PeakRSSBytes    float64 `json:"peak_rss_bytes"`
	PeakOpenFDs     float64 `json:"peak_open_fds"`
	OpenFDLimit     float64 `json:"open_fd_limit"`
	PeakUDPSockets  float64 `json:"peak_udp_sockets"`
	PeakUDP6Sockets float64 `json:"peak_udp6_sockets"`
	NetworkRXBytes  float64 `json:"network_rx_bytes_delta"`
	NetworkTXBytes  float64 `json:"network_tx_bytes_delta"`
}

type comparisonMetrics struct {
	EstablishmentRate    float64 `json:"establishment_sessions_per_second"`
	UploadMbps           float64 `json:"upload_mbps"`
	UploadSessionP50     float64 `json:"upload_per_session_p50_mbps"`
	UploadSessionP95     float64 `json:"upload_per_session_p95_mbps"`
	UploadSessionP99     float64 `json:"upload_per_session_p99_mbps"`
	DownloadMbps         float64 `json:"download_mbps"`
	DownloadSessionP50   float64 `json:"download_per_session_p50_mbps"`
	DownloadSessionP95   float64 `json:"download_per_session_p95_mbps"`
	DownloadSessionP99   float64 `json:"download_per_session_p99_mbps"`
	DialP50MS            float64 `json:"dial_p50_ms"`
	DialP95MS            float64 `json:"dial_p95_ms"`
	DialP99MS            float64 `json:"dial_p99_ms"`
	RTTP50MS             float64 `json:"rtt_p50_ms"`
	RTTP95MS             float64 `json:"rtt_p95_ms"`
	RTTP99MS             float64 `json:"rtt_p99_ms"`
	OpusCompletedPackets float64 `json:"opus_completed_packets"`
	OpusCompletedBytes   float64 `json:"opus_completed_bytes"`
	OpusPacketsPS        float64 `json:"opus_packets_per_second"`
	OpusBytesPS          float64 `json:"opus_bytes_per_second"`
	OpusWriteP50MS       float64 `json:"opus_write_p50_ms"`
	OpusWriteP95MS       float64 `json:"opus_write_p95_ms"`
	OpusWriteP99MS       float64 `json:"opus_write_p99_ms"`
}

type coturnPathEvidence struct {
	UpstreamPath  string             `json:"upstream_path"`
	Passed        bool               `json:"passed"`
	LiveBefore    coturnEvidencePair `json:"live_before"`
	AfterWorkload coturnEvidencePair `json:"after_workload"`
	Cleanup       coturnCleanupPair  `json:"cleanup"`
	TrafficDelta  coturnTrafficDelta `json:"traffic_delta"`
}

type coturnEvidencePair struct {
	CoturnA coturnCounters `json:"coturn_a"`
	CoturnB coturnCounters `json:"coturn_b"`
}

type coturnCleanupPair struct {
	CoturnA                                coturnCounters `json:"coturn_a"`
	CoturnB                                coturnCounters `json:"coturn_b"`
	AllocationsReturnedToZeroWithinSeconds int            `json:"allocations_returned_to_zero_within_seconds"`
}

type coturnCounters struct {
	Allocations   int64 `json:"allocations"`
	ReceivedBytes int64 `json:"received_bytes"`
	SentBytes     int64 `json:"sent_bytes"`
}

type coturnTrafficDelta struct {
	ReceivedBytes int64 `json:"received_bytes"`
	SentBytes     int64 `json:"sent_bytes"`
}

type giznetCoturnDiagnostic struct {
	RepositoryHead  string                        `json:"repository_head"`
	RepositoryDirty bool                          `json:"repository_dirty"`
	DialSamples     int                           `json:"dial_samples"`
	RTTSamples      int                           `json:"rtt_samples"`
	ThroughputRuns  int                           `json:"throughput_runs"`
	ThroughputBytes int                           `json:"throughput_bytes"`
	Paths           []giznetCoturnDiagnosticPath  `json:"paths"`
	Comparisons     []giznetCoturnDiagnosticRatio `json:"direct_to_coturn_comparisons"`
}

type giznetCoturnDiagnosticPath struct {
	Name               string         `json:"name"`
	DialTotalMS        latencySummary `json:"dial_total_ms"`
	RTTMS              latencySummary `json:"rtt_ms"`
	ClientMedianMbps   float64        `json:"client_to_listener_median_mbps"`
	ListenerMedianMbps float64        `json:"listener_to_client_median_mbps"`
	CoturnBefore       coturnCounters `json:"coturn_before"`
	CoturnAfter        coturnCounters `json:"coturn_after"`
}

type giznetCoturnDiagnosticRatio struct {
	Path                      string  `json:"path"`
	ClientToListenerMbpsRatio float64 `json:"client_to_listener_relay_to_direct_ratio"`
	ListenerToClientMbpsRatio float64 `json:"listener_to_client_relay_to_direct_ratio"`
}

func compareRelayCapacityArtifacts(directory string) (relayComparisonReport, error) {
	report := relayComparisonReport{Version: relayComparisonVersion, Cells: make(map[string]comparisonCell)}
	var causalNeeds causalRequirements
	runs := make(map[string]map[string][]comparisonRun)
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") ||
			strings.HasSuffix(entry.Name(), "-path.json") || strings.HasSuffix(entry.Name(), "-coturn.json") ||
			entry.Name() == "comparison.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var candidate artifact
		if err := json.Unmarshal(data, &candidate); err != nil || candidate.Config.UpstreamPath == "" {
			return nil
		}
		if !candidate.Passed || candidate.Extended == nil || candidate.Extended.Environment.RepositoryDirty {
			return fmt.Errorf("run %s is not passed clean-head evidence", path)
		}
		commit := candidate.Extended.Environment.RepositoryCommit
		if report.RepositoryCommit == "" {
			report.RepositoryCommit = commit
		}
		if commit == "" || commit != report.RepositoryCommit {
			return fmt.Errorf("run %s has mixed repository head", path)
		}
		if candidate.Config.Sessions != 100 && candidate.Config.Sessions != 500 {
			return fmt.Errorf("run %s has unsupported sessions=%d", path, candidate.Config.Sessions)
		}
		if candidate.Config.Repetition < 1 || candidate.Config.Repetition > 3 {
			return fmt.Errorf("run %s has invalid repetition", path)
		}
		if candidate.Config.OpusPackets != 50 || candidate.Opus.Failures != 0 {
			return fmt.Errorf("run %s lacks exact Opus evidence", path)
		}
		if err := validateFrozenRelayRun(path, candidate); err != nil {
			return err
		}
		base := strings.TrimSuffix(path, ".json")
		if err := validateRunSidecars(base, candidate.Config.UpstreamPath); err != nil {
			return err
		}
		key := strconv.Itoa(candidate.Config.Sessions)
		if runs[key] == nil {
			runs[key] = make(map[string][]comparisonRun)
		}
		runs[key][candidate.Config.UpstreamPath] = append(runs[key][candidate.Config.UpstreamPath], comparisonRun{
			Repetition: candidate.Config.Repetition,
			Metrics: comparisonMetrics{
				EstablishmentRate:  candidate.Establishment.UsableSessionsPerSecond,
				UploadMbps:         candidate.SpeedTest.Upload.Concurrent.AggregateMbps,
				UploadSessionP50:   candidate.SpeedTest.Upload.Concurrent.PerSessionMbps.P50,
				UploadSessionP95:   candidate.SpeedTest.Upload.Concurrent.PerSessionMbps.P95,
				UploadSessionP99:   candidate.SpeedTest.Upload.Concurrent.PerSessionMbps.P99,
				DownloadMbps:       candidate.SpeedTest.Download.Concurrent.AggregateMbps,
				DownloadSessionP50: candidate.SpeedTest.Download.Concurrent.PerSessionMbps.P50,
				DownloadSessionP95: candidate.SpeedTest.Download.Concurrent.PerSessionMbps.P95,
				DownloadSessionP99: candidate.SpeedTest.Download.Concurrent.PerSessionMbps.P99,
				DialP50MS:          candidate.Establishment.Dial.P50, DialP95MS: candidate.Establishment.Dial.P95,
				DialP99MS: candidate.Establishment.Dial.P99,
				RTTP50MS:  candidate.RTT.P50, RTTP95MS: candidate.RTT.P95, RTTP99MS: candidate.RTT.P99,
				OpusCompletedPackets: float64(candidate.Opus.Completed), OpusCompletedBytes: float64(candidate.Opus.CompletedBytes),
				OpusPacketsPS: candidate.Opus.PacketsPerSecond, OpusBytesPS: candidate.Opus.BytesPerSecond,
				OpusWriteP50MS: candidate.Opus.WriteLatency.P50, OpusWriteP95MS: candidate.Opus.WriteLatency.P95,
				OpusWriteP99MS: candidate.Opus.WriteLatency.P99,
			},
			Phases:    comparisonPhases(candidate.Establishment.Phases),
			Resources: comparisonResources(candidate.Extended.Roles),
		})
		return nil
	})
	if err != nil {
		return report, err
	}
	for _, sessions := range []int{100, 500} {
		key := strconv.Itoa(sessions)
		direct, err := validateComparisonRuns(runs[key]["direct"])
		if err != nil {
			return report, fmt.Errorf("sessions %d direct: %w", sessions, err)
		}
		relay, err := validateComparisonRuns(runs[key]["relay"])
		if err != nil {
			return report, fmt.Errorf("sessions %d relay: %w", sessions, err)
		}
		cell := comparisonCell{Sessions: sessions}
		directPhases, err := medianPhases(direct)
		if err != nil {
			return report, fmt.Errorf("sessions %d direct phases: %w", sessions, err)
		}
		relayPhases, err := medianPhases(relay)
		if err != nil {
			return report, fmt.Errorf("sessions %d relay phases: %w", sessions, err)
		}
		cell.Direct = comparisonPath{Runs: direct, Median: medianMetrics(direct), PhaseMedians: directPhases, ResourceMedians: medianResources(direct)}
		cell.Relay = comparisonPath{Runs: relay, Median: medianMetrics(relay), PhaseMedians: relayPhases, ResourceMedians: medianResources(relay)}
		cell.Ratios = ratioMetrics(cell.Relay.Median, cell.Direct.Median)
		cell.PhaseRatios = ratioPhases(cell.Relay.PhaseMedians, cell.Direct.PhaseMedians)
		cell.ResourceRatios = ratioResources(cell.Relay.ResourceMedians, cell.Direct.ResourceMedians)
		for direction, ratio := range map[string]float64{"upload": cell.Ratios.UploadMbps, "download": cell.Ratios.DownloadMbps} {
			if ratio < 0.9 {
				report.Material = true
				if direction == "upload" {
					causalNeeds.Upload = true
				} else {
					causalNeeds.Download = true
				}
				report.MaterialReasons = append(report.MaterialReasons, fmt.Sprintf("sessions-%d %s relay/direct throughput %.3f < 0.900", sessions, direction, ratio))
			}
		}
		for metric, values := range map[string][2]float64{
			"dial_p95": {cell.Direct.Median.DialP95MS, cell.Relay.Median.DialP95MS}, "dial_p99": {cell.Direct.Median.DialP99MS, cell.Relay.Median.DialP99MS},
			"rtt_p95": {cell.Direct.Median.RTTP95MS, cell.Relay.Median.RTTP95MS}, "rtt_p99": {cell.Direct.Median.RTTP99MS, cell.Relay.Median.RTTP99MS},
		} {
			if values[1]-values[0] >= 5 && values[1] > values[0]*1.2 {
				report.Material = true
				switch metric {
				case "dial_p95":
					causalNeeds.DialP95 = true
				case "dial_p99":
					causalNeeds.DialP99 = true
				case "rtt_p95":
					causalNeeds.RTTP95 = true
				case "rtt_p99":
					causalNeeds.RTTP99 = true
				}
				report.MaterialReasons = append(report.MaterialReasons, fmt.Sprintf("sessions-%d %s relay %.3fms vs direct %.3fms", sessions, metric, values[1], values[0]))
			}
		}
		for role, resource := range cell.Relay.ResourceMedians {
			if resource.OpenFDLimit > 0 && resource.PeakOpenFDs >= resource.OpenFDLimit*0.9 {
				report.Material = true
				causalNeeds.ResourceBound = true
				report.MaterialReasons = append(report.MaterialReasons, fmt.Sprintf(
					"sessions-%d relay %s median open FDs %.0f reached at least 90%% of limit %.0f",
					sessions, role, resource.PeakOpenFDs, resource.OpenFDLimit,
				))
			}
		}
		report.Cells[key] = cell
	}
	if report.Material {
		evidence, err := validateGiznetCoturnDiagnostic(filepath.Join(directory, "giznet-coturn.json"), report.RepositoryCommit, causalNeeds)
		if err != nil {
			return report, fmt.Errorf("material delta lacks bounded causal evidence: %w", err)
		}
		report.CausalOwner = "Coturn relay path on the local Docker host"
		report.CausalEvidence = &evidence
		report.CausalConclusion = "the same clean-head pure-Giznet diagnostic reproduces every material affected phase while Coturn carries traffic, excluding the product Edge and Server; the measured owner boundary is the local Coturn relay path, not a GizClaw Edge/Server capacity limit"
	} else {
		report.CausalOwner = "none"
		report.CausalConclusion = "no material owner observed within the tested 100/500 envelope"
	}
	report.Qualified = true
	return report, nil
}

func validateGiznetCoturnDiagnostic(path, repositoryCommit string, requirements causalRequirements) (relayCausalEvidence, error) {
	var evidence relayCausalEvidence
	data, err := os.ReadFile(path)
	if err != nil {
		return evidence, fmt.Errorf("read %s: %w", path, err)
	}
	var diagnostic giznetCoturnDiagnostic
	if err := json.Unmarshal(data, &diagnostic); err != nil {
		return evidence, fmt.Errorf("decode %s: %w", path, err)
	}
	if diagnostic.RepositoryDirty || diagnostic.RepositoryHead == "" || diagnostic.RepositoryHead != repositoryCommit {
		return evidence, errors.New("Giznet diagnostic is dirty or from a different repository head")
	}
	if diagnostic.DialSamples != 30 || diagnostic.RTTSamples != 200 ||
		diagnostic.ThroughputRuns != 3 || diagnostic.ThroughputBytes != 32<<20 {
		return evidence, errors.New("Giznet diagnostic changed its fixed sample counts or payload")
	}
	paths := make(map[string]giznetCoturnDiagnosticPath, len(diagnostic.Paths))
	for _, item := range diagnostic.Paths {
		if _, exists := paths[item.Name]; exists {
			return evidence, fmt.Errorf("duplicate Giznet diagnostic path %q", item.Name)
		}
		paths[item.Name] = item
	}
	direct, directOK := paths["direct"]
	relay, relayOK := paths["turn_rest"]
	if len(paths) != 3 || !directOK || !relayOK {
		return evidence, errors.New("Giznet diagnostic must contain direct, static, and turn_rest paths")
	}
	var ratio giznetCoturnDiagnosticRatio
	foundRatio := false
	for _, item := range diagnostic.Comparisons {
		if item.Path == "turn_rest" {
			ratio, foundRatio = item, true
			break
		}
	}
	receivedDelta := relay.CoturnAfter.ReceivedBytes - relay.CoturnBefore.ReceivedBytes
	sentDelta := relay.CoturnAfter.SentBytes - relay.CoturnBefore.SentBytes
	if !foundRatio || direct.ClientMedianMbps <= 0 || direct.ListenerMedianMbps <= 0 ||
		relay.ClientMedianMbps <= 0 || relay.ListenerMedianMbps <= 0 ||
		ratio.ClientToListenerMbpsRatio <= 0 || ratio.ListenerToClientMbpsRatio <= 0 ||
		receivedDelta <= 0 || sentDelta <= 0 {
		return evidence, errors.New("Giznet diagnostic lacks valid traffic-carrying Coturn evidence")
	}
	if direct.DialTotalMS.Count != diagnostic.DialSamples || relay.DialTotalMS.Count != diagnostic.DialSamples ||
		direct.RTTMS.Count != diagnostic.RTTSamples || relay.RTTMS.Count != diagnostic.RTTSamples {
		return evidence, errors.New("Giznet diagnostic path summaries changed their fixed sample counts")
	}
	evidence = relayCausalEvidence{
		Diagnostic:                 filepath.Base(path),
		DirectClientToListenerMbps: direct.ClientMedianMbps, RelayClientToListenerMbps: relay.ClientMedianMbps,
		ClientToListenerRatio:      ratio.ClientToListenerMbpsRatio,
		DirectListenerToClientMbps: direct.ListenerMedianMbps, RelayListenerToClientMbps: relay.ListenerMedianMbps,
		ListenerToClientRatio:    ratio.ListenerToClientMbpsRatio,
		CoturnReceivedBytesDelta: receivedDelta, CoturnSentBytesDelta: sentDelta,
		ProductEdgeAndServerExcluded: true,
		DirectDialP95MS:              direct.DialTotalMS.P95, RelayDialP95MS: relay.DialTotalMS.P95,
		DirectDialP99MS: direct.DialTotalMS.P99, RelayDialP99MS: relay.DialTotalMS.P99,
		DirectRTTP95MS: direct.RTTMS.P95, RelayRTTP95MS: relay.RTTMS.P95,
		DirectRTTP99MS: direct.RTTMS.P99, RelayRTTP99MS: relay.RTTMS.P99,
	}
	if err := validateCausalAlignment(requirements, evidence); err != nil {
		return relayCausalEvidence{}, err
	}
	return evidence, nil
}

func validateCausalAlignment(requirements causalRequirements, evidence relayCausalEvidence) error {
	if requirements.Upload && evidence.ClientToListenerRatio >= 0.9 {
		return errors.New("Giznet diagnostic did not reproduce the material upload phase")
	}
	if requirements.Download && evidence.ListenerToClientRatio >= 0.9 {
		return errors.New("Giznet diagnostic did not reproduce the material download phase")
	}
	regressed := func(direct, relay float64) bool {
		return direct > 0 && relay > direct*1.2
	}
	for metric, requiredAndAligned := range map[string][2]bool{
		"dial_p95": {requirements.DialP95, regressed(evidence.DirectDialP95MS, evidence.RelayDialP95MS)},
		"dial_p99": {requirements.DialP99, regressed(evidence.DirectDialP99MS, evidence.RelayDialP99MS)},
		"rtt_p95":  {requirements.RTTP95, regressed(evidence.DirectRTTP95MS, evidence.RelayRTTP95MS)},
		"rtt_p99":  {requirements.RTTP99, regressed(evidence.DirectRTTP99MS, evidence.RelayRTTP99MS)},
	} {
		if requiredAndAligned[0] && !requiredAndAligned[1] {
			return fmt.Errorf("Giznet diagnostic did not reproduce the material %s phase", metric)
		}
	}
	if requirements.ResourceBound {
		return errors.New("Giznet diagnostic lacks owner-local resource evidence for the material resource bound")
	}
	return nil
}

func comparisonResources(roles map[string]roleResourceEvidence) map[string]comparisonResource {
	resources := make(map[string]comparisonResource, len(roles))
	for role, evidence := range roles {
		start, peak, end := evidence.Summary.Start, evidence.Summary.Peak, evidence.Summary.End
		resources[role] = comparisonResource{
			CPUSecondsDelta: max(end.CPUSeconds-start.CPUSeconds, 0),
			PeakRSSBytes:    float64(peak.RSSBytes), PeakOpenFDs: float64(peak.OpenFDs), OpenFDLimit: float64(evidence.OpenFDLimit),
			PeakUDPSockets: float64(peak.UDPSockets), PeakUDP6Sockets: float64(peak.UDP6Sockets),
			NetworkRXBytes: float64(counterDelta(start.NetworkRXBytes, end.NetworkRXBytes)),
			NetworkTXBytes: float64(counterDelta(start.NetworkTXBytes, end.NetworkTXBytes)),
		}
	}
	return resources
}

func comparisonPhases(phases map[string]establishmentPhaseSummary) map[string]comparisonPhase {
	result := make(map[string]comparisonPhase, len(phases))
	for name, phase := range phases {
		result[name] = comparisonPhase{
			Supported: phase.Supported, Reason: phase.Reason,
			P50MS: phase.Latency.P50, P95MS: phase.Latency.P95, P99MS: phase.Latency.P99,
		}
	}
	return result
}

func medianPhases(runs []comparisonRun) (map[string]comparisonPhase, error) {
	if len(runs) == 0 {
		return nil, errors.New("no runs")
	}
	result := make(map[string]comparisonPhase, len(runs[0].Phases))
	for name, first := range runs[0].Phases {
		phases := make([]comparisonPhase, 0, len(runs))
		for _, run := range runs {
			phase, ok := run.Phases[name]
			if !ok || phase.Supported != first.Supported || phase.Reason != first.Reason {
				return nil, fmt.Errorf("phase %s support changed across repetitions", name)
			}
			phases = append(phases, phase)
		}
		if !first.Supported {
			result[name] = comparisonPhase{Reason: first.Reason}
			continue
		}
		median := func(selectValue func(comparisonPhase) float64) float64 {
			values := make([]float64, len(phases))
			for index, phase := range phases {
				values[index] = selectValue(phase)
			}
			slices.Sort(values)
			return values[len(values)/2]
		}
		result[name] = comparisonPhase{
			Supported: true,
			P50MS:     median(func(value comparisonPhase) float64 { return value.P50MS }),
			P95MS:     median(func(value comparisonPhase) float64 { return value.P95MS }),
			P99MS:     median(func(value comparisonPhase) float64 { return value.P99MS }),
		}
	}
	for _, run := range runs[1:] {
		if len(run.Phases) != len(result) {
			return nil, errors.New("phase set changed across repetitions")
		}
	}
	return result, nil
}

func ratioPhases(relay, direct map[string]comparisonPhase) map[string]comparisonPhase {
	result := make(map[string]comparisonPhase, len(relay))
	ratio := func(value, baseline float64) float64 {
		if baseline == 0 {
			return 0
		}
		return value / baseline
	}
	for name, value := range relay {
		baseline, ok := direct[name]
		if !ok || !value.Supported || !baseline.Supported {
			result[name] = comparisonPhase{Reason: value.Reason}
			continue
		}
		result[name] = comparisonPhase{
			Supported: true,
			P50MS:     ratio(value.P50MS, baseline.P50MS),
			P95MS:     ratio(value.P95MS, baseline.P95MS),
			P99MS:     ratio(value.P99MS, baseline.P99MS),
		}
	}
	return result
}

func medianResources(runs []comparisonRun) map[string]comparisonResource {
	roles := make(map[string]struct{})
	for _, run := range runs {
		for role := range run.Resources {
			roles[role] = struct{}{}
		}
	}
	result := make(map[string]comparisonResource, len(roles))
	for role := range roles {
		values := func(selectValue func(comparisonResource) float64) float64 {
			items := make([]float64, 0, len(runs))
			for _, run := range runs {
				items = append(items, selectValue(run.Resources[role]))
			}
			slices.Sort(items)
			return items[len(items)/2]
		}
		result[role] = comparisonResource{
			CPUSecondsDelta: values(func(v comparisonResource) float64 { return v.CPUSecondsDelta }), PeakRSSBytes: values(func(v comparisonResource) float64 { return v.PeakRSSBytes }),
			PeakOpenFDs: values(func(v comparisonResource) float64 { return v.PeakOpenFDs }), OpenFDLimit: values(func(v comparisonResource) float64 { return v.OpenFDLimit }),
			PeakUDPSockets: values(func(v comparisonResource) float64 { return v.PeakUDPSockets }), PeakUDP6Sockets: values(func(v comparisonResource) float64 { return v.PeakUDP6Sockets }),
			NetworkRXBytes: values(func(v comparisonResource) float64 { return v.NetworkRXBytes }), NetworkTXBytes: values(func(v comparisonResource) float64 { return v.NetworkTXBytes }),
		}
	}
	return result
}

func ratioResources(relay, direct map[string]comparisonResource) map[string]comparisonResource {
	result := make(map[string]comparisonResource, len(relay))
	ratio := func(value, baseline float64) float64 {
		if baseline == 0 {
			return 0
		}
		return value / baseline
	}
	for role, value := range relay {
		baseline := direct[role]
		result[role] = comparisonResource{
			CPUSecondsDelta: ratio(value.CPUSecondsDelta, baseline.CPUSecondsDelta), PeakRSSBytes: ratio(value.PeakRSSBytes, baseline.PeakRSSBytes),
			PeakOpenFDs: ratio(value.PeakOpenFDs, baseline.PeakOpenFDs), OpenFDLimit: ratio(value.OpenFDLimit, baseline.OpenFDLimit),
			PeakUDPSockets: ratio(value.PeakUDPSockets, baseline.PeakUDPSockets), PeakUDP6Sockets: ratio(value.PeakUDP6Sockets, baseline.PeakUDP6Sockets),
			NetworkRXBytes: ratio(value.NetworkRXBytes, baseline.NetworkRXBytes), NetworkTXBytes: ratio(value.NetworkTXBytes, baseline.NetworkTXBytes),
		}
	}
	return result
}

func validateRunSidecars(base, upstreamPath string) error {
	for suffix, target := range map[string]any{"-path.json": &icePathEvidence{}, "-coturn.json": &coturnPathEvidence{}} {
		data, err := os.ReadFile(base + suffix)
		if err != nil {
			return fmt.Errorf("read %s: %w", base+suffix, err)
		}
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("decode %s: %w", base+suffix, err)
		}
		switch evidence := target.(type) {
		case *icePathEvidence:
			if !evidence.Passed || evidence.UpstreamPath != upstreamPath {
				return fmt.Errorf("%s is path-invalid", base+suffix)
			}
			for _, role := range []string{"edge", "edge2"} {
				if err := validateICEObservations(upstreamPath, role, evidence.Roles[role]); err != nil {
					return fmt.Errorf("%s: %w", base+suffix, err)
				}
			}
		case *coturnPathEvidence:
			if !evidence.Passed || evidence.UpstreamPath != upstreamPath {
				return fmt.Errorf("%s is allocation-invalid", base+suffix)
			}
			if err := validateCoturnEvidence(upstreamPath, *evidence); err != nil {
				return fmt.Errorf("%s: %w", base+suffix, err)
			}
		}
	}
	return nil
}

func validateFrozenRelayRun(path string, run artifact) error {
	config := run.Config
	if len(config.Edges) != 2 || config.Ramp != 0 || config.Duration != 0 || config.PingInterval != 30*time.Second ||
		config.DialTimeout != 20*time.Second || config.PingTimeout != 28*time.Second ||
		config.SpeedBytes != 1<<20 || config.SpeedBaselineBytes != 32<<20 || config.SpeedTimeout != 2*time.Minute ||
		config.MinSpeedAggregateRatio != 0 || config.MinUploadAggregateMbps != 200 || config.MinDownloadAggregateMbps != 200 ||
		config.MinEstablishmentRate != 20 || config.MaxDialP95 != time.Second || config.MaxDialP99 != 5*time.Second ||
		config.Concurrency != config.Sessions || config.MaxEstablishmentFailures != 0 || config.MaxPingFailures != 0 ||
		config.MaxPingRoundDuration != 30*time.Second || !config.RequireBalancedEdges || config.RequiredUpstreamsPerEdge != 4 ||
		config.OpusPackets != 50 || config.OpusPacketBytes != 3 || config.OpusInterval != 20*time.Millisecond {
		return fmt.Errorf("run %s changed the frozen workload or gates", path)
	}
	expectedBytes := int64(config.Sessions) * config.SpeedBytes
	for direction, speed := range map[string]speedDirectionSummary{"upload": run.SpeedTest.Upload, "download": run.SpeedTest.Download} {
		if speed.Concurrent.Attempted != config.Sessions || speed.Concurrent.Completed != config.Sessions ||
			speed.Concurrent.Failures != 0 || speed.Concurrent.TransferredBytes != expectedBytes {
			return fmt.Errorf("run %s has incomplete %s bytes", path, direction)
		}
	}
	if run.Opus.Attempted != config.Sessions*50 || run.Opus.Completed != run.Opus.Attempted ||
		run.Opus.AttemptedBytes != int64(config.Sessions*50*3) || run.Opus.CompletedBytes != run.Opus.AttemptedBytes {
		return fmt.Errorf("run %s has incomplete Opus accounting", path)
	}
	mandatoryEvent, ok := run.Establishment.Phases[phaseMandatoryEventStream]
	if !ok || !mandatoryEvent.Supported || mandatoryEvent.Latency.Count != config.Sessions {
		return fmt.Errorf("run %s has incomplete mandatory event stream evidence", path)
	}
	for _, edge := range config.Edges {
		if run.EdgeDistribution[edge] != config.Sessions/2 || len(run.UpstreamDistribution[edge]) != 4 {
			return fmt.Errorf("run %s has invalid Edge/upstream distribution", path)
		}
	}
	return nil
}

func validateCoturnEvidence(path string, evidence coturnPathEvidence) error {
	allocations := func(pair coturnEvidencePair) int64 { return pair.CoturnA.Allocations + pair.CoturnB.Allocations }
	cleanupAllocations := evidence.Cleanup.CoturnA.Allocations + evidence.Cleanup.CoturnB.Allocations
	if cleanupAllocations != 0 || evidence.Cleanup.AllocationsReturnedToZeroWithinSeconds != 15 {
		return errors.New("Coturn cleanup did not return to zero within 15 seconds")
	}
	if path == "direct" {
		if allocations(evidence.LiveBefore) != 0 || allocations(evidence.AfterWorkload) != 0 ||
			evidence.TrafficDelta.ReceivedBytes != 0 || evidence.TrafficDelta.SentBytes != 0 {
			return errors.New("direct path used Coturn")
		}
		return nil
	}
	if allocations(evidence.LiveBefore) != 10 || allocations(evidence.AfterWorkload) != 10 ||
		evidence.TrafficDelta.ReceivedBytes <= 0 || evidence.TrafficDelta.SentBytes <= 0 {
		return errors.New("relay path lacks ten live traffic-carrying Coturn allocations")
	}
	return nil
}

func validateComparisonRuns(runs []comparisonRun) ([]comparisonRun, error) {
	if len(runs) != 3 {
		return nil, fmt.Errorf("found %d runs, want 3", len(runs))
	}
	slices.SortFunc(runs, func(a, b comparisonRun) int { return a.Repetition - b.Repetition })
	for index, run := range runs {
		if run.Repetition != index+1 {
			return nil, errors.New("repetitions must be exactly 1,2,3")
		}
	}
	return runs, nil
}

func medianMetrics(runs []comparisonRun) comparisonMetrics {
	values := func(selectValue func(comparisonMetrics) float64) float64 {
		items := make([]float64, len(runs))
		for index, run := range runs {
			items[index] = selectValue(run.Metrics)
		}
		slices.Sort(items)
		return items[len(items)/2]
	}
	return comparisonMetrics{
		EstablishmentRate:    values(func(v comparisonMetrics) float64 { return v.EstablishmentRate }),
		UploadMbps:           values(func(v comparisonMetrics) float64 { return v.UploadMbps }),
		UploadSessionP50:     values(func(v comparisonMetrics) float64 { return v.UploadSessionP50 }),
		UploadSessionP95:     values(func(v comparisonMetrics) float64 { return v.UploadSessionP95 }),
		UploadSessionP99:     values(func(v comparisonMetrics) float64 { return v.UploadSessionP99 }),
		DownloadMbps:         values(func(v comparisonMetrics) float64 { return v.DownloadMbps }),
		DownloadSessionP50:   values(func(v comparisonMetrics) float64 { return v.DownloadSessionP50 }),
		DownloadSessionP95:   values(func(v comparisonMetrics) float64 { return v.DownloadSessionP95 }),
		DownloadSessionP99:   values(func(v comparisonMetrics) float64 { return v.DownloadSessionP99 }),
		DialP50MS:            values(func(v comparisonMetrics) float64 { return v.DialP50MS }),
		DialP95MS:            values(func(v comparisonMetrics) float64 { return v.DialP95MS }),
		DialP99MS:            values(func(v comparisonMetrics) float64 { return v.DialP99MS }),
		RTTP50MS:             values(func(v comparisonMetrics) float64 { return v.RTTP50MS }),
		RTTP95MS:             values(func(v comparisonMetrics) float64 { return v.RTTP95MS }),
		RTTP99MS:             values(func(v comparisonMetrics) float64 { return v.RTTP99MS }),
		OpusCompletedPackets: values(func(v comparisonMetrics) float64 { return v.OpusCompletedPackets }),
		OpusCompletedBytes:   values(func(v comparisonMetrics) float64 { return v.OpusCompletedBytes }),
		OpusPacketsPS:        values(func(v comparisonMetrics) float64 { return v.OpusPacketsPS }),
		OpusBytesPS:          values(func(v comparisonMetrics) float64 { return v.OpusBytesPS }),
		OpusWriteP50MS:       values(func(v comparisonMetrics) float64 { return v.OpusWriteP50MS }),
		OpusWriteP95MS:       values(func(v comparisonMetrics) float64 { return v.OpusWriteP95MS }),
		OpusWriteP99MS:       values(func(v comparisonMetrics) float64 { return v.OpusWriteP99MS }),
	}
}

func ratioMetrics(relay, direct comparisonMetrics) comparisonMetrics {
	ratio := func(value, baseline float64) float64 {
		if baseline == 0 {
			return 0
		}
		return value / baseline
	}
	return comparisonMetrics{
		EstablishmentRate:    ratio(relay.EstablishmentRate, direct.EstablishmentRate),
		UploadMbps:           ratio(relay.UploadMbps, direct.UploadMbps),
		UploadSessionP50:     ratio(relay.UploadSessionP50, direct.UploadSessionP50),
		UploadSessionP95:     ratio(relay.UploadSessionP95, direct.UploadSessionP95),
		UploadSessionP99:     ratio(relay.UploadSessionP99, direct.UploadSessionP99),
		DownloadMbps:         ratio(relay.DownloadMbps, direct.DownloadMbps),
		DownloadSessionP50:   ratio(relay.DownloadSessionP50, direct.DownloadSessionP50),
		DownloadSessionP95:   ratio(relay.DownloadSessionP95, direct.DownloadSessionP95),
		DownloadSessionP99:   ratio(relay.DownloadSessionP99, direct.DownloadSessionP99),
		DialP50MS:            ratio(relay.DialP50MS, direct.DialP50MS),
		DialP95MS:            ratio(relay.DialP95MS, direct.DialP95MS),
		DialP99MS:            ratio(relay.DialP99MS, direct.DialP99MS),
		RTTP50MS:             ratio(relay.RTTP50MS, direct.RTTP50MS),
		RTTP95MS:             ratio(relay.RTTP95MS, direct.RTTP95MS),
		RTTP99MS:             ratio(relay.RTTP99MS, direct.RTTP99MS),
		OpusCompletedPackets: ratio(relay.OpusCompletedPackets, direct.OpusCompletedPackets),
		OpusCompletedBytes:   ratio(relay.OpusCompletedBytes, direct.OpusCompletedBytes),
		OpusPacketsPS:        ratio(relay.OpusPacketsPS, direct.OpusPacketsPS),
		OpusBytesPS:          ratio(relay.OpusBytesPS, direct.OpusBytesPS),
		OpusWriteP50MS:       ratio(relay.OpusWriteP50MS, direct.OpusWriteP50MS),
		OpusWriteP95MS:       ratio(relay.OpusWriteP95MS, direct.OpusWriteP95MS),
		OpusWriteP99MS:       ratio(relay.OpusWriteP99MS, direct.OpusWriteP99MS),
	}
}

func writeJSONArtifact(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
