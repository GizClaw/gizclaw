package flowcraft

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	flowagent "github.com/GizClaw/flowcraft/sdk/agent"
	"github.com/GizClaw/flowcraft/sdk/engine"
	"github.com/GizClaw/flowcraft/sdk/event"
	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/flowcraft/sdk/graph/node"
	"github.com/GizClaw/flowcraft/sdk/graph/node/knowledgenode"
	"github.com/GizClaw/flowcraft/sdk/graph/node/llmnode"
	"github.com/GizClaw/flowcraft/sdk/graph/node/scriptnode"
	"github.com/GizClaw/flowcraft/sdk/graph/runner"
	"github.com/GizClaw/flowcraft/sdk/script/jsrt"
	flowtool "github.com/GizClaw/flowcraft/sdk/tool"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func buildGraph(config Config, tools []string, registry *flowtool.Registry) (flowagent.Agent, engine.Engine, error) {
	graphDefinition, err := ownedGraph(config.Graph, tools)
	if err != nil {
		return flowagent.Agent{}, nil, err
	}
	factory := node.NewFactory()
	llmnode.Register(factory, config.Resolver, registry)
	knowledgenode.Register(factory, nil)
	scriptnode.Register(factory, scriptnode.Deps{ScriptRuntime: jsrt.New(), Workspace: config.Workspace})
	var options []runner.Option
	if config.MaxIterations > 0 {
		options = append(options, runner.WithMaxIterations(config.MaxIterations))
	}
	if config.Parallel.Enabled {
		options = append(options, runner.WithParallel(config.Parallel))
	}
	graphRunner, err := runner.New(&graphDefinition, factory, options...)
	if err != nil {
		return flowagent.Agent{}, nil, fmt.Errorf("agent/flowcraft: build graph: %w", err)
	}
	return flowagent.Agent{ID: config.ID, Tools: tools}, graphRunner, nil
}

func ownedGraph(source flowgraph.GraphDefinition, tools []string) (flowgraph.GraphDefinition, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return flowgraph.GraphDefinition{}, fmt.Errorf("agent/flowcraft: clone graph: %w", err)
	}
	var owned flowgraph.GraphDefinition
	if err := json.Unmarshal(data, &owned); err != nil {
		return flowgraph.GraphDefinition{}, fmt.Errorf("agent/flowcraft: clone graph: %w", err)
	}
	for index := range owned.Nodes {
		node := &owned.Nodes[index]
		if node.Type != "llm" {
			continue
		}
		if node.Config == nil {
			node.Config = make(map[string]any)
		}
		if _, configured := node.Config["tool_names"]; !configured {
			node.Config["tool_names"] = slices.Clone(tools)
		}
	}
	return owned, nil
}

// runHost owns one graph run's stream routing and ordered ToolCall sequence.
type runHost struct {
	engine.NoopHost
	response *commonagent.Response
	sequence *toolSequencer
	publish  map[string]bool

	mu      sync.Mutex
	tokens  int
	buffers map[string][]bufferedDelta
}

type bufferedDelta struct {
	nodeID string
	delta  engine.StreamDeltaPayload
}

func (h *runHost) Publish(_ context.Context, envelope event.Envelope) error {
	if !engine.IsStreamDelta(envelope.Subject) {
		return nil
	}
	delta, err := engine.DecodeStreamDelta(envelope)
	if err != nil {
		return err
	}
	if delta.Type == engine.StreamDeltaToolCall {
		if err := h.sequence.record(delta.ID); err != nil {
			return err
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if delta.Speculative && delta.ForkID != "" && delta.BranchID != "" {
		key := delta.ForkID + "\x00" + delta.BranchID
		h.buffers[key] = append(h.buffers[key], bufferedDelta{nodeID: envelope.NodeID(), delta: delta})
		return nil
	}
	switch delta.Type {
	case engine.StreamDeltaParallelBranchAccept:
		key := delta.ForkID + "\x00" + delta.BranchID
		for _, buffered := range h.buffers[key] {
			h.publishLocked(buffered.nodeID, buffered.delta)
		}
		delete(h.buffers, key)
	case engine.StreamDeltaParallelBranchCancel:
		delete(h.buffers, delta.ForkID+"\x00"+delta.BranchID)
	default:
		h.publishLocked(envelope.NodeID(), delta)
	}
	return nil
}

func (h *runHost) publishLocked(nodeID string, delta engine.StreamDeltaPayload) {
	if delta.Type != engine.StreamDeltaToken || delta.Content == "" {
		return
	}
	if h.publish != nil && !h.publish[nodeID] {
		return
	}
	if err := h.response.Push(textChunk(nodeID, delta.Content)); err == nil {
		h.tokens++
	}
}

func (h *runHost) tokenCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tokens
}

func textChunk(name, content string) *genx.MessageChunk {
	return &genx.MessageChunk{Role: genx.RoleModel, Name: name, Part: genx.Text(content), Ctrl: &genx.StreamCtrl{Label: assistantLabel}}
}
