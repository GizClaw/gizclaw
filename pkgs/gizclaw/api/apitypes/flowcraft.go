package apitypes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	if err := validateFlowcraftVoiceAdapter(s.VoiceAdapter); err != nil {
		return fmt.Errorf("voice_adapter: %w", err)
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
		case string(FlowcraftLLMNodeTypeLlm):
			var node FlowcraftLLMNode
			if err := decodeStrictJSON(data, &node); err != nil {
				return fmt.Errorf("nodes[%d]: %w", index, err)
			}
			id = node.Id
			if strings.TrimSpace(node.Config.Model) == "" {
				return fmt.Errorf("nodes[%d].config.model is required", index)
			}
			if err := validateFlowcraftAlias("model", node.Config.Model); err != nil {
				return fmt.Errorf("nodes[%d].config.%w", index, err)
			}
			if node.Config.MaxTokens != nil && *node.Config.MaxTokens < 1 {
				return fmt.Errorf("nodes[%d].config.max_tokens must be positive", index)
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
