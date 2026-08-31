package apitypes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
)

// UnmarshalJSON keeps the generated Flowcraft shape strict at every public
// JSON boundary. FlowcraftNode is a generated raw union, so its selected
// variant is decoded a second time to reject unknown node and config fields.
func (s *FlowcraftWorkflowSpec) UnmarshalJSON(data []byte) error {
	type flowcraftWorkflowSpec FlowcraftWorkflowSpec
	var decoded flowcraftWorkflowSpec
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return err
	}
	value := FlowcraftWorkflowSpec(decoded)
	if err := value.Validate(); err != nil {
		return err
	}
	*s = value
	return nil
}

// Validate checks a Flowcraft Workflow assembled through generated Go types.
// JSON decoding calls the same validation, so HTTP, YAML, and in-process
// construction share one contract.
func (s FlowcraftWorkflowSpec) Validate() error {
	if s.MaxIterations != nil && *s.MaxIterations < 1 {
		return errors.New("max_iterations must be positive")
	}
	if err := validateFlowcraftGraph(s.Graph); err != nil {
		return fmt.Errorf("graph: %w", err)
	}
	if s.Conversation != nil && s.Conversation.Starts != nil && !s.Conversation.Starts.Valid() {
		return fmt.Errorf("conversation.starts %q is invalid", *s.Conversation.Starts)
	}
	if err := validateFlowcraftMemoryHooks(s.MemoryHooks); err != nil {
		return fmt.Errorf("memory_hooks: %w", err)
	}
	if err := validateFlowcraftVoiceAdapter(s.VoiceAdapter); err != nil {
		return fmt.Errorf("voice_adapter: %w", err)
	}
	return nil
}

func validateFlowcraftMemoryHooks(hooks *FlowcraftMemoryHooks) error {
	if hooks == nil {
		return nil
	}
	if hooks.Context == nil && hooks.Turn == nil {
		return errors.New("context or turn is required")
	}
	if hooks.Context == nil {
		return nil
	}
	contextHook := hooks.Context
	if contextHook.Budget != nil {
		for _, bound := range []struct {
			name  string
			value *int
		}{
			{name: "max_tokens", value: contextHook.Budget.MaxTokens},
			{name: "max_items", value: contextHook.Budget.MaxItems},
			{name: "max_chars", value: contextHook.Budget.MaxChars},
		} {
			if bound.value != nil && *bound.value < 0 {
				return fmt.Errorf("context.budget.%s must be non-negative", bound.name)
			}
		}
	}
	if contextHook.MinScore != nil {
		score := float64(*contextHook.MinScore)
		if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
			return errors.New("context.min_score must be between 0 and 1")
		}
	}
	selected := 0
	if contextHook.Query.Literal != nil && strings.TrimSpace(*contextHook.Query.Literal) != "" {
		selected++
	}
	if contextHook.Query.Board != nil && strings.TrimSpace(*contextHook.Query.Board) != "" {
		selected++
	}
	if contextHook.Query.CurrentMessage != nil && *contextHook.Query.CurrentMessage {
		selected++
	}
	if selected != 1 {
		return errors.New("context.query must select exactly one of literal, board, or current_message")
	}
	if strings.TrimSpace(contextHook.Output) == "" || strings.HasPrefix(contextHook.Output, "__") {
		return errors.New("context.output must be a non-reserved board variable")
	}
	if contextHook.Render != nil {
		if contextHook.Render.MaxChars != nil && *contextHook.Render.MaxChars < 0 {
			return errors.New("context.render.max_chars must be non-negative")
		}
		if strings.TrimSpace(contextHook.Render.Output) == "" || strings.HasPrefix(contextHook.Render.Output, "__") {
			return errors.New("context.render.output must be a non-reserved board variable")
		}
		if contextHook.Render.Output == contextHook.Output {
			return errors.New("context.render.output must differ from context.output")
		}
	}
	return nil
}

func validateFlowcraftGraph(graph FlowcraftGraph) error {
	if strings.TrimSpace(graph.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(graph.Entry) == "" {
		return errors.New("entry is required")
	}
	if len(graph.Nodes) == 0 {
		return errors.New("nodes must not be empty")
	}
	nodes := make(map[string]struct{}, len(graph.Nodes))
	publishers := 0
	for index, raw := range graph.Nodes {
		data, err := raw.MarshalJSON()
		if err != nil {
			return fmt.Errorf("nodes[%d]: %w", index, err)
		}
		var discriminator struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &discriminator); err != nil {
			return fmt.Errorf("nodes[%d]: %w", index, err)
		}
		var id string
		switch discriminator.Type {
		case string(FlowcraftInferenceNodeTypeInference):
			var node FlowcraftInferenceNode
			if err := decodeStrictJSON(data, &node); err != nil {
				return fmt.Errorf("nodes[%d]: %w", index, err)
			}
			id = node.Id
			if node.Config.Model.Id.Provider != FlowcraftInferenceModelIDProviderGizclaw {
				return fmt.Errorf("nodes[%d].config.model.id.provider must be gizclaw", index)
			}
			if err := validateFlowcraftAlias("model.id.name", node.Config.Model.Id.Name); err != nil {
				return fmt.Errorf("nodes[%d].config.%w", index, err)
			}
			if err := validateFlowcraftInferenceIntent(node.Config.Intent); err != nil {
				return fmt.Errorf("nodes[%d].config.intent: %w", index, err)
			}
			if node.Publish != nil && *node.Publish {
				publishers++
			}
		case string(FlowcraftScriptNodeTypeScript):
			var node FlowcraftScriptNode
			if err := decodeStrictJSON(data, &node); err != nil {
				return fmt.Errorf("nodes[%d]: %w", index, err)
			}
			id = node.Id
			if node.Publish != nil && *node.Publish {
				publishers++
			}
			if strings.TrimSpace(node.Config.Source) == "" {
				return fmt.Errorf("nodes[%d].config.source is required", index)
			}
			if node.Config.Runtime != FlowcraftScriptNodeConfigRuntimeJs {
				return fmt.Errorf("nodes[%d].config.runtime must be js", index)
			}
		case string(FlowcraftPassthroughNodeTypePassthrough):
			var node FlowcraftPassthroughNode
			if err := decodeStrictJSON(data, &node); err != nil {
				return fmt.Errorf("nodes[%d]: %w", index, err)
			}
			id = node.Id
			if node.Publish != nil && *node.Publish {
				publishers++
			}
		case string(FlowcraftMemoryRecallNodeTypeMemoryRecall):
			var node FlowcraftMemoryRecallNode
			if err := decodeStrictJSON(data, &node); err != nil {
				return fmt.Errorf("nodes[%d]: %w", index, err)
			}
			id = node.Id
			if strings.TrimSpace(node.Config.Query.TextFrom) == "" {
				return fmt.Errorf("nodes[%d].config.query.text_from is required", index)
			}
			if strings.TrimSpace(node.Config.Output) == "" {
				return fmt.Errorf("nodes[%d].config.output is required", index)
			}
			if node.Config.TopK < 1 {
				return fmt.Errorf("nodes[%d].config.top_k must be positive", index)
			}
			if node.Publish != nil && *node.Publish {
				publishers++
			}
		case string(FlowcraftMemoryObserveNodeTypeMemoryObserve):
			var node FlowcraftMemoryObserveNode
			if err := decodeStrictJSON(data, &node); err != nil {
				return fmt.Errorf("nodes[%d]: %w", index, err)
			}
			id = node.Id
			if len(node.Config.Observations) == 0 {
				return fmt.Errorf("nodes[%d].config.observations must not be empty", index)
			}
			for observationIndex, observation := range node.Config.Observations {
				sources := 0
				if observation.TurnsFrom != nil && strings.TrimSpace(*observation.TurnsFrom) != "" {
					sources++
				}
				if observation.TextFrom != nil && strings.TrimSpace(*observation.TextFrom) != "" {
					sources++
				}
				if observation.Facts != nil && len(*observation.Facts) != 0 {
					sources++
				}
				if sources != 1 {
					return fmt.Errorf("nodes[%d].config.observations[%d] must select exactly one of turns_from, text_from, or facts", index, observationIndex)
				}
			}
			if node.Publish != nil && *node.Publish {
				publishers++
			}
		default:
			return fmt.Errorf("nodes[%d].type %q is unsupported", index, discriminator.Type)
		}
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("nodes[%d].id is required", index)
		}
		if _, exists := nodes[id]; exists {
			return fmt.Errorf("node id %q is duplicated", id)
		}
		nodes[id] = struct{}{}
	}
	if _, exists := nodes[graph.Entry]; !exists {
		return fmt.Errorf("entry %q is not a defined node", graph.Entry)
	}
	if publishers == 0 {
		return errors.New("at least one node must set publish=true")
	}
	for index, edge := range valueOrEmpty(graph.Edges) {
		if _, exists := nodes[edge.From]; !exists {
			return fmt.Errorf("edges[%d].from %q is not a defined node", index, edge.From)
		}
		if edge.To != "__end__" {
			if _, exists := nodes[edge.To]; !exists {
				return fmt.Errorf("edges[%d].to %q is not a defined node", index, edge.To)
			}
		}
	}
	return nil
}

func validateFlowcraftInferenceIntent(intent *FlowcraftInferenceIntent) error {
	if intent == nil {
		return nil
	}
	text := intent.Text
	if text.MaxOutputTokens != nil && *text.MaxOutputTokens < 1 {
		return errors.New("text.max_output_tokens must be positive")
	}
	if text.Temperature != nil && (*text.Temperature < 0 || *text.Temperature > 2) {
		return errors.New("text.temperature must be between 0 and 2")
	}
	if text.TopP != nil && (*text.TopP < 0 || *text.TopP > 1) {
		return errors.New("text.top_p must be between 0 and 1")
	}
	if text.Response == nil {
		return nil
	}
	response := text.Response
	if !response.Kind.Valid() {
		return fmt.Errorf("text.response.kind %q is unsupported", response.Kind)
	}
	switch response.Kind {
	case FlowcraftInferenceResponseFormatKindText, FlowcraftInferenceResponseFormatKindJsonObject:
		if response.Name != nil || response.Schema != nil {
			return fmt.Errorf("text.response kind %q cannot carry name or schema", response.Kind)
		}
	case FlowcraftInferenceResponseFormatKindJsonSchema:
		if response.Name == nil || strings.TrimSpace(*response.Name) == "" || response.Schema == nil {
			return errors.New("text.response json_schema requires name and schema")
		}
	}
	return nil
}

func validateFlowcraftVoiceAdapter(adapter *VoiceAdapter) error {
	if adapter == nil {
		return nil
	}
	aliases := make(map[string]string)
	if adapter.AsrModel != nil {
		aliases["asr_model"] = *adapter.AsrModel
	}
	if adapter.DefaultVoice != nil {
		aliases["default_voice"] = *adapter.DefaultVoice
	}
	if adapter.NodeVoices != nil {
		for node, alias := range *adapter.NodeVoices {
			aliases["node_voices."+node] = alias
		}
	}
	for field, alias := range aliases {
		if err := validateFlowcraftAlias(field, alias); err != nil {
			return err
		}
	}
	return nil
}

func validateFlowcraftAlias(field, alias string) error {
	return runtimealias.Validate(field, alias)
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func valueOrEmpty[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return *value
}
