package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolkitrun"
	"github.com/cloudwego/eino/schema"
)

// Transformer owns one immutable compiled Eino Graph. Transform may be called
// concurrently; every call receives independent invocation-local run state.
type Transformer struct {
	config    *normalizedConfig
	graph     *compiledGraph
	contextID string
	history   *conversationHistory
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
	return &Transformer{
		config: config, graph: graph, contextID: contextID,
		history: &conversationHistory{
			config: config.History, agentID: config.Agent.ID, contextID: contextID,
		},
	}, nil
}

// Transform consumes one long-lived GenX Stream. Every completed text input
// route executes a fresh Graph run.
func (transformer *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
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
	var pendingBOS []*genx.MessageChunk
	inText := false
	activeInputID := ""
	activeBypassID := ""
	var inputFailure error
	var previous <-chan struct{}
	for {
		chunk, err := session.input.Next()
		if err != nil {
			if !isStreamEnd(err) {
				inputFailure = err
			}
			break
		}
		if chunk == nil {
			continue
		}
		if chunk.IsBeginOfStream() {
			if chunk.Part == nil {
				session.interruptActive()
				text.Reset()
				parts = nil
				inText = false
				activeInputID = ""
				pendingBOS = append(pendingBOS[:0], chunk.Clone())
				continue
			}
			if _, ok := chunk.Part.(genx.Text); ok {
				session.interruptActive()
				text.Reset()
				parts = nil
				inText = false
				activeInputID = messageStreamID(chunk)
				pendingBOS = nil
			}
		}
		if chunk.IsEndOfStream() && chunk.Part == nil && inText {
			streamID := messageStreamID(chunk)
			if streamID == "" || activeInputID == "" || streamID == activeInputID {
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					inputFailure = fmt.Errorf("eino: input text Stream failed: %s", chunk.Ctrl.Error)
					break
				}
				previous = session.startTurn(text.String(), parts, previous)
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
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					inputFailure = fmt.Errorf("eino: input text Stream failed: %s", chunk.Ctrl.Error)
					break
				}
				previous = session.startTurn(text.String(), parts, previous)
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
				if chunk.IsEndOfStream() && chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					inputFailure = fmt.Errorf("eino: input part Stream failed: %s", chunk.Ctrl.Error)
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

type outputRoute struct {
	definition OutputDefinition
	response   *streamkit.Response
}

func (session *session) startTurn(user string, parts []any, previous <-chan struct{}) <-chan struct{} {
	runCtx, cancel := context.WithCancelCause(session.invocation.Context())
	run := &turnRun{
		session: session, user: user, parts: parts, ctx: runCtx, cancel: cancel,
		routes: make(map[string]outputRoute), streamIDs: make(map[string]struct{}),
		accepting: true, changed: make(chan struct{}, 1), done: make(chan struct{}), previous: previous,
	}
	for _, output := range session.transformer.graph.definition.Outputs {
		response, err := session.invocation.StartResponse(streamkit.ResponseConfig{
			Role: genx.RoleModel, Name: output.Name, Label: output.Name,
		}, output.MIMEType)
		if err != nil {
			_ = session.invocation.Fail(err)
			cancel(err)
			close(run.done)
			return run.done
		}
		route := outputRoute{definition: output, response: response}
		run.routes[output.Name] = route
		run.streamIDs[response.StreamID()] = struct{}{}
		if output.Primary {
			run.primary = route
		}
		if err := session.invocation.Emit(response, genx.NewBeginOfStream(response.StreamID())); err != nil {
			_ = session.invocation.Fail(err)
			cancel(err)
			close(run.done)
			return run.done
		}
	}
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

type turnRun struct {
	session  *session
	user     string
	parts    []any
	ctx      context.Context
	cancel   context.CancelCauseFunc
	previous <-chan struct{}
	done     chan struct{}

	routes    map[string]outputRoute
	primary   outputRoute
	streamIDs map[string]struct{}

	mu             sync.Mutex
	accepting      bool
	interrupted    bool
	terminal       bool
	emittedPrimary int
	deliveredBytes int
	delivered      strings.Builder
	changed        chan struct{}
}

func (run *turnRun) Emit(output OutputDefinition, value any) error {
	run.mu.Lock()
	defer run.mu.Unlock()
	if !run.accepting {
		return streamkit.ErrInactiveResponse
	}
	route, ok := run.routes[output.Name]
	if !ok {
		return fmt.Errorf("eino: output route %q is not active", output.Name)
	}
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Name: output.Name}
	var size int
	switch typed := value.(type) {
	case string:
		chunk.Part = genx.Text(typed)
		size = len(typed)
	case []byte:
		chunk.Part = &genx.Blob{MIMEType: output.MIMEType, Data: append([]byte(nil), typed...)}
		size = len(typed)
	default:
		return fmt.Errorf("eino: output %q has unsupported value %T", output.Name, value)
	}
	if err := run.session.invocation.Emit(route.response, chunk); err != nil {
		return err
	}
	if output.Primary {
		run.emittedPrimary += size
	}
	return nil
}

func (run *turnRun) observe(chunk *genx.MessageChunk) {
	if chunk == nil || chunk.IsEndOfStream() || chunk.Ctrl == nil || chunk.Ctrl.StreamID != run.primary.response.StreamID() {
		return
	}
	run.mu.Lock()
	switch part := chunk.Part.(type) {
	case genx.Text:
		run.delivered.WriteString(string(part))
		run.deliveredBytes += len(part)
	case *genx.Blob:
		if part != nil {
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
	run.mu.Unlock()
	run.cancel(errors.New("interrupted"))
	for _, route := range run.routes {
		streamID := route.response.StreamID()
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
		select {
		case <-run.previous:
		case <-run.ctx.Done():
		}
	}
	var state *runState
	var version string
	var runErr error
	if run.ctx.Err() == nil {
		state, version, runErr = run.runGraph()
	} else {
		runErr = context.Cause(run.ctx)
	}
	run.mu.Lock()
	run.accepting = false
	run.mu.Unlock()
	run.waitUntilDelivered()
	run.mu.Lock()
	run.terminal = true
	delivered := run.delivered.String()
	interrupted := run.interrupted
	run.mu.Unlock()
	if state != nil {
		finalizeErr := run.finalize(run.session.invocation.Context(), state, version, delivered, interrupted || runErr != nil)
		if runErr == nil {
			runErr = finalizeErr
		}
	}
	errorText := ""
	if interrupted {
		errorText = "interrupted"
	} else if runErr != nil {
		errorText = runErr.Error()
	}
	run.finishRoutes(errorText, interrupted)
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

func (run *turnRun) runGraph() (*runState, string, error) {
	config := run.session.transformer.config
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
	messages := append(cloneMessages(history), schemaUserMessage(run.user, run.parts))
	state, err := newRunState(config.fields, graphInput{
		Text: run.user, Messages: messages, Parts: run.parts, History: history,
	}, initial, run)
	if err != nil {
		return nil, "", err
	}
	if err := recallMemory(run.ctx, config.Memory, state); err != nil {
		return nil, "", err
	}
	runContext := toolkitrun.WithContext(
		run.ctx,
		toolkitrun.New(config.Toolkit, config.MaxToolCalls),
	)
	if err := run.session.transformer.graph.execute(runContext, state); err != nil {
		return state, version, err
	}
	if _, err := state.value(run.session.transformer.graph.primary.Field); err != nil {
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
	if err := run.session.transformer.history.append(ctx, historyMessages(run.user, delivered), failed); err != nil {
		return err
	}
	if err := observeMemory(ctx, run.session.transformer.config.Memory, state, run.primary.response.StreamID(), run.user, delivered, failed); err != nil {
		return err
	}
	if failed {
		return nil
	}
	return commitPersistentState(ctx, run.session.transformer.config.State, state, version)
}

func (run *turnRun) finishRoutes(errorText string, interrupted bool) {
	names := make([]string, 0, len(run.routes))
	for name := range run.routes {
		if name != run.primary.definition.Name {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append(names, run.primary.definition.Name)
	for _, name := range names {
		route := run.routes[name]
		if interrupted {
			_ = run.session.invocation.Interrupt(route.response, errorText)
		} else {
			_ = run.session.invocation.FinishResponse(route.response, errorText)
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

var _ genx.Transformer = (*Transformer)(nil)
var _ genx.Stream = (*sessionStream)(nil)
var _ outputEmitter = (*turnRun)(nil)
