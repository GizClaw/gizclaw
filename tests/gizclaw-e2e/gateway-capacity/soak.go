package main

import (
	"fmt"
	"math"
	"slices"
	"time"
)

const (
	soakQualificationWindow = 10 * time.Minute
	maximumSoakGrowth       = 0.20
	minimumRTTIncreaseMS    = 10.0
	minimumCPUIncrease      = 0.10
	minimumNetworkRateDelta = 1024.0
)

var soakResourceRoles = []string{"load_driver", "edge", "edge2", "coturn-a", "coturn-b", "server"}

type soakQualification struct {
	WindowDuration time.Duration                `json:"window_duration"`
	MaximumGrowth  float64                      `json:"maximum_growth"`
	EarlyRTT       soakRTTWindow                `json:"early_rtt"`
	LateRTT        soakRTTWindow                `json:"late_rtt"`
	RTTP99Growth   float64                      `json:"rtt_p99_growth"`
	Roles          map[string]soakRoleStability `json:"roles"`
	Qualified      bool                         `json:"qualified"`
	Reasons        []string                     `json:"reasons,omitempty"`
}

type soakRTTWindow struct {
	Rounds       int     `json:"rounds"`
	MedianP99MS  float64 `json:"median_p99_ms"`
	MaximumP99MS float64 `json:"maximum_p99_ms"`
}

type soakRoleWindow struct {
	Samples                   int      `json:"samples"`
	MedianRSSBytes            float64  `json:"median_rss_bytes"`
	AverageCPUCores           float64  `json:"average_cpu_cores"`
	MedianOpenFDs             float64  `json:"median_open_fds"`
	MedianGoHeapAllocBytes    *float64 `json:"median_go_heap_alloc_bytes"`
	MedianGoroutines          *float64 `json:"median_goroutines"`
	SocketAndNetworkSupported bool     `json:"socket_and_network_supported"`
	MedianUDPSockets          float64  `json:"median_udp_sockets,omitempty"`
	MedianUDP6Sockets         float64  `json:"median_udp6_sockets,omitempty"`
	NetworkRXBytesPerSecond   float64  `json:"network_rx_bytes_per_second,omitempty"`
	NetworkTXBytesPerSecond   float64  `json:"network_tx_bytes_per_second,omitempty"`
	UnsupportedMetrics        []string `json:"unsupported_metrics,omitempty"`
}

type soakRoleStability struct {
	Early               soakRoleWindow `json:"early"`
	Late                soakRoleWindow `json:"late"`
	RSSGrowth           float64        `json:"rss_growth"`
	CPUCoresGrowth      float64        `json:"cpu_cores_growth"`
	OpenFDGrowth        float64        `json:"open_fd_growth"`
	GoHeapGrowth        *float64       `json:"go_heap_growth,omitempty"`
	GoroutineGrowth     *float64       `json:"goroutine_growth,omitempty"`
	UDPSocketGrowth     float64        `json:"udp_socket_growth,omitempty"`
	UDP6SocketGrowth    float64        `json:"udp6_socket_growth,omitempty"`
	NetworkRXRateChange float64        `json:"network_rx_rate_change,omitempty"`
	NetworkTXRateChange float64        `json:"network_tx_rate_change,omitempty"`
	Qualified           bool           `json:"qualified"`
	Reasons             []string       `json:"reasons,omitempty"`
}

func summarizeSoakQualification(report artifact) soakQualification {
	qualification := soakQualification{
		WindowDuration: soakQualificationWindow,
		MaximumGrowth:  maximumSoakGrowth,
		Roles:          make(map[string]soakRoleStability),
		Qualified:      true,
	}
	reject := func(reason string) {
		qualification.Qualified = false
		qualification.Reasons = append(qualification.Reasons, reason)
	}
	if !report.Config.Soak {
		reject("artifact is not marked as a soak")
		return qualification
	}
	if report.Extended == nil {
		reject("extended role evidence is missing")
		return qualification
	}
	if report.Config.PingInterval <= 0 || report.Config.Duration < 2*soakQualificationWindow {
		reject("soak duration or ping interval cannot provide two complete comparison windows")
		return qualification
	}
	if report.HoldStartedAt.IsZero() || report.HoldFinishedAt.IsZero() ||
		report.HoldFinishedAt.Sub(report.HoldStartedAt) < report.Config.Duration {
		reject("complete hold boundaries are missing")
		return qualification
	}

	earlyRounds, lateRounds := soakRTTWindows(report)
	wantRounds := int(soakQualificationWindow / report.Config.PingInterval)
	if len(earlyRounds) != wantRounds || len(lateRounds) != wantRounds {
		reject(fmt.Sprintf(
			"RTT windows contain %d/%d rounds, want %d each",
			len(earlyRounds), len(lateRounds), wantRounds,
		))
		return qualification
	}
	qualification.EarlyRTT = summarizeSoakRTTWindow(earlyRounds)
	qualification.LateRTT = summarizeSoakRTTWindow(lateRounds)
	qualification.RTTP99Growth = soakRelativeGrowth(
		qualification.EarlyRTT.MedianP99MS,
		qualification.LateRTT.MedianP99MS,
	)
	if materiallyGrew(
		qualification.EarlyRTT.MedianP99MS,
		qualification.LateRTT.MedianP99MS,
		minimumRTTIncreaseMS,
	) {
		reject(fmt.Sprintf(
			"median round p99 RTT growth %.3f exceeds %.2f with more than %.0fms increase",
			qualification.RTTP99Growth, maximumSoakGrowth, minimumRTTIncreaseMS,
		))
	}

	earlyStart := report.HoldStartedAt
	earlyEnd := earlyStart.Add(soakQualificationWindow)
	lateEnd := report.HoldFinishedAt
	lateStart := lateEnd.Add(-soakQualificationWindow)
	for _, role := range soakResourceRoles {
		evidence, ok := report.Extended.Roles[role]
		if !ok {
			reject(role + " evidence is missing")
			continue
		}
		stability := summarizeSoakRole(role, evidence.Samples, earlyStart, earlyEnd, lateStart, lateEnd)
		qualification.Roles[role] = stability
		for _, reason := range stability.Reasons {
			reject(role + ": " + reason)
		}
	}
	return qualification
}

func soakRTTWindows(report artifact) ([]pingRoundSummary, []pingRoundSummary) {
	holdRounds := make([]pingRoundSummary, 0, len(report.PingRounds))
	var finalRounds []pingRoundSummary
	for _, round := range report.PingRounds {
		switch {
		case round.Phase == "hold" && round.Round > 0:
			holdRounds = append(holdRounds, round)
		case round.Phase == "final":
			finalRounds = append(finalRounds, round)
		}
	}
	want := int(soakQualificationWindow / report.Config.PingInterval)
	if want <= 0 || len(holdRounds) < 2*want-1 || len(finalRounds) != 1 {
		return nil, nil
	}
	early := append([]pingRoundSummary(nil), holdRounds[:want]...)
	late := append([]pingRoundSummary(nil), holdRounds[len(holdRounds)-(want-1):]...)
	late = append(late, finalRounds[0])
	return early, late
}

func summarizeSoakRTTWindow(rounds []pingRoundSummary) soakRTTWindow {
	p99 := make([]float64, 0, len(rounds))
	for _, round := range rounds {
		p99 = append(p99, round.RTT.P99)
	}
	slices.Sort(p99)
	if len(p99) == 0 {
		return soakRTTWindow{}
	}
	return soakRTTWindow{
		Rounds: len(p99), MedianP99MS: p99[len(p99)/2], MaximumP99MS: p99[len(p99)-1],
	}
}

func summarizeSoakRole(
	role string,
	samples []roleResourcePoint,
	earlyStart, earlyEnd, lateStart, lateEnd time.Time,
) soakRoleStability {
	stability := soakRoleStability{Qualified: true}
	early, earlyErr := completeSoakResourceWindow(samples, earlyStart, earlyEnd)
	late, lateErr := completeSoakResourceWindow(samples, lateStart, lateEnd)
	if earlyErr != nil {
		stability.Reasons = append(stability.Reasons, "early window: "+earlyErr.Error())
	}
	if lateErr != nil {
		stability.Reasons = append(stability.Reasons, "late window: "+lateErr.Error())
	}
	if earlyErr != nil || lateErr != nil {
		stability.Qualified = false
		return stability
	}
	supported := role != "load_driver"
	stability.Early = summarizeSoakRoleWindow(early, supported)
	stability.Late = summarizeSoakRoleWindow(late, supported)
	stability.RSSGrowth = soakRelativeGrowth(stability.Early.MedianRSSBytes, stability.Late.MedianRSSBytes)
	stability.CPUCoresGrowth = soakRelativeGrowth(stability.Early.AverageCPUCores, stability.Late.AverageCPUCores)
	stability.OpenFDGrowth = soakRelativeGrowth(stability.Early.MedianOpenFDs, stability.Late.MedianOpenFDs)
	if stability.RSSGrowth > maximumSoakGrowth {
		stability.Reasons = append(stability.Reasons, fmt.Sprintf("RSS growth %.3f exceeds %.2f", stability.RSSGrowth, maximumSoakGrowth))
	}
	if materiallyGrew(stability.Early.AverageCPUCores, stability.Late.AverageCPUCores, minimumCPUIncrease) {
		stability.Reasons = append(stability.Reasons, fmt.Sprintf(
			"CPU growth %.3f exceeds %.2f with more than %.2f core increase",
			stability.CPUCoresGrowth, maximumSoakGrowth, minimumCPUIncrease,
		))
	}
	if stability.OpenFDGrowth > maximumSoakGrowth {
		stability.Reasons = append(stability.Reasons, fmt.Sprintf("open-FD growth %.3f exceeds %.2f", stability.OpenFDGrowth, maximumSoakGrowth))
	}
	if stability.Early.MedianGoHeapAllocBytes != nil && stability.Late.MedianGoHeapAllocBytes != nil {
		growth := soakRelativeGrowth(*stability.Early.MedianGoHeapAllocBytes, *stability.Late.MedianGoHeapAllocBytes)
		stability.GoHeapGrowth = &growth
		if growth > maximumSoakGrowth {
			stability.Reasons = append(stability.Reasons, fmt.Sprintf("Go-heap growth %.3f exceeds %.2f", growth, maximumSoakGrowth))
		}
	}
	if stability.Early.MedianGoroutines != nil && stability.Late.MedianGoroutines != nil {
		growth := soakRelativeGrowth(*stability.Early.MedianGoroutines, *stability.Late.MedianGoroutines)
		stability.GoroutineGrowth = &growth
		if growth > maximumSoakGrowth {
			stability.Reasons = append(stability.Reasons, fmt.Sprintf("goroutine growth %.3f exceeds %.2f", growth, maximumSoakGrowth))
		}
	}
	if supported {
		stability.UDPSocketGrowth = soakRelativeGrowth(stability.Early.MedianUDPSockets, stability.Late.MedianUDPSockets)
		stability.UDP6SocketGrowth = soakRelativeGrowth(stability.Early.MedianUDP6Sockets, stability.Late.MedianUDP6Sockets)
		stability.NetworkRXRateChange = soakRelativeGrowth(stability.Early.NetworkRXBytesPerSecond, stability.Late.NetworkRXBytesPerSecond)
		stability.NetworkTXRateChange = soakRelativeGrowth(stability.Early.NetworkTXBytesPerSecond, stability.Late.NetworkTXBytesPerSecond)
		if stability.UDPSocketGrowth > maximumSoakGrowth {
			stability.Reasons = append(stability.Reasons, fmt.Sprintf("UDP-socket growth %.3f exceeds %.2f", stability.UDPSocketGrowth, maximumSoakGrowth))
		}
		if stability.UDP6SocketGrowth > maximumSoakGrowth {
			stability.Reasons = append(stability.Reasons, fmt.Sprintf("UDP6-socket growth %.3f exceeds %.2f", stability.UDP6SocketGrowth, maximumSoakGrowth))
		}
		if materiallyChanged(
			stability.Early.NetworkRXBytesPerSecond,
			stability.Late.NetworkRXBytesPerSecond,
			minimumNetworkRateDelta,
		) {
			stability.Reasons = append(stability.Reasons, fmt.Sprintf("network-RX rate change %.3f exceeds %.2f", stability.NetworkRXRateChange, maximumSoakGrowth))
		}
		if materiallyChanged(
			stability.Early.NetworkTXBytesPerSecond,
			stability.Late.NetworkTXBytesPerSecond,
			minimumNetworkRateDelta,
		) {
			stability.Reasons = append(stability.Reasons, fmt.Sprintf("network-TX rate change %.3f exceeds %.2f", stability.NetworkTXRateChange, maximumSoakGrowth))
		}
	}
	if len(stability.Reasons) > 0 {
		stability.Qualified = false
	}
	return stability
}

func completeSoakResourceWindow(samples []roleResourcePoint, start, end time.Time) ([]roleResourcePoint, error) {
	window := resourceWindow(samples, start, end)
	if len(window) < 2 {
		return nil, fmt.Errorf("contains %d samples", len(window))
	}
	if window[0].At.Sub(start) > maximumResourceSampleGap {
		return nil, fmt.Errorf("first sample is %s after the boundary", window[0].At.Sub(start))
	}
	if end.Sub(window[len(window)-1].At) > maximumResourceSampleGap {
		return nil, fmt.Errorf("last sample is %s before the boundary", end.Sub(window[len(window)-1].At))
	}
	for index := 1; index < len(window); index++ {
		gap := window[index].At.Sub(window[index-1].At)
		if gap <= 0 || gap > maximumResourceSampleGap {
			return nil, fmt.Errorf("sample gap is %s", gap)
		}
	}
	return window, nil
}

func summarizeSoakRoleWindow(samples []roleResourcePoint, supported bool) soakRoleWindow {
	window := soakRoleWindow{
		Samples:                   len(samples),
		MedianRSSBytes:            medianOf(samples, func(value roleResourcePoint) float64 { return float64(value.RSSBytes) }),
		AverageCPUCores:           averageCPUCores(samples),
		MedianOpenFDs:             medianOf(samples, func(value roleResourcePoint) float64 { return float64(value.OpenFDs) }),
		MedianGoHeapAllocBytes:    medianOptional(samples, func(value roleResourcePoint) *uint64 { return value.GoHeapAllocBytes }),
		MedianGoroutines:          medianOptional(samples, func(value roleResourcePoint) *int { return value.Goroutines }),
		SocketAndNetworkSupported: supported,
		UnsupportedMetrics:        append([]string(nil), samples[0].UnsupportedMetrics...),
	}
	if !supported {
		return window
	}
	window.MedianUDPSockets = medianOf(samples, func(value roleResourcePoint) float64 { return float64(value.UDPSockets) })
	window.MedianUDP6Sockets = medianOf(samples, func(value roleResourcePoint) float64 { return float64(value.UDP6Sockets) })
	duration := samples[len(samples)-1].At.Sub(samples[0].At).Seconds()
	if duration > 0 {
		window.NetworkRXBytesPerSecond = float64(counterDelta(samples[0].NetworkRXBytes, samples[len(samples)-1].NetworkRXBytes)) / duration
		window.NetworkTXBytesPerSecond = float64(counterDelta(samples[0].NetworkTXBytes, samples[len(samples)-1].NetworkTXBytes)) / duration
	}
	return window
}

func medianOptional[T ~int | ~uint64](samples []roleResourcePoint, selectValue func(roleResourcePoint) *T) *float64 {
	values := make([]float64, 0, len(samples))
	for _, sample := range samples {
		value := selectValue(sample)
		if value == nil {
			return nil
		}
		values = append(values, float64(*value))
	}
	slices.Sort(values)
	median := values[len(values)/2]
	return &median
}

func materiallyGrew(early, late, absoluteFloor float64) bool {
	return late-early > absoluteFloor && soakRelativeGrowth(early, late) > maximumSoakGrowth
}

func materiallyChanged(early, late, absoluteFloor float64) bool {
	return math.Abs(late-early) > absoluteFloor &&
		math.Abs(soakRelativeGrowth(early, late)) > maximumSoakGrowth
}

func soakRelativeGrowth(early, late float64) float64 {
	if early == 0 {
		switch {
		case late > 0:
			return 1
		case late < 0:
			return -1
		default:
			return 0
		}
	}
	return (late - early) / math.Abs(early)
}
