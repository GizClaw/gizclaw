package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	extendedArtifactVersion         = 18
	maximumResourceSampleGap        = 2100 * time.Millisecond
	maximumResourceSampleFutureSkew = 100 * time.Millisecond
)

var dockerRolePIDFiles = map[string]string{
	"coturn-a": "/tmp/gizclaw-coturn.pid",
	"coturn-b": "/tmp/gizclaw-coturn.pid",
	"edge":     "/src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.pid",
	"edge2":    "/src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.pid",
	"server":   "/src/tests/gizclaw-e2e/testdata/server-workspace/gizclaw-server.pid",
}

type extendedRunEvidence struct {
	Environment capacityEnvironment             `json:"environment"`
	Roles       map[string]roleResourceEvidence `json:"roles"`
	Errors      []string                        `json:"errors,omitempty"`
}

type capacityEnvironment struct {
	RepositoryCommit string            `json:"repository_commit"`
	RepositoryDirty  bool              `json:"repository_dirty"`
	HostMemoryBytes  uint64            `json:"host_memory_bytes"`
	HostOpenFDLimit  uint64            `json:"host_open_fd_limit"`
	Docker           dockerEnvironment `json:"docker"`
	TrafficShaping   shapingEvidence   `json:"traffic_shaping"`
}

type shapingEvidence struct {
	Source string `json:"source"`
	Status string `json:"status"`
}

type dockerEnvironment struct {
	OperatingSystem string `json:"operating_system"`
	OSType          string `json:"os_type"`
	Architecture    string `json:"architecture"`
	LogicalCPU      int    `json:"logical_cpu"`
	MemoryBytes     uint64 `json:"memory_bytes"`
	ServerVersion   string `json:"server_version"`
	OrbStackVersion string `json:"orbstack_version"`
}

type roleResourcePoint struct {
	At                 time.Time `json:"at"`
	RSSBytes           uint64    `json:"rss_bytes"`
	RSSSource          string    `json:"rss_source"`
	CPUSeconds         float64   `json:"cpu_seconds"`
	CPUSecondsSource   string    `json:"cpu_seconds_source"`
	OpenFDs            int       `json:"open_fds"`
	OpenFDsSource      string    `json:"open_fds_source"`
	GoHeapAllocBytes   *uint64   `json:"go_heap_alloc_bytes"`
	GoHeapLiveBytes    *uint64   `json:"go_heap_live_bytes"`
	Goroutines         *int      `json:"goroutines"`
	UDPSockets         int       `json:"udp_sockets"`
	UDP6Sockets        int       `json:"udp6_sockets"`
	SocketSource       string    `json:"socket_source"`
	NetworkRXBytes     uint64    `json:"network_rx_bytes"`
	NetworkTXBytes     uint64    `json:"network_tx_bytes"`
	NetworkSource      string    `json:"network_source"`
	UnsupportedMetrics []string  `json:"unsupported_metrics,omitempty"`
}

type roleResourceSummary struct {
	Start roleResourcePoint `json:"start"`
	Peak  roleResourcePoint `json:"peak"`
	End   roleResourcePoint `json:"end"`
}

type roleResourceEvidence struct {
	Role              string              `json:"role"`
	ContainerID       string              `json:"container_id,omitempty"`
	ImageID           string              `json:"image_id,omitempty"`
	ProcessID         int                 `json:"process_id"`
	ProcessStartTicks uint64              `json:"process_start_ticks"`
	OpenFDLimit       uint64              `json:"open_fd_limit"`
	Samples           []roleResourcePoint `json:"samples"`
	Summary           roleResourceSummary `json:"summary"`
	TrafficShaping    shapingEvidence     `json:"traffic_shaping"`
}

type dockerProcessSample struct {
	Point             roleResourcePoint
	ProcessID         int
	ProcessStartTicks uint64
	OpenFDLimit       uint64
}

type dockerRoleState struct {
	role              string
	containerID       string
	imageID           string
	restartCount      int
	processID         int
	processStartTicks uint64
	openFDLimit       uint64
	samples           []roleResourcePoint
	maximumSampleGap  time.Duration
	trafficShaping    shapingEvidence
}

type dockerResourceSampler struct {
	project       string
	composeFile   string
	streamContext context.Context
	cancelStreams context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.Mutex
	roles         map[string]*dockerRoleState
	errors        []string
	stopped       bool
}

type extendedSamplerState struct {
	environment capacityEnvironment
	docker      *dockerResourceSampler
}

type extendedSamplingProgress struct {
	MinimumSamples int
	MaximumGap     time.Duration
	MaximumAge     time.Duration
}

func startExtendedSampler(ctx context.Context, project, composeFile string) (*extendedSamplerState, error) {
	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	environment, err := readCapacityEnvironment(startCtx)
	if err != nil {
		return nil, err
	}
	dockerSampler, err := startDockerResourceSampler(startCtx, project, composeFile)
	if err != nil {
		return nil, err
	}
	return &extendedSamplerState{environment: environment, docker: dockerSampler}, nil
}

func (s *extendedSamplerState) finish(ctx context.Context, load *resourceSampler) *extendedRunEvidence {
	_ = load.summary()
	roles, errs := s.docker.stop(ctx)
	loadEvidence := loadDriverEvidence(load.samples())
	roles["load_driver"] = loadEvidence
	for _, role := range []string{"load_driver", "edge", "edge2", "server", "coturn-a", "coturn-b"} {
		if err := validateRequiredRoleEvidence(roles[role]); err != nil {
			errs = append(errs, err.Error())
		}
	}
	return &extendedRunEvidence{Environment: s.environment, Roles: roles, Errors: errs}
}

func (s *extendedSamplerState) liveHealth(now time.Time) (extendedSamplingProgress, error) {
	if s == nil || s.docker == nil {
		return extendedSamplingProgress{}, nil
	}

	s.docker.mu.Lock()
	defer s.docker.mu.Unlock()
	progress := extendedSamplingProgress{MinimumSamples: math.MaxInt}
	for _, role := range []string{"edge", "edge2", "server", "coturn-a", "coturn-b"} {
		state, ok := s.docker.roles[role]
		if !ok {
			return progress, fmt.Errorf("%s resource sampler is missing", role)
		}
		progress.MinimumSamples = min(progress.MinimumSamples, len(state.samples))
		progress.MaximumGap = max(progress.MaximumGap, state.maximumSampleGap)
		if len(state.samples) == 0 {
			return progress, fmt.Errorf("%s has no resource samples", role)
		}
		latest := state.samples[len(state.samples)-1]
		var previous *roleResourcePoint
		if len(state.samples) > 1 {
			previous = &state.samples[len(state.samples)-2]
		}
		if err := validateRoleResourcePoint(role, latest, previous); err != nil {
			return progress, err
		}
		age := now.Sub(latest.At)
		if age < -maximumResourceSampleFutureSkew {
			return progress, fmt.Errorf("%s latest resource sample is %s in the future", role, -age)
		}
		age = max(age, 0)
		progress.MaximumAge = max(progress.MaximumAge, age)
		if age > maximumResourceSampleGap {
			return progress, fmt.Errorf("%s resource sample stream is stale by %s", role, age)
		}
	}
	if len(s.docker.errors) > 0 {
		return progress, fmt.Errorf("resource sampler reported: %s", s.docker.errors[0])
	}
	if progress.MinimumSamples == math.MaxInt {
		progress.MinimumSamples = 0
	}
	return progress, nil
}

func validateRequiredRoleEvidence(evidence roleResourceEvidence) error {
	if len(evidence.Samples) == 0 {
		return fmt.Errorf("%s has no resource samples", evidence.Role)
	}
	for index, sample := range evidence.Samples {
		var previous *roleResourcePoint
		if index > 0 {
			previous = &evidence.Samples[index-1]
		}
		if err := validateRoleResourcePoint(evidence.Role, sample, previous); err != nil {
			return err
		}
	}
	return nil
}

func validateRoleResourcePoint(role string, sample roleResourcePoint, previous *roleResourcePoint) error {
	if previous != nil {
		gap := sample.At.Sub(previous.At)
		if gap <= 0 || gap > maximumResourceSampleGap {
			return fmt.Errorf("%s resource sample gap is %s", role, gap)
		}
		if sample.CPUSeconds < previous.CPUSeconds {
			return fmt.Errorf("%s cumulative CPU counter decreased", role)
		}
		if role != "load_driver" &&
			(sample.NetworkRXBytes < previous.NetworkRXBytes || sample.NetworkTXBytes < previous.NetworkTXBytes) {
			return fmt.Errorf("%s cumulative network counter decreased", role)
		}
	}
	if sample.RSSBytes == 0 || sample.RSSSource == "go_memstats_sys" ||
		sample.RSSSource == "go_runtime_memory_total" || sample.RSSSource == "unsupported" {
		return fmt.Errorf("%s has unsupported process RSS source %q", role, sample.RSSSource)
	}
	if sample.CPUSecondsSource == "" || sample.CPUSecondsSource == "unsupported" {
		return fmt.Errorf("%s has unsupported CPU source", role)
	}
	if sample.OpenFDs < 0 || sample.OpenFDsSource == "" || sample.OpenFDsSource == "unsupported" {
		return fmt.Errorf("%s has unsupported open-file sampling", role)
	}
	if role == "load_driver" {
		if sample.GoHeapAllocBytes == nil || sample.GoHeapLiveBytes == nil || sample.Goroutines == nil {
			return fmt.Errorf("%s is missing Go heap or goroutine sampling", role)
		}
		if sample.SocketSource != "unsupported" || sample.NetworkSource != "unsupported" ||
			!containsAll(sample.UnsupportedMetrics, "udp_sockets", "udp6_sockets", "network_rx_bytes", "network_tx_bytes") {
			return fmt.Errorf("%s has incomplete unsupported socket or network declarations", role)
		}
		return nil
	}
	if sample.GoHeapAllocBytes != nil || sample.GoHeapLiveBytes != nil || sample.Goroutines != nil ||
		!containsAll(sample.UnsupportedMetrics, "go_heap_alloc_bytes", "go_heap_live_bytes", "goroutines") {
		return fmt.Errorf("%s has inconsistent unsupported Go runtime declarations", role)
	}
	if sample.SocketSource != "proc_pid_net_udp" || sample.NetworkSource != "proc_pid_net_dev" {
		return fmt.Errorf("%s has unsupported socket or network sampling", role)
	}
	return nil
}

func containsAll(values []string, required ...string) bool {
	for _, value := range required {
		if !slices.Contains(values, value) {
			return false
		}
	}
	return true
}

func startDockerResourceSampler(ctx context.Context, project, composeFile string) (*dockerResourceSampler, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(composeFile) == "" {
		return nil, errors.New("docker project and compose file are required for role sampling")
	}
	streamContext, cancelStreams := context.WithCancel(context.Background())
	s := &dockerResourceSampler{
		project: project, composeFile: composeFile,
		streamContext: streamContext, cancelStreams: cancelStreams,
		roles: make(map[string]*dockerRoleState),
	}
	for _, role := range []string{"edge", "edge2", "server", "coturn-a", "coturn-b"} {
		state, err := resolveDockerRole(ctx, project, composeFile, role)
		if err != nil {
			cancelStreams()
			return nil, err
		}
		s.roles[role] = state
	}
	ready := make(chan error, len(s.roles))
	for role, state := range s.roles {
		s.wg.Go(func() { s.streamDockerRole(role, state, ready) })
	}
	for range s.roles {
		select {
		case err := <-ready:
			if err != nil {
				s.shutdownStreams()
				return nil, err
			}
		case <-ctx.Done():
			s.shutdownStreams()
			return nil, fmt.Errorf("start Docker process sampling: %w", ctx.Err())
		}
	}
	return s, nil
}

func resolveDockerRole(ctx context.Context, project, composeFile, role string) (*dockerRoleState, error) {
	_ = composeFile
	containerID, err := commandText(
		ctx,
		"docker", "ps", "-q",
		"--filter", "label=com.docker.compose.project="+project,
		"--filter", "label=com.docker.compose.service="+role,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve %s container: %w", role, err)
	}
	if containerID == "" {
		return nil, fmt.Errorf("resolve %s container: empty container id", role)
	}
	metadata, err := inspectContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect %s container: %w", role, err)
	}
	if !metadata.State.Running {
		return nil, fmt.Errorf("%s container is not running (status=%s)", role, metadata.State.Status)
	}
	trafficShaping, err := readContainerShaping(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect %s traffic shaping: %w", role, err)
	}
	if trafficShaping.Status == "active" {
		return nil, fmt.Errorf("%s has active traffic shaping", role)
	}
	return &dockerRoleState{
		role: role, containerID: containerID, imageID: metadata.Image,
		restartCount: metadata.RestartCount, trafficShaping: trafficShaping,
	}, nil
}

type containerMetadata struct {
	Image        string `json:"Image"`
	RestartCount int    `json:"RestartCount"`
	State        struct {
		Running bool   `json:"Running"`
		Status  string `json:"Status"`
	} `json:"State"`
}

func inspectContainer(ctx context.Context, containerID string) (containerMetadata, error) {
	output, err := exec.CommandContext(ctx, "docker", "inspect", containerID).Output()
	if err != nil {
		return containerMetadata{}, err
	}
	var items []containerMetadata
	if err := json.Unmarshal(output, &items); err != nil {
		return containerMetadata{}, err
	}
	if len(items) != 1 {
		return containerMetadata{}, fmt.Errorf("docker inspect returned %d entries", len(items))
	}
	return items[0], nil
}

func recordDockerRoleSample(state *dockerRoleState, sample dockerProcessSample) error {
	var errs []error
	if state.processID != 0 &&
		(state.processID != sample.ProcessID || state.processStartTicks != sample.ProcessStartTicks) {
		errs = append(errs, fmt.Errorf(
			"%s process changed from pid=%d start=%d to pid=%d start=%d",
			state.role, state.processID, state.processStartTicks,
			sample.ProcessID, sample.ProcessStartTicks,
		))
	}
	var previous *roleResourcePoint
	if len(state.samples) > 0 {
		previous = &state.samples[len(state.samples)-1]
		state.maximumSampleGap = max(state.maximumSampleGap, sample.Point.At.Sub(previous.At))
	}
	if err := validateRoleResourcePoint(state.role, sample.Point, previous); err != nil {
		errs = append(errs, err)
	}
	state.processID = sample.ProcessID
	state.processStartTicks = sample.ProcessStartTicks
	state.openFDLimit = sample.OpenFDLimit
	state.samples = append(state.samples, sample.Point)
	return errors.Join(errs...)
}

const dockerProcessSampleScript = `
set -euo pipefail
pid_file="$1"
page_size="$(getconf PAGESIZE)"
clock_ticks="$(getconf CLK_TCK)"
test -n "${EPOCHREALTIME:-}"
shopt -s nullglob
while true; do
  sampled_at="${EPOCHREALTIME/./}000"
  pid="$(<"$pid_file")"
  test -r "/proc/$pid/statm"
  test -r "/proc/$pid/stat"
  read -r _ resident _ < "/proc/$pid/statm"
  stat_line="$(<"/proc/$pid/stat")"
  stat_tail="${stat_line##*) }"
  read -r -a stat_fields <<< "$stat_tail"
  user_ticks="${stat_fields[11]}"
  system_ticks="${stat_fields[12]}"
  start_ticks="${stat_fields[19]}"
  fd_paths=("/proc/$pid/fd/"*)
  open_fds="${#fd_paths[@]}"
  fd_limit=""
  while read -r first second third soft _; do
    if [[ "$first" == "Max" && "$second" == "open" && "$third" == "files" ]]; then
      fd_limit="$soft"
      break
    fi
  done < "/proc/$pid/limits"
  test -n "$fd_limit"
  udp_sockets="$(awk 'END { print (NR > 0 ? NR - 1 : 0) }' "/proc/$pid/net/udp")"
  udp6_sockets="$(awk 'END { print (NR > 0 ? NR - 1 : 0) }' "/proc/$pid/net/udp6")"
  network_totals="$(awk '
    NR > 2 {
      sub(":", "", $1)
      rx += $2
      tx += $10
    }
    END { printf "%.0f %.0f\n", rx, tx }
  ' "/proc/$pid/net/dev")"
  read -r network_rx network_tx <<< "$network_totals"
  [[ -n "$udp_sockets" && -n "$udp6_sockets" && -n "$network_rx" && -n "$network_tx" ]]
  printf '%s %s %s %s %s %s %s %s %s %s %s %s %s %s\n' "$sampled_at" "$pid" "$resident" "$page_size" "$user_ticks" "$system_ticks" "$clock_ticks" "$open_fds" "$start_ticks" "$fd_limit" "$udp_sockets" "$udp6_sockets" "$network_rx" "$network_tx"
  now_nanoseconds="${EPOCHREALTIME/./}000"
  sleep_milliseconds=$(((sampled_at + 1000000000 - now_nanoseconds) / 1000000))
  if ((sleep_milliseconds > 0)); then
    sleep_seconds="$(awk -v milliseconds="$sleep_milliseconds" 'BEGIN { printf "%.3f", milliseconds / 1000 }')"
    sleep "$sleep_seconds"
  fi
done
`

func (s *dockerResourceSampler) streamDockerRole(
	role string,
	state *dockerRoleState,
	ready chan<- error,
) {
	command := exec.CommandContext(
		s.streamContext,
		"docker", "exec", state.containerID,
		"bash", "-c", dockerProcessSampleScript, "gateway-capacity-sampler", dockerRolePIDFiles[role],
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		ready <- fmt.Errorf("sample %s: open Docker stream: %w", role, err)
		return
	}
	if err := command.Start(); err != nil {
		ready <- fmt.Errorf("sample %s: start Docker stream: %w", role, err)
		return
	}
	started := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		sample, parseErr := parseDockerProcessSample(scanner.Text())
		if parseErr != nil {
			if !started {
				ready <- fmt.Errorf("sample %s: %w", role, parseErr)
				started = true
			} else {
				s.recordError(fmt.Errorf("sample %s: %w", role, parseErr))
			}
			continue
		}
		s.mu.Lock()
		recordErr := recordDockerRoleSample(state, sample)
		s.mu.Unlock()
		if recordErr != nil {
			s.recordError(recordErr)
		}
		if !started {
			ready <- nil
			started = true
		}
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return
	}
	streamErr := fmt.Errorf("sample %s: Docker stream stopped", role)
	if scanErr != nil {
		streamErr = fmt.Errorf("sample %s: read Docker stream: %w", role, scanErr)
	} else if waitErr != nil {
		streamErr = fmt.Errorf("sample %s: Docker stream: %w: %s", role, waitErr, strings.TrimSpace(stderr.String()))
	}
	if !started {
		ready <- streamErr
		return
	}
	s.recordError(streamErr)
}

func parseDockerProcessSample(output string) (dockerProcessSample, error) {
	fields := strings.Fields(output)
	if len(fields) != 14 {
		return dockerProcessSample{}, fmt.Errorf("expected 14 fields, got %d", len(fields))
	}
	timestamp, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return dockerProcessSample{}, fmt.Errorf("sample timestamp overflows int64: %w", err)
		}
		return dockerProcessSample{}, fmt.Errorf("field 0: %w", err)
	}
	if timestamp < 0 {
		return dockerProcessSample{}, errors.New("sample timestamp must be non-negative")
	}
	processID, err := strconv.Atoi(fields[1])
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return dockerProcessSample{}, fmt.Errorf("process ID overflows int: %w", err)
		}
		return dockerProcessSample{}, fmt.Errorf("field 1: %w", err)
	}
	if processID < 0 {
		return dockerProcessSample{}, errors.New("process ID must be non-negative")
	}
	openFDs, err := strconv.Atoi(fields[7])
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return dockerProcessSample{}, fmt.Errorf("open FD count overflows int: %w", err)
		}
		return dockerProcessSample{}, fmt.Errorf("field 7: %w", err)
	}
	if openFDs < 0 {
		return dockerProcessSample{}, errors.New("open FD count must be non-negative")
	}
	udpSockets, err := strconv.Atoi(fields[10])
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return dockerProcessSample{}, fmt.Errorf("UDP socket count overflows int: %w", err)
		}
		return dockerProcessSample{}, fmt.Errorf("field 10: %w", err)
	}
	udp6Sockets, err := strconv.Atoi(fields[11])
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return dockerProcessSample{}, fmt.Errorf("UDP6 socket count overflows int: %w", err)
		}
		return dockerProcessSample{}, fmt.Errorf("field 11: %w", err)
	}
	if udpSockets < 0 || udp6Sockets < 0 {
		return dockerProcessSample{}, errors.New("UDP socket counts must be non-negative")
	}
	values := make([]uint64, len(fields))
	for _, index := range []int{2, 3, 4, 5, 6, 8, 9, 12, 13} {
		value, err := strconv.ParseUint(fields[index], 10, 64)
		if err != nil {
			return dockerProcessSample{}, fmt.Errorf("field %d: %w", index, err)
		}
		values[index] = value
	}
	if values[3] == 0 || values[6] == 0 {
		return dockerProcessSample{}, errors.New("page size and clock ticks must be positive")
	}
	if values[2] > math.MaxUint64/values[3] {
		return dockerProcessSample{}, errors.New("resident byte count overflows uint64")
	}
	return dockerProcessSample{
		Point: roleResourcePoint{
			At:       time.Unix(0, timestamp),
			RSSBytes: values[2] * values[3], RSSSource: "proc_pid_statm",
			CPUSeconds: (float64(values[4]) + float64(values[5])) / float64(values[6]), CPUSecondsSource: "proc_pid_stat",
			OpenFDs: openFDs, OpenFDsSource: "proc_pid_fd",
			UDPSockets: udpSockets, UDP6Sockets: udp6Sockets, SocketSource: "proc_pid_net_udp",
			NetworkRXBytes: values[12], NetworkTXBytes: values[13], NetworkSource: "proc_pid_net_dev",
			UnsupportedMetrics: []string{"go_heap_alloc_bytes", "go_heap_live_bytes", "goroutines"},
		},
		ProcessID: processID, ProcessStartTicks: values[8], OpenFDLimit: values[9],
	}, nil
}

func (s *dockerResourceSampler) recordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.errors) < 100 {
		s.errors = append(s.errors, err.Error())
	}
}

func (s *dockerResourceSampler) stop(ctx context.Context) (map[string]roleResourceEvidence, []string) {
	s.shutdownStreams()

	s.mu.Lock()
	defer s.mu.Unlock()
	evidence := make(map[string]roleResourceEvidence, len(s.roles))
	for role, state := range s.roles {
		inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		metadata, err := inspectContainer(inspectCtx, state.containerID)
		cancel()
		if err != nil {
			s.errors = append(s.errors, fmt.Sprintf("inspect final %s container: %v", role, err))
		} else if err := validateContainerFinalState(role, state.restartCount, metadata); err != nil {
			s.errors = append(s.errors, err.Error())
		}
		if len(state.samples) == 0 {
			s.errors = append(s.errors, fmt.Sprintf("%s has no resource samples", role))
		}
		evidence[role] = roleResourceEvidence{
			Role: role, ContainerID: state.containerID, ImageID: state.imageID,
			ProcessID: state.processID, ProcessStartTicks: state.processStartTicks,
			OpenFDLimit:    state.openFDLimit,
			Samples:        append([]roleResourcePoint(nil), state.samples...),
			Summary:        summarizeRoleResources(state.samples),
			TrafficShaping: state.trafficShaping,
		}
	}
	return evidence, append([]string(nil), s.errors...)
}

func (s *dockerResourceSampler) shutdownStreams() {
	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		s.cancelStreams()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func validateContainerFinalState(role string, initialRestarts int, metadata containerMetadata) error {
	if metadata.State.Running && metadata.RestartCount == initialRestarts {
		return nil
	}
	return fmt.Errorf(
		"%s container changed state (running=%t status=%s restarts=%d->%d)",
		role, metadata.State.Running, metadata.State.Status, initialRestarts, metadata.RestartCount,
	)
}

func summarizeRoleResources(samples []roleResourcePoint) roleResourceSummary {
	if len(samples) == 0 {
		return roleResourceSummary{}
	}
	peak := samples[0]
	for _, point := range samples[1:] {
		improved := false
		if point.RSSBytes > peak.RSSBytes {
			peak.RSSBytes = point.RSSBytes
			improved = true
		}
		if point.CPUSeconds > peak.CPUSeconds {
			peak.CPUSeconds = point.CPUSeconds
			improved = true
		}
		if point.OpenFDs > peak.OpenFDs {
			peak.OpenFDs = point.OpenFDs
			improved = true
		}
		if point.UDPSockets > peak.UDPSockets {
			peak.UDPSockets = point.UDPSockets
			improved = true
		}
		if point.UDP6Sockets > peak.UDP6Sockets {
			peak.UDP6Sockets = point.UDP6Sockets
			improved = true
		}
		if point.NetworkRXBytes > peak.NetworkRXBytes {
			peak.NetworkRXBytes = point.NetworkRXBytes
			improved = true
		}
		if point.NetworkTXBytes > peak.NetworkTXBytes {
			peak.NetworkTXBytes = point.NetworkTXBytes
			improved = true
		}
		if point.GoHeapAllocBytes != nil &&
			(peak.GoHeapAllocBytes == nil || *point.GoHeapAllocBytes > *peak.GoHeapAllocBytes) {
			value := *point.GoHeapAllocBytes
			peak.GoHeapAllocBytes = &value
			improved = true
		}
		if point.GoHeapLiveBytes != nil &&
			(peak.GoHeapLiveBytes == nil || *point.GoHeapLiveBytes > *peak.GoHeapLiveBytes) {
			value := *point.GoHeapLiveBytes
			peak.GoHeapLiveBytes = &value
			improved = true
		}
		if point.Goroutines != nil &&
			(peak.Goroutines == nil || *point.Goroutines > *peak.Goroutines) {
			value := *point.Goroutines
			peak.Goroutines = &value
			improved = true
		}
		if improved {
			peak.At = point.At
		}
	}
	return roleResourceSummary{Start: samples[0], Peak: peak, End: samples[len(samples)-1]}
}

func loadDriverEvidence(samples []resourcePoint) roleResourceEvidence {
	points := make([]roleResourcePoint, 0, len(samples))
	for _, sample := range samples {
		heap, live, goroutines := sample.HeapAllocBytes, sample.HeapLiveBytes, sample.Goroutines
		points = append(points, roleResourcePoint{
			At:       sample.At,
			RSSBytes: sample.RSSBytes, RSSSource: sample.RSSSource,
			CPUSeconds: sample.CPUSeconds, CPUSecondsSource: sample.CPUSecondsSource,
			OpenFDs: sample.OpenFDs, OpenFDsSource: sample.OpenFDsSource,
			GoHeapAllocBytes: &heap, GoHeapLiveBytes: &live, Goroutines: &goroutines,
			SocketSource: "unsupported", NetworkSource: "unsupported",
			UnsupportedMetrics: []string{"udp_sockets", "udp6_sockets", "network_rx_bytes", "network_tx_bytes"},
		})
	}
	limit, _ := hostOpenFDLimit()
	return roleResourceEvidence{
		Role: "load_driver", ProcessID: os.Getpid(), OpenFDLimit: limit,
		Samples: points, Summary: summarizeRoleResources(points),
		TrafficShaping: shapingEvidence{Source: "host_process", Status: "not_applicable"},
	}
}

func readCapacityEnvironment(ctx context.Context) (capacityEnvironment, error) {
	commit, err := commandText(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return capacityEnvironment{}, fmt.Errorf("read repository commit: %w", err)
	}
	status, err := commandText(ctx, "git", "status", "--porcelain")
	if err != nil {
		return capacityEnvironment{}, fmt.Errorf("read repository status: %w", err)
	}
	hostMemory, err := hostMemoryBytes(ctx)
	if err != nil {
		return capacityEnvironment{}, err
	}
	fdLimit, err := hostOpenFDLimit()
	if err != nil {
		return capacityEnvironment{}, err
	}
	trafficShaping := readHostShaping(ctx)
	if trafficShaping.Status == "active" {
		return capacityEnvironment{}, errors.New("host has active traffic shaping")
	}
	output, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .}}").Output()
	if err != nil {
		return capacityEnvironment{}, fmt.Errorf("read Docker capacity: %w", err)
	}
	var info struct {
		OperatingSystem string `json:"OperatingSystem"`
		OSType          string `json:"OSType"`
		Architecture    string `json:"Architecture"`
		NCPU            int    `json:"NCPU"`
		MemTotal        uint64 `json:"MemTotal"`
		ServerVersion   string `json:"ServerVersion"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return capacityEnvironment{}, fmt.Errorf("decode Docker capacity: %w", err)
	}
	if info.NCPU <= 0 || info.MemTotal == 0 {
		return capacityEnvironment{}, errors.New("Docker CPU and memory budgets must be positive")
	}
	return capacityEnvironment{
		RepositoryCommit: commit, RepositoryDirty: status != "",
		HostMemoryBytes: hostMemory, HostOpenFDLimit: fdLimit,
		TrafficShaping: trafficShaping,
		Docker: dockerEnvironment{
			OperatingSystem: info.OperatingSystem, OSType: info.OSType,
			Architecture: info.Architecture, LogicalCPU: info.NCPU, MemoryBytes: info.MemTotal,
			ServerVersion: info.ServerVersion, OrbStackVersion: orbStackVersion(ctx, info.OperatingSystem),
		},
	}, nil
}

func readHostShaping(ctx context.Context) shapingEvidence {
	if runtime.GOOS == "darwin" {
		output, err := commandText(ctx, "dnctl", "list")
		if err != nil {
			return shapingEvidence{Source: "dnctl", Status: "unsupported"}
		}
		if output == "" {
			return shapingEvidence{Source: "dnctl", Status: "inactive"}
		}
		return shapingEvidence{Source: "dnctl", Status: "active"}
	}
	if runtime.GOOS == "linux" {
		output, err := commandText(ctx, "tc", "qdisc", "show")
		if err != nil {
			return shapingEvidence{Source: "tc_qdisc", Status: "unsupported"}
		}
		return classifyTCShaping(output)
	}
	return shapingEvidence{Source: "platform", Status: "unsupported"}
}

func readContainerShaping(ctx context.Context, containerID string) (shapingEvidence, error) {
	output, err := commandText(ctx, "docker", "exec", containerID, "sh", "-c", "if command -v tc >/dev/null 2>&1; then tc qdisc show; else echo __unsupported__; fi")
	if err != nil {
		return shapingEvidence{}, err
	}
	if output == "__unsupported__" {
		return shapingEvidence{Source: "tc_qdisc", Status: "unsupported"}, nil
	}
	return classifyTCShaping(output), nil
}

func classifyTCShaping(output string) shapingEvidence {
	lower := strings.ToLower(output)
	for _, owner := range []string{" netem ", " tbf ", " htb ", " cake "} {
		if strings.Contains(" "+lower+" ", owner) {
			return shapingEvidence{Source: "tc_qdisc", Status: "active"}
		}
	}
	return shapingEvidence{Source: "tc_qdisc", Status: "inactive"}
}

func orbStackVersion(ctx context.Context, operatingSystem string) string {
	if !strings.Contains(strings.ToLower(operatingSystem), "orbstack") {
		return "unsupported"
	}
	version, err := commandText(ctx, "orb", "version")
	if err != nil || version == "" {
		return "unsupported"
	}
	return version
}

func commandText(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func hostMemoryBytes(ctx context.Context) (uint64, error) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0, fmt.Errorf("read host memory: %w", err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 3 && fields[0] == "MemTotal:" && fields[2] == "kB" {
				kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return 0, fmt.Errorf("parse host memory: %w", err)
				}
				return kilobytes * 1024, nil
			}
		}
		return 0, errors.New("read host memory: MemTotal not found")
	}
	if runtime.GOOS == "darwin" {
		value, err := commandText(ctx, "sysctl", "-n", "hw.memsize")
		if err != nil {
			return 0, fmt.Errorf("read host memory: %w", err)
		}
		bytes, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse host memory: %w", err)
		}
		return bytes, nil
	}
	return 0, fmt.Errorf("host memory sampling unsupported on %s", runtime.GOOS)
}

func hostOpenFDLimit() (uint64, error) {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, fmt.Errorf("read host open-file limit: %w", err)
	}
	if limit.Cur == 0 {
		return 0, errors.New("host open-file limit is zero")
	}
	return limit.Cur, nil
}
