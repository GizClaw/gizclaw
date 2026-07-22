package apitypes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var flowcraftRuntimeAliasPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

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
	if strings.TrimSpace(s.Agent.Id) == "" {
		return errors.New("agent.id is required")
	}
	if strings.TrimSpace(s.Agent.Name) == "" {
		return errors.New("agent.name is required")
	}
	if s.Agent.MaxIterations != nil && *s.Agent.MaxIterations < 1 {
		return errors.New("agent.max_iterations must be positive")
	}
	if err := validateFlowcraftGraph(s.Agent.Graph); err != nil {
		return fmt.Errorf("agent.graph: %w", err)
	}
	if s.Conversation != nil && s.Conversation.Starts != nil && !s.Conversation.Starts.Valid() {
		return fmt.Errorf("conversation.starts %q is invalid", *s.Conversation.Starts)
	}
	if err := validateFlowcraftMemory(s.Memory); err != nil {
		return fmt.Errorf("memory: %w", err)
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

func validateFlowcraftMemory(memory *FlowcraftMemory) error {
	if memory == nil || !memory.Enabled {
		return nil
	}
	if memory.Extract != nil && (memory.Extract.Enabled == nil || *memory.Extract.Enabled) {
		if memory.Extract.Model == nil || strings.TrimSpace(*memory.Extract.Model) == "" {
			return errors.New("extract.model is required when extraction is enabled")
		}
		if memory.Extract.Mode != nil && !memory.Extract.Mode.Valid() {
			return fmt.Errorf("extract.mode %q is invalid", *memory.Extract.Mode)
		}
	}
	if memory.Extract != nil && memory.Extract.Model != nil && strings.TrimSpace(*memory.Extract.Model) != "" {
		if err := validateFlowcraftAlias("extract.model", *memory.Extract.Model); err != nil {
			return err
		}
	}
	if memory.Embedding != nil && memory.Embedding.Enabled != nil && *memory.Embedding.Enabled &&
		(memory.Embedding.Model == nil || strings.TrimSpace(*memory.Embedding.Model) == "") {
		return errors.New("embedding.model is required when embedding is enabled")
	}
	if memory.Embedding != nil && memory.Embedding.Model != nil && strings.TrimSpace(*memory.Embedding.Model) != "" {
		if err := validateFlowcraftAlias("embedding.model", *memory.Embedding.Model); err != nil {
			return err
		}
	}
	if memory.Rerank != nil && memory.Rerank.Enabled != nil && *memory.Rerank.Enabled &&
		(memory.Rerank.Model == nil || strings.TrimSpace(*memory.Rerank.Model) == "") {
		return errors.New("rerank.model is required when rerank is enabled")
	}
	if memory.Rerank != nil && memory.Rerank.Model != nil && strings.TrimSpace(*memory.Rerank.Model) != "" {
		if err := validateFlowcraftAlias("rerank.model", *memory.Rerank.Model); err != nil {
			return err
		}
	}
	if memory.Write != nil && memory.Write.Mode != nil && !memory.Write.Mode.Valid() {
		return fmt.Errorf("write.mode %q is invalid", *memory.Write.Mode)
	}
	if memory.Write != nil && memory.Write.Tier != nil && !memory.Write.Tier.Valid() {
		return fmt.Errorf("write.tier %q is invalid", *memory.Write.Tier)
	}
	if memory.Write != nil && memory.Write.BoardFacts != nil {
		for index, fact := range *memory.Write.BoardFacts {
			if strings.TrimSpace(fact.BoardVar) == "" {
				return fmt.Errorf("write.board_facts[%d].board_var is required", index)
			}
		}
	}
	if memory.Recall != nil && memory.Recall.Profiles != nil {
		for name, profile := range *memory.Recall.Profiles {
			if strings.TrimSpace(name) == "" || strings.TrimSpace(profile.Output) == "" || profile.TopK < 1 {
				return fmt.Errorf("recall.profiles[%q] requires output and a positive top_k", name)
			}
		}
	}
	return nil
}

func validateFlowcraftVoiceAdapter(adapter *FlowcraftVoiceAdapter) error {
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
		if strings.TrimSpace(alias) == "" {
			continue
		}
		if err := validateFlowcraftAlias(field, alias); err != nil {
			return err
		}
	}
	return nil
}

func validateFlowcraftAlias(field, alias string) error {
	if len(alias) > 63 || !flowcraftRuntimeAliasPattern.MatchString(alias) {
		return fmt.Errorf("%s must be a 1-63 character lowercase kebab-case RuntimeProfile alias", field)
	}
	return nil
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
