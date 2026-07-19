// Package doubaorealtime implements the Tool-capable Doubao Realtime Agent.
package doubaorealtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/doubao-speech-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
)

var _ commonagent.Agent = (*Agent)(nil)

// Agent owns Doubao dialogue turns and the automatic Toolkit continuation.
type Agent struct {
	config Config
	tools  []doubaospeech.RealtimeDuplexFunctionTool
}

// New constructs a Tool-capable Doubao Realtime Agent.
func New(config Config) (*Agent, error) {
	if config.Transformer == nil {
		return nil, fmt.Errorf("agent/doubaorealtime: transformer is required")
	}
	config.Pattern = strings.TrimSpace(config.Pattern)
	if config.Pattern == "" {
		return nil, fmt.Errorf("agent/doubaorealtime: pattern is required")
	}
	config.Model = strings.TrimSpace(config.Model)
	if config.Model == "" {
		config.Model = Model
	}
	if config.Model != Model {
		return nil, fmt.Errorf("agent/doubaorealtime: model %q does not support the required function-call contract; want %q", config.Model, Model)
	}
	if config.Toolkit == nil {
		return nil, fmt.Errorf("agent/doubaorealtime: toolkit is required")
	}
	if config.MaxToolCalls < 0 {
		return nil, fmt.Errorf("agent/doubaorealtime: max tool calls cannot be negative")
	}
	if config.ToolTimeout < 0 {
		return nil, fmt.Errorf("agent/doubaorealtime: tool timeout cannot be negative")
	}
	tools, err := providerTools(config.Toolkit.Tools())
	if err != nil {
		return nil, err
	}
	return &Agent{config: config, tools: tools}, nil
}

// Transform starts one bidirectional Agent session using the configured model,
// Toolkit, and provider transformer.
func (a *Agent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if a == nil {
		return nil, fmt.Errorf("agent/doubaorealtime: agent is nil")
	}
	runtime := transformers.DoubaoRealtimeDuplexCtxOptions{
		Model:        a.config.Model,
		Tools:        append([]doubaospeech.RealtimeDuplexFunctionTool(nil), a.tools...),
		MaxToolCalls: a.config.MaxToolCalls,
		FunctionCallHandler: func(callCtx context.Context, calls []doubaospeech.RealtimeDuplexFunctionCall) ([]doubaospeech.RealtimeDuplexFunctionCallOutput, error) {
			return a.invoke(callCtx, calls)
		},
	}
	output, err := a.config.Transformer.Transform(
		transformers.WithDoubaoRealtimeDuplexCtxOptions(ctx, runtime),
		a.config.Pattern,
		input,
	)
	if err != nil {
		return nil, err
	}
	return &responseStream{Stream: output, ids: make(map[string]string)}, nil
}

func (a *Agent) invoke(ctx context.Context, calls []doubaospeech.RealtimeDuplexFunctionCall) ([]doubaospeech.RealtimeDuplexFunctionCallOutput, error) {
	commonCalls := make([]commonagent.ToolCall, 0, len(calls))
	for _, call := range calls {
		commonCalls = append(commonCalls, commonagent.ToolCall{
			ID:        call.CallID,
			Name:      call.Name,
			Arguments: []byte(call.Arguments),
		})
	}
	results, err := commonagent.InvokeToolCalls(ctx, a.config.Toolkit, commonCalls, commonagent.ToolLoopConfig{
		MaxCalls: a.config.MaxToolCalls,
		Timeout:  a.config.ToolTimeout,
	})
	if err != nil {
		return nil, err
	}
	outputs := make([]doubaospeech.RealtimeDuplexFunctionCallOutput, 0, len(results))
	for _, result := range results {
		outputs = append(outputs, doubaospeech.RealtimeDuplexFunctionCallOutput{
			CallID: result.ID,
			Output: string(result.Content),
		})
	}
	return outputs, nil
}

type responseStream struct {
	genx.Stream
	ids      map[string]string
	activeID string
	terminal bool
}

func (s *responseStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if chunk == nil || chunk.Role != genx.RoleModel {
		return chunk, err
	}
	owned := chunk.Clone()
	if owned.Ctrl == nil {
		owned.Ctrl = &genx.StreamCtrl{}
	}
	if s.activeID == "" || (s.terminal && !owned.IsEndOfStream()) {
		s.activeID = genx.NewStreamID()
		s.terminal = false
		clear(s.ids)
	}
	providerID := strings.TrimSpace(owned.Ctrl.StreamID)
	streamID := s.ids[providerID]
	if streamID == "" {
		streamID = s.activeID
		s.ids[providerID] = streamID
	}
	owned.Ctrl.StreamID = streamID
	if owned.IsEndOfStream() {
		s.terminal = true
	}
	return owned, err
}
