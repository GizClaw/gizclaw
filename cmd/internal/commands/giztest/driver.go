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

func (d *driver) Operations() []string {
	return []string{"rpc", "rpc_stream", "client_rpc", "http", "speech", "peer_stream", "reconnect", "workspace_relay"}
}

func (d *driver) ValidateStep(doc *giztest.Document, step giztest.Step) error {
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
		return invokeSpeech(ctx, client, step, request, input, inputSpec, outputSpec)
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
	step := req.Step
	vars := req.Vars
	client, err := s.clients.get(step.Client)
	if err != nil {
		return giztest.StepResult{}, err
	}
	input, err := vars.Resolve(step.PeerStream.Input)
	if err != nil && step.PeerStream.Input != nil {
		return giztest.StepResult{}, err
	}
	if spec, ok := vars.ReferencedSpec(step.PeerStream.Input); ok && step.PeerStream.Mode != "text" {
		if spec.Type != "audio" || spec.Codec != "opus" || (spec.MediaType != "audio/ogg" && spec.MediaType != "audio/opus") {
			return giztest.StepResult{}, fmt.Errorf("peer_stream audio input must declare audio/ogg or audio/opus with opus codec")
		}
	}
	audioCaptureMaxBytes, err := peerStreamAudioCaptureMaxBytes(step, vars)
	if err != nil {
		return giztest.StepResult{}, err
	}
	open := openClientPeerStream(client)
	if s.driver.openPeerStream != nil {
		open = s.driver.openPeerStream(client)
	}
	result, err := invokePeerStreamWithSessions(
		ctx, client, open, s.streams, step, input, audioCaptureMaxBytes, s.driver.audioObserver)
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
