package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectICEPathEvidenceValidatesExactRelaySetAndRedacts(t *testing.T) {
	directory := t.TempDir()
	paths := make([]string, 0, 2)
	for _, role := range []string{"edge", "edge2"} {
		path := filepath.Join(directory, role+".log")
		var lines []string
		for index := range 5 {
			kind, id := "gateway", fmt.Sprintf("%d", index)
			if index == 0 {
				kind, id = "control", "control"
			}
			lines = append(lines, fmt.Sprintf(
				`time=x level=INFO msg="edge: upstream ICE selected" upstream_kind=%s upstream_id=%s connection_epoch=%d local_candidate_type=relay local_protocol=udp local_address_family=ipv4 local_component=1 remote_candidate_type=host remote_protocol=udp remote_address_family=ipv4 remote_component=1 pair_state=succeeded nominated=true counters_supported=true packets_sent=1 packets_received=2 bytes_sent=3 bytes_received=4 current_rtt_seconds=0.001 retransmissions_sent=0 retransmissions_received=0 relay_member=1 address=192.0.2.10 credential=secret`,
				kind, id, index+1,
			))
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, role+"="+path)
	}
	report, err := collectICEPathEvidence("relay", strings.Join(paths, ","))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || len(report.Roles["edge"]) != 5 || len(report.Roles["edge2"]) != 5 {
		t.Fatalf("path report = %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"192.0.2.10", "secret", `"address":`, "credential"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("path artifact contains %q: %s", forbidden, encoded)
		}
	}
}

func TestValidateICEObservationsRejectsDirectRelayCandidate(t *testing.T) {
	observations := make([]iceObservation, 5)
	for index := range observations {
		observations[index] = iceObservation{
			Role: "edge", UpstreamKind: "gateway", UpstreamID: fmt.Sprint(index), ConnectionEpoch: uint64(index + 1),
			LocalCandidateType: "host", LocalProtocol: "udp", LocalAddressFamily: "ipv4", LocalComponent: 1,
			RemoteCandidateType: "host", RemoteProtocol: "udp", RemoteAddressFamily: "ipv4", RemoteComponent: 1,
			PairState: "succeeded", Nominated: true,
		}
	}
	observations[0].UpstreamKind, observations[0].UpstreamID = "control", "control"
	if err := validateICEObservations("direct", "edge", observations); err != nil {
		t.Fatal(err)
	}
	observations[4].LocalCandidateType = "relay"
	if err := validateICEObservations("direct", "edge", observations); err == nil || !strings.Contains(err.Error(), "unexpectedly used relay") {
		t.Fatalf("validate direct observations error = %v", err)
	}
}

func TestMedianAndRatioMetrics(t *testing.T) {
	runs := []comparisonRun{
		{Repetition: 1, Metrics: comparisonMetrics{UploadMbps: 100, DialP95MS: 10}},
		{Repetition: 2, Metrics: comparisonMetrics{UploadMbps: 300, DialP95MS: 30}},
		{Repetition: 3, Metrics: comparisonMetrics{UploadMbps: 200, DialP95MS: 20}},
	}
	median := medianMetrics(runs)
	if median.UploadMbps != 200 || median.DialP95MS != 20 {
		t.Fatalf("median = %+v", median)
	}
	ratio := ratioMetrics(comparisonMetrics{UploadMbps: 180, DialP95MS: 24}, median)
	if ratio.UploadMbps != 0.9 || ratio.DialP95MS != 1.2 {
		t.Fatalf("ratio = %+v", ratio)
	}
}

func TestValidateFrozenRelayRunRejectsWorkloadDrift(t *testing.T) {
	run := artifact{
		Config: artifactConfig{
			Edges: []string{"edge-a", "edge-b"}, Sessions: 100, PingInterval: 30 * time.Second,
			DialTimeout: 20 * time.Second, PingTimeout: 28 * time.Second,
			SpeedBytes: 1 << 20, SpeedBaselineBytes: 32 << 20, SpeedTimeout: 2 * time.Minute,
			MinUploadAggregateMbps: 200, MinDownloadAggregateMbps: 200, MinEstablishmentRate: 20,
			MaxDialP95: time.Second, MaxDialP99: 5 * time.Second, Concurrency: 100,
			MaxPingRoundDuration: 30 * time.Second, RequireBalancedEdges: true, RequiredUpstreamsPerEdge: 4,
			OpusPackets: 50, OpusPacketBytes: 3, OpusInterval: 20 * time.Millisecond,
		},
		SpeedTest: speedTestSummary{
			Upload:   speedDirectionSummary{Concurrent: speedRunSummary{Attempted: 100, Completed: 100, TransferredBytes: 100 << 20}},
			Download: speedDirectionSummary{Concurrent: speedRunSummary{Attempted: 100, Completed: 100, TransferredBytes: 100 << 20}},
		},
		Establishment: establishmentSummary{Phases: map[string]establishmentPhaseSummary{
			phaseMandatoryEventStream: {Supported: true, Latency: latencySummary{Count: 100}},
		}},
		Opus:             opusSummary{Attempted: 5000, Completed: 5000, AttemptedBytes: 15000, CompletedBytes: 15000},
		EdgeDistribution: map[string]int{"edge-a": 50, "edge-b": 50},
		UpstreamDistribution: map[string]map[string]int{
			"edge-a": {"1": 13, "2": 13, "3": 12, "4": 12},
			"edge-b": {"1": 13, "2": 13, "3": 12, "4": 12},
		},
	}
	if err := validateFrozenRelayRun("run.json", run); err != nil {
		t.Fatal(err)
	}
	run.Config.SpeedBytes--
	if err := validateFrozenRelayRun("run.json", run); err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("workload drift error = %v", err)
	}
}

func TestMedianPhasesPreservesSupportedAndUnsupportedEvidence(t *testing.T) {
	runs := []comparisonRun{
		{Phases: map[string]comparisonPhase{
			"dial": {Supported: true, P50MS: 10, P95MS: 20, P99MS: 30},
			"sctp": {Reason: "unsupported"},
		}},
		{Phases: map[string]comparisonPhase{
			"dial": {Supported: true, P50MS: 30, P95MS: 40, P99MS: 50},
			"sctp": {Reason: "unsupported"},
		}},
		{Phases: map[string]comparisonPhase{
			"dial": {Supported: true, P50MS: 20, P95MS: 30, P99MS: 40},
			"sctp": {Reason: "unsupported"},
		}},
	}
	median, err := medianPhases(runs)
	if err != nil {
		t.Fatal(err)
	}
	if got := median["dial"]; !got.Supported || got.P50MS != 20 || got.P95MS != 30 || got.P99MS != 40 {
		t.Fatalf("dial median = %+v", got)
	}
	if got := median["sctp"]; got.Supported || got.Reason != "unsupported" {
		t.Fatalf("unsupported median = %+v", got)
	}
	ratio := ratioPhases(map[string]comparisonPhase{"dial": {Supported: true, P50MS: 30}}, map[string]comparisonPhase{"dial": {Supported: true, P50MS: 20}})
	if ratio["dial"].P50MS != 1.5 {
		t.Fatalf("phase ratio = %+v", ratio["dial"])
	}
}

func TestValidateCoturnEvidenceSeparatesDirectAndRelay(t *testing.T) {
	direct := coturnPathEvidence{
		Cleanup: coturnCleanupPair{AllocationsReturnedToZeroWithinSeconds: 15},
	}
	if err := validateCoturnEvidence("direct", direct); err != nil {
		t.Fatal(err)
	}
	direct.TrafficDelta.ReceivedBytes = 1
	if err := validateCoturnEvidence("direct", direct); err == nil {
		t.Fatal("direct evidence accepted Coturn traffic")
	}
	relay := coturnPathEvidence{
		LiveBefore:    coturnEvidencePair{CoturnA: coturnCounters{Allocations: 5}, CoturnB: coturnCounters{Allocations: 5}},
		AfterWorkload: coturnEvidencePair{CoturnA: coturnCounters{Allocations: 5}, CoturnB: coturnCounters{Allocations: 5}},
		Cleanup:       coturnCleanupPair{AllocationsReturnedToZeroWithinSeconds: 15},
		TrafficDelta:  coturnTrafficDelta{ReceivedBytes: 1, SentBytes: 1},
	}
	if err := validateCoturnEvidence("relay", relay); err != nil {
		t.Fatal(err)
	}
}

func TestValidateGiznetCoturnDiagnosticAttributesMaterialDelta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "giznet-coturn.json")
	diagnostic := giznetCoturnDiagnostic{
		RepositoryHead: "clean-head", DialSamples: 30, RTTSamples: 200,
		ThroughputRuns: 3, ThroughputBytes: 32 << 20,
		Paths: []giznetCoturnDiagnosticPath{
			{Name: "direct", ClientMedianMbps: 800, ListenerMedianMbps: 780},
			{Name: "static", ClientMedianMbps: 400, ListenerMedianMbps: 500},
			{
				Name: "turn_rest", ClientMedianMbps: 480, ListenerMedianMbps: 520,
				CoturnBefore: coturnCounters{ReceivedBytes: 10, SentBytes: 20},
				CoturnAfter:  coturnCounters{ReceivedBytes: 210, SentBytes: 240},
			},
		},
		Comparisons: []giznetCoturnDiagnosticRatio{{
			Path: "turn_rest", ClientToListenerMbpsRatio: 0.6, ListenerToClientMbpsRatio: 2.0 / 3.0,
		}},
	}
	data, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evidence, err := validateGiznetCoturnDiagnostic(path, "clean-head")
	if err != nil {
		t.Fatal(err)
	}
	if evidence.ClientToListenerRatio != 0.6 || evidence.CoturnReceivedBytesDelta != 200 ||
		evidence.CoturnSentBytesDelta != 220 || !evidence.ProductEdgeAndServerExcluded {
		t.Fatalf("causal evidence = %+v", evidence)
	}

	diagnostic.RepositoryDirty = true
	data, err = json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateGiznetCoturnDiagnostic(path, "clean-head"); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("dirty diagnostic error = %v", err)
	}
}
