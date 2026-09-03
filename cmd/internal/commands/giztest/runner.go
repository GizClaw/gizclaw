package giztest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type runOptions struct {
	parallel      int
	in            io.Reader
	out           io.Writer
	fullEvidence  bool
	audioObserver audioObserver
	speechCache   *speechFixtureCache
	// openPeerStream overrides how peer_stream steps dial the PeerStream;
	// nil uses the selected client's real PeerStream.
	openPeerStream func(client *gizcli.Client) peerStreamOpener
	// openRelayStreams substitutes paired in-memory streams in runner tests.
	openRelayStreams func() (relayStream, relayStream, error)
	// connectClients substitutes already-connected clients in runner tests.
	connectClients func(context.Context, map[string]ClientSpec, []Step, *variables) (*clientSet, error)
	// backgroundCancelGrace shortens the wait for a cancelled background step
	// in runner tests; zero uses backgroundCancelGrace.
	backgroundCancelGrace time.Duration
}
type task struct {
	doc     *Document
	index   int
	barrier *taskBarrier
}

func runDocuments(ctx context.Context, docs []*Document, opts runOptions) Report {
	started := time.Now()
	report := Report{Version: "v1", StartedAt: started}
	if opts.speechCache == nil {
		opts.speechCache = newSpeechFixtureCache()
	}
	var groups [][]task
	taskCount := 0
	for _, doc := range docs {
		var barrier *taskBarrier
		if documentHasBarrier(doc) {
			barrier = newTaskBarrier(doc.Repeat)
			if opts.parallel < doc.Repeat {
				for i := range doc.Repeat {
					report.Tasks = append(report.Tasks, TaskReport{Path: doc.Path, Name: doc.Name, TaskID: fmt.Sprintf("%s-%04d", doc.Name, i), RepeatIndex: i, Status: "failed", Error: fmt.Sprintf("parallelism %d is below barrier group %d", opts.parallel, doc.Repeat)})
				}
				continue
			}
		}
		group := make([]task, 0, doc.Repeat)
		for i := range doc.Repeat {
			item := task{doc: doc, index: i, barrier: barrier}
			if barrier == nil {
				groups = append(groups, []task{item})
			} else {
				group = append(group, item)
			}
			taskCount++
		}
		if barrier != nil {
			groups = append(groups, group)
		}
	}
	results := make(chan TaskReport, taskCount)
	slots := make(chan struct{}, opts.parallel)
	var wg sync.WaitGroup
	go func() {
		defer func() {
			wg.Wait()
			close(results)
		}()
		for groupIndex, group := range groups {
			acquired := 0
			for acquired < len(group) {
				if ctx.Err() != nil {
					break
				}
				select {
				case slots <- struct{}{}:
					acquired++
				case <-ctx.Done():
				}
			}
			if acquired != len(group) {
				for range acquired {
					<-slots
				}
				for _, remaining := range groups[groupIndex:] {
					for _, item := range remaining {
						results <- cancelledTaskReport(item, ctx)
					}
				}
				return
			}
			for _, item := range group {
				wg.Go(func() {
					defer func() { <-slots }()
					results <- runTask(ctx, item, opts)
				})
			}
		}
	}()
	for result := range results {
		report.Tasks = append(report.Tasks, result)
	}
	report.finish(started)
	return report
}

func cancelledTaskReport(item task, ctx context.Context) TaskReport {
	err := context.Cause(ctx)
	if err == nil {
		err = ctx.Err()
	}
	return TaskReport{
		Path: item.doc.Path, Name: item.doc.Name,
		TaskID:      fmt.Sprintf("%s-%04d", item.doc.Name, item.index),
		RepeatIndex: item.index, Status: "failed", Error: safeError(err),
	}
}

func documentHasBarrier(doc *Document) bool {
	for _, s := range doc.Steps {
		if s.Barrier != nil {
			return true
		}
	}
	return false
}

func runTask(parent context.Context, item task, opts runOptions) TaskReport {
	started := time.Now()
	result := TaskReport{Path: item.doc.Path, Name: item.doc.Name, TaskID: fmt.Sprintf("%s-%04d", item.doc.Name, item.index), RepeatIndex: item.index, Status: "failed"}
	timeout, _ := item.doc.taskTimeout()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	vars, err := newVariables(item.doc.Variables)
	if err != nil {
		item.barrier.Abort(err)
		result.Error = safeError(err)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	defer vars.release()
	redactions := vars.redactions(item.doc.Report.Redact)
	connect := connectClients
	if opts.connectClients != nil {
		connect = opts.connectClients
	}
	clients, err := connect(ctx, item.doc.Clients, item.doc.Steps, vars)
	// closeClients stays false while a background goroutine that ignored
	// cancellation still owns a PeerStream on these clients.
	closeClients := true
	if clients != nil {
		result.Clients = clients.fingerprints()
		defer func() {
			if closeClients {
				clients.Close()
			}
		}()
	}
	if err != nil {
		item.barrier.Abort(err)
		result.Error = safeError(err, redactions...)
		result.Cleanup, _ = runFinalizers(parent, item.doc.Path, item.doc.Finally, clients, vars, item.barrier, opts, redactions)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	peerSessions := newPeerStreamSessions()
	background := newBackgroundSteps(item.doc.Steps, opts.backgroundCancelGrace)
	failed := false
	for _, step := range item.doc.Steps {
		var stepResult StepReport
		var err error
		switch {
		case step.Background:
			stepResult, err = background.start(ctx, step, clients, vars, opts)
		case step.Await != "":
			stepResult, err = background.await(ctx, step, vars, redactions)
		default:
			stepResult, err = runStep(ctx, item.doc.Path, step, clients, vars, item.barrier, opts, redactions, peerSessions)
		}
		result.Steps = append(result.Steps, stepResult)
		if err != nil {
			item.barrier.Abort(err)
			result.Error = safeError(err, redactions...)
			failed = true
			break
		}
	}
	if cancelled := background.cancelRemaining(redactions); len(cancelled) != 0 {
		result.Steps = append(result.Steps, cancelled...)
		if result.Error == "" {
			result.Error = cancelled[0].Error
		}
		failed = true
	}
	// A background PeerStream that ignored cancellation still owns the task's
	// shared clients. Session teardown, finalizers, and client close would all
	// race it, so the task ends here and reports the leak instead.
	if running := background.join(); len(running) != 0 {
		closeClients = false
		leak := safeError(fmt.Errorf("background steps %s still hold their PeerStream after cancellation; skipped finally steps and client teardown", strings.Join(running, ", ")), redactions...)
		if result.Error == "" {
			result.Error = leak
		} else {
			result.Error += "; " + leak
		}
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	if err := peerSessions.Close(); err != nil {
		if result.Error == "" {
			result.Error = safeError(err, redactions...)
		}
		failed = true
	}
	var cleanupErr error
	result.Cleanup, cleanupErr = runFinalizers(parent, item.doc.Path, item.doc.Finally, clients, vars, item.barrier, opts, redactions)
	if cleanupErr != nil {
		if result.Error == "" {
			result.Error = safeError(cleanupErr, redactions...)
		}
		failed = true
	}
	if !failed {
		result.Status = "passed"
	}
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

func runFinalizers(parent context.Context, documentPath string, steps []Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string) ([]StepReport, error) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
	defer cleanupCancel()
	results := make([]StepReport, 0, len(steps))
	var firstErr error
	for _, step := range steps {
		stepResult, err := runStep(cleanupCtx, documentPath, step, clients, vars, barrier, opts, redactions)
		stepResult.Stage = "cleanup"
		results = append(results, stepResult)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return results, firstErr
}

type assertionFailure struct{ err error }

func (e *assertionFailure) Error() string { return e.err.Error() }
func (e *assertionFailure) Unwrap() error { return e.err }

func runStep(ctx context.Context, documentPath string, step Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string, sessionSets ...*peerStreamSessions) (StepReport, error) {
	var sessions *peerStreamSessions
	if len(sessionSets) > 0 {
		sessions = sessionSets[0]
	}
	if sessions == nil {
		sessions = newPeerStreamSessions()
		defer func() { _ = sessions.Close() }()
	}
	if step.Retry == nil {
		return runStepOnce(ctx, documentPath, step, clients, vars, barrier, opts, redactions, sessions)
	}
	return runStepWithRetry(ctx, documentPath, step, clients, vars, barrier, opts, redactions, sessions)
}

func runStepWithRetry(ctx context.Context, documentPath string, step Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string, sessions *peerStreamSessions) (StepReport, error) {
	started := time.Now()
	retryOn := step.Retry.On
	if retryOn == nil {
		retryOn = []string{"timeout"}
	}
	var delay time.Duration
	if step.Retry.Delay != "" {
		delay, _ = time.ParseDuration(step.Retry.Delay)
	}
	attempts := make([]AttemptReport, 0, step.Retry.Attempts)
	var report StepReport
	var finalErr error
	for attempt := 1; attempt <= step.Retry.Attempts; attempt++ {
		attemptVars := cloneVariables(vars)
		attemptReport, err := runStepOnce(ctx, documentPath, step, clients, attemptVars, barrier, opts, redactions, sessions)
		if err == nil {
			if commitErr := commitAttemptVariables(vars, attemptVars); commitErr != nil {
				err = commitErr
				attemptReport.Status = "failed"
				attemptReport.Error = safeError(err, redactions...)
			}
		}
		kind := failureKind(err)
		attempts = append(attempts, AttemptReport{
			Attempt: attempt, Status: attemptReport.Status, FailureKind: kind,
			DurationMS: attemptReport.DurationMS, Error: attemptReport.Error,
			Evidence: maps.Clone(attemptReport.Evidence),
		})
		releaseAttemptVariables(vars, attemptVars)
		report, finalErr = attemptReport, err
		if err == nil || attempt == step.Retry.Attempts || !slices.Contains(retryOn, kind) || ctx.Err() != nil {
			break
		}
		if delay > 0 {
			if delayErr := waitRetryDelay(ctx, delay); delayErr != nil {
				finalErr = delayErr
				break
			}
		}
	}
	report.Attempts = attempts
	report.DurationMS = time.Since(started).Milliseconds()
	return report, finalErr
}

func waitRetryDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func cloneVariables(input *variables) *variables {
	return &variables{values: maps.Clone(input.values)}
}

func releaseAttemptVariables(base, attempt *variables) {
	protected := make(map[variableDataIdentity]struct{})
	for _, current := range base.values {
		collectVariableDataIdentities(current.data, protected)
	}
	seen := make(map[variableDataIdentity]struct{})
	for name, original := range base.values {
		candidate := attempt.values[name]
		if original.data != nil || candidate.data == nil {
			continue
		}
		clearDiscardedVariableData(candidate.data, protected, seen)
		candidate.data = nil
		attempt.values[name] = candidate
	}
}

type variableDataIdentity struct {
	typeOf  reflect.Type
	pointer uintptr
}

func collectVariableDataIdentities(input any, identities map[variableDataIdentity]struct{}) {
	switch value := input.(type) {
	case []byte:
		identities[variableDataIdentity{typeOf: reflect.TypeFor[[]byte](), pointer: reflect.ValueOf(value).Pointer()}] = struct{}{}
	case map[string]any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[map[string]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := identities[identity]; ok {
			return
		}
		identities[identity] = struct{}{}
		for _, item := range value {
			collectVariableDataIdentities(item, identities)
		}
	case []any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[[]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := identities[identity]; ok {
			return
		}
		identities[identity] = struct{}{}
		for _, item := range value {
			collectVariableDataIdentities(item, identities)
		}
	}
}

func clearDiscardedVariableData(input any, protected, seen map[variableDataIdentity]struct{}) {
	switch value := input.(type) {
	case []byte:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[[]byte](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := protected[identity]; !ok {
			clear(value)
		}
	case map[string]any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[map[string]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := protected[identity]; ok {
			return
		}
		if _, ok := seen[identity]; ok {
			return
		}
		seen[identity] = struct{}{}
		for key, item := range value {
			clearDiscardedVariableData(item, protected, seen)
			value[key] = nil
		}
	case []any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[[]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := protected[identity]; ok {
			return
		}
		if _, ok := seen[identity]; ok {
			return
		}
		seen[identity] = struct{}{}
		for index, item := range value {
			clearDiscardedVariableData(item, protected, seen)
			value[index] = nil
		}
	}
}

func commitAttemptVariables(dst, src *variables) error {
	pending := make(map[string]any)
	for name, current := range dst.values {
		candidate := src.values[name]
		if current.data != nil || candidate.data == nil {
			continue
		}
		if err := checkValueType(current.spec, candidate.data); err != nil {
			return fmt.Errorf("variable %q: %w", name, err)
		}
		pending[name] = candidate.data
	}
	for name, data := range pending {
		current := dst.values[name]
		current.data = data
		dst.values[name] = current
	}
	return nil
}

func failureKind(err error) string {
	if err == nil {
		return ""
	}
	var assertion *assertionFailure
	if errors.As(err, &assertion) {
		return "assertion"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "operation"
}

func runStepOnce(ctx context.Context, documentPath string, step Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string, sessions *peerStreamSessions) (StepReport, error) {
	started := time.Now()
	op := step.operation()
	report := StepReport{ID: step.ID, Operation: op, Client: step.Client, Status: "failed", Stage: op}
	stepCtx, cancel := context.WithCancel(ctx)
	if step.Timeout != "" {
		duration, err := time.ParseDuration(step.Timeout)
		if err != nil {
			return report, err
		}
		stepCtx, cancel = context.WithTimeout(ctx, duration)
	}
	defer cancel()
	var evidence map[string]any
	var value any
	var saved any
	var err error
	switch op {
	case "rpc":
		client, getErr := clients.get(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		params := step.RPC.Request
		if params == nil {
			params = map[string]any{}
		}
		params, err = vars.resolve(params)
		if err == nil {
			value, err = invokeUnary(stepCtx, client, step, params)
			saved = value
		}
	case "rpc_stream":
		client, getErr := clients.get(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		request, resolveErr := vars.resolve(step.RPCStream.Request)
		if resolveErr != nil {
			err = resolveErr
			break
		}
		streamResult, invokeErr := invokeRPCStream(stepCtx, client, step, request)
		err = invokeErr
		value, saved, evidence = streamResult.assertion, streamResult.saved, streamResult.evidence
	case "speech":
		client, getErr := clients.get(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		request, resolveErr := vars.resolve(step.Speech.Request)
		if resolveErr != nil {
			err = resolveErr
			break
		}
		input, resolveErr := vars.resolve(step.Speech.Input)
		if resolveErr != nil && step.Speech.Input != nil {
			err = resolveErr
			break
		}
		var outputSpec VariableSpec
		if step.SaveAs != "" {
			outputSpec = vars.values[step.SaveAs].spec
		}
		inputSpec, _ := vars.referencedSpec(step.Speech.Input)
		invoke := func() (operationResult, error) {
			return invokeSpeech(stepCtx, client, step, request, input, inputSpec, outputSpec, opts.fullEvidence)
		}
		var speechResult operationResult
		var invokeErr error
		if step.Speech.Cache == "run" {
			key, keyErr := speechFixtureKey(documentPath, step, request, outputSpec)
			if keyErr != nil {
				err = keyErr
				break
			}
			var hit bool
			speechResult, hit, invokeErr = opts.speechCache.Do(stepCtx, key, invoke)
			speechResult.evidence = maps.Clone(speechResult.evidence)
			if speechResult.evidence == nil {
				speechResult.evidence = make(map[string]any)
			}
			if hit {
				speechResult.evidence["cache"] = "hit"
			} else {
				speechResult.evidence["cache"] = "miss"
			}
		} else {
			speechResult, invokeErr = invoke()
		}
		err = invokeErr
		value, saved, evidence = speechResult.assertion, speechResult.saved, speechResult.evidence
	case "peer_stream":
		invocation, prepareErr := preparePeerStream(step, step, clients, vars, opts)
		if prepareErr != nil {
			err = prepareErr
			break
		}
		streamResult, invokeErr := invocation.run(stepCtx, sessions)
		err = invokeErr
		value, saved, evidence = streamResult.assertion, streamResult.saved, streamResult.evidence
	case "await":
		err = fmt.Errorf("await step %q can only run inside a task", step.Await)
	case "workspace_relay":
		input, resolveErr := vars.resolve(step.WorkspaceRelay.Input)
		if resolveErr != nil {
			err = resolveErr
			break
		}
		if spec, ok := vars.referencedSpec(step.WorkspaceRelay.Input); ok && step.WorkspaceRelay.Media == "audio" {
			if spec.Type != "audio" || spec.Codec != "opus" || (spec.MediaType != "audio/ogg" && spec.MediaType != "audio/opus") {
				err = fmt.Errorf("workspace_relay audio input must declare audio/ogg or audio/opus with opus codec")
				break
			}
		}
		audioCaptureMaxBytes, captureErr := relayAudioCaptureMaxBytes(step, vars)
		if captureErr != nil {
			err = captureErr
			break
		}
		var relayOutcome operationResult
		var invokeErr error
		if opts.openRelayStreams == nil {
			relayOutcome, invokeErr = invokeWorkspaceRelay(stepCtx, clients, step, input, audioCaptureMaxBytes, opts.fullEvidence, opts.audioObserver)
		} else {
			firstStream, secondStream, openErr := opts.openRelayStreams()
			if openErr != nil {
				invokeErr = openErr
			} else {
				relayOutcome, invokeErr = runWorkspaceRelayWithEvidence(stepCtx, step.WorkspaceRelay, firstStream, secondStream, input, audioCaptureMaxBytes, opts.fullEvidence, opts.audioObserver)
			}
		}
		err = invokeErr
		value, saved, evidence = relayOutcome.assertion, relayOutcome.saved, relayOutcome.evidence
	case "barrier":
		if barrier == nil {
			err = fmt.Errorf("barrier not initialized")
		} else {
			err = barrier.Wait(stepCtx)
			evidence = map[string]any{"participants": barrier.total}
		}
	case "output":
		evidence, err = emitOutput(opts.out, vars, step.Output.Variable)
	case "review":
		err = runReview(opts.in, opts.out, step.ReviewOp.Message)
	case "http":
		endpoint, getErr := clients.endpoint(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		var httpResult httpStepResult
		httpResult, err = invokeHTTP(stepCtx, endpoint, step, vars)
		value, saved, evidence = httpResult.body, httpResult.body, httpResult.evidence
	case "client_rpc":
		key := step.Client + ":" + step.ClientRPC.Method
		counter := clients.inbound[key]
		if counter == nil {
			err = fmt.Errorf("client RPC %s was not installed", step.ClientRPC.Method)
			break
		}
		expected := int64(step.ClientRPC.ExpectCalls)
		if expected == 0 {
			expected = 1
		}
		calls := counter.Load()
		ticker := time.NewTicker(10 * time.Millisecond)
		for calls < expected && err == nil {
			select {
			case <-ticker.C:
				calls = counter.Load()
			case <-stepCtx.Done():
				err = fmt.Errorf("client RPC %s calls = %d, want at least %d: %w", step.ClientRPC.Method, calls, expected, context.Cause(stepCtx))
			}
		}
		ticker.Stop()
		if err != nil {
			break
		}
		evidence = map[string]any{"method": step.ClientRPC.Method, "calls": calls}
		value, saved = evidence, evidence
	default:
		err = fmt.Errorf("unsupported operation %q", op)
	}
	return completeStepReport(report, step, vars, value, saved, evidence, err, started, redactions)
}

// peerStreamInvocation is a peer_stream step whose variable reads and client
// lookup are complete, so the stream can be driven on the task goroutine or
// from a background step without touching task variables.
type peerStreamInvocation struct {
	client               *gizcli.Client
	open                 peerStreamOpener
	step                 Step
	input                any
	audioCaptureMaxBytes int
	observer             audioObserver
}

// preparePeerStream resolves the step input and the /audio capture bound.
// captureStep owns the capture map: the step itself, or the await step of a
// background peer_stream.
func preparePeerStream(step, captureStep Step, clients *clientSet, vars *variables, opts runOptions) (peerStreamInvocation, error) {
	client, err := clients.get(step.Client)
	if err != nil {
		return peerStreamInvocation{}, err
	}
	var input any
	if step.PeerStream.Input != nil {
		input, err = vars.resolve(step.PeerStream.Input)
		if err != nil {
			return peerStreamInvocation{}, err
		}
	}
	if spec, ok := vars.referencedSpec(step.PeerStream.Input); ok && step.PeerStream.Mode != "text" {
		if spec.Type != "audio" || spec.Codec != "opus" || (spec.MediaType != "audio/ogg" && spec.MediaType != "audio/opus") {
			return peerStreamInvocation{}, fmt.Errorf("peer_stream audio input must declare audio/ogg or audio/opus with opus codec")
		}
	}
	audioCaptureMaxBytes, err := peerStreamAudioCaptureMaxBytes(captureStep, vars)
	if err != nil {
		return peerStreamInvocation{}, err
	}
	open := openClientPeerStream(client)
	if opts.openPeerStream != nil {
		open = opts.openPeerStream(client)
	}
	return peerStreamInvocation{client: client, open: open, step: step, input: input, audioCaptureMaxBytes: audioCaptureMaxBytes, observer: opts.audioObserver}, nil
}

func (p peerStreamInvocation) run(ctx context.Context, sessions *peerStreamSessions) (operationResult, error) {
	return invokePeerStreamWithSessions(ctx, p.client, p.open, sessions, p.step, p.input, p.audioCaptureMaxBytes, p.observer)
}

// completeStepReport applies the step's expect_error, save_as, capture, and
// expect declarations to an operation outcome and fills in the report.
func completeStepReport(report StepReport, step Step, vars *variables, value, saved any, evidence map[string]any, err error, started time.Time, redactions []string) (StepReport, error) {
	if step.ExpectError != nil {
		code, message, matched := structuredRPCError(err)
		if !matched {
			if err == nil {
				err = &assertionFailure{err: fmt.Errorf("expected RPC error code %d, got success", step.ExpectError.Code)}
			}
		} else if code != step.ExpectError.Code {
			err = &assertionFailure{err: fmt.Errorf("expected RPC error code %d, got %d", step.ExpectError.Code, code)}
		} else if step.ExpectError.MessageContains != "" && !strings.Contains(message, step.ExpectError.MessageContains) {
			err = &assertionFailure{err: fmt.Errorf("RPC error message does not contain expected text")}
		} else {
			err = nil
			evidence = map[string]any{"rpc_error_code": code}
		}
	}
	if err == nil && value != nil {
		if step.SaveAs != "" {
			if saved == nil {
				saved = value
			}
			err = vars.assign(step.SaveAs, saved)
		}
		if err == nil {
			err = applyCaptures(vars, step.Capture, value)
		}
		if err == nil {
			expectations, resolveErr := resolveExpectations(vars, step.Expect)
			if resolveErr != nil {
				err = resolveErr
			} else if assertionErr := assertValue(expectations, value); assertionErr != nil {
				err = &assertionFailure{err: assertionErr}
			}
		}
		if evidence == nil {
			evidence = map[string]any{"result": "captured"}
		}
	}
	if err == nil {
		report.Status = "passed"
		report.Evidence = evidence
	} else {
		report.Error = safeError(err, redactions...)
		if len(evidence) != 0 {
			report.Evidence = evidence
		}
	}
	report.DurationMS = time.Since(started).Milliseconds()
	return report, err
}

func peerStreamAudioCaptureMaxBytes(step Step, vars *variables) (int, error) {
	limit := 0
	for name, pointer := range step.Capture {
		if pointer != "/audio" {
			continue
		}
		item, ok := vars.values[name]
		if !ok {
			return 0, fmt.Errorf("capture references unknown variable %q", name)
		}
		if item.spec.Type != "audio" || item.spec.MaxBytes <= 0 {
			return 0, fmt.Errorf("peer_stream /audio capture variable %q must be audio with max_bytes", name)
		}
		if limit == 0 || item.spec.MaxBytes < limit {
			limit = item.spec.MaxBytes
		}
	}
	return limit, nil
}

func relayAudioCaptureMaxBytes(step Step, vars *variables) (int, error) {
	limit := 0
	for name, pointer := range step.Capture {
		if pointer != "/terminal/audio" {
			continue
		}
		item, ok := vars.values[name]
		if !ok {
			return 0, fmt.Errorf("capture references unknown variable %q", name)
		}
		if item.spec.Type != "audio" || item.spec.MaxBytes <= 0 || item.spec.MaxBytes > relayMaxAudioBytes {
			return 0, fmt.Errorf("workspace_relay /terminal/audio capture variable %q must be audio with max_bytes up to %d", name, relayMaxAudioBytes)
		}
		if limit == 0 || item.spec.MaxBytes < limit {
			limit = item.spec.MaxBytes
		}
	}
	return limit, nil
}

func structuredRPCError(err error) (int32, string, bool) {
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

func applyCaptures(vars *variables, captures map[string]string, input any) error {
	for name, pointer := range captures {
		value, ok := jsonPointer(input, pointer)
		if !ok {
			return fmt.Errorf("capture pointer %q not found", pointer)
		}
		if err := vars.assign(name, value); err != nil {
			return err
		}
	}
	return nil
}

// resolveExpectations substitutes ${variable} references in the string
// operands of equals and contains, so a document can assert a captured value
// directly (for example that both sides of a Friend share one workspace_name).
// Other matchers keep their literal operands. Unassigned variables fail the
// step like an unresolved request field would.
func resolveExpectations(vars *variables, assertions map[string]Expectation) (map[string]Expectation, error) {
	if vars == nil || len(assertions) == 0 {
		return assertions, nil
	}
	resolved := make(map[string]Expectation, len(assertions))
	for path, expectation := range assertions {
		if text, ok := expectation.Equals.(string); ok && strings.Contains(text, "${") {
			value, err := vars.resolve(text)
			if err != nil {
				return nil, fmt.Errorf("expect %s equals: %w", path, err)
			}
			expectation.Equals = value
		}
		if strings.Contains(expectation.Contains, "${") {
			value, err := vars.resolve(expectation.Contains)
			if err != nil {
				return nil, fmt.Errorf("expect %s contains: %w", path, err)
			}
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("expect %s contains: variable must be string", path)
			}
			expectation.Contains = text
		}
		resolved[path] = expectation
	}
	return resolved, nil
}

func assertValue(assertions map[string]Expectation, input any) error {
	for path, a := range assertions {
		value, ok := jsonPointer(input, path)
		if a.Present != nil && ok != *a.Present {
			return fmt.Errorf("assert %s presence = %v", path, ok)
		}
		if !ok {
			if a.Present != nil && !*a.Present {
				continue
			}
			return fmt.Errorf("assert path %q not found", path)
		}
		if a.Equals != nil {
			matched := jsonEqual(value, a.Equals)
			if a.Normalize != nil {
				text, ok := stringTarget(value)
				operand, operandOK := a.Equals.(string)
				matched = ok && operandOK && normalizeString(text, a.Normalize) == normalizeString(operand, a.Normalize)
			}
			if !matched {
				return fmt.Errorf("assert %s equals failed", path)
			}
		}
		if a.Count != nil {
			array, ok := value.([]any)
			if !ok || len(array) != *a.Count {
				return fmt.Errorf("assert %s count failed", path)
			}
		}
		if a.NonEmpty != nil {
			empty := value == nil
			switch x := value.(type) {
			case string:
				empty = x == ""
			case []any:
				empty = len(x) == 0
			case map[string]any:
				empty = len(x) == 0
			}
			if empty == *a.NonEmpty {
				return fmt.Errorf("assert %s non_empty failed", path)
			}
		}
		if err := assertStringMatchers(path, a, value); err != nil {
			return err
		}
		if err := assertNumericBounds(path, a, value); err != nil {
			return err
		}
	}
	return nil
}

// assertStringMatchers evaluates the content, pattern, and rune-length
// matchers against a string value or the empty-separator join of a
// text-fragment array. Failure messages never include the asserted content.
func assertStringMatchers(path string, a Expectation, value any) error {
	notContains, err := a.notContainsList()
	if err != nil {
		return fmt.Errorf("assert %s: %w", path, err)
	}
	hasStringMatcher := a.Contains != "" || len(a.ContainsAll) > 0 || len(a.ContainsAny) > 0 ||
		len(notContains) > 0 || a.Pattern != "" || a.MinLength != nil || a.MaxLength != nil
	if !hasStringMatcher {
		return nil
	}
	text, ok := stringTarget(value)
	if !ok {
		return fmt.Errorf("assert %s requires a string or text-fragment array target", path)
	}
	matchText := normalizeString(text, a.Normalize)
	if a.Contains != "" && !strings.Contains(matchText, normalizeString(a.Contains, a.Normalize)) {
		return fmt.Errorf("assert %s contains failed", path)
	}
	for _, needle := range a.ContainsAll {
		if !strings.Contains(matchText, normalizeString(needle, a.Normalize)) {
			return fmt.Errorf("assert %s contains_all failed", path)
		}
	}
	if len(a.ContainsAny) > 0 {
		matched := false
		for _, needle := range a.ContainsAny {
			if strings.Contains(matchText, normalizeString(needle, a.Normalize)) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("assert %s contains_any failed", path)
		}
	}
	for _, needle := range notContains {
		if strings.Contains(matchText, normalizeString(needle, a.Normalize)) {
			return fmt.Errorf("assert %s not_contains failed", path)
		}
	}
	if a.Pattern != "" {
		matcher, err := regexp.Compile(a.Pattern)
		if err != nil {
			return fmt.Errorf("assert %s pattern does not compile", path)
		}
		if !matcher.MatchString(text) {
			return fmt.Errorf("assert %s pattern failed", path)
		}
	}
	if a.MinLength != nil || a.MaxLength != nil {
		runes := utf8.RuneCountInString(text)
		if a.MinLength != nil && runes < *a.MinLength {
			return fmt.Errorf("assert %s min_length failed: %d runes < %d", path, runes, *a.MinLength)
		}
		if a.MaxLength != nil && runes > *a.MaxLength {
			return fmt.Errorf("assert %s max_length failed: %d runes > %d", path, runes, *a.MaxLength)
		}
	}
	return nil
}

func normalizeString(input string, options []string) string {
	if len(options) == 0 {
		return input
	}
	whitespace := slices.Contains(options, "whitespace")
	punctuation := slices.Contains(options, "punctuation")
	digits := slices.Contains(options, "digits")
	caseFold := slices.Contains(options, "case")
	return strings.Map(func(r rune) rune {
		if whitespace && unicode.IsSpace(r) {
			return -1
		}
		if punctuation && unicode.IsPunct(r) {
			return -1
		}
		if digits {
			r = normalizeDigit(r)
		}
		if caseFold {
			r = unicode.ToLower(r)
		}
		return r
	}, input)
}

func normalizeDigit(r rune) rune {
	const asciiDigits = "0123456789"
	for _, digits := range []string{"０１２３４５６７８９", "零一二三四五六七八九"} {
		index := 0
		for _, candidate := range digits {
			if r == candidate {
				return rune(asciiDigits[index])
			}
			index++
		}
	}
	return r
}

func assertNumericBounds(path string, a Expectation, value any) error {
	if a.Minimum == nil && a.Maximum == nil {
		return nil
	}
	number, ok := numericTarget(value)
	if !ok {
		return fmt.Errorf("assert %s requires a numeric target", path)
	}
	if a.Minimum != nil && number < *a.Minimum {
		return fmt.Errorf("assert %s minimum failed: %v < %v", path, number, *a.Minimum)
	}
	if a.Maximum != nil && number > *a.Maximum {
		return fmt.Errorf("assert %s maximum failed: %v > %v", path, number, *a.Maximum)
	}
	return nil
}

func stringTarget(value any) (string, bool) {
	switch x := value.(type) {
	case string:
		return x, true
	case []string:
		return strings.Join(x, ""), true
	case []any:
		var joined strings.Builder
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return "", false
			}
			joined.WriteString(s)
		}
		return joined.String(), true
	}
	return "", false
}

func numericTarget(value any) (float64, bool) {
	number, ok := rawNumericTarget(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func rawNumericTarget(value any) (float64, bool) {
	switch x := value.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case json.Number:
		number, err := x.Float64()
		return number, err == nil
	case string:
		// protojson encodes int64/uint64 fields as decimal strings, so numeric
		// RPC fields such as timestamps and byte counts arrive as strings.
		number, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return number, err == nil
	}
	return 0, false
}
func jsonPointer(input any, pointer string) (any, bool) {
	if pointer == "" {
		return input, true
	}
	current := input
	for part := range strings.SplitSeq(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch value := current.(type) {
		case map[string]any:
			next, ok := value[part]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil, false
			}
			current = value[index]
		default:
			return nil, false
		}
	}
	return current, true
}
func jsonEqual(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}
func safeError(err error, redactions ...string) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	for _, secret := range redactions {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	for _, word := range []string{"token", "credential", "authorization", "private_key"} {
		if strings.Contains(strings.ToLower(text), word) {
			return "redacted execution error"
		}
	}
	if len(text) > 512 {
		text = text[:512]
	}
	return text
}
func loadDocuments(paths []string) ([]*Document, error) {
	docs := make([]*Document, 0, len(paths))
	names := map[string]string{}
	for _, path := range paths {
		doc, err := loadDocument(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if other, ok := names[doc.Name]; ok {
			return nil, fmt.Errorf("duplicate document name %q in %s and %s", doc.Name, other, path)
		}
		names[doc.Name] = path
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, nil
}
