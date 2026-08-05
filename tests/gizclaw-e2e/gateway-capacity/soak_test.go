package main

import (
	"strings"
	"testing"
	"time"
)

func TestSummarizeSoakQualificationAcceptsStableEvidence(t *testing.T) {
	report := stableSoakArtifact()
	got := summarizeSoakQualification(report)
	if !got.Qualified || len(got.Reasons) != 0 {
		t.Fatalf("soak qualification = %+v", got)
	}
	if got.EarlyRTT.Rounds != 20 || got.LateRTT.Rounds != 20 || got.RTTP99Growth != 0 {
		t.Fatalf("RTT comparison = early %+v late %+v growth %f", got.EarlyRTT, got.LateRTT, got.RTTP99Growth)
	}
	if len(got.Roles) != len(soakResourceRoles) {
		t.Fatalf("role comparisons = %d, want %d", len(got.Roles), len(soakResourceRoles))
	}
	if got.Roles["load_driver"].Early.SocketAndNetworkSupported {
		t.Fatal("load-driver socket/network evidence was reported as supported")
	}
	if !got.Roles["edge"].Early.SocketAndNetworkSupported ||
		got.Roles["edge"].Early.NetworkRXBytesPerSecond != 100_000 {
		t.Fatalf("edge early resource window = %+v", got.Roles["edge"].Early)
	}
}

func TestSummarizeSoakQualificationRejectsMaterialDegradation(t *testing.T) {
	report := stableSoakArtifact()
	for index := range report.PingRounds {
		if report.PingRounds[index].Phase == "final" || report.PingRounds[index].Round >= 101 {
			report.PingRounds[index].RTT.P99 = 150
		}
	}
	edge := report.Extended.Roles["edge"]
	lateStart := report.HoldFinishedAt.Add(-soakQualificationWindow)
	for index := range edge.Samples {
		if !edge.Samples[index].At.Before(lateStart) {
			edge.Samples[index].RSSBytes *= 2
			edge.Samples[index].OpenFDs *= 2
			edge.Samples[index].UDPSockets *= 2
		}
	}
	report.Extended.Roles["edge"] = edge
	got := summarizeSoakQualification(report)
	joined := strings.Join(got.Reasons, "\n")
	for _, want := range []string{"RTT growth", "edge: RSS growth", "edge: open-FD growth", "edge: UDP-socket growth"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("soak reasons = %q, want %q", joined, want)
		}
	}
	if got.Qualified || got.Roles["edge"].Qualified {
		t.Fatalf("degraded soak qualified: %+v", got)
	}
}

func TestSummarizeSoakQualificationRejectsWindowGap(t *testing.T) {
	report := stableSoakArtifact()
	server := report.Extended.Roles["server"]
	server.Samples = append(server.Samples[:10], server.Samples[12:]...)
	report.Extended.Roles["server"] = server
	got := summarizeSoakQualification(report)
	if got.Qualified || !strings.Contains(strings.Join(got.Reasons, "\n"), "server: early window: sample gap") {
		t.Fatalf("soak qualification = %+v", got)
	}
}

func TestSummarizeSoakQualificationGatesCompletedGCLiveHeap(t *testing.T) {
	report := stableSoakArtifact()
	loadDriver := report.Extended.Roles["load_driver"]
	lateStart := report.HoldFinishedAt.Add(-soakQualificationWindow)
	for index := range loadDriver.Samples {
		if !loadDriver.Samples[index].At.Before(lateStart) {
			alloc, live := uint64(1_000), uint64(400)
			loadDriver.Samples[index].GoHeapAllocBytes = &alloc
			loadDriver.Samples[index].GoHeapLiveBytes = &live
		}
	}
	report.Extended.Roles["load_driver"] = loadDriver

	got := summarizeSoakQualification(report)
	stability := got.Roles["load_driver"]
	if !got.Qualified || stability.GoHeapAllocGrowth == nil || *stability.GoHeapAllocGrowth <= maximumSoakGrowth {
		t.Fatalf("current heap growth should be diagnostic only: %+v", stability)
	}
	if stability.GoHeapLiveGrowth == nil || *stability.GoHeapLiveGrowth != 0 {
		t.Fatalf("live heap comparison = %+v, want zero growth", stability.GoHeapLiveGrowth)
	}

	loadDriver = report.Extended.Roles["load_driver"]
	for index := range loadDriver.Samples {
		if !loadDriver.Samples[index].At.Before(lateStart) {
			live := uint64(800)
			loadDriver.Samples[index].GoHeapLiveBytes = &live
		}
	}
	report.Extended.Roles["load_driver"] = loadDriver
	got = summarizeSoakQualification(report)
	if got.Qualified || !strings.Contains(strings.Join(got.Reasons, "\n"), "Go live-heap growth") {
		t.Fatalf("growing completed-GC live heap qualified: %+v", got)
	}
}

func TestMateriallyGrewUsesTheActualLowBaseline(t *testing.T) {
	if !materiallyGrew(0.10, 0.25, minimumCPUIncrease) {
		t.Fatal("materiallyGrew accepted a 150% and 0.15-core increase")
	}
	if materiallyGrew(0.10, 0.19, minimumCPUIncrease) {
		t.Fatal("materiallyGrew rejected an increase below the absolute CPU floor")
	}
	if got := soakRelativeGrowth(0, 1); got != 1 {
		t.Fatalf("soakRelativeGrowth(0, 1) = %f, want bounded material growth", got)
	}
}

func stableSoakArtifact() artifact {
	start := time.Unix(1_800_000_000, 0)
	report := artifact{
		Config: artifactConfig{
			Soak: true, Duration: time.Hour, PingInterval: 30 * time.Second,
		},
		HoldStartedAt:  start,
		HoldFinishedAt: start.Add(time.Hour),
		Extended:       &extendedRunEvidence{Roles: make(map[string]roleResourceEvidence)},
	}
	report.PingRounds = append(report.PingRounds, pingRoundSummary{
		Phase: "hold", Round: 0, StartedAt: start.Add(-time.Second), RTT: latencySummary{Count: 1000, P99: 100},
	})
	for round := 1; round < 120; round++ {
		report.PingRounds = append(report.PingRounds, pingRoundSummary{
			Phase: "hold", Round: round, StartedAt: start.Add(time.Duration(round) * 30 * time.Second),
			RTT: latencySummary{Count: 1000, P99: 100},
		})
	}
	report.PingRounds = append(report.PingRounds, pingRoundSummary{
		Phase: "final", StartedAt: start.Add(time.Hour), RTT: latencySummary{Count: 1000, P99: 100},
	})
	for _, role := range soakResourceRoles {
		samples := make([]roleResourcePoint, 0, int(time.Hour/(2*time.Second))+1)
		for elapsed := time.Duration(0); elapsed <= time.Hour; elapsed += 2 * time.Second {
			heap, live, goroutines := uint64(500), uint64(400), 100
			point := roleResourcePoint{
				At: start.Add(elapsed), RSSBytes: 1_000_000, RSSSource: "test_process_rss",
				CPUSeconds: 0.5 * elapsed.Seconds(), CPUSecondsSource: "test_process_cpu",
				OpenFDs: 10, OpenFDsSource: "test_process_fds",
			}
			if role == "load_driver" {
				point.GoHeapAllocBytes = &heap
				point.GoHeapLiveBytes = &live
				point.Goroutines = &goroutines
				point.SocketSource = "unsupported"
				point.NetworkSource = "unsupported"
				point.UnsupportedMetrics = []string{"udp_sockets", "udp6_sockets", "network_rx_bytes", "network_tx_bytes"}
			} else {
				point.UDPSockets = 5
				point.UDP6Sockets = 1
				point.SocketSource = "proc_pid_net_udp"
				point.NetworkRXBytes = uint64(100_000 * elapsed.Seconds())
				point.NetworkTXBytes = uint64(80_000 * elapsed.Seconds())
				point.NetworkSource = "proc_pid_net_dev"
				point.UnsupportedMetrics = []string{"go_heap_alloc_bytes", "go_heap_live_bytes", "goroutines"}
			}
			samples = append(samples, point)
		}
		report.Extended.Roles[role] = roleResourceEvidence{Role: role, Samples: samples}
	}
	return report
}
