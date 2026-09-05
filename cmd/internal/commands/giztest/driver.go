package giztestcmd

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// driver executes Giztest steps with the Go device SDK in sdk/go/gizcli. It
// dials one WebRTC Peer per declared client and drives Peer RPC, Peer
// streams, workspace relays, speech, and the Public HTTP surface.
type driver struct {
	fullEvidence  bool
	audioObserver audioObserver
	speechCache   *speechFixtureCache

	// The remaining fields substitute transports in this package's tests.
	// They are nil in production.
	openPeerStream   func(client *gizcli.Client) peerStreamOpener
	openRelayStreams func() (relayStream, relayStream, error)
	connectClients   func(context.Context, map[string]giztest.ClientSpec, []giztest.Step, *giztest.Variables) (*clientSet, error)
}

// newDriver builds the production driver.
func newDriver(fullEvidence bool, observer audioObserver) *driver {
	return &driver{fullEvidence: fullEvidence, audioObserver: observer, speechCache: newSpeechFixtureCache()}
}

// Operations lists "parallel" because session implements
// giztest.ParallelSession for peer_stream steps.
func (d *driver) Operations() []string {
	return []string{"telemetry", "rpc", "rpc_stream", "client_rpc", "http", "speech", "peer_stream", "reconnect", "workspace_relay", "parallel"}
}

func (d *driver) ValidateStep(doc *giztest.Document, step giztest.Step) error {
	if step.Telemetry != nil {
		normalized, err := validationValue(step.Telemetry.Frame, doc.Variables)
		if err != nil {
			return err
		}
		_, err = decodeTelemetryFrame(normalized)
		return err
	}
	if step.RPC != nil {
		if err := validateRPCRequestShape(step.RPC.Method, step.RPC.Request, doc.Variables); err != nil {
			return err
		}
	}
	if step.RPCStream != nil {
		if err := validateRPCRequestShape(step.RPCStream.Method, step.RPCStream.Request, doc.Variables); err != nil {
			return err
		}
	}
	return nil
}

func (d *driver) FailureCode(err error) (int32, string, bool) {
	var failure *rpcFailure
	if errors.As(err, &failure) {
		return failure.code, failure.message, true
	}
	var apiError rpcapi.Error
	if errors.As(err, &apiError) {
		return int32(apiError.Code), apiError.Message, true
	}
	return 0, "", false
}

func (d *driver) Open(ctx context.Context, doc *giztest.Document, vars *giztest.Variables) (giztest.Session, error) {
	connect := connectClients
	if d.connectClients != nil {
		connect = d.connectClients
	}
	clients, err := connect(ctx, doc.Clients, doc.Steps, vars)
	if clients == nil {
		return nil, err
	}
	return &session{driver: d, clients: clients, streams: newPeerStreamSessions()}, err
}

// session is one task's set of dialed clients plus the peer streams the
// document is holding open across steps.
type session struct {
	driver  *driver
	clients *clientSet
	streams *peerStreamSessions
}

func (s *session) Fingerprints() map[string]string { return s.clients.fingerprints() }

func (s *session) CloseStreams() error { return s.streams.Close() }

func (s *session) Close() { s.clients.Close() }

func (s *session) Execute(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	step := req.Step
	vars := req.Vars
	switch step.Operation() {
	case "telemetry":
		return s.executeTelemetry(req)
	case "rpc":
		client, err := s.clients.get(step.Client)
		if err != nil {
			return giztest.StepResult{}, err
		}
		params := step.RPC.Request
		if params == nil {
			params = map[string]any{}
		}
		params, err = vars.Resolve(params)
		if err != nil {
			return giztest.StepResult{}, err
		}
		value, err := invokeUnary(ctx, client, step, params)
		if err != nil {
			return giztest.StepResult{}, err
		}
		return giztest.StepResult{Value: value, Saved: value}, nil
	case "rpc_stream":
		client, err := s.clients.get(step.Client)
		if err != nil {
			return giztest.StepResult{}, err
		}
		request, err := vars.Resolve(step.RPCStream.Request)
		if err != nil {
			return giztest.StepResult{}, err
		}
		result, err := invokeRPCStream(ctx, client, step, request)
		return result.stepResult(), err
	case "speech":
		return s.executeSpeech(ctx, req)
	case "peer_stream":
		return s.executePeerStream(ctx, req)
	case "workspace_relay":
		return s.executeWorkspaceRelay(ctx, req)
	case "http":
		endpoint, err := s.clients.endpoint(step.Client)
		if err != nil {
			return giztest.StepResult{}, err
		}
		result, err := invokeHTTP(ctx, endpoint, step, vars)
		return giztest.StepResult{Value: result.body, Saved: result.body, Evidence: result.evidence}, err
	case "client_rpc":
		return s.executeClientRPC(ctx, step)
	case "reconnect":
		return giztest.StepResult{}, s.clients.reconnect(ctx, step.Client, step.Reconnect.AwaitMs)
	}
	return giztest.StepResult{}, fmt.Errorf("unsupported operation %q", step.Operation())
}

func (s *session) executeSpeech(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	step := req.Step
	vars := req.Vars
	client, err := s.clients.get(step.Client)
	if err != nil {
		return giztest.StepResult{}, err
	}
	request, err := vars.Resolve(step.Speech.Request)
	if err != nil {
		return giztest.StepResult{}, err
	}
	input, err := vars.Resolve(step.Speech.Input)
	if err != nil && step.Speech.Input != nil {
		return giztest.StepResult{}, err
	}
	var outputSpec giztest.VariableSpec
	if step.SaveAs != "" {
		outputSpec, _ = vars.Spec(step.SaveAs)
	}
	inputSpec, _ := vars.ReferencedSpec(step.Speech.Input)
	invoke := func() (operationResult, error) {
		return invokeSpeech(ctx, client, step, request, input, inputSpec, outputSpec, s.driver.fullEvidence)
	}
	if step.Speech.Cache != "run" {
		result, err := invoke()
		return result.stepResult(), err
	}
	key, err := speechFixtureKey(req.DocumentPath, step, request, outputSpec)
	if err != nil {
		return giztest.StepResult{}, err
	}
	result, hit, err := s.driver.speechCache.Do(ctx, key, invoke)
	result.evidence = maps.Clone(result.evidence)
	if result.evidence == nil {
		result.evidence = make(map[string]any)
	}
	if hit {
		result.evidence["cache"] = "hit"
	} else {
		result.evidence["cache"] = "miss"
	}
	return result.stepResult(), err
}

func (s *session) executePeerStream(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	invocation, err := s.preparePeerStream(req.Step, req.Step, req.Vars)
	if err != nil {
		return giztest.StepResult{}, err
	}
	result, err := invocation.run(ctx, s.streams)
	return result.stepResult(), err
}

// PrepareParallel implements giztest.ParallelSession for peer_stream steps.
// Every variable read and the client lookup happen here, on the task
// goroutine; the returned run phase only drives the PeerStream. The parallel
// step owns the capture map, so the bound it declared for this child, such as
// the /audio limit, applies from the start.
func (s *session) PrepareParallel(req giztest.StepRequest) (giztest.ParallelChild, error) {
	step := req.Step
	if step.PeerStream == nil {
		return nil, fmt.Errorf("parallel child %s requires peer_stream", step.ID)
	}
	if s.driver.audioObserver != nil {
		return nil, fmt.Errorf("parallel children cannot play audio interactively")
	}
	captureStep := step
	if req.Parent != nil {
		captureStep = giztest.Step{ID: step.ID, Capture: giztest.ParallelChildCaptures(*req.Parent, step.ID)}
	}
	invocation, err := s.preparePeerStream(step, captureStep, req.Vars)
	if err != nil {
		return nil, err
	}
	return invocation, nil
}

// peerStreamInvocation is a peer_stream step whose variable reads and client
// lookup are complete, so the stream can be driven on the task goroutine or
// from a parallel child goroutine without touching task variables.
type peerStreamInvocation struct {
	client               *gizcli.Client
	open                 peerStreamOpener
	step                 giztest.Step
	input                any
	audioCaptureMaxBytes int
	observer             audioObserver
}

// preparePeerStream resolves the step input and the /audio capture bound.
// captureStep owns the capture map: the step itself, or the child-scoped view
// of the capture map of the parallel step that owns it.
func (s *session) preparePeerStream(step, captureStep giztest.Step, vars *giztest.Variables) (peerStreamInvocation, error) {
	client, err := s.clients.get(step.Client)
	if err != nil {
		return peerStreamInvocation{}, err
	}
	var input any
	if step.PeerStream.Input != nil {
		input, err = vars.Resolve(step.PeerStream.Input)
		if err != nil {
			return peerStreamInvocation{}, err
		}
	}
	if spec, ok := vars.ReferencedSpec(step.PeerStream.Input); ok && step.PeerStream.Mode != "text" {
		if spec.Type != "audio" || spec.Codec != "opus" || (spec.MediaType != "audio/ogg" && spec.MediaType != "audio/opus") {
			return peerStreamInvocation{}, fmt.Errorf("peer_stream audio input must declare audio/ogg or audio/opus with opus codec")
		}
	}
	audioCaptureMaxBytes, err := peerStreamAudioCaptureMaxBytes(captureStep, vars)
	if err != nil {
		return peerStreamInvocation{}, err
	}
	open := openClientPeerStream(client)
	if s.driver.openPeerStream != nil {
		open = s.driver.openPeerStream(client)
	}
	return peerStreamInvocation{client: client, open: open, step: step, input: input, audioCaptureMaxBytes: audioCaptureMaxBytes, observer: s.driver.audioObserver}, nil
}

// run drives the stream. sessions is the task's held-open stream set, or nil
// for a parallel child, which validation keeps out of sessions.
func (p peerStreamInvocation) run(ctx context.Context, sessions *peerStreamSessions) (operationResult, error) {
	return invokePeerStreamWithSessions(ctx, p.client, p.open, sessions, p.step, p.input, p.audioCaptureMaxBytes, p.observer)
}

// Run implements giztest.ParallelChild.
func (p peerStreamInvocation) Run(ctx context.Context) (giztest.StepResult, error) {
	result, err := p.run(ctx, nil)
	return result.stepResult(), err
}

func (s *session) executeWorkspaceRelay(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	step := req.Step
	vars := req.Vars
	input, err := vars.Resolve(step.WorkspaceRelay.Input)
	if err != nil {
		return giztest.StepResult{}, err
	}
	if spec, ok := vars.ReferencedSpec(step.WorkspaceRelay.Input); ok && step.WorkspaceRelay.Media == "audio" {
		if spec.Type != "audio" || spec.Codec != "opus" || (spec.MediaType != "audio/ogg" && spec.MediaType != "audio/opus") {
			return giztest.StepResult{}, fmt.Errorf("workspace_relay audio input must declare audio/ogg or audio/opus with opus codec")
		}
	}
	audioCaptureMaxBytes, err := relayAudioCaptureMaxBytes(step, vars)
	if err != nil {
		return giztest.StepResult{}, err
	}
	if s.driver.openRelayStreams == nil {
		result, err := invokeWorkspaceRelay(
			ctx, s.clients, step, input, audioCaptureMaxBytes, s.driver.fullEvidence, s.driver.audioObserver)
		return result.stepResult(), err
	}
	first, second, err := s.driver.openRelayStreams()
	if err != nil {
		return giztest.StepResult{}, err
	}
	result, err := runWorkspaceRelayWithEvidence(
		ctx, step.WorkspaceRelay, first, second, input, audioCaptureMaxBytes,
		s.driver.fullEvidence, s.driver.audioObserver)
	return result.stepResult(), err
}

func (s *session) executeClientRPC(ctx context.Context, step giztest.Step) (giztest.StepResult, error) {
	counter := s.clients.inbound[step.Client+":"+step.ClientRPC.Method]
	if counter == nil {
		return giztest.StepResult{}, fmt.Errorf("client RPC %s was not installed", step.ClientRPC.Method)
	}
	expected := int64(step.ClientRPC.ExpectCalls)
	if expected == 0 {
		expected = 1
	}
	calls, err := awaitInboundCalls(ctx, counter, expected, step.ClientRPC.Method)
	if err != nil {
		return giztest.StepResult{}, err
	}
	evidence := map[string]any{"method": step.ClientRPC.Method, "calls": calls}
	return giztest.StepResult{Value: evidence, Saved: evidence, Evidence: evidence}, nil
}

// stepResult adapts an operation outcome to the runner's contract.
func (r operationResult) stepResult() giztest.StepResult {
	return giztest.StepResult{Value: r.assertion, Saved: r.saved, Evidence: r.evidence}
}

// peerStreamAudioCaptureMaxBytes reports the tightest max_bytes among the
// variables the step captures /audio into, or 0 when it captures none.
func peerStreamAudioCaptureMaxBytes(step giztest.Step, vars *giztest.Variables) (int, error) {
	limit := 0
	for name, pointer := range step.Capture {
		if pointer != "/audio" {
			continue
		}
		spec, ok := vars.Spec(name)
		if !ok {
			return 0, fmt.Errorf("capture references unknown variable %q", name)
		}
		if spec.Type != "audio" || spec.MaxBytes <= 0 {
			return 0, fmt.Errorf("peer_stream /audio capture variable %q must be audio with max_bytes", name)
		}
		if limit == 0 || spec.MaxBytes < limit {
			limit = spec.MaxBytes
		}
	}
	return limit, nil
}

// relayAudioCaptureMaxBytes is the workspace_relay counterpart, bounded by the
// document-level relay audio limit.
func relayAudioCaptureMaxBytes(step giztest.Step, vars *giztest.Variables) (int, error) {
	limit := 0
	for name, pointer := range step.Capture {
		if pointer != "/terminal/audio" {
			continue
		}
		spec, ok := vars.Spec(name)
		if !ok {
			return 0, fmt.Errorf("capture references unknown variable %q", name)
		}
		if spec.Type != "audio" || spec.MaxBytes <= 0 || spec.MaxBytes > giztest.MaxRelayAudioBytes {
			return 0, fmt.Errorf("workspace_relay /terminal/audio capture variable %q must be audio with max_bytes up to %d", name, giztest.MaxRelayAudioBytes)
		}
		if limit == 0 || spec.MaxBytes < limit {
			limit = spec.MaxBytes
		}
	}
	return limit, nil
}
