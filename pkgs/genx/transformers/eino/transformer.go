package eino

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolrun"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/streamlog"
	"github.com/cloudwego/eino/schema"
)

const maxSupersededInputRoutes = 64

// Transformer owns one immutable compiled Eino Graph. Transform may be called
// concurrently; every call receives independent invocation-local run state.
type Transformer struct {
	config    *normalizedConfig
	graph     *compiledGraph
	contextID string
	history   *conversationHistory

	initiativeMu      sync.Mutex
	initiativeClaimed bool

	taskContext context.Context
	cancelTasks context.CancelCauseFunc
	taskMu      sync.Mutex
	tasksClosed bool
	tasks       sync.WaitGroup
	closeOnce   sync.Once
}

// New validates Config, resolves components, and compiles the Graph exactly
// once.
func New(ctx context.Context, source Config) (*Transformer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	config, err := normalizeConfig(source)
	if err != nil {
		return nil, err
	}
	graph, err := buildGraph(ctx, config, config.Graph, "Graph")
	if err != nil {
		return nil, err
	}
	contextID := config.Agent.ContextID
	if contextID == "" {
		contextID = genx.NewStreamID()
	}
	transformer := &Transformer{
		config: config, graph: graph, contextID: contextID,
		history: &conversationHistory{
			config: config.History, agentID: config.Agent.ID, contextID: contextID,
		},
	}
	transformer.taskContext, transformer.cancelTasks = context.WithCancelCause(context.Background())
	if transformer.config.Memory != nil {
		transformer.config.Memory.runAsync = transformer.runAsync
	}
	return transformer, nil
}

func (transformer *Transformer) runAsync(task func(context.Context)) {
	if transformer == nil || task == nil {
		return
	}
	transformer.taskMu.Lock()
	if transformer.tasksClosed {
		transformer.taskMu.Unlock()
		return
	}
	transformer.tasks.Add(1)
	transformer.taskMu.Unlock()
	go func() {
		defer transformer.tasks.Done()
		task(transformer.taskContext)
	}()
}

// Close cancels and joins asynchronous Memory work owned by this generation.
func (transformer *Transformer) Close() error {
	if transformer == nil {
		return nil
	}
	transformer.closeOnce.Do(func() {
		transformer.taskMu.Lock()
		transformer.tasksClosed = true
		transformer.cancelTasks(io.EOF)
		transformer.taskMu.Unlock()
		transformer.tasks.Wait()
	})
	return nil
}

// Transform consumes one long-lived GenX Stream. Every completed text input
// route executes a fresh Graph run.
func (transformer *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	ctx = streamlog.StartStage(ctx, "eino")
	if transformer == nil || transformer.graph == nil {
		return nil, fmt.Errorf("eino: Transformer is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("eino: input Stream is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session := newSession(ctx, transformer, input)
	go session.closeInputOnCancellation()
	go session.run()
	return &sessionStream{Output: session.invocation.Output(), session: session}, nil
}

type session struct {
	transformer *Transformer
	input       genx.Stream
	invocation  *streamkit.Invocation

	mu        sync.Mutex
	runs      map[string]*turnRun
	active    *turnRun
	turns     sync.WaitGroup
	done      chan struct{}
	inputOnce sync.Once
}

func newSession(ctx context.Context, transformer *Transformer, input genx.Stream) *session {
	session := &session{
		transformer: transformer, input: input, runs: make(map[string]*turnRun), done: make(chan struct{}),
	}
	session.invocation = streamkit.NewInvocation(ctx, streamkit.OutputConfig{
		InitialCapacity: 64, MaxBytes: int64(transformer.config.Limits.MaxOutputBytes),
		Observe: session.observeOutput,
	})
	return session
}

func (session *session) observeOutput(chunk *genx.MessageChunk) {
	if chunk == nil || chunk.Ctrl == nil {
		return
	}
	session.mu.Lock()
	run := session.runs[chunk.Ctrl.StreamID]
	session.mu.Unlock()
	if run != nil {
		run.observe(chunk)
	}
}

func (session *session) interruptActive() {
	session.mu.Lock()
	run := session.active
	session.active = nil
	session.mu.Unlock()
	if run != nil {
		run.interrupt()
	}
}

func (session *session) closeInput(err error) {
	session.inputOnce.Do(func() {
		if err == nil {
			_ = session.input.Close()
		} else {
			_ = session.input.CloseWithError(err)
		}
	})
}

func (session *session) closeInputOnCancellation() {
	select {
	case <-session.invocation.Context().Done():
		session.closeInput(context.Cause(session.invocation.Context()))
	case <-session.done:
	}
}

func (session *session) run() {
	defer close(session.done)
	defer session.closeInput(nil)
	var text strings.Builder
	var parts []any
	var audio []*genx.Blob
	var pendingBOS []*genx.MessageChunk
	supersededInputIDs := make(map[string]struct{})
	inText := false
	activeInputID := ""
	activeBypassID := ""
	inAudio := false
	activeAudioID := ""
	acceptsAudio := AcceptsAudioInput(session.transformer.config.Config)
	var inputFailure error
	var previous <-chan struct{}
	initiative, err := session.transformer.claimInitiative(session.invocation.Context())
	if err != nil {
		_ = session.invocation.Output().CloseWithError(err)
		return
	}
	if initiative {
		previous = session.startTurn("", "", nil, nil, true)
	}
	for {
		chunk, err := streamlog.ReadInput(session.invocation.Context(), session.input)
		if err != nil {
			if !isStreamEnd(err) {
				inputFailure = err
			}
			break
		}
		if chunk == nil {
			continue
		}
		if acceptsAudio {
			if blob, ok := audioInputBlob(chunk); ok || (inAudio && chunk.Part == nil && messageStreamID(chunk) == activeAudioID) {
				streamID := messageStreamID(chunk)
				if ok && (!inAudio || chunk.IsBeginOfStream() || streamID != activeAudioID) {
					if inText && activeInputID != "" {
						if err := rememberSupersededInputRoute(supersededInputIDs, activeInputID); err != nil {
							inputFailure = err
							break
						}
					}
					session.interruptActive()
					text.Reset()
					parts = nil
					inText = false
					activeInputID = ""
					pendingBOS = slices.DeleteFunc(pendingBOS, func(begin *genx.MessageChunk) bool {
						return messageStreamID(begin) == streamID
					})
					audio = nil
					inAudio = true
					activeAudioID = streamID
				}
				if ok && len(blob.Data) != 0 {
					audio = append(audio, &genx.Blob{MIMEType: blob.MIMEType, Data: append([]byte(nil), blob.Data...)})
				}
				if chunk.IsEndOfStream() {
					audioErr := ""
					if chunk.Ctrl != nil {
						audioErr = chunk.Ctrl.Error
					}
					if audioErr != "" && audioErr != "interrupted" {
						inputFailure = fmt.Errorf("eino: input audio Stream failed: %w", genx.StreamError(chunk.Ctrl))
						break
					}
					if audioErr == "" {
						previous = session.startAudioTurn(activeAudioID, audio, previous)
					}
					audio = nil
					inAudio = false
					activeAudioID = ""
				}
				continue
			}
		}
		if isInterruptedTextInputEnd(chunk) {
			streamID := messageStreamID(chunk)
			if inText && streamID == activeInputID {
				text.Reset()
				parts = nil
				inText = false
				activeInputID = ""
				continue
			}
			if _, ok := supersededInputIDs[streamID]; ok {
				delete(supersededInputIDs, streamID)
				continue
			}
		}
		if chunk.IsBeginOfStream() {
			if chunk.Part == nil {
				if inText && activeInputID != "" {
					if err := rememberSupersededInputRoute(supersededInputIDs, activeInputID); err != nil {
						inputFailure = err
						break
					}
				}
				session.interruptActive()
				text.Reset()
				parts = nil
				inText = false
				activeInputID = ""
				audio = nil
				inAudio = false
				activeAudioID = ""
				pendingBOS = append(pendingBOS[:0], chunk.Clone())
				continue
			}
			if _, ok := chunk.Part.(genx.Text); ok {
				if streamID := messageStreamID(chunk); inText && activeInputID != "" && streamID != activeInputID {
					if err := rememberSupersededInputRoute(supersededInputIDs, activeInputID); err != nil {
						inputFailure = err
						break
					}
				}
				session.interruptActive()
				text.Reset()
				parts = nil
				inText = false
				activeInputID = messageStreamID(chunk)
				audio = nil
				inAudio = false
				activeAudioID = ""
				pendingBOS = nil
			}
		}
		if chunk.IsEndOfStream() && chunk.Part == nil && inText {
			streamID := messageStreamID(chunk)
			if streamID == "" || activeInputID == "" || streamID == activeInputID {
				if terminalErr := genx.StreamError(chunk.Ctrl); terminalErr != nil {
					inputFailure = fmt.Errorf("eino: input text Stream failed: %w", genx.StreamError(chunk.Ctrl))
					break
				}
				previous = session.startTurn(text.String(), activeInputID, parts, previous)
				text.Reset()
				parts = nil
				inText = false
				activeInputID = ""
				continue
			}
		}
		if textPart, ok := chunk.Part.(genx.Text); ok {
			if !inText && activeInputID == "" {
				activeInputID = messageStreamID(chunk)
			}
			if streamID := messageStreamID(chunk); streamID != "" && activeInputID != "" && streamID != activeInputID {
				inputFailure = fmt.Errorf("eino: text chunk StreamID %q does not match active StreamID %q", streamID, activeInputID)
				break
			}
			inText = true
			text.WriteString(string(textPart))
			if chunk.IsEndOfStream() {
				if terminalErr := genx.StreamError(chunk.Ctrl); terminalErr != nil {
					inputFailure = fmt.Errorf("eino: input text Stream failed: %w", genx.StreamError(chunk.Ctrl))
					break
				}
				previous = session.startTurn(text.String(), activeInputID, parts, previous)
				text.Reset()
				parts = nil
				inText = false
				activeInputID = ""
			}
			continue
		}
		if inText && messageStreamID(chunk) == activeInputID {
			switch part := chunk.Part.(type) {
			case *genx.Blob:
				if part != nil {
					parts = append(parts, &genx.Blob{MIMEType: part.MIMEType, Data: append([]byte(nil), part.Data...)})
				}
				if chunk.IsEndOfStream() && genx.StreamError(chunk.Ctrl) != nil {
					inputFailure = fmt.Errorf("eino: input part Stream failed: %w", genx.StreamError(chunk.Ctrl))
					break
				}
				continue
			}
		}
		streamID := messageStreamID(chunk)
		if chunk.IsBeginOfStream() {
			pendingBOS = append(pendingBOS, chunk.Clone())
			continue
		}
		if streamID == "" {
			streamID = activeBypassID
		}
		for index, begin := range pendingBOS {
			if messageStreamID(begin) == "" || messageStreamID(begin) == streamID {
				if err := session.invocation.Output().Push(begin); err != nil {
					inputFailure = err
					break
				}
				pendingBOS = append(pendingBOS[:index], pendingBOS[index+1:]...)
				break
			}
		}
		copyChunk := chunk.Clone()
		if streamID != "" {
			if copyChunk.Ctrl == nil {
				copyChunk.Ctrl = &genx.StreamCtrl{}
			}
			if copyChunk.Ctrl.StreamID == "" {
				copyChunk.Ctrl.StreamID = streamID
			}
			activeBypassID = streamID
		}
		if err := session.invocation.Output().Push(copyChunk); err != nil {
			inputFailure = err
			break
		}
		if copyChunk.IsEndOfStream() && streamID == activeBypassID {
			activeBypassID = ""
		}
	}
	if inputFailure != nil {
		session.interruptActive()
		session.turns.Wait()
		_ = session.invocation.Output().CloseWithError(inputFailure)
		return
	}
	session.turns.Wait()
	_ = session.invocation.Close()
}

func (transformer *Transformer) claimInitiative(ctx context.Context) (bool, error) {
	if transformer == nil || transformer.config.Initiative == InitiativeDisabled {
		return false, nil
	}
	transformer.initiativeMu.Lock()
	defer transformer.initiativeMu.Unlock()
	if transformer.initiativeClaimed {
		return false, nil
	}
	transformer.initiativeClaimed = true
	if transformer.config.Initiative == InitiativeOnReload {
		return true, nil
	}
	messages, err := transformer.history.load(ctx)
	if err != nil {
		transformer.initiativeClaimed = false
		return false, fmt.Errorf("eino: inspect History for initiative: %w", err)
	}
	return len(messages) == 0, nil
}

type outputRoute struct {
	streamID   string
	definition OutputDefinition
	response   *streamkit.Response
}

func (session *session) startTurn(user, inputID string, parts []any, previous <-chan struct{}, initiative ...bool) <-chan struct{} {
	if strings.TrimSpace(user) == "" && len(parts) == 0 && (len(initiative) == 0 || !initiative[0]) {
		return previous
	}
	runCtx, cancel := context.WithCancelCause(session.invocation.Context())
	run := &turnRun{
		session: session, user: user, parts: parts, ctx: runCtx, cancel: cancel,
		routes: make(map[string]outputRoute), streamIDs: make(map[string]struct{}),
		accepting: true, changed: make(chan struct{}, 1), done: make(chan struct{}), previous: previous,
	}
	run.initiative = len(initiative) != 0 && initiative[0]
	return session.launch(run, inputID)
}

// startAudioTurn starts one turn whose user message is the completed audio
// route. The transcribing ChatModel node supplies the user text later.
func (session *session) startAudioTurn(inputID string, audio []*genx.Blob, previous <-chan struct{}) <-chan struct{} {
	if len(audio) == 0 {
		return previous
	}
	if inputID == "" {
		inputID = genx.NewStreamID()
	}
	// Record the user audio before any reply so History orders the user entry
	// first even when the transcript arrives after the reply text.
	for _, chunk := range historyUserAudioChunks(inputID, audio) {
		if err := session.invocation.Output().Push(chunk); err != nil {
			_ = session.invocation.Fail(err)
			return previous
		}
	}
	runCtx, cancel := context.WithCancelCause(session.invocation.Context())
	run := &turnRun{
		session: session, audio: audio, audioInputID: inputID, ctx: runCtx, cancel: cancel,
		routes: make(map[string]outputRoute), streamIDs: make(map[string]struct{}),
		accepting: true, changed: make(chan struct{}, 1), done: make(chan struct{}), previous: previous,
	}
	return session.launch(run, inputID)
}

func (session *session) launch(run *turnRun, inputID string) <-chan struct{} {
	cancel := run.cancel
	run.automaticPrimary = session.transformer.graph.definition.Compile.PrimaryOutputMode == PrimaryFirstOutput
	for _, output := range session.transformer.graph.definition.Outputs {
		outputID := genx.NewStreamID()
		streamlog.OutputRecorder(session.invocation.Context()).LinkOutput(inputID, outputID)
		route := outputRoute{definition: output, streamID: outputID}
		if !run.automaticPrimary {
			response, err := session.invocation.StartResponse(streamkit.ResponseConfig{
				StreamID: outputID, Role: genx.RoleModel, Name: output.Name, Label: output.Name,
			}, output.MIMEType)
			if err != nil {
				_ = session.invocation.Fail(err)
				cancel(err)
				close(run.done)
				return run.done
			}
			route.response = response
		}
		run.routes[output.Name] = route
		run.streamIDs[outputID] = struct{}{}
		if output.Primary {
			run.primary = route
		}
	}
	run.anchorID = run.primary.streamID
	session.mu.Lock()
	for streamID := range run.streamIDs {
		session.runs[streamID] = run
	}
	session.active = run
	session.mu.Unlock()
	session.turns.Add(1)
	go run.execute()
	return run.done
}

func newOutputRouteBegin(streamID, mimeType string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Part: newOutputRoutePart(mimeType, nil),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true},
	}
}

func newOutputRoutePart(mimeType string, data []byte) genx.Part {
	probe := &genx.MessageChunk{Part: &genx.Blob{MIMEType: mimeType}}
	if canonical, ok := probe.MIMEType(); ok && canonical == "text/plain" {
		return genx.Text(data)
	}
	return &genx.Blob{MIMEType: mimeType, Data: data}
}

type turnRun struct {
	session *session
	user    string
	parts   []any
	// audio is the user audio of an audio turn; user stays empty until the
	// input callback or transcribing ChatModel publishes the transcript.
	audio        []*genx.Blob
	audioInputID string
	ctx          context.Context
	cancel       context.CancelCauseFunc
	previous     <-chan struct{}
	done         chan struct{}

	routes           map[string]outputRoute
	primary          outputRoute
	anchorID         string
	automaticPrimary bool
	primaryChosen    bool
	streamIDs        map[string]struct{}

	mu               sync.Mutex
	accepting        bool
	interrupted      bool
	terminal         bool
	initiative       bool
	transcribed      bool
	emittedPrimary   int
	deliveredBytes   int
	delivered        strings.Builder
	changed          chan struct{}
	interruptionDone chan struct{}
}

func (run *turnRun) Emit(output OutputDefinition, value any) error {
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Name: output.Name}
	var size int
	switch typed := value.(type) {
	case string:
		chunk.Part = newOutputRoutePart(output.MIMEType, []byte(typed))
		size = len(typed)
	case []byte:
		chunk.Part = &genx.Blob{MIMEType: output.MIMEType, Data: append([]byte(nil), typed...)}
		size = len(typed)
	default:
		return fmt.Errorf("eino: output %q has unsupported value %T", output.Name, value)
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if !run.accepting {
		return streamkit.ErrInactiveResponse
	}
	route, ok := run.routes[output.Name]
	if !ok {
		return fmt.Errorf("eino: output route %q is not active", output.Name)
	}
	if route.response == nil {
		var err error
		route, err = run.startAutomaticRoute(route)
		if err != nil {
			return err
		}
	}
	if err := run.session.invocation.Emit(route.response, chunk); err != nil {
		return err
	}
	if run.automaticPrimary && !run.primaryChosen {
		run.primary = route
		run.primaryChosen = true
	}
	if output.Primary || run.automaticPrimary {
		run.emittedPrimary += size
	}

	return nil
}

// startAutomaticRoute starts only a route that actually publishes. The caller
// owns run.mu before calling this method.
func (run *turnRun) startAutomaticRoute(route outputRoute) (outputRoute, error) {
	response, err := run.session.invocation.StartResponse(streamkit.ResponseConfig{
		StreamID: route.streamID, Role: genx.RoleModel, Name: route.definition.Name, Label: "assistant",
	}, route.definition.MIMEType)
	if err != nil {
		return route, err
	}
	route.response = response
	run.routes[route.definition.Name] = route
	if err := run.session.invocation.Emit(response, newOutputRouteBegin(route.streamID, route.definition.MIMEType)); err != nil {
		return route, err
	}
	return route, nil
}

func (run *turnRun) audioTurn() bool {
	return len(run.audio) != 0
}

func (run *turnRun) userText() string {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.user
}

// PublishTranscript records the transcript of an audio turn as its user text
// and publishes it as a transcript route under the audio input StreamID, the
// shape an ASR stage produces for the recorder and client. It may arrive
// before, during, or after the reply text; a turn without one keeps an empty
// user text.
func (run *turnRun) PublishTranscript(text string) error {
	if !run.audioTurn() {
		return fmt.Errorf("eino: transcript published for a non-audio turn")
	}
	if err := run.claimTranscript(text); err != nil {
		return err
	}
	// Publication runs outside run.mu so interruption never waits on output.
	// The transcript is the user's own utterance and stays valid when the
	// reply is interrupted afterwards.
	invocation := run.session.invocation
	if strings.TrimSpace(text) == "" {
		return nil
	}
	response, err := invocation.StartResponse(streamkit.ResponseConfig{
		StreamID: run.audioInputID, Role: genx.RoleUser, Name: "transcript", Label: "transcript",
		ResponseEpoch: genx.NewResponseEpoch(run.audioInputID),
	})
	if err != nil {
		return fmt.Errorf("eino: start transcript route: %w", err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{
		Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: run.audioInputID, Label: "transcript", BeginOfStream: true},
	}); err != nil {
		return fmt.Errorf("eino: emit transcript: %w", err)
	}
	if err := invocation.FinishResponse(response, ""); err != nil {
		return fmt.Errorf("eino: finish transcript route: %w", err)
	}
	return nil
}

// claimTranscript records the transcript as the turn's user text once, while
// the turn still accepts output.
func (run *turnRun) claimTranscript(text string) error {
	run.mu.Lock()
	defer run.mu.Unlock()
	if !run.accepting {
		return streamkit.ErrInactiveResponse
	}
	if run.transcribed {
		return fmt.Errorf("eino: audio turn transcript was already published")
	}
	run.transcribed = true
	run.user = text
	return nil
}

// historyUserAudioChunks replays the turn audio as the History-only user
// audio sideband that shares the transcript StreamID.
func historyUserAudioChunks(streamID string, audio []*genx.Blob) []*genx.MessageChunk {
	if len(audio) == 0 {
		return nil
	}
	mimeType := audio[0].MIMEType
	sideband := func(part *genx.Blob, begin, end bool) *genx.MessageChunk {
		return &genx.MessageChunk{
			Role: genx.RoleUser, Name: "transcript", Part: part,
			Ctrl: &genx.StreamCtrl{
				StreamID: streamID, Label: genx.HistoryUserAudioLabel, BeginOfStream: begin, EndOfStream: end,
			},
		}
	}
	chunks := make([]*genx.MessageChunk, 0, len(audio)+2)
	chunks = append(chunks, sideband(&genx.Blob{MIMEType: mimeType}, true, false))
	for _, blob := range audio {
		chunks = append(chunks, sideband(&genx.Blob{MIMEType: blob.MIMEType, Data: append([]byte(nil), blob.Data...)}, false, false))
	}
	return append(chunks, sideband(&genx.Blob{MIMEType: mimeType}, false, true))
}

func (run *turnRun) observe(chunk *genx.MessageChunk) {
	if chunk == nil || chunk.IsEndOfStream() || chunk.Ctrl == nil {
		return
	}
	run.mu.Lock()
	_, known := run.streamIDs[chunk.Ctrl.StreamID]
	if !known || (!run.automaticPrimary && chunk.Ctrl.StreamID != run.primary.streamID) {
		run.mu.Unlock()
		return
	}
	switch part := chunk.Part.(type) {
	case genx.Text:
		run.delivered.WriteString(string(part))
		run.deliveredBytes += len(part)
	case *genx.Blob:
		if part != nil {
			if mimeType, ok := chunk.MIMEType(); ok && strings.HasPrefix(mimeType, "text/") {
				run.delivered.Write(part.Data)
			}
			run.deliveredBytes += len(part.Data)
		}
	}
	run.mu.Unlock()
	run.signal()
}

func (run *turnRun) signal() {
	select {
	case run.changed <- struct{}{}:
	default:
	}
}

func (run *turnRun) interrupt() {
	run.mu.Lock()
	if run.interrupted || run.terminal {
		run.mu.Unlock()
		return
	}
	run.interrupted = true
	run.accepting = false
	run.interruptionDone = make(chan struct{})
	cleanupDone := run.interruptionDone
	routes := make([]outputRoute, 0, len(run.routes))
	for _, route := range run.routes {
		routes = append(routes, route)
	}
	run.mu.Unlock()
	defer close(cleanupDone)
	run.cancel(errors.New("interrupted"))
	for _, route := range routes {
		streamID := route.streamID
		run.session.invocation.Output().Discard(func(chunk *genx.MessageChunk) bool {
			return chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID == streamID
		})
	}
	run.session.invocation.Output().WaitForObservers()
	run.signal()
}

func (run *turnRun) execute() {
	defer run.session.turns.Done()
	defer close(run.done)
	if run.previous != nil {
		<-run.previous
	}
	var state *runState
	var version string
	runErr := run.beginRoutes()
	if runErr == nil && run.ctx.Err() == nil {
		state, version, runErr = run.runGraph()
	} else if runErr == nil {
		runErr = context.Cause(run.ctx)
	}
	run.mu.Lock()
	run.accepting = false
	run.mu.Unlock()
	run.waitUntilDelivered()
	run.mu.Lock()
	cleanupDone := run.interruptionDone
	run.terminal = true
	run.mu.Unlock()
	if cleanupDone != nil {
		<-cleanupDone
	}
	run.mu.Lock()
	delivered := run.delivered.String()
	interrupted := run.interrupted
	run.mu.Unlock()
	if state != nil {
		finalizeErr := run.finalize(run.session.invocation.Context(), state, version, delivered, interrupted || runErr != nil)
		if runErr == nil {
			runErr = finalizeErr
		}
	}
	run.finishRoutes(runErr, interrupted)
	if run.initiative && (interrupted || runErr != nil) {
		run.session.transformer.initiativeMu.Lock()
		run.session.transformer.initiativeClaimed = false
		run.session.transformer.initiativeMu.Unlock()
	}
	run.session.mu.Lock()
	for streamID := range run.streamIDs {
		delete(run.session.runs, streamID)
	}
	if run.session.active == run {
		run.session.active = nil
	}
	run.session.mu.Unlock()
	run.cancel(io.EOF)
}

func (run *turnRun) beginRoutes() error {
	if run.automaticPrimary {
		return nil
	}
	for _, output := range run.session.transformer.graph.definition.Outputs {
		route, ok := run.routes[output.Name]
		if !ok {
			return fmt.Errorf("eino: output route %q is not registered", output.Name)
		}
		if err := run.session.invocation.Emit(
			route.response,
			newOutputRouteBegin(route.response.StreamID(), output.MIMEType),
		); err != nil {
			return fmt.Errorf("eino: begin output route %q: %w", output.Name, err)
		}
	}
	return nil
}

func (run *turnRun) runGraph() (*runState, string, error) {
	config := run.session.transformer.config
	if run.audioTurn() && config.TranscribeInput != nil {
		text, err := config.TranscribeInput(run.ctx, run.audio)
		if err != nil {
			return nil, "", fmt.Errorf("eino: transcribe input: %w", err)
		}
		if err := run.PublishTranscript(text); err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(text) == "" {
			return nil, "", nil
		}
	}
	if len(run.parts) > 0 && !graphUsesBinding(config.Graph, "input.parts") {
		return nil, "", fmt.Errorf("eino: multimodal input is unsupported by this Graph")
	}
	initial, version, err := loadPersistentState(run.ctx, config.State, config.fields)
	if err != nil {
		return nil, "", err
	}
	history, err := run.session.transformer.history.load(run.ctx)
	if err != nil {
		return nil, "", err
	}
	messages := cloneMessages(history)
	switch {
	case run.audioTurn() && config.TranscribeInput == nil:
		messages = append(messages, schemaAudioUserMessage(run.audio))
	case !run.initiative:
		messages = append(messages, schemaUserMessage(run.user, run.parts))
	}
	state, err := newRunState(config.fields, graphInput{
		ObservationID: run.anchorID,
		Text:          run.user,
		Messages:      messages,
		Parts:         run.parts,
		History:       history,
		SafetyFence:   config.SafetyFence,
	}, initial, run)
	if err != nil {
		return nil, "", err
	}
	if err := recallMemory(run.ctx, config.Memory, state); err != nil {
		return nil, "", err
	}
	runContext := toolrun.WithContext(
		run.ctx,
		toolrun.New(config.ToolInvoker, config.MaxToolCalls),
	)
	if err := run.session.transformer.graph.execute(runContext, state); err != nil {
		return state, version, err
	}
	if run.automaticPrimary && !run.primaryChosen {
		return state, version, errors.New("eino: Graph did not publish any declared output")
	}
	if _, err := state.value(run.primary.definition.Field); err != nil {
		return state, version, fmt.Errorf("eino: primary output was not produced: %w", err)
	}
	return state, version, nil
}

func schemaUserMessage(text string, parts []any) *schema.Message {
	// Eino's provider-neutral Message can preserve text now. Blob parts remain
	// available separately through input.parts until a component-specific
	// multimodal adapter consumes them.
	return schema.UserMessage(text)
}

// schemaAudioUserMessage carries one audio turn to the transcribing ChatModel
// node. Each input Blob stays a separate part because packet formats such as
// raw Opus cannot be concatenated.
func schemaAudioUserMessage(audio []*genx.Blob) *schema.Message {
	parts := make([]schema.MessageInputPart, 0, len(audio))
	for _, blob := range audio {
		data := base64.StdEncoding.EncodeToString(blob.Data)
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeAudioURL,
			Audio: &schema.MessageInputAudio{MessagePartCommon: schema.MessagePartCommon{
				Base64Data: &data, MIMEType: blob.MIMEType,
			}},
		})
	}
	return &schema.Message{Role: schema.User, UserInputMultiContent: parts}
}

func graphUsesBinding(graph GraphDefinition, source string) bool {
	for _, node := range graph.Nodes {
		for _, binding := range node.Inputs {
			if binding.From == source {
				return true
			}
		}
		if node.Retriever != nil && node.Retriever.Query.From == source {
			return true
		}
		if node.Batch != nil && node.Batch.Items.From == source {
			return true
		}
		if node.Subgraph != nil && graphUsesBinding(node.Subgraph.Graph, source) {
			return true
		}
		if node.Batch != nil && graphUsesBinding(node.Batch.Graph, source) {
			return true
		}
		if node.Race != nil {
			for _, branch := range node.Race.Branches {
				if graphUsesBinding(branch.Graph, source) {
					return true
				}
			}
		}
	}
	return false
}

func (run *turnRun) waitUntilDelivered() {
	for {
		run.mu.Lock()
		done := run.interrupted || run.deliveredBytes >= run.emittedPrimary
		run.mu.Unlock()
		if done {
			return
		}
		select {
		case <-run.changed:
		case <-run.session.invocation.Context().Done():
			return
		}
	}
}

func (run *turnRun) finalize(ctx context.Context, state *runState, version, delivered string, failed bool) error {
	user := run.userText()
	if err := run.session.transformer.history.append(ctx, historyMessages(user, delivered), failed); err != nil {
		return err
	}
	if err := observeMemory(ctx, run.session.transformer.config.Memory, state, run.anchorID, user, delivered, failed); err != nil {
		return err
	}
	if failed {
		return nil
	}
	return commitPersistentState(ctx, run.session.transformer.config.State, state, version)
}

func (run *turnRun) finishRoutes(cause error, interrupted bool) {
	run.mu.Lock()
	if run.automaticPrimary && !run.primaryChosen {
		route, err := run.startAutomaticRoute(run.primary)
		if err != nil {
			run.mu.Unlock()
			_ = run.session.invocation.Fail(err)
			return
		}
		run.primary = route
	}
	names := make([]string, 0, len(run.routes))
	for name := range run.routes {
		if name != run.primary.definition.Name {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append(names, run.primary.definition.Name)
	routes := make([]outputRoute, 0, len(names))
	for _, name := range names {
		routes = append(routes, run.routes[name])
	}
	run.mu.Unlock()
	for _, route := range routes {
		if route.response == nil {
			continue
		}
		if interrupted {
			_ = run.session.invocation.Interrupt(route.response, "interrupted")
		} else {
			_ = run.session.invocation.FinishResponseError(route.response, cause)
		}
	}
}

type sessionStream struct {
	*streamkit.Output
	session *session
	once    sync.Once
}

func (stream *sessionStream) Close() error {
	if stream == nil {
		return nil
	}
	stream.once.Do(func() {
		stream.session.closeInput(nil)
		_ = stream.session.invocation.Cancel(io.EOF)
	})
	return nil
}

func (stream *sessionStream) CloseWithError(err error) error {
	if stream == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	stream.once.Do(func() {
		stream.session.closeInput(err)
		_ = stream.session.invocation.Cancel(err)
	})
	return nil
}

func isStreamEnd(err error) bool {
	if err == nil || errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) || errors.Is(err, buffer.ErrIteratorDone) {
		return true
	}
	var state *genx.State
	return errors.As(err, &state) && state.Status() == genx.StatusDone
}

func isInterruptedTextInputEnd(chunk *genx.MessageChunk) bool {
	if chunk == nil || chunk.Ctrl == nil || !chunk.IsEndOfStream() || chunk.Ctrl.Error != "interrupted" || messageStreamID(chunk) == "" {
		return false
	}
	_, ok := chunk.Part.(genx.Text)
	return ok
}

func rememberSupersededInputRoute(routes map[string]struct{}, streamID string) error {
	if streamID == "" {
		return nil
	}
	if _, exists := routes[streamID]; exists {
		return nil
	}
	if len(routes) >= maxSupersededInputRoutes {
		return fmt.Errorf("eino: more than %d superseded input routes remain without terminals", maxSupersededInputRoutes)
	}
	routes[streamID] = struct{}{}
	return nil
}

// audioInputBlob reports an ordinary user audio chunk. The History-only user
// audio sideband is not input.
func audioInputBlob(chunk *genx.MessageChunk) (*genx.Blob, bool) {
	if chunk == nil || chunk.Role == genx.RoleModel {
		return nil, false
	}
	if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Label) == genx.HistoryUserAudioLabel {
		return nil, false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil {
		return nil, false
	}
	mimeType, ok := chunk.MIMEType()
	return blob, ok && strings.HasPrefix(mimeType, "audio/")
}

func messageStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}

var _ genx.Transformer = (*Transformer)(nil)
var _ genx.Stream = (*sessionStream)(nil)
var _ outputEmitter = (*turnRun)(nil)
