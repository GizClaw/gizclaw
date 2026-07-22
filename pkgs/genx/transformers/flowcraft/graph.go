package flowcraft

import (
	"context"
	"fmt"
	"sync"

	flowagent "github.com/GizClaw/flowcraft/sdk/agent"
	"github.com/GizClaw/flowcraft/sdk/engine"
	"github.com/GizClaw/flowcraft/sdk/event"
	"github.com/GizClaw/flowcraft/sdk/graph/node"
	"github.com/GizClaw/flowcraft/sdk/graph/node/llmnode"
	"github.com/GizClaw/flowcraft/sdk/graph/node/scriptnode"
	"github.com/GizClaw/flowcraft/sdk/graph/runner"
	"github.com/GizClaw/flowcraft/sdk/script/jsrt"
)

func buildRuntime(config Config) (flowagent.Agent, engine.Engine, error) {
	factory := node.NewFactory()
	llmnode.Register(factory, &modelResolver{generator: config.Models}, nil)
	// Inline scripts are supported, while a nil Workspace deliberately leaves
	// filesystem operations unavailable.
	scriptnode.Register(factory, scriptnode.Deps{ScriptRuntime: jsrt.New()})
	// Supply complete values because Flowcraft's default-filling closure mutates
	// its captured config on first use and is therefore not concurrency-safe.
	options := []runner.Option{runner.WithParallel(runner.ParallelConfig{
		Enabled: true, MaxBranches: 10, MaxNesting: 3, MergeStrategy: runner.MergeLastWins,
	})}
	if config.MaxIterations > 0 {
		options = append(options, runner.WithMaxIterations(config.MaxIterations))
	}
	graphRunner, err := runner.New(&config.Graph, factory, options...)
	if err != nil {
		return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: build Graph: %w", err)
	}
	return flowagent.Agent{
		ID: config.ID,
		Card: flowagent.AgentCard{
			Name: config.Name, Description: config.Description,
			DefaultInputModes: []string{"text/plain"}, DefaultOutputModes: []string{"text/plain"},
			Capabilities: flowagent.AgentCapabilities{Streaming: true},
		},
	}, graphRunner, nil
}

type bufferedDelta struct {
	nodeID string
	delta  engine.StreamDeltaPayload
}

type runHost struct {
	engine.NoopHost
	publish map[string]struct{}
	emit    func(string, string) error

	mu       sync.Mutex
	tokens   int
	buffers  map[string][]bufferedDelta
	terminal map[string]struct{}
}

func (h *runHost) Publish(_ context.Context, envelope event.Envelope) error {
	if !engine.IsStreamDelta(envelope.Subject) {
		return nil
	}
	delta, err := engine.DecodeStreamDelta(envelope)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	switch delta.Type {
	case engine.StreamDeltaParallelBranchAccept:
		key := delta.ForkID + "\x00" + delta.BranchID
		if _, done := h.terminal[key]; done {
			return nil
		}
		for _, buffered := range h.buffers[key] {
			if err := h.emitLocked(buffered.nodeID, buffered.delta); err != nil {
				return err
			}
		}
		delete(h.buffers, key)
		h.terminal[key] = struct{}{}
	case engine.StreamDeltaParallelBranchCancel:
		key := delta.ForkID + "\x00" + delta.BranchID
		delete(h.buffers, key)
		h.terminal[key] = struct{}{}
	default:
		if delta.Speculative && delta.ForkID != "" && delta.BranchID != "" {
			key := delta.ForkID + "\x00" + delta.BranchID
			if _, done := h.terminal[key]; done {
				return nil
			}
			h.buffers[key] = append(h.buffers[key], bufferedDelta{nodeID: envelope.NodeID(), delta: delta})
			return nil
		}
		return h.emitLocked(envelope.NodeID(), delta)
	}
	return nil
}

func (h *runHost) emitLocked(nodeID string, delta engine.StreamDeltaPayload) error {
	if delta.Type != engine.StreamDeltaToken || delta.Content == "" {
		return nil
	}
	if _, ok := h.publish[nodeID]; !ok {
		return nil
	}
	if err := h.emit(nodeID, delta.Content); err != nil {
		return err
	}
	h.tokens++
	return nil
}

func (h *runHost) tokenCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tokens
}
