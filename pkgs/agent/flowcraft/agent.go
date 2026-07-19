package flowcraft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	flowagent "github.com/GizClaw/flowcraft/sdk/agent"
	"github.com/GizClaw/flowcraft/sdk/engine"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const assistantLabel = "agent.flowcraft.assistant"

var _ commonagent.Agent = (*Agent)(nil)

// Agent owns one Flowcraft graph runtime, its Tool loop, and conversation state.
type Agent struct {
	config         Config
	agent          flowagent.Agent
	engine         engine.Engine
	history        *conversationHistory
	externalPulled *pulledHistory
}

// New constructs a GizClaw-owned Flowcraft graph Agent.
func New(config Config) (*Agent, error) {
	config.ID = strings.TrimSpace(config.ID)
	if config.ID == "" {
		return nil, fmt.Errorf("agent/flowcraft: ID is required")
	}
	config.Conversation = strings.TrimSpace(config.Conversation)
	if config.Conversation == "" {
		config.Conversation = config.ID
	}
	if config.Resolver == nil {
		return nil, fmt.Errorf("agent/flowcraft: resolver is required")
	}
	if config.Toolkit == nil {
		return nil, fmt.Errorf("agent/flowcraft: toolkit is required")
	}
	if config.MaxIterations < 0 || config.MaxToolCalls < 0 || config.ToolTimeout < 0 {
		return nil, fmt.Errorf("agent/flowcraft: iteration, tool-call, and timeout limits cannot be negative")
	}
	if config.MemoryLimit < 0 {
		return nil, fmt.Errorf("agent/flowcraft: memory limit cannot be negative")
	}
	if config.Memory != nil && config.MemoryLimit == 0 {
		config.MemoryLimit = 8
	}
	if err := config.Graph.Validate(); err != nil {
		return nil, fmt.Errorf("agent/flowcraft: invalid graph: %w", err)
	}
	historyWorkspace := strings.TrimSpace(config.HistoryWorkspace)
	if historyWorkspace == "" {
		historyWorkspace = config.ID
	}
	persistentHistory, err := newHistoryStore(
		config.History,
		historyWorkspace,
		config.Conversation,
		config.LegacyHistoryWorkspace,
		config.LegacyConversation,
	)
	if err != nil {
		return nil, err
	}
	history := &conversationHistory{store: persistentHistory}
	registry, names, err := buildToolRegistry(config.Toolkit, config.ToolTimeout, history)
	if err != nil {
		return nil, err
	}
	sdkAgent, graphEngine, err := buildGraph(config, names, registry)
	if err != nil {
		return nil, err
	}
	result := &Agent{config: config, agent: sdkAgent, engine: graphEngine, history: history}
	if config.ExternalOutputObservation {
		result.externalPulled = newPulledHistory(history, config.Memory, config.OnBackgroundError)
	}
	return result, nil
}

// Transform consumes GenX user turns and streams pull-visible graph output.
func (a *Agent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if a == nil || a.engine == nil {
		return nil, fmt.Errorf("agent/flowcraft: agent is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("agent/flowcraft: input stream is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeCtx, cancel := context.WithCancelCause(ctx)
	var pulled *pulledHistory
	if !a.config.ExternalOutputObservation {
		pulled = newPulledHistory(a.history, a.config.Memory, a.config.OnBackgroundError)
	}
	outputConfig := a.config.Output
	configuredObserver := outputConfig.Observe
	outputConfig.Observe = func(chunk *genx.MessageChunk) {
		if pulled != nil {
			pulled.observe(chunk)
		}
		if configuredObserver != nil {
			configuredObserver(chunk)
		}
	}
	output := commonagent.NewOutput(outputConfig)
	events := make(chan inputEvent)
	runs := make(chan runResult, 8)
	go readInput(runtimeCtx, input, events)
	go a.coordinate(runtimeCtx, cancel, input, output, pulled, events, runs)
	return &stream{Output: output, cancel: cancel, input: input}, nil
}

// BeginOutput associates an outer, device-visible stream with its user turn.
// It is active only when ExternalOutputObservation is configured.
func (a *Agent) BeginOutput(streamID, user string) {
	if a != nil && a.externalPulled != nil {
		a.externalPulled.track(streamID, user)
	}
}

// ObserveOutput records a chunk after it crosses the outermost pull boundary.
// It is active only when ExternalOutputObservation is configured.
func (a *Agent) ObserveOutput(chunk *genx.MessageChunk) {
	if a != nil && a.externalPulled != nil {
		a.externalPulled.observe(chunk)
	}
}

// InterruptOutput commits only the externally observed portion of a stream as
// interrupted before a replacement turn reads conversation history.
func (a *Agent) InterruptOutput(streamID string) {
	if a != nil && a.externalPulled != nil {
		a.externalPulled.commitInterrupted(streamID)
	}
}

// History returns the recent owned conversation transcript for product APIs
// and diagnostics. The returned messages are defensive copies.
func (a *Agent) History(ctx context.Context, limit int) ([]flowmodel.Message, error) {
	if a == nil || a.history == nil {
		return nil, fmt.Errorf("agent/flowcraft: agent is nil")
	}
	return a.history.recent(ctx, limit)
}

type inputEvent struct {
	begin bool
	text  *string
	err   error
}

func readInput(ctx context.Context, input genx.Stream, events chan<- inputEvent) {
	defer close(events)
	var content strings.Builder
	begun := false
	send := func(event inputEvent) bool {
		select {
		case events <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for {
		chunk, err := input.Next()
		if err != nil {
			if commonagent.IsStreamEnd(err) {
				if begun && content.Len() > 0 {
					text := content.String()
					send(inputEvent{text: &text})
				}
				return
			}
			send(inputEvent{err: err})
			return
		}
		if chunk == nil {
			continue
		}
		if chunk.IsBeginOfStream() {
			content.Reset()
			begun = true
			if !send(inputEvent{begin: true}) {
				return
			}
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			if !begun {
				begun = true
				if !send(inputEvent{begin: true}) {
					return
				}
			}
			content.WriteString(string(text))
		}
		if chunk.IsEndOfStream() && begun {
			text := content.String()
			if !send(inputEvent{text: &text}) {
				return
			}
			content.Reset()
			begun = false
		}
	}
}

type activeRun struct {
	id       uint64
	cancel   context.CancelCauseFunc
	response *commonagent.Response
}

type runResult struct {
	id  uint64
	err error
}

func (a *Agent) coordinate(ctx context.Context, cancel context.CancelCauseFunc, input genx.Stream, output *commonagent.Output, pulled *pulledHistory, events <-chan inputEvent, runs chan runResult) {
	defer cancel(io.EOF)
	var active *activeRun
	var pending *commonagent.Response
	var nextID uint64
	inputDone := false
	interrupt := func() {
		if active != nil {
			active.cancel(errors.New(commonagent.Interrupted))
			active = nil
		}
		if pending != nil {
			_ = pending.Interrupt()
			if pending.Interrupted() && pulled != nil {
				pulled.commitInterrupted(pending.StreamID())
			}
			pending = nil
		}
	}
	for {
		if inputDone && active == nil {
			_ = output.Close()
			return
		}
		select {
		case <-ctx.Done():
			interrupt()
			_ = input.CloseWithError(context.Cause(ctx))
			_ = output.Close()
			return
		case event, ok := <-events:
			if !ok {
				inputDone = true
				events = nil
				continue
			}
			if event.err != nil {
				interrupt()
				_ = output.CloseWithError(event.err)
				return
			}
			if event.begin {
				interrupt()
			}
			if event.text == nil {
				continue
			}
			interrupt()
			if err := a.history.append(ctx, []flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleUser, *event.text)}, false); err != nil {
				_ = output.CloseWithError(err)
				return
			}
			response, err := output.Begin(ctx)
			if err != nil {
				_ = output.CloseWithError(err)
				return
			}
			if err := response.Push(textChunk("", "")); err != nil {
				_ = output.CloseWithError(err)
				return
			}
			nextID++
			pending = response
			if pulled != nil {
				pulled.track(response.StreamID(), *event.text)
			}
			runCtx, runCancel := context.WithCancelCause(response.Context())
			active = &activeRun{id: nextID, cancel: runCancel, response: response}
			go a.run(runCtx, runCancel, nextID, *event.text, response, runs)
		case result := <-runs:
			if active == nil || active.id != result.id {
				continue
			}
			if result.err != nil && !errors.Is(result.err, context.Canceled) {
				_ = active.response.Fail(result.err.Error())
			} else if result.err == nil {
				_ = active.response.Finish()
			}
			active.cancel(io.EOF)
			active = nil
		}
	}
}

func (a *Agent) run(ctx context.Context, abort context.CancelCauseFunc, id uint64, text string, response *commonagent.Response, results chan<- runResult) {
	sequence := newToolSequencer()
	host := &runHost{response: response, sequence: sequence, publish: a.config.PublishNodes, buffers: make(map[string][]bufferedDelta)}
	runCtx := withSequencer(withTurnState(ctx, a.config.MaxToolCalls, abort), sequence)
	inputs := map[string]any{}
	if a.config.InputProvider != nil {
		provided, err := a.config.InputProvider(runCtx)
		if err != nil {
			sendRunResult(runCtx, results, runResult{id: id, err: fmt.Errorf("agent/flowcraft: provide inputs: %w", err)})
			return
		}
		maps.Copy(inputs, provided)
	}
	seed := flowagent.BoardSeederFunc(func(seedCtx context.Context, _ flowagent.RunInfo, _ *flowagent.Request) (*engine.Board, error) {
		board := engine.NewBoard()
		messages, err := a.history.recent(seedCtx, logstore.MaxLimit)
		if err != nil {
			return nil, err
		}
		if a.config.Memory != nil {
			recalled, err := a.config.Memory.Recall(seedCtx, memory.Query{Text: text, Limit: a.config.MemoryLimit})
			if err != nil {
				return nil, fmt.Errorf("agent/flowcraft: recall memory: %w", err)
			}
			if rendered := renderMemory(recalled); rendered != "" {
				messages = append([]flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleSystem, rendered)}, messages...)
			}
		}
		board.SetChannel(engine.MainChannel, messages)
		for key, value := range inputs {
			board.SetVar(key, value)
		}
		return board, nil
	})
	result, err := flowagent.Run(runCtx, a.agent, a.engine, flowagent.Request{
		ContextID: a.config.Conversation,
		Message:   flowmodel.NewTextMessage(flowmodel.RoleUser, text),
		Inputs:    inputs,
	}, flowagent.WithEngineHost(host), flowagent.WithBoardSeed(seed))
	if err == nil && result != nil && result.Err != nil {
		err = result.Err
	}
	if err == nil && result != nil && host.tokenCount() == 0 && result.Text() != "" {
		err = response.Push(textChunk("", result.Text()))
	}
	sendRunResult(runCtx, results, runResult{id: id, err: err})
}

func sendRunResult(ctx context.Context, results chan<- runResult, result runResult) {
	select {
	case results <- result:
	case <-ctx.Done():
	}
}

func renderMemory(result memory.RecallResult) string {
	var rendered strings.Builder
	for _, match := range result.Matches {
		text := strings.TrimSpace(match.Fact.Text)
		if text == "" {
			continue
		}
		if rendered.Len() == 0 {
			rendered.WriteString("Relevant long-term memory:\n")
		}
		rendered.WriteString("- ")
		rendered.WriteString(text)
		rendered.WriteByte('\n')
	}
	return strings.TrimSpace(rendered.String())
}

type stream struct {
	*commonagent.Output
	cancel context.CancelCauseFunc
	input  genx.Stream
}

func (s *stream) Close() error {
	if s == nil {
		return nil
	}
	s.cancel(io.EOF)
	_ = s.input.Close()
	return s.Output.Close()
}

func (s *stream) CloseWithError(err error) error {
	if s == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.cancel(err)
	_ = s.input.CloseWithError(err)
	return s.Output.CloseWithError(err)
}
