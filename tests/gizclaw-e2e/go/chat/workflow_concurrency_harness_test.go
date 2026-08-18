//go:build gizclaw_e2e

package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const workflowConcurrency10 = 10

type workflowConcurrencyScenario string

const (
	workflowConcurrencyConversation workflowConcurrencyScenario = "conversation"
	workflowConcurrencyInterrupt    workflowConcurrencyScenario = "interrupt"
)

type workflowConcurrencySpec struct {
	Name                  string
	Fixture               string
	InputMode             string
	RequireText           bool
	RequireAudio          bool
	KeepRealtimeInputOpen bool
	RealtimeTailSilence   time.Duration
}

var (
	realtimeWorkflowConcurrencySpec = workflowConcurrencySpec{
		Name: "realtime", Fixture: "doubao-realtime.json", InputMode: string(rpcapi.WorkspaceInputModeRealtime), RequireText: true, RequireAudio: true,
	}
	flowcraftWorkflowConcurrencySpec = workflowConcurrencySpec{
		Name: "flowcraft", Fixture: "flowcraft-basic.json", InputMode: string(rpcapi.WorkspaceInputModeRealtime), RequireText: true, RequireAudio: true,
	}
	einoWorkflowConcurrencySpec = workflowConcurrencySpec{
		Name: "eino", Fixture: "eino-concurrency.json", RequireText: true,
	}
	translateWorkflowConcurrencySpec = workflowConcurrencySpec{
		Name: "translate", Fixture: "ast-translate.json", InputMode: string(rpcapi.WorkspaceInputModeRealtime), RequireText: true, RequireAudio: true,
	}
	realtimeWorkflowRealtimeConcurrencySpec = workflowConcurrencySpec{
		Name: "realtime", Fixture: "doubao-realtime.json", InputMode: string(rpcapi.WorkspaceInputModeRealtime), RequireText: true, RequireAudio: true, KeepRealtimeInputOpen: true,
	}
	flowcraftWorkflowRealtimeConcurrencySpec = workflowConcurrencySpec{
		Name: "flowcraft", Fixture: "flowcraft-basic.json", InputMode: string(rpcapi.WorkspaceInputModeRealtime), RequireText: true, RequireAudio: true, KeepRealtimeInputOpen: true, RealtimeTailSilence: 4 * time.Second,
	}
	einoWorkflowRealtimeConcurrencySpec = workflowConcurrencySpec{
		Name: "eino", Fixture: "eino-concurrency.json", InputMode: string(rpcapi.WorkspaceInputModeRealtime), RequireText: true, RequireAudio: true, KeepRealtimeInputOpen: true, RealtimeTailSilence: 4 * time.Second,
	}
	translateWorkflowRealtimeConcurrencySpec = workflowConcurrencySpec{
		Name: "translate", Fixture: "ast-translate.json", InputMode: string(rpcapi.WorkspaceInputModeRealtime), RequireText: true, RequireAudio: true, KeepRealtimeInputOpen: true,
	}
)

type workflowConcurrencyLane struct {
	Index     int
	PublicKey string
	Config    config
	Client    *gizcli.Client
	ServeDone <-chan error
	Transport *chatTransport
	Driver    *personaDriver
	StartedAt time.Time
	Result    workflowConcurrencyLaneResult
}

type workflowConcurrencyInputs struct {
	Texts   []string
	Packets [][][]byte
}

type workflowConcurrencyBarrier struct {
	total   int
	ready   chan int
	release chan struct{}
	once    sync.Once
}

func newWorkflowConcurrencyBarrier(total int) *workflowConcurrencyBarrier {
	return &workflowConcurrencyBarrier{
		total: total, ready: make(chan int, total), release: make(chan struct{}),
	}
}

func (b *workflowConcurrencyBarrier) arriveAndWait(ctx context.Context, lane int) error {
	select {
	case b.ready <- lane:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *workflowConcurrencyBarrier) waitReady(ctx context.Context) ([]int, error) {
	ready := make([]int, 0, b.total)
	for len(ready) < b.total {
		select {
		case lane := <-b.ready:
			ready = append(ready, lane)
		case <-ctx.Done():
			return ready, ctx.Err()
		}
	}
	return ready, nil
}

func (b *workflowConcurrencyBarrier) releaseAll() {
	b.once.Do(func() { close(b.release) })
}

func runWorkflowConcurrency10(t *testing.T, spec workflowConcurrencySpec, scenario workflowConcurrencyScenario) {
	t.Helper()
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}

	fixture := selectedWorkspaceConfigPaths(t, spec.Fixture)[0]
	base, err := loadConfig(fixture, clientContextConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	base.Rounds = 1
	if spec.InputMode != "" {
		base.Workflow.Parameters.Input = spec.InputMode
	}
	token := createChatRegistrationToken(t, workspaceCase("workflow-concurrency-"+spec.Name))
	adminClient, adminAPI := newWorkflowConcurrencyAdmin(t, spec.Name+"-"+string(scenario))
	defer adminClient.Close()

	runID := fmt.Sprintf("%x", time.Now().UnixNano())
	waveCtx, cancel := context.WithTimeout(context.Background(), base.timeout)
	defer cancel()
	wave := newWorkflowConcurrencyArtifact(spec, scenario, workflowConcurrency10)
	wave.captureBefore()

	lanes, prepareErr := prepareWorkflowConcurrencyLanes(waveCtx, base, token, spec, scenario, runID)
	var inputs workflowConcurrencyInputs
	var inputErr error
	if prepareErr == nil {
		inputs, inputErr = prepareWorkflowConcurrencyInputs(waveCtx, lanes, spec, scenario)
		if inputErr != nil {
			for _, lane := range lanes {
				if lane != nil {
					lane.Result.Error = workflowConcurrencyErrorText(inputErr)
				}
			}
		}
	}
	var scenarioErr error
	if prepareErr == nil && inputErr == nil {
		scenarioErr = executeWorkflowConcurrencyWave(waveCtx, lanes, spec, scenario, inputs, wave)
	}

	cleanupErr := cleanupWorkflowConcurrencyLanes(lanes, adminAPI)
	wave.captureAfter()
	wave.captureLanes(lanes)
	artifactErr := writeWorkflowConcurrencyArtifact(wave)
	if err := errors.Join(prepareErr, inputErr, scenarioErr, cleanupErr, artifactErr); err != nil {
		t.Fatalf("workflow concurrency %s/%s: %v", spec.Name, scenario, err)
	}
}

func newWorkflowConcurrencyAdmin(t *testing.T, story string) (*gizcli.Client, *adminhttp.ClientWithResponses) {
	t.Helper()
	h := clitest.NewSetupHarness(t, "go-chat-workflow-concurrency-"+story)
	identitiesHome := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_IDENTITIES_HOME"))
	if identitiesHome == "" {
		identitiesHome = filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities")
	}
	adminContext := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_IDENTITY"))
	if adminContext == "" {
		adminContext = "admin"
	}
	h.SetContextDirAlias("workflow-concurrency-admin", filepath.Join(identitiesHome, adminContext))
	client := h.ConnectClientFromContextEventually("workflow-concurrency-admin", 30*time.Second)
	api, err := client.ServerAdminClient()
	if err != nil {
		client.Close()
		t.Fatalf("create workflow concurrency Admin client: %v", err)
	}
	return client, api
}

type workflowConcurrencyPrepareResult struct {
	lane *workflowConcurrencyLane
	err  error
}

func prepareWorkflowConcurrencyLanes(
	ctx context.Context,
	base config,
	token string,
	spec workflowConcurrencySpec,
	scenario workflowConcurrencyScenario,
	runID string,
) ([]*workflowConcurrencyLane, error) {
	results := make(chan workflowConcurrencyPrepareResult, workflowConcurrency10)
	for index := 1; index <= workflowConcurrency10; index++ {
		index := index
		go func() {
			lane, err := prepareWorkflowConcurrencyLane(ctx, base, token, spec, scenario, runID, index)
			results <- workflowConcurrencyPrepareResult{lane: lane, err: err}
		}()
	}
	lanes := make([]*workflowConcurrencyLane, workflowConcurrency10)
	var errs []error
	for range workflowConcurrency10 {
		result := <-results
		if result.lane != nil {
			lanes[result.lane.Index-1] = result.lane
			result.lane.Result.Error = workflowConcurrencyErrorText(result.err)
		}
		if result.err != nil {
			errs = append(errs, result.err)
		}
	}
	return lanes, errors.Join(errs...)
}

func prepareWorkflowConcurrencyLane(
	ctx context.Context,
	base config,
	token string,
	spec workflowConcurrencySpec,
	scenario workflowConcurrencyScenario,
	runID string,
	index int,
) (*workflowConcurrencyLane, error) {
	lane := &workflowConcurrencyLane{Index: index}
	lane.Result.Index = index
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		return lane, fmt.Errorf("lane %d generate key: %w", index, err)
	}
	lane.PublicKey = keyPair.Public.String()
	lane.Result.PublicKey = lane.PublicKey
	cfg := base
	cfg.ClientPrivateKey = keyPair.Private.String()
	cfg.Workspace = compactWorkspaceName(fmt.Sprintf("wc-%s-%s-%s-%02d", spec.Name, scenario, runID, index))
	cfg.Ensure = ptr(true)
	lane.Config = cfg
	lane.Result.Workspace = cfg.Workspace

	client, serveDone, err := dialClient(cfg)
	if err != nil {
		return lane, fmt.Errorf("lane %d dial: %w", index, err)
	}
	lane.Client = client
	lane.ServeDone = serveDone
	if _, err := client.Register(ctx, fmt.Sprintf("workflow-concurrency.register.%02d", index), token); err != nil {
		return lane, fmt.Errorf("lane %d register: %w", index, err)
	}
	ensured, err := ensureWorkspace(ctx, client, cfg)
	if err != nil {
		return lane, fmt.Errorf("lane %d ensure Workspace: %w", index, err)
	}
	lane.Config = ensured
	transport, err := newChatTransport(client)
	if err != nil {
		return lane, fmt.Errorf("lane %d open logical PeerStream: %w", index, err)
	}
	lane.Transport = transport
	if err := selectAndReloadAgent(ctx, client, ensured); err != nil {
		return lane, fmt.Errorf("lane %d start Workspace: %w", index, err)
	}
	state, err := client.GetServerRunWorkspace(ctx, fmt.Sprintf("workflow-concurrency.runtime.%02d", index))
	if err != nil {
		return lane, fmt.Errorf("lane %d get runtime: %w", index, err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning || state.WorkspaceName != ensured.Workspace || state.StartedAt == nil || state.StartedAt.IsZero() {
		return lane, fmt.Errorf("lane %d runtime not ready: state=%s workspace=%q started_at=%v", index, state.RuntimeState, state.WorkspaceName, state.StartedAt)
	}
	lane.StartedAt = *state.StartedAt
	lane.Result.StartedAt = lane.StartedAt

	openaiHTTPClient := client.HTTPClient(gizcli.ServicePeerOpenAI)
	openaiHTTPClient.Timeout = cfg.timeout
	openaiClient := openai.NewClient(
		option.WithAPIKey("gizclaw-peer"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(openaiHTTPClient),
	)
	lane.Driver = &personaDriver{cfg: ensured, client: openaiClient, runtimeClient: client, transport: transport}
	transport.drain()
	lane.Result.ReadyAt = time.Now()
	return lane, nil
}

func prepareWorkflowConcurrencyInputs(
	ctx context.Context,
	lanes []*workflowConcurrencyLane,
	spec workflowConcurrencySpec,
	scenario workflowConcurrencyScenario,
) (workflowConcurrencyInputs, error) {
	turns := 1
	if scenario == workflowConcurrencyInterrupt {
		turns = 3
	}
	texts := []string{
		"请依次输出数字一到三十，每个数字之间用逗号分隔，不要解释",
		"请依次输出十二生肖，每个生肖之间用逗号分隔，不要解释",
		"请只回答收到",
	}[:turns]
	inputs := workflowConcurrencyInputs{Texts: append([]string(nil), texts...)}
	if !spec.RequireAudio {
		return inputs, nil
	}
	if len(lanes) == 0 || lanes[0] == nil || lanes[0].Driver == nil {
		return inputs, fmt.Errorf("voice input preparation requires lane 1")
	}
	inputs.Packets = make([][][]byte, turns)
	for index, text := range texts {
		_, packets, err := lanes[0].Driver.synthesizeOpusOnce(ctx, text)
		if err != nil {
			return inputs, fmt.Errorf("prepare voice input turn %d: %w", index+1, err)
		}
		if spec.KeepRealtimeInputOpen {
			packets, err = realtimePacketsWithTailSilenceDuration(packets, workflowConcurrencyRealtimeTailSilence(spec))
		} else {
			packets, err = realtimePacketsWithTailSilence(packets)
		}
		if err != nil {
			return inputs, fmt.Errorf("prepare voice input turn %d tail silence: %w", index+1, err)
		}
		inputs.Packets[index] = cloneOpusPackets(packets)
	}
	return inputs, nil
}

func workflowConcurrencyRealtimeTailSilence(spec workflowConcurrencySpec) time.Duration {
	if spec.RealtimeTailSilence > 0 {
		return spec.RealtimeTailSilence
	}
	return realtimeInputTailSilence
}

func cloneOpusPackets(packets [][]byte) [][]byte {
	clone := make([][]byte, len(packets))
	for index := range packets {
		clone[index] = append([]byte(nil), packets[index]...)
	}
	return clone
}

type workflowConcurrencyLaneExecution struct {
	index int
	err   error
}

func executeWorkflowConcurrencyWave(
	ctx context.Context,
	lanes []*workflowConcurrencyLane,
	spec workflowConcurrencySpec,
	scenario workflowConcurrencyScenario,
	inputs workflowConcurrencyInputs,
	wave *workflowConcurrencyArtifact,
) error {
	barrier := newWorkflowConcurrencyBarrier(len(lanes))
	results := make(chan workflowConcurrencyLaneExecution, len(lanes))
	for _, lane := range lanes {
		lane := lane
		go func() {
			if lane == nil {
				results <- workflowConcurrencyLaneExecution{err: fmt.Errorf("prepared lane is nil")}
				return
			}
			if err := barrier.arriveAndWait(ctx, lane.Index); err != nil {
				results <- workflowConcurrencyLaneExecution{index: lane.Index, err: fmt.Errorf("lane %d barrier: %w", lane.Index, err)}
				return
			}
			lane.Result.ReleasedAt = time.Now()
			laneCtx, cancel := context.WithCancel(ctx)
			err := runWorkflowConcurrencyScenario(laneCtx, lane, spec, scenario, inputs)
			cancel()
			lane.Result.FinishedAt = time.Now()
			lane.Result.Error = workflowConcurrencyErrorText(err)
			results <- workflowConcurrencyLaneExecution{index: lane.Index, err: err}
		}()
	}
	ready, barrierErr := barrier.waitReady(ctx)
	wave.Barrier.Ready = len(ready)
	wave.Barrier.ReadyAt = time.Now()
	if barrierErr == nil {
		wave.Barrier.ReleasedAt = time.Now()
	}
	barrier.releaseAll()
	var errs []error
	if barrierErr != nil {
		errs = append(errs, fmt.Errorf("ready barrier %d/%d: %w", len(ready), len(lanes), barrierErr))
	}
	for range lanes {
		result := <-results
		if result.err != nil {
			errs = append(errs, fmt.Errorf("lane %d: %w", result.index, result.err))
		}
	}
	return errors.Join(errs...)
}

func cleanupWorkflowConcurrencyLanes(lanes []*workflowConcurrencyLane, adminAPI *adminhttp.ClientWithResponses) error {
	results := make(chan error, len(lanes))
	for _, lane := range lanes {
		lane := lane
		go func() { results <- cleanupWorkflowConcurrencyLane(lane) }()
	}
	var errs []error
	for range lanes {
		if err := <-results; err != nil {
			errs = append(errs, err)
		}
	}
	for _, lane := range lanes {
		if lane == nil || lane.PublicKey == "" || lane.Client == nil {
			continue
		}
		offline, err := waitWorkflowConcurrencyPeerOffline(adminAPI, lane.PublicKey, 15*time.Second)
		lane.Result.Cleanup.PeerOffline = offline
		if err != nil {
			lane.Result.Cleanup.Errors = append(lane.Result.Cleanup.Errors, err.Error())
			errs = append(errs, fmt.Errorf("lane %d Peer offline: %w", lane.Index, err))
		}
	}
	return errors.Join(errs...)
}

func cleanupWorkflowConcurrencyLane(lane *workflowConcurrencyLane) error {
	if lane == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var errs []error
	if lane.Transport != nil {
		lane.Transport.close()
		lane.Result.Cleanup.StreamClosed = true
	}
	if lane.Client != nil {
		if _, err := lane.Client.StopServerRun(ctx, fmt.Sprintf("workflow-concurrency.cleanup.stop.%02d", lane.Index)); err != nil {
			errs = append(errs, fmt.Errorf("stop runtime: %w", err))
		} else if err := waitWorkflowConcurrencyRuntimeStopped(ctx, lane.Client); err != nil {
			errs = append(errs, err)
		} else {
			lane.Result.Cleanup.RuntimeStopped = true
		}
		if lane.Config.Workspace != "" {
			if _, err := lane.Client.DeleteWorkspace(ctx, fmt.Sprintf("workflow-concurrency.cleanup.delete.%02d", lane.Index), rpcapi.WorkspaceDeleteRequest{Name: lane.Config.Workspace}); err != nil && !isRPCNotFound(err) {
				errs = append(errs, fmt.Errorf("delete Workspace: %w", err))
			} else {
				lane.Result.Cleanup.WorkspaceDeleted = true
			}
		}
		_ = lane.Client.Close()
		lane.Result.Cleanup.ClientClosed = true
		if lane.ServeDone != nil {
			select {
			case <-lane.ServeDone:
				lane.Result.Cleanup.ServeJoined = true
			case <-ctx.Done():
				errs = append(errs, fmt.Errorf("join client Serve: %w", ctx.Err()))
			}
		}
	}
	for _, err := range errs {
		lane.Result.Cleanup.Errors = append(lane.Result.Cleanup.Errors, err.Error())
	}
	return errors.Join(errs...)
}

func waitWorkflowConcurrencyRuntimeStopped(ctx context.Context, client *gizcli.Client) error {
	for {
		state, err := client.GetServerRunWorkspace(ctx, "workflow-concurrency.cleanup.runtime")
		if err == nil && state.RuntimeState == rpcapi.PeerRunStatusStateStopped {
			return nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("wait runtime stopped: %w", err)
			}
			return fmt.Errorf("wait runtime stopped: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func waitWorkflowConcurrencyPeerOffline(api *adminhttp.ClientWithResponses, publicKey string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		response, err := api.GetPeerRuntimeWithResponse(ctx, publicKey)
		cancel()
		if err == nil && response.JSON200 != nil && !response.JSON200.Online {
			return true, nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return false, err
			}
			if response == nil {
				return false, fmt.Errorf("empty Admin response")
			}
			return false, fmt.Errorf("Peer remained online: status=%d", response.StatusCode())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func captureWorkflowConcurrencyRuntime() workflowConcurrencyRuntimeSnapshot {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return workflowConcurrencyRuntimeSnapshot{
		CapturedAt: time.Now(), Goroutines: runtime.NumGoroutine(), HeapAlloc: stats.HeapAlloc, HeapObjects: stats.HeapObjects,
	}
}
