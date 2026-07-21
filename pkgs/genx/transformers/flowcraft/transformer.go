package flowcraft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	flowagent "github.com/GizClaw/flowcraft/sdk/agent"
	"github.com/GizClaw/flowcraft/sdk/engine"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const assistantLabel = "agent.flowcraft.assistant"

// Agent owns one immutable Flowcraft graph runtime. Transform may be
// called concurrently; mutable conversation state is allocated per call.
type Agent struct {
	config Config
	agent  flowagent.Agent
	engine engine.Engine
}

// New validates Config and constructs a reusable Flowcraft Agent.
func New(source Config) (*Agent, error) {
	config, err := normalizeConfig(source)
	if err != nil {
		return nil, err
	}
	sdkAgent, graphEngine, err := buildRuntime(config)
	if err != nil {
		return nil, err
	}
	return &Agent{config: config, agent: sdkAgent, engine: graphEngine}, nil
}

// Transform consumes a long-lived GenX stream. Each completed text BOS/EOS
// route runs one graph turn and produces a fresh output StreamID.
func (t *Agent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.engine == nil {
		return nil, fmt.Errorf("flowcraft: Transformer is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("flowcraft: input Stream is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s := newSession(ctx, t, input)
	go s.closeInputOnCancellation()
	go s.run()
	return &sessionStream{Output: s.invocation.Output(), session: s}, nil
}

type session struct {
	transformer *Agent
	input       genx.Stream
	invocation  *streamkit.Invocation
	contextID   string
	history     *conversationHistory

	mu     sync.Mutex
	runs   map[string]*turnRun
	active *turnRun

	turns     sync.WaitGroup
	done      chan struct{}
	inputOnce sync.Once
}

func newSession(ctx context.Context, transformer *Agent, input genx.Stream) *session {
	contextID := genx.NewStreamID()
	s := &session{
		transformer: transformer, input: input, contextID: contextID,
		runs: make(map[string]*turnRun), done: make(chan struct{}),
	}
	s.history = &conversationHistory{store: transformer.config.History, agentID: transformer.config.ID, contextID: contextID}
	s.invocation = streamkit.NewInvocation(ctx, streamkit.OutputConfig{InitialCapacity: 64, Observe: s.observeOutput})
	return s
}

func (s *session) observeOutput(chunk *genx.MessageChunk) {
	if chunk == nil || chunk.Ctrl == nil {
		return
	}
	s.mu.Lock()
	run := s.runs[chunk.Ctrl.StreamID]
	s.mu.Unlock()
	if run != nil {
		run.observe(chunk)
	}
}

func (s *session) interruptActive() {
	s.mu.Lock()
	run := s.active
	s.active = nil
	s.mu.Unlock()
	if run != nil {
		run.interrupt()
	}
}

func (s *session) closeInput(err error) {
	s.inputOnce.Do(func() {
		if err == nil {
			_ = s.input.Close()
			return
		}
		_ = s.input.CloseWithError(err)
	})
}

func (s *session) closeInputOnCancellation() {
	select {
	case <-s.invocation.Context().Done():
		s.closeInput(context.Cause(s.invocation.Context()))
	case <-s.done:
	}
}

func (s *session) run() {
	defer close(s.done)
	defer s.closeInput(nil)
	var text strings.Builder
	var pendingBOS []pendingBegin
	inText := false
	activeInputID := ""
	activeBypassID := ""
	var inputFailure error
	var previous <-chan struct{}
	for {
		chunk, err := s.input.Next()
		if err != nil {
			if !isStreamEnd(err) {
				s.interruptActive()
				s.turns.Wait()
				_ = s.invocation.Output().CloseWithError(err)
				return
			}
			break
		}
		if chunk == nil {
			continue
		}
		if chunk.IsBeginOfStream() {
			// A control-only BOS does not declare a MIME channel. Treat it as the
			// next text turn eagerly so stale output cannot escape while waiting
			// for the first text delta. MIME-bearing non-text BOS remains bypass.
			if chunk.Part == nil {
				s.interruptActive()
				storePendingBegin(&pendingBOS, chunk.Clone())
				continue
			}
			if _, ok := chunk.Part.(genx.Text); ok {
				s.interruptActive()
			}
		}
		if chunk.IsEndOfStream() && chunk.Part == nil && inText {
			streamID := messageStreamID(chunk)
			if streamID == "" || activeInputID == "" || streamID == activeInputID {
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					inputFailure = fmt.Errorf("flowcraft: input text stream failed: %s", chunk.Ctrl.Error)
					break
				}
				if strings.TrimSpace(text.String()) != "" {
					previous = s.startTurn(text.String(), previous)
				}
				text.Reset()
				inText = false
				activeInputID = ""
				continue
			}
		}
		if part, ok := chunk.Part.(genx.Text); ok {
			if begin := takePendingBegin(&pendingBOS, messageStreamID(chunk)); begin != nil {
				s.interruptActive()
				text.Reset()
				inText = false
				activeInputID = messageStreamID(begin)
			}
			streamID := messageStreamID(chunk)
			if !inText && activeInputID == "" {
				activeInputID = streamID
			}
			if streamID != "" && activeInputID != "" && streamID != activeInputID {
				inputFailure = fmt.Errorf("flowcraft: text chunk StreamID %q does not match active StreamID %q", streamID, activeInputID)
				break
			}
			inText = true
			text.WriteString(string(part))
			if chunk.IsEndOfStream() {
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					inputFailure = fmt.Errorf("flowcraft: input text stream failed: %s", chunk.Ctrl.Error)
					break
				}
				if strings.TrimSpace(text.String()) != "" {
					previous = s.startTurn(text.String(), previous)
				}
				text.Reset()
				inText = false
				activeInputID = ""
			}
			continue
		}
		streamID := messageStreamID(chunk)
		lookupID := streamID
		if lookupID == "" {
			lookupID = activeBypassID
		}
		if begin := takePendingBegin(&pendingBOS, lookupID); begin != nil {
			if streamID == "" {
				streamID = messageStreamID(begin)
			}
			if err := s.invocation.Output().Push(begin); err != nil {
				break
			}
		}
		copyChunk := chunk.Clone()
		if streamID == "" {
			streamID = activeBypassID
		}
		if streamID != "" {
			if copyChunk.Ctrl == nil {
				copyChunk.Ctrl = &genx.StreamCtrl{}
			}
			if strings.TrimSpace(copyChunk.Ctrl.StreamID) == "" {
				copyChunk.Ctrl.StreamID = streamID
			}
			activeBypassID = streamID
		}
		if err := s.invocation.Output().Push(copyChunk); err != nil {
			break
		}
		if copyChunk.IsEndOfStream() && streamID == activeBypassID {
			activeBypassID = ""
		}
	}
	if inputFailure != nil {
		s.interruptActive()
		s.turns.Wait()
		_ = s.invocation.Output().CloseWithError(inputFailure)
		return
	}
	for _, begin := range pendingBOS {
		if err := s.invocation.Output().Push(begin.chunk); err != nil {
			break
		}
	}
	s.turns.Wait()
	_ = s.invocation.Close()
}

type pendingBegin struct {
	streamID string
	chunk    *genx.MessageChunk
}

func storePendingBegin(pending *[]pendingBegin, chunk *genx.MessageChunk) {
	streamID := messageStreamID(chunk)
	if streamID != "" {
		for index := range *pending {
			if (*pending)[index].streamID == streamID {
				(*pending)[index].chunk = chunk
				return
			}
		}
	}
	*pending = append(*pending, pendingBegin{streamID: streamID, chunk: chunk})
}

func takePendingBegin(pending *[]pendingBegin, streamID string) *genx.MessageChunk {
	if len(*pending) == 0 {
		return nil
	}
	index := 0
	if streamID != "" {
		index = -1
		for candidate := range *pending {
			if (*pending)[candidate].streamID == streamID {
				index = candidate
				break
			}
		}
		if index < 0 {
			return nil
		}
	}
	chunk := (*pending)[index].chunk
	*pending = append((*pending)[:index], (*pending)[index+1:]...)
	return chunk
}

func (s *session) startTurn(user string, previous <-chan struct{}) <-chan struct{} {
	response, err := s.invocation.StartResponse(streamkit.ResponseConfig{
		Role: genx.RoleModel, Name: s.transformer.config.Name, Label: assistantLabel,
	}, "text/plain")
	if err != nil {
		_ = s.invocation.Fail(err)
		done := make(chan struct{})
		close(done)
		return done
	}
	runCtx, cancel := context.WithCancelCause(s.invocation.Context())
	run := &turnRun{
		session: s, user: user, response: response, ctx: runCtx, cancel: cancel,
		accepting: true, changed: make(chan struct{}, 1), done: make(chan struct{}), previous: previous,
	}
	if err := s.invocation.Emit(response, genx.NewBeginOfStream(response.StreamID())); err != nil {
		_ = s.invocation.FinishResponse(response, err.Error())
		done := make(chan struct{})
		close(done)
		return done
	}
	s.mu.Lock()
	s.runs[response.StreamID()] = run
	s.active = run
	s.mu.Unlock()
	s.turns.Add(1)
	go run.execute()
	return run.done
}

type turnRun struct {
	session  *session
	user     string
	response *streamkit.Response
	ctx      context.Context
	cancel   context.CancelCauseFunc
	previous <-chan struct{}
	done     chan struct{}

	mu          sync.Mutex
	accepting   bool
	interrupted bool
	terminal    bool
	emitted     int
	delivered   strings.Builder
	changed     chan struct{}
}

func (r *turnRun) signal() {
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

func (r *turnRun) emit(nodeID, content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.accepting {
		return streamkit.ErrInactiveResponse
	}
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Name: nodeID, Part: genx.Text(content)}
	if err := r.session.invocation.Emit(r.response, chunk); err != nil {
		return err
	}
	r.emitted += len(content)
	return nil
}

func (r *turnRun) observe(chunk *genx.MessageChunk) {
	text, ok := chunk.Part.(genx.Text)
	if !ok || chunk.IsEndOfStream() {
		return
	}
	r.mu.Lock()
	r.delivered.WriteString(string(text))
	r.mu.Unlock()
	r.signal()
}

func (r *turnRun) interrupt() {
	r.mu.Lock()
	if r.interrupted || r.terminal {
		r.mu.Unlock()
		return
	}
	r.interrupted = true
	r.accepting = false
	r.mu.Unlock()
	r.cancel(errors.New("interrupted"))
	streamID := r.response.StreamID()
	r.session.invocation.Output().Discard(func(chunk *genx.MessageChunk) bool {
		return chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID == streamID
	})
	r.session.invocation.Output().WaitForObservers()
	r.signal()
}

func (r *turnRun) execute() {
	defer r.session.turns.Done()
	defer close(r.done)
	if r.previous != nil {
		select {
		case <-r.previous:
		case <-r.ctx.Done():
		}
	}
	var result *flowagent.Result
	var runErr error
	if r.ctx.Err() != nil {
		runErr = context.Cause(r.ctx)
	} else {
		result, runErr = r.runGraph()
	}
	r.mu.Lock()
	r.accepting = false
	r.mu.Unlock()
	var finalBoard *engine.Board
	if runErr == nil && result != nil {
		if result.Err != nil {
			runErr = result.Err
		}
		finalBoard = result.LastBoard
	}
	r.waitUntilDelivered()
	r.mu.Lock()
	r.terminal = true
	delivered := r.delivered.String()
	interrupted := r.interrupted
	r.mu.Unlock()
	finalizeCtx := r.session.invocation.Context()
	if err := r.finalize(finalizeCtx, delivered, interrupted, finalBoard); runErr == nil && err != nil {
		runErr = err
	}
	if interrupted {
		_ = r.session.invocation.Interrupt(r.response, "interrupted")
	} else {
		errorText := ""
		if runErr != nil {
			errorText = runErr.Error()
		}
		_ = r.session.invocation.FinishResponse(r.response, errorText)
	}
	r.session.mu.Lock()
	delete(r.session.runs, r.response.StreamID())
	if r.session.active == r {
		r.session.active = nil
	}
	r.session.mu.Unlock()
	r.cancel(io.EOF)
}

func (r *turnRun) runGraph() (*flowagent.Result, error) {
	config := r.session.transformer.config
	publish := make(map[string]struct{}, len(config.PublishNodes))
	for _, nodeID := range config.PublishNodes {
		publish[nodeID] = struct{}{}
	}
	host := &runHost{
		publish: publish, emit: r.emit,
		buffers: make(map[string][]bufferedDelta), terminal: make(map[string]struct{}),
	}
	seed := flowagent.BoardSeederFunc(func(ctx context.Context, _ flowagent.RunInfo, req *flowagent.Request) (*engine.Board, error) {
		board := engine.NewBoard()
		state, err := loadBoardState(ctx, config.State, r.session.contextID)
		if err != nil {
			return nil, err
		}
		for key, value := range state {
			board.SetVar(key, value)
		}
		messages, err := r.session.history.load(ctx)
		if err != nil {
			return nil, err
		}
		board.SetChannel(engine.MainChannel, messages)
		board.AppendChannelMessage(engine.MainChannel, req.Message)
		for _, profile := range config.RecallProfiles {
			recalled, err := config.Memory.Recall(ctx, memory.Query{Scope: config.MemoryScope, Text: r.user, Limit: profile.Limit, Filters: profile.Filters})
			if err != nil {
				return nil, fmt.Errorf("flowcraft: recall Memory for %q: %w", profile.BoardVariable, err)
			}
			rendered, err := config.RecallRenderer(ctx, recalled.Matches)
			if err != nil {
				return nil, fmt.Errorf("flowcraft: render Memory for %q: %w", profile.BoardVariable, err)
			}
			board.SetVar(profile.BoardVariable, rendered)
		}
		return board, nil
	})
	result, err := flowagent.Run(r.ctx, r.session.transformer.agent, r.session.transformer.engine, flowagent.Request{
		ContextID: r.session.contextID, RunID: r.response.StreamID(),
		Message: flowmodel.NewTextMessage(flowmodel.RoleUser, r.user),
	}, flowagent.WithEngineHost(host), flowagent.WithBoardSeed(seed))
	return result, err
}

func (r *turnRun) waitUntilDelivered() {
	for {
		r.mu.Lock()
		done := r.interrupted || r.delivered.Len() >= r.emitted
		r.mu.Unlock()
		if done {
			return
		}
		select {
		case <-r.changed:
		case <-r.session.invocation.Context().Done():
			return
		}
	}
}

func (r *turnRun) finalize(ctx context.Context, delivered string, interrupted bool, finalBoard *engine.Board) error {
	messages := []flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleUser, r.user)}
	if delivered != "" || interrupted {
		messages = append(messages, flowmodel.NewTextMessage(flowmodel.RoleAssistant, delivered))
	}
	if err := r.session.history.append(ctx, messages, interrupted); err != nil {
		return err
	}
	config := r.session.transformer.config
	if err := saveBoardState(ctx, config.State, r.session.contextID, finalBoard); err != nil {
		return err
	}
	if !config.ObserveEnabled {
		return nil
	}
	boardVariables, err := serializableBoardVariables(finalBoard)
	if err != nil {
		return err
	}
	observation, err := config.ObservationBuilder(ctx, ObservationInput{
		StreamID: r.response.StreamID(), UserText: r.user, DeliveredAssistantText: delivered,
		BoardVariables: boardVariables, Interrupted: interrupted,
	})
	if err != nil {
		return fmt.Errorf("flowcraft: build Memory observation: %w", err)
	}
	observation.Scope = config.MemoryScope
	if err := memory.ValidateObservation(observation); err != nil {
		return fmt.Errorf("flowcraft: validate Memory observation: %w", err)
	}
	observed, err := config.Memory.Observe(ctx, observation)
	if err != nil {
		return fmt.Errorf("flowcraft: observe Memory: %w", err)
	}
	if observed.Operation != nil && observed.Operation.Status == memory.OperationFailed {
		return fmt.Errorf("flowcraft: Memory operation %q failed: %s", observed.Operation.ID, observed.Operation.Error)
	}
	if config.ObserveWaitForCompletion && observed.Operation != nil && observed.Operation.Status == memory.OperationPending {
		if strings.TrimSpace(observed.Operation.ID) == "" {
			return fmt.Errorf("flowcraft: pending Memory operation has no ID")
		}
		waiter := config.Memory.(memory.OperationWaiter)
		completed, err := waiter.Wait(ctx, observed.Operation.ID)
		if err != nil {
			return fmt.Errorf("flowcraft: wait Memory operation %q: %w", observed.Operation.ID, err)
		}
		if completed.Operation != nil && completed.Operation.Status == memory.OperationFailed {
			return fmt.Errorf("flowcraft: Memory operation %q failed: %s", completed.Operation.ID, completed.Operation.Error)
		}
	}
	return nil
}

type sessionStream struct {
	*streamkit.Output
	session *session
	once    sync.Once
}

func (s *sessionStream) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		_ = s.inputClose(nil)
		_ = s.session.invocation.Cancel(io.EOF)
	})
	return nil
}

func (s *sessionStream) CloseWithError(err error) error {
	if s == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.once.Do(func() {
		_ = s.inputClose(err)
		_ = s.session.invocation.Cancel(err)
	})
	return nil
}

func (s *sessionStream) inputClose(err error) error {
	s.session.closeInput(err)
	return nil
}

func isStreamEnd(err error) bool {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, buffer.ErrIteratorDone) {
		return true
	}
	var state *genx.State
	return errors.As(err, &state) && state.Status() == genx.StatusDone
}

func messageStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}

var _ genx.Transformer = (*Agent)(nil)
var _ genx.Stream = (*sessionStream)(nil)
