package audiodock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

const initialOutputCapacity = 64

var _ genx.Transformer = (*Dock)(nil)

// Dock is a concurrency-safe ASR -> text Agent -> TTS composition. Every
// Transform call owns independent streams, cancellation, routes, and buffers.
type Dock struct {
	config Config
}

// New validates Config without opening provider sessions.
func New(config Config) (*Dock, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	return &Dock{config: normalized}, nil
}

// Transform starts one independent Audio Dock invocation.
func (d *Dock) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if d == nil || d.config.Agent == nil {
		return nil, fmt.Errorf("audiodock: Dock is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("audiodock: input Stream is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	invocation := streamkit.NewInvocation(ctx, streamkit.OutputConfig{InitialCapacity: initialOutputCapacity})
	router, err := newInputRouter(invocation.Context(), input, d.config.ASR)
	if err != nil {
		_ = invocation.Cancel(err)
		return nil, err
	}

	agentOutput, err := d.config.Agent.Transform(invocation.Context(), router.AgentInput())
	if err != nil {
		router.CloseWithError(err)
		_ = invocation.Cancel(err)
		return nil, fmt.Errorf("audiodock: start Agent: %w", err)
	}
	responseStream, err := streamkit.NewResponseStream(agentOutput)
	if err != nil {
		closePipelineStream(agentOutput, err)
		router.CloseWithError(err)
		_ = invocation.Cancel(err)
		return nil, err
	}
	responseStream.DeferOutputObservation()

	run := &dockRun{
		dock:             d,
		invocation:       invocation,
		source:           responseStream,
		router:           router,
		routes:           make(map[string]*dockRoute),
		discardSourceIDs: make(map[string]bool),
	}
	for _, event := range router.ActivateEvents() {
		if event.begin {
			run.beginInputTurn(event.streamID)
		} else {
			run.endInputTurn(event.streamID)
		}
	}
	go run.execute()
	return invocation.Output(), nil
}

type dockRun struct {
	dock       *Dock
	invocation *streamkit.Invocation
	source     *streamkit.ResponseStream
	router     *inputRouter

	// routes is keyed by the Agent's source StreamID, which can be empty. The
	// response itself may synthesize a non-empty StreamID for downstream use;
	// using that generated ID here would create a new route for every later
	// chunk from an unnamed source stream.
	stateMu          sync.Mutex
	routes           map[string]*dockRoute
	discardSourceIDs map[string]bool
	tts              sync.WaitGroup
}

type inputEvent struct {
	begin           bool
	streamID        string
	acknowledgement chan struct{}
}

type dockRoute struct {
	response *streamkit.Response
	mu       sync.Mutex

	deferredEOS *genx.MessageChunk
	ttsRoutes   map[string]bool
	ttsPipes    map[string]*ttsPipe
	ttsDone     sync.WaitGroup
	finish      sync.Once
	closed      atomic.Bool
}

type ttsPipe struct {
	name   string
	input  *streamkit.Output
	output genx.Stream
	cancel context.CancelFunc
}

func (r *dockRun) execute() {
	ctx := r.invocation.Context()
	beginWatchDone := make(chan struct{})
	go func() {
		defer close(beginWatchDone)
		for {
			select {
			case event := <-r.router.InputEvents():
				if event.begin {
					r.beginInputTurn(event.streamID)
				} else {
					r.endInputTurn(event.streamID)
				}
				close(event.acknowledgement)
			case <-ctx.Done():
				return
			}
		}
	}()
	transcriptDone := make(chan error, 1)
	if transcriptOutput := r.router.TranscriptOutput(); transcriptOutput != nil {
		go func() {
			transcriptDone <- r.forwardTranscripts(transcriptOutput)
		}()
	} else {
		transcriptDone <- nil
	}
	stopSource := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = r.source.CloseWithError(ctx.Err())
		case <-stopSource:
		}
	}()
	defer close(stopSource)
	defer func() {
		r.closeSources(nil)
		<-beginWatchDone
	}()

	for {
		chunk, err := r.source.Next()
		if err != nil {
			if streamDone(err) {
				if err := <-transcriptDone; err != nil {
					r.closeRoutes(err)
					_ = r.invocation.Fail(err)
					return
				}
				r.finishOpenRoutes()
				r.tts.Wait()
				for _, route := range r.routeSnapshot() {
					r.finishRoute(route, "")
				}
				_ = r.invocation.Close()
				return
			}
			r.closeRoutes(err)
			r.router.CloseWithError(err)
			<-transcriptDone
			_ = r.invocation.Fail(err)
			return
		}
		if chunk == nil {
			continue
		}
		if chunk.Role != genx.RoleModel {
			if err := r.invocation.Output().PushObserved(chunk, r.source.ObserveOutput); err != nil {
				r.closeRoutes(err)
				return
			}
			continue
		}
		if err := r.forwardModelChunk(ctx, chunk); err != nil {
			r.closeRoutes(err)
			_ = r.invocation.Fail(err)
			return
		}
	}
}

func (r *dockRun) forwardTranscripts(output genx.Stream) error {
	defer output.Close()
	routes := make(map[string]*streamkit.Response)
	finishRoutes := func(errorText string) {
		for key, response := range routes {
			_ = r.invocation.FinishResponse(response, errorText)
			delete(routes, key)
		}
	}
	for {
		chunk, err := output.Next()
		if err != nil {
			if streamDone(err) {
				finishRoutes("")
				return nil
			}
			finishRoutes(err.Error())
			return fmt.Errorf("audiodock: read visible ASR output: %w", err)
		}
		if chunk == nil {
			continue
		}
		streamID := ""
		label := "transcript"
		if chunk.Ctrl != nil {
			streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
			if configured := strings.TrimSpace(chunk.Ctrl.Label); configured != "" {
				label = configured
			}
		}
		response := routes[streamID]
		if response == nil {
			response, err = r.invocation.StartResponse(streamkit.ResponseConfig{
				StreamID: streamID,
				Role:     genx.RoleUser,
				Name:     chunk.Name,
				Label:    label,
			})
			if err != nil {
				return fmt.Errorf("audiodock: start transcript response: %w", err)
			}
			routes[streamID] = response
		}
		if err := r.invocation.Emit(response, chunk); err != nil {
			return fmt.Errorf("audiodock: emit transcript: %w", err)
		}
		if chunk.IsEndOfStream() {
			if err := r.invocation.FinishResponse(response, ""); err != nil {
				return fmt.Errorf("audiodock: finish transcript: %w", err)
			}
			delete(routes, streamID)
		}
	}
}

func (r *dockRun) forwardModelChunk(ctx context.Context, chunk *genx.MessageChunk) error {
	if r.discardSourceChunk(chunk) {
		r.source.AbandonOutputObservation(chunk)
		return nil
	}
	route, err := r.route(chunk)
	if err != nil {
		return err
	}
	if route.closed.Load() {
		r.source.AbandonOutputObservation(chunk)
		return nil
	}
	deferTextEOS := chunk.IsEndOfStream() && route.hasTTSPipes()
	if deferTextEOS {
		route.mu.Lock()
		route.deferredEOS = chunk
		route.mu.Unlock()
	} else {
		if err := r.invocation.EmitTracked(route.response, chunk, func(*genx.MessageChunk) {
			r.source.ObserveOutput(chunk)
		}, func(*genx.MessageChunk) {
			r.source.AbandonOutputObservation(chunk)
		}); err != nil {
			if route.closed.Load() && errors.Is(err, streamkit.ErrInactiveResponse) {
				r.source.AbandonOutputObservation(chunk)
				return nil
			}
			return err
		}
	}

	text, textChunk := chunk.Part.(genx.Text)
	if textChunk && strings.TrimSpace(string(text)) != "" && r.dock.config.TTS != nil {
		pipe, err := r.ttsPipe(ctx, route, chunk)
		if err != nil {
			r.finishRoute(route, err.Error())
			return nil
		}
		if pipe != nil {
			if err := pipe.input.Push(chunk); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				r.finishRoute(route, err.Error())
				return nil
			}
		}
	}
	if !chunk.IsEndOfStream() {
		return nil
	}
	if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
		r.abortTTS(route, errors.New(chunk.Ctrl.Error))
		r.finishRoute(route, chunk.Ctrl.Error)
		return nil
	}
	if route.hasTTSPipes() {
		r.endTTS(route, chunk)
		return nil
	}
	r.finishRoute(route, "")
	return nil
}

func (r *dockRun) route(chunk *genx.MessageChunk) (*dockRoute, error) {
	streamID := ""
	label := ""
	if chunk.Ctrl != nil {
		streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
		label = strings.TrimSpace(chunk.Ctrl.Label)
	}
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if route := r.routes[streamID]; route != nil {
		return route, nil
	}
	response, err := r.invocation.StartResponse(streamkit.ResponseConfig{
		StreamID: streamID,
		Role:     chunk.Role,
		Name:     chunk.Name,
		Label:    label,
	})
	if err != nil {
		return nil, err
	}
	route := &dockRoute{response: response, ttsPipes: make(map[string]*ttsPipe)}
	r.routes[streamID] = route
	return route, nil
}

func (r *dockRun) ttsPipe(ctx context.Context, route *dockRoute, chunk *genx.MessageChunk) (*ttsPipe, error) {
	key := strings.TrimSpace(chunk.Name)
	route.mu.Lock()
	pipe, resolved := route.ttsPipes[key]
	route.mu.Unlock()
	if resolved {
		return pipe, nil
	}
	pattern, err := resolveVoice(ctx, r.dock.config.ResolveVoice, chunk)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		route.mu.Lock()
		route.ttsPipes[key] = nil
		route.mu.Unlock()
		return nil, nil
	}
	return r.startTTS(route, key, chunk.Name, pattern)
}

func (r *dockRun) startTTS(route *dockRoute, key, name, pattern string) (*ttsPipe, error) {
	ctx, cancel := context.WithCancel(r.invocation.Context())
	input := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: initialOutputCapacity})
	output, err := r.dock.config.TTS.Transform(ctx, pattern, input)
	if err != nil {
		cancel()
		_ = input.CloseWithError(err)
		return nil, fmt.Errorf("audiodock: start TTS pattern=%q: %w", pattern, err)
	}
	pipe := &ttsPipe{name: name, input: input, output: output, cancel: cancel}
	route.mu.Lock()
	if existing, ok := route.ttsPipes[key]; ok {
		route.mu.Unlock()
		cancel()
		_ = errors.Join(input.Close(), output.Close())
		return existing, nil
	}
	route.ttsPipes[key] = pipe
	if route.ttsRoutes == nil {
		route.ttsRoutes = make(map[string]bool)
	}
	route.ttsDone.Add(1)
	route.mu.Unlock()
	r.tts.Add(1)
	go r.forwardTTS(route, pipe)
	return pipe, nil
}

func (r *dockRun) forwardTTS(route *dockRoute, pipe *ttsPipe) {
	defer r.tts.Done()
	defer route.ttsDone.Done()
	defer pipe.cancel()
	defer pipe.output.Close()
	for {
		chunk, err := pipe.output.Next()
		if err != nil {
			if !streamDone(err) {
				r.finishRoute(route, err.Error())
			}
			return
		}
		if chunk == nil {
			continue
		}
		emitted := chunk.Clone()
		if emitted.Ctrl == nil {
			emitted.Ctrl = &genx.StreamCtrl{}
		}
		emitted.Ctrl.StreamID = route.response.StreamID()
		route.mu.Lock()
		if mimeType, ok := emitted.MIMEType(); ok {
			route.ttsRoutes[mimeType] = false
		}
		route.mu.Unlock()
		// Provider-level audio EOS is combined into one MIME-route EOS after all
		// publisher-node TTS pipes complete.
		if emitted.IsEndOfStream() {
			if emitted.Ctrl.Error != "" {
				err := errors.New(emitted.Ctrl.Error)
				r.abortTTS(route, err)
				r.finishRoute(route, emitted.Ctrl.Error)
			}
			continue
		}
		if err := r.invocation.Emit(route.response, emitted); err != nil {
			return
		}
	}
}

func (r *dockRun) endTTS(route *dockRoute, sourceEOS *genx.MessageChunk) {
	if route == nil {
		return
	}
	route.mu.Lock()
	pipes := make([]*ttsPipe, 0, len(route.ttsPipes))
	for _, pipe := range route.ttsPipes {
		if pipe != nil {
			pipes = append(pipes, pipe)
		}
	}
	route.mu.Unlock()
	for _, pipe := range pipes {
		end := &genx.MessageChunk{Role: sourceEOS.Role, Name: pipe.name, Part: genx.Text(""), Ctrl: cloneCtrl(sourceEOS.Ctrl)}
		_ = pipe.input.Push(end)
		_ = pipe.input.Close()
	}
	go func() {
		route.ttsDone.Wait()
		r.finishRoute(route, "")
	}()
}

func (r *dockRun) finishRoute(route *dockRoute, errorText string) {
	if route == nil {
		return
	}
	route.finish.Do(func() {
		if err := r.emitDeferredTextEOS(route, errorText); err != nil && errorText == "" {
			errorText = err.Error()
		}
		if err := r.emitPendingTTSEOS(route, errorText); err != nil && errorText == "" {
			errorText = err.Error()
		}
		route.closed.Store(true)
		_ = r.invocation.FinishResponse(route.response, errorText)
	})
}

func (r *dockRoute) hasTTSPipes() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, pipe := range r.ttsPipes {
		if pipe != nil {
			return true
		}
	}
	return false
}

// emitPendingTTSEOS normalizes TTS implementations that end their Stream
// without emitting a MIME-route EOS. The first TTS chunk opens the audio route
// before a deferred text EOS, while this helper closes any route still open
// when the provider Stream itself ends.
func (r *dockRun) emitPendingTTSEOS(route *dockRoute, errorText string) error {
	if route == nil {
		return nil
	}
	route.mu.Lock()
	mimeTypes := make([]string, 0, len(route.ttsRoutes))
	for mimeType, done := range route.ttsRoutes {
		if !done {
			mimeTypes = append(mimeTypes, mimeType)
			route.ttsRoutes[mimeType] = true
		}
	}
	route.mu.Unlock()
	sort.Strings(mimeTypes)
	for _, mimeType := range mimeTypes {
		if err := r.invocation.Emit(route.response, &genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: mimeType},
			Ctrl: &genx.StreamCtrl{
				StreamID:    route.response.StreamID(),
				Error:       errorText,
				EndOfStream: true,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *dockRun) emitDeferredTextEOS(route *dockRoute, errorText string) error {
	if route == nil {
		return nil
	}
	route.mu.Lock()
	source := route.deferredEOS
	route.deferredEOS = nil
	route.mu.Unlock()
	if source == nil {
		return nil
	}
	emitted := source.Clone()
	if emitted.Part == nil {
		emitted.Part = genx.Text("")
	}
	if emitted.Ctrl == nil {
		emitted.Ctrl = &genx.StreamCtrl{}
	}
	if emitted.Ctrl.Error == "" {
		emitted.Ctrl.Error = errorText
	}
	return r.invocation.EmitObserved(route.response, emitted, func(*genx.MessageChunk) {
		r.source.ObserveOutput(source)
	})
}

func (r *dockRun) abortTTS(route *dockRoute, err error) {
	if route == nil {
		return
	}
	route.mu.Lock()
	pipes := make([]*ttsPipe, 0, len(route.ttsPipes))
	for _, pipe := range route.ttsPipes {
		if pipe != nil {
			pipes = append(pipes, pipe)
		}
	}
	route.mu.Unlock()
	for _, pipe := range pipes {
		_ = pipe.input.CloseWithError(err)
		_ = pipe.output.CloseWithError(err)
		pipe.cancel()
	}
}

func (r *dockRun) finishOpenRoutes() {
	for _, route := range r.routeSnapshot() {
		if route.hasTTSPipes() {
			r.endTTS(route, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{EndOfStream: true}})
			continue
		}
		r.finishRoute(route, "")
	}
}

func (r *dockRun) closeRoutes(err error) {
	for _, route := range r.routeSnapshot() {
		r.abortTTS(route, err)
	}
}

func (r *dockRun) interruptOpenRoutes(errorText string) {
	for _, route := range r.routeSnapshot() {
		if route == nil || route.closed.Load() {
			continue
		}
		route.finish.Do(func() {
			route.closed.Store(true)
			r.abortTTS(route, errors.New(errorText))
			_ = r.invocation.Interrupt(route.response, errorText)
		})
	}
}

func (r *dockRun) beginInputTurn(streamID string) {
	ids := r.source.AbandonAllOutputObservations()
	r.stateMu.Lock()
	for _, id := range ids {
		r.discardSourceIDs[id] = true
	}
	for id, route := range r.routes {
		if route != nil && !route.closed.Load() {
			r.discardSourceIDs[id] = true
		}
	}
	r.stateMu.Unlock()
	r.interruptOpenRoutes("interrupted")
}

func (r *dockRun) endInputTurn(streamID string) {
	// Input EOS is not required to release newly created output routes. Realtime
	// providers may delimit turns from silence while keeping the original audio
	// route open. beginInputTurn records the response IDs that existed at BOS;
	// only those stale routes remain blocked until their own EOS arrives.
	_ = streamID
}

func (r *dockRun) discardSourceChunk(chunk *genx.MessageChunk) bool {
	streamID := ""
	if chunk != nil && chunk.Ctrl != nil {
		streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	r.stateMu.Lock()
	discard := r.discardSourceIDs[streamID]
	if discard && chunk != nil && chunk.IsEndOfStream() {
		delete(r.discardSourceIDs, streamID)
	}
	r.stateMu.Unlock()
	return discard
}

func (r *dockRun) routeSnapshot() []*dockRoute {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	routes := make([]*dockRoute, 0, len(r.routes))
	for _, route := range r.routes {
		routes = append(routes, route)
	}
	return routes
}

func (r *dockRun) closeSources(err error) {
	closePipelineStream(r.source, err)
	if r.router != nil {
		r.router.CloseWithError(err)
	}
}

// inputRouter keeps ASR optional at the MIME boundary: audio is converted to
// text while text and every other non-audio chunk bypass ASR unchanged.
type inputRouter struct {
	ctx context.Context

	input      genx.Stream
	agentInput *streamkit.Output
	asrInput   *streamkit.Output
	asrOutput  genx.Stream
	transcript *streamkit.Output
	events     chan inputEvent

	producers   sync.WaitGroup
	closeOnce   sync.Once
	failOnce    sync.Once
	eventsMu    sync.Mutex
	eventsReady bool
	pending     []inputEvent
}

func newInputRouter(ctx context.Context, input genx.Stream, asr genx.Transformer) (*inputRouter, error) {
	router := &inputRouter{
		ctx: ctx, input: input,
		events: make(chan inputEvent),
	}
	router.agentInput = streamkit.NewOutput(streamkit.OutputConfig{
		InitialCapacity: initialOutputCapacity,
		Observe:         router.observeAgentInput,
	})
	if asr == nil {
		router.producers.Add(1)
		go router.routeInput()
		go func() {
			router.producers.Wait()
			_ = router.agentInput.Close()
		}()
		go func() {
			<-ctx.Done()
			router.CloseWithError(ctx.Err())
		}()
		return router, nil
	}
	router.asrInput = streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: initialOutputCapacity})
	router.transcript = streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: initialOutputCapacity})
	asrOutput, err := asr.Transform(ctx, router.asrInput)
	if err != nil {
		router.CloseWithError(err)
		return nil, fmt.Errorf("audiodock: start ASR: %w", err)
	}
	router.asrOutput = asrOutput
	router.producers.Add(2)
	go router.routeInput()
	go router.forwardASR()
	go func() {
		router.producers.Wait()
		_ = router.agentInput.Close()
	}()
	go func() {
		<-ctx.Done()
		router.CloseWithError(ctx.Err())
	}()
	return router, nil
}

func (r *inputRouter) AgentInput() genx.Stream {
	return r.agentInput
}

func (r *inputRouter) InputEvents() <-chan inputEvent {
	if r == nil {
		closed := make(chan inputEvent)
		close(closed)
		return closed
	}
	return r.events
}

func (r *inputRouter) ActivateEvents() []inputEvent {
	if r == nil {
		return nil
	}
	r.eventsMu.Lock()
	r.eventsReady = true
	pending := r.pending
	r.pending = nil
	r.eventsMu.Unlock()
	return pending
}

func (r *inputRouter) TranscriptOutput() genx.Stream {
	if r == nil {
		return nil
	}
	return r.transcript
}

func (r *inputRouter) routeInput() {
	defer r.producers.Done()
	if r.asrInput != nil {
		defer r.asrInput.Close()
	}
	for {
		chunk, err := r.input.Next()
		if err != nil {
			if streamDone(err) {
				return
			}
			r.fail(fmt.Errorf("audiodock: read input: %w", err))
			return
		}
		if chunk == nil {
			continue
		}
		if chunk.IsBeginOfStream() {
			if !r.sendInputEvent(true, dockStreamID(chunk)) {
				return
			}
		}
		target := r.agentInput
		if r.asrInput != nil {
			if mimeType, ok := chunk.MIMEType(); ok && strings.HasPrefix(mimeType, "audio/") {
				target = r.asrInput
			}
		}
		if target == r.asrInput && chunk.IsBeginOfStream() {
			// The raw audio payload belongs only to ASR, but the downstream text
			// Agent must observe the replacement BOS immediately so it can cancel
			// an active turn. Realtime ASR implementations are not required to
			// repeat BOS on their transcript stream.
			begin := chunk.Clone()
			begin.Part = nil
			if err := r.agentInput.Push(begin); err != nil {
				if r.ctx.Err() == nil && !errors.Is(err, io.ErrClosedPipe) {
					r.fail(fmt.Errorf("audiodock: forward audio BOS to Agent: %w", err))
				}
				return
			}
		}
		if err := target.Push(chunk); err != nil {
			if r.ctx.Err() == nil && !errors.Is(err, io.ErrClosedPipe) {
				r.fail(fmt.Errorf("audiodock: route input: %w", err))
			}
			return
		}
	}
}

func (r *inputRouter) observeAgentInput(chunk *genx.MessageChunk) {
	if chunk != nil && chunk.IsEndOfStream() {
		_ = r.sendInputEvent(false, dockStreamID(chunk))
	}
}

func (r *inputRouter) sendInputEvent(begin bool, streamID string) bool {
	r.eventsMu.Lock()
	if !r.eventsReady {
		r.pending = append(r.pending, inputEvent{begin: begin, streamID: streamID})
		r.eventsMu.Unlock()
		return true
	}
	r.eventsMu.Unlock()
	acknowledgement := make(chan struct{})
	select {
	case r.events <- inputEvent{begin: begin, streamID: streamID, acknowledgement: acknowledgement}:
	case <-r.ctx.Done():
		return false
	}
	select {
	case <-acknowledgement:
		return true
	case <-r.ctx.Done():
		return false
	}
}

func (r *inputRouter) forwardASR() {
	defer r.producers.Done()
	defer closePipelineStream(r.asrOutput, nil)
	defer r.transcript.Close()
	for {
		chunk, err := r.asrOutput.Next()
		if err != nil {
			if streamDone(err) {
				return
			}
			r.fail(fmt.Errorf("audiodock: read ASR output: %w", err))
			return
		}
		if chunk == nil {
			continue
		}
		if err := r.transcript.Push(chunk.Clone()); err != nil {
			if r.ctx.Err() == nil && !errors.Is(err, io.ErrClosedPipe) {
				r.fail(fmt.Errorf("audiodock: expose ASR output: %w", err))
			}
			return
		}
		if err := r.agentInput.Push(chunk); err != nil {
			if r.ctx.Err() == nil && !errors.Is(err, io.ErrClosedPipe) {
				r.fail(fmt.Errorf("audiodock: forward ASR output: %w", err))
			}
			return
		}
	}
}

func (r *inputRouter) fail(err error) {
	r.failOnce.Do(func() {
		_ = r.agentInput.CloseWithError(err)
		_ = r.asrInput.CloseWithError(err)
		_ = r.transcript.CloseWithError(err)
		closePipelineStream(r.asrOutput, err)
		closePipelineStream(r.input, err)
	})
}

func (r *inputRouter) CloseWithError(err error) {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		if r.agentInput == nil {
			closePipelineStream(r.input, err)
			return
		}
		if err == nil {
			err = context.Canceled
		}
		r.fail(err)
	})
}

func closePipelineStream(stream genx.Stream, err error) {
	if stream == nil {
		return
	}
	if err != nil {
		_ = stream.CloseWithError(err)
		return
	}
	_ = stream.Close()
}

func streamDone(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone)
}

func cloneCtrl(ctrl *genx.StreamCtrl) *genx.StreamCtrl {
	if ctrl == nil {
		return nil
	}
	copyCtrl := *ctrl
	return &copyCtrl
}

func dockStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}
