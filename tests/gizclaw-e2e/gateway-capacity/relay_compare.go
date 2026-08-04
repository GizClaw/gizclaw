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
	CausalConclusion string                    `json:"causal_conclusion"`
	Qualified        bool                      `json:"qualified"`
}

type comparisonCell struct {
	Sessions       int                           `json:"sessions"`
	Direct         comparisonPath                `json:"direct"`
	Relay          comparisonPath                `json:"relay"`
	Ratios         comparisonMetrics             `json:"relay_to_direct_ratio"`
	ResourceRatios map[string]comparisonResource `json:"relay_to_direct_resource_ratio"`
}

type comparisonPath struct {
	Runs            []comparisonRun               `json:"runs"`
	Median          comparisonMetrics             `json:"median"`
	ResourceMedians map[string]comparisonResource `json:"resource_medians"`
}

type comparisonRun struct {
	Repetition int                           `json:"repetition"`
	Metrics    comparisonMetrics             `json:"metrics"`
	Resources  map[string]comparisonResource `json:"resources"`
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
	UploadMbps     float64 `json:"upload_mbps"`
	DownloadMbps   float64 `json:"download_mbps"`
	DialP95MS      float64 `json:"dial_p95_ms"`
	DialP99MS      float64 `json:"dial_p99_ms"`
	RTTP95MS       float64 `json:"rtt_p95_ms"`
	RTTP99MS       float64 `json:"rtt_p99_ms"`
	OpusPacketsPS  float64 `json:"opus_packets_per_second"`
	OpusWriteP95MS float64 `json:"opus_write_p95_ms"`
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

func compareRelayCapacityArtifacts(directory string) (relayComparisonReport, error) {
	report := relayComparisonReport{Version: relayComparisonVersion, Cells: make(map[string]comparisonCell)}
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
				UploadMbps:   candidate.SpeedTest.Upload.Concurrent.AggregateMbps,
				DownloadMbps: candidate.SpeedTest.Download.Concurrent.AggregateMbps,
				DialP95MS:    candidate.Establishment.Dial.P95, DialP99MS: candidate.Establishment.Dial.P99,
				RTTP95MS: candidate.RTT.P95, RTTP99MS: candidate.RTT.P99,
				OpusPacketsPS: candidate.Opus.PacketsPerSecond, OpusWriteP95MS: candidate.Opus.WriteLatency.P95,
			},
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
		cell.Direct = comparisonPath{Runs: direct, Median: medianMetrics(direct), ResourceMedians: medianResources(direct)}
		cell.Relay = comparisonPath{Runs: relay, Median: medianMetrics(relay), ResourceMedians: medianResources(relay)}
		cell.Ratios = ratioMetrics(cell.Relay.Median, cell.Direct.Median)
		cell.ResourceRatios = ratioResources(cell.Relay.ResourceMedians, cell.Direct.ResourceMedians)
		for direction, ratio := range map[string]float64{"upload": cell.Ratios.UploadMbps, "download": cell.Ratios.DownloadMbps} {
			if ratio < 0.9 {
				report.Material = true
				report.MaterialReasons = append(report.MaterialReasons, fmt.Sprintf("sessions-%d %s relay/direct throughput %.3f < 0.900", sessions, direction, ratio))
			}
		}
		for metric, values := range map[string][2]float64{
			"dial_p95": {cell.Direct.Median.DialP95MS, cell.Relay.Median.DialP95MS}, "dial_p99": {cell.Direct.Median.DialP99MS, cell.Relay.Median.DialP99MS},
			"rtt_p95": {cell.Direct.Median.RTTP95MS, cell.Relay.Median.RTTP95MS}, "rtt_p99": {cell.Direct.Median.RTTP99MS, cell.Relay.Median.RTTP99MS},
		} {
			if values[1]-values[0] >= 5 && values[1] > values[0]*1.2 {
				report.Material = true
				report.MaterialReasons = append(report.MaterialReasons, fmt.Sprintf("sessions-%d %s relay %.3fms vs direct %.3fms", sessions, metric, values[1], values[0]))
			}
		}
		for role, resource := range cell.Relay.ResourceMedians {
			if resource.OpenFDLimit > 0 && resource.PeakOpenFDs >= resource.OpenFDLimit*0.9 {
				report.Material = true
				report.MaterialReasons = append(report.MaterialReasons, fmt.Sprintf(
					"sessions-%d relay %s median open FDs %.0f reached at least 90%% of limit %.0f",
					sessions, role, resource.PeakOpenFDs, resource.OpenFDLimit,
				))
			}
		}
		report.Cells[key] = cell
	}
	if report.Material {
		report.CausalConclusion = "material delta requires owner evidence from the recorded role and transport counters"
	} else {
		report.CausalConclusion = "no material owner observed within the tested 100/500 envelope"
	}
	report.Qualified = true
	return report, nil
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
		UploadMbps: values(func(v comparisonMetrics) float64 { return v.UploadMbps }), DownloadMbps: values(func(v comparisonMetrics) float64 { return v.DownloadMbps }),
		DialP95MS: values(func(v comparisonMetrics) float64 { return v.DialP95MS }), DialP99MS: values(func(v comparisonMetrics) float64 { return v.DialP99MS }),
		RTTP95MS: values(func(v comparisonMetrics) float64 { return v.RTTP95MS }), RTTP99MS: values(func(v comparisonMetrics) float64 { return v.RTTP99MS }),
		OpusPacketsPS: values(func(v comparisonMetrics) float64 { return v.OpusPacketsPS }), OpusWriteP95MS: values(func(v comparisonMetrics) float64 { return v.OpusWriteP95MS }),
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
		UploadMbps: ratio(relay.UploadMbps, direct.UploadMbps), DownloadMbps: ratio(relay.DownloadMbps, direct.DownloadMbps),
		DialP95MS: ratio(relay.DialP95MS, direct.DialP95MS), DialP99MS: ratio(relay.DialP99MS, direct.DialP99MS),
		RTTP95MS: ratio(relay.RTTP95MS, direct.RTTP95MS), RTTP99MS: ratio(relay.RTTP99MS, direct.RTTP99MS),
		OpusPacketsPS: ratio(relay.OpusPacketsPS, direct.OpusPacketsPS), OpusWriteP95MS: ratio(relay.OpusWriteP95MS, direct.OpusWriteP95MS),
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
