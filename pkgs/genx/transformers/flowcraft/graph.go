package flowcraft

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	flowagent "github.com/GizClaw/flowcraft/core/agent"
	"github.com/GizClaw/flowcraft/core/agent/scriptrt/jsrt"
	"github.com/GizClaw/flowcraft/core/event"
	coregraph "github.com/GizClaw/flowcraft/core/graph"
	flownodes "github.com/GizClaw/flowcraft/core/graph/nodes"
	flowscript "github.com/GizClaw/flowcraft/core/graph/nodes/script"
	memoryhook "github.com/GizClaw/flowcraft/core/memory/hook"
	"github.com/GizClaw/flowcraft/core/message"
	"github.com/GizClaw/flowcraft/core/resource"
)

func buildRuntime(config Config) (flowagent.Agent, flowagent.Engine, error) {
	registry := coregraph.NewRegistry()
	var modelNames []string
	for _, node := range config.Graph.Nodes {
		if node.Type != "inference" {
			continue
		}
		var nodeConfig struct {
			Model struct {
				ID struct {
					Name string `json:"name"`
				} `json:"id"`
			} `json:"model"`
		}
		if err := json.Unmarshal(node.Config, &nodeConfig); err != nil {
			return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: decode inference node %q: %w", node.ID, err)
		}
		modelNames = append(modelNames, nodeConfig.Model.ID.Name)
	}
	assembly, err := newInferenceAssembly(config.Models, config.ToolInvoker, modelNames)
	if err != nil {
		return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: build inference Assembly: %w", err)
	}
	if err := flownodes.RegisterInference(registry, flownodes.InferenceNodeDeps{Assembly: assembly}); err != nil {
		return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: register inference node: %w", err)
	}
	if err := flowscript.Register(registry, flowscript.ScriptNodeDeps{
		Runtimes: map[string]flowagent.ScriptRuntime{"js": jsrt.New()},
	}); err != nil {
		return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: register script node: %w", err)
	}
	if err := registerMemoryNodes(registry, config); err != nil {
		return flowagent.Agent{}, nil, err
	}
	if err := registerMatchNodes(registry, config); err != nil {
		return flowagent.Agent{}, nil, err
	}
	if err := registerPassthroughNode(registry); err != nil {
		return flowagent.Agent{}, nil, err
	}
	options := []coregraph.BuildOption{coregraph.WithParallel(coregraph.ParallelConfig{
		Enabled: true, MaxBranches: 10, MergeStrategy: coregraph.LastWriteWins,
	})}
	if config.MaxIterations > 0 {
		options = append(options, coregraph.WithMaxIterations(config.MaxIterations))
	}
	graphRunner, err := coregraph.Build(&config.Graph, registry, options...)
	if err != nil {
		return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: build Graph: %w", err)
	}
	agentRuntime := flowagent.Agent{
		ID: config.ID,
		Card: flowagent.AgentCard{
			Name: config.Name, Description: config.Description,
			DefaultInputModes: []string{"text/plain"}, DefaultOutputModes: []string{"text/plain"},
			Capabilities: flowagent.AgentCapabilities{Streaming: true},
		},
		Engine: graphRunner,
	}
	if config.Memory != nil {
		assembly := &memoryAssembly{store: config.Memory, tasks: config.asyncTasks}
		deps := map[string]any{"memory": assembly}
		if config.MemoryContext != nil {
			settings := *config.MemoryContext
			settings.Scope = memoryhook.ScopeSettings{
				RuntimeID: config.MemoryScope.AppID, UserID: config.MemoryScope.UserID, AgentID: config.MemoryScope.AgentID,
			}
			data, err := json.Marshal(settings)
			if err != nil {
				return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: encode memory.context hook: %w", err)
			}
			hook, err := (memoryhook.ContextPreparer{}).New(context.Background(), resource.Input{Settings: data, Deps: deps})
			if err != nil {
				return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: build memory.context hook: %w", err)
			}
			preparer, ok := hook.(flowagent.Preparer)
			if !ok {
				return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: memory.context hook has type %T, want agent.Preparer", hook)
			}
			agentRuntime.Prepare = append(agentRuntime.Prepare, preparer)
		}
		if config.MemoryTurn != nil {
			settings := *config.MemoryTurn
			settings.Scope = memoryhook.ScopeSettings{
				RuntimeID: config.MemoryScope.AppID, UserID: config.MemoryScope.UserID, AgentID: config.MemoryScope.AgentID,
			}
			data, err := json.Marshal(settings)
			if err != nil {
				return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: encode memory.turn hook: %w", err)
			}
			hook, err := (memoryhook.TurnCommitter{}).New(context.Background(), resource.Input{Settings: data, Deps: deps})
			if err != nil {
				return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: build memory.turn hook: %w", err)
			}
			committer, ok := hook.(flowagent.Committer)
			if !ok {
				return flowagent.Agent{}, nil, fmt.Errorf("flowcraft: memory.turn hook has type %T, want agent.Committer", hook)
			}
			agentRuntime.Commit = append(agentRuntime.Commit, committer)
		}
	}
	return agentRuntime, graphRunner, nil
}

func registerPassthroughNode(registry *coregraph.Registry) error {
	return coregraph.RegisterType(registry, "passthrough", coregraph.NodeType[struct{}]{
		Handler: func(coregraph.ExecutionContext, *flowagent.Board, struct{}) error { return nil },
	})
}

type bufferedDelta struct {
	nodeID string
	delta  flowagent.StreamDeltaPayload
}

type runHost struct {
	flowagent.NoopHost
	publish map[string]struct{}
	emit    func(string, string) error

	mu       sync.Mutex
	tokens   int
	buffers  map[string][]bufferedDelta
	terminal map[string]struct{}
}

func (h *runHost) Publish(_ context.Context, envelope event.Envelope) error {
	if !flowagent.IsStreamDelta(envelope.Subject) {
		return nil
	}
	delta, err := flowagent.DecodeStreamDelta(envelope)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	switch delta.Type {
	case flowagent.StreamDeltaParallelBranchAccept:
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
	case flowagent.StreamDeltaParallelBranchCancel:
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

func (h *runHost) emitLocked(nodeID string, delta flowagent.StreamDeltaPayload) error {
	if delta.Type != flowagent.StreamDeltaPart || delta.Part == nil {
		return nil
	}
	part, err := message.NormalizePart(delta.Part)
	if err != nil {
		return err
	}
	text, ok := part.(message.TextPart)
	if !ok || text.Text == "" {
		return nil
	}
	if _, ok := h.publish[nodeID]; !ok {
		return nil
	}
	if err := h.emit(nodeID, text.Text); err != nil {
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
