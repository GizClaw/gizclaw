package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/compose"
	flowagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

const assistantLabel = "agent.eino.assistant"

var _ commonagent.Agent = (*Agent)(nil)

type reactAgent interface {
	Stream(context.Context, []*schema.Message, ...flowagent.AgentOption) (*schema.StreamReader[*schema.Message], error)
}

// Agent owns Eino graph execution, automatic Tool continuation, history, and
// the GenX pull-stream lifecycle.
type Agent struct {
	config  Config
	runtime reactAgent
	history *conversationHistory
}

// New constructs an Eino ReAct Agent from resolved runtime dependencies.
func New(ctx context.Context, config Config) (*Agent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if config.Model == nil {
		return nil, fmt.Errorf("agent/eino: model is required")
	}
	if config.Toolkit == nil {
		return nil, fmt.Errorf("agent/eino: toolkit is required")
	}
	if config.MaxSteps < 0 {
		return nil, fmt.Errorf("agent/eino: max steps cannot be negative")
	}
	if config.MaxToolCalls < 0 {
		return nil, fmt.Errorf("agent/eino: max tool calls cannot be negative")
	}
	if config.ToolTimeout < 0 {
		return nil, fmt.Errorf("agent/eino: tool timeout cannot be negative")
	}
	if config.MemoryLimit < 0 {
		return nil, fmt.Errorf("agent/eino: memory limit cannot be negative")
	}
	if config.Memory != nil && config.MemoryLimit == 0 {
		config.MemoryLimit = 8
	}
	if config.MaxSteps == 0 {
		config.MaxSteps = defaultMaxSteps
	}
	persistent, err := newHistory(config.History)
	if err != nil {
		return nil, err
	}
	history := &conversationHistory{store: persistent}
	tools, err := nativeTools(config.Toolkit, history, config.ToolTimeout)
	if err != nil {
		return nil, err
	}
	runtime, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: config.Model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools:               tools,
			ExecuteSequentially: true,
		},
		MaxStep: config.MaxSteps,
	})
	if err != nil {
		return nil, fmt.Errorf("agent/eino: build ReAct graph: %w", err)
	}
	return &Agent{config: config, runtime: runtime, history: history}, nil
}

// Transform consumes GenX user turns and streams pull-visible assistant output.
func (a *Agent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if a == nil || a.runtime == nil {
		return nil, fmt.Errorf("agent/eino: agent is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("agent/eino: input stream is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeCtx, cancel := context.WithCancelCause(ctx)
	pulled := newPulledHistory(a.history, a.config.Memory, a.config.OnBackgroundError)
	outputConfig := a.config.Output
	configuredObserver := outputConfig.Observe
	outputConfig.Observe = func(chunk *genx.MessageChunk) {
		pulled.observe(chunk)
		if configuredObserver != nil {
			configuredObserver(chunk)
		}
	}
	output := commonagent.NewOutput(outputConfig)
	events := make(chan inputEvent)
	runs := make(chan runResult, 8)

	go a.readInput(runtimeCtx, input, events)
	go a.coordinate(runtimeCtx, cancel, input, output, pulled, events, runs)
	return &stream{Output: output, cancel: cancel, input: input}, nil
}

// History returns defensive copies of the recent ordered conversation for
// product-owned status and history APIs.
func (a *Agent) History(ctx context.Context) ([]*schema.Message, error) {
	if a == nil || a.history == nil {
		return nil, fmt.Errorf("agent/eino: agent is nil")
	}
	return a.history.recent(ctx)
}

type inputEvent struct {
	beginID string
	turn    *inputTurn
	err     error
}

type inputTurn struct {
	id      string
	content string
}

func (a *Agent) readInput(ctx context.Context, input genx.Stream, events chan<- inputEvent) {
	defer close(events)
	var streamID string
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
	begin := func(id string) bool {
		streamID = strings.TrimSpace(id)
		if streamID == "" {
			streamID = genx.NewStreamID()
		}
		content.Reset()
		begun = true
		return send(inputEvent{beginID: streamID})
	}

	for {
		chunk, err := input.Next()
		if err != nil {
			if commonagent.IsStreamEnd(err) {
				if begun && content.Len() > 0 {
					send(inputEvent{turn: &inputTurn{id: streamID, content: content.String()}})
				}
				return
			}
			send(inputEvent{err: err})
			return
		}
		if chunk == nil {
			continue
		}
		chunkID := ""
		if chunk.Ctrl != nil {
			chunkID = chunk.Ctrl.StreamID
		}
		if chunk.IsBeginOfStream() {
			if !begin(chunkID) {
				return
			}
		}
		if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
			if !begun || (chunkID != "" && chunkID != streamID) {
				if !begin(chunkID) {
					return
				}
			}
			content.WriteString(string(text))
		}
		if chunk.IsEndOfStream() && begun {
			if !send(inputEvent{turn: &inputTurn{id: streamID, content: content.String()}}) {
				return
			}
			begun = false
			streamID = ""
			content.Reset()
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

func (a *Agent) coordinate(
	ctx context.Context,
	cancel context.CancelCauseFunc,
	input genx.Stream,
	output *commonagent.Output,
	pulled *pulledHistory,
	events <-chan inputEvent,
	runs chan runResult,
) {
	defer cancel(io.EOF)
	var active *activeRun
	var pending *commonagent.Response
	var nextRunID uint64
	inputDone := false

	interrupt := func() {
		if active != nil {
			active.cancel(errors.New(commonagent.Interrupted))
			active = nil
		}
		if pending != nil {
			_ = pending.Interrupt()
			if pending.Interrupted() {
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
			if event.beginID != "" {
				interrupt()
			}
			if event.turn == nil {
				continue
			}
			interrupt()
			if err := a.history.append(ctx, schema.UserMessage(event.turn.content), false); err != nil {
				_ = output.CloseWithError(err)
				return
			}
			messages, err := a.history.recent(ctx)
			if err != nil {
				_ = output.CloseWithError(err)
				return
			}
			if a.config.Memory != nil {
				recalled, err := a.config.Memory.Recall(ctx, memory.Query{Text: event.turn.content, Limit: a.config.MemoryLimit})
				if err != nil {
					_ = output.CloseWithError(fmt.Errorf("agent/eino: recall memory: %w", err))
					return
				}
				if rendered := renderMemory(recalled); rendered != "" {
					messages = append([]*schema.Message{schema.SystemMessage(rendered)}, messages...)
				}
			}
			if prompt := strings.TrimSpace(a.config.SystemPrompt); prompt != "" {
				messages = append([]*schema.Message{schema.SystemMessage(prompt)}, messages...)
			}
			response, err := output.Begin(ctx)
			if err != nil {
				_ = output.CloseWithError(err)
				return
			}
			if err := response.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{Label: assistantLabel},
			}); err != nil {
				_ = output.CloseWithError(err)
				return
			}
			nextRunID++
			pending = response
			pulled.track(response.StreamID(), event.turn.content)
			runCtx, runCancel := context.WithCancelCause(response.Context())
			active = &activeRun{id: nextRunID, cancel: runCancel, response: response}
			go a.run(withToolBudget(runCtx, a.config.MaxToolCalls), nextRunID, response, messages, runs)
		case result := <-runs:
			if active == nil || result.id != active.id {
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

func (a *Agent) run(ctx context.Context, id uint64, response *commonagent.Response, messages []*schema.Message, results chan<- runResult) {
	stream, err := a.runtime.Stream(ctx, messages)
	if err == nil {
		defer stream.Close()
		for {
			var message *schema.Message
			message, err = stream.Recv()
			if errors.Is(err, io.EOF) {
				err = nil
				break
			}
			if err != nil {
				break
			}
			if message == nil || message.Content == "" {
				continue
			}
			if pushErr := response.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text(message.Content),
				Ctrl: &genx.StreamCtrl{Label: assistantLabel},
			}); pushErr != nil {
				err = pushErr
				break
			}
		}
	}
	select {
	case results <- runResult{id: id, err: err}:
	case <-ctx.Done():
	}
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
