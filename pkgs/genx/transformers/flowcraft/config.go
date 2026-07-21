package flowcraft

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// Config declares one reusable Flowcraft-backed Transformer.
type Config struct {
	// ID is the stable Flowcraft Agent identity.
	ID string
	// Name is copied to Agent metadata and GenX output chunks.
	Name string
	// Description is copied to the Flowcraft Agent card.
	Description string

	// Graph is the required Flowcraft Graph definition.
	Graph flowgraph.GraphDefinition
	// MaxIterations overrides the SDK default when positive.
	MaxIterations int
	// PublishNodes is the required allow-list of node IDs exposed as output.
	PublishNodes []string
	// Models resolves every LLM node alias through model/<alias>.
	Models genx.Generator

	// History stores ordered conversation messages. The caller owns its lifecycle.
	History logstore.MutableStore
	// Memory stores provider-neutral long-term facts. The caller owns its lifecycle.
	Memory memory.Store
	// State stores serializable Board variables. The caller owns its lifecycle.
	State kv.Store

	// MemoryScope is the fixed opaque scope used by every Recall and Observe.
	MemoryScope memory.Scope

	// RecallProfiles populate Board variables before each Graph run.
	RecallProfiles []MemoryRecallProfile
	// RecallRenderer overrides DefaultRecallRenderer when non-nil.
	RecallRenderer RecallRenderer

	// ObserveEnabled submits completed pull-visible turns to Memory.
	ObserveEnabled bool
	// ObservationBuilder overrides DefaultObservationBuilder when non-nil.
	ObservationBuilder ObservationBuilder
	// ObserveWaitForCompletion waits for pending Memory operations before EOS.
	ObserveWaitForCompletion bool
}

// MemoryRecallProfile recalls long-term memory into one Board variable.
type MemoryRecallProfile struct {
	// BoardVariable receives the rendered recall result.
	BoardVariable string
	// Limit is the positive maximum number of matches requested.
	Limit int
	// Filters are passed unchanged to memory.Store.Recall.
	Filters []memory.Filter
}

// ObservationInput is the completed, downstream-delivered turn presented to
// an ObservationBuilder.
type ObservationInput struct {
	// StreamID identifies the generated response.
	StreamID string
	// UserText is the complete input turn.
	UserText string
	// DeliveredAssistantText is only the text delivered through downstream Next calls.
	DeliveredAssistantText string
	// BoardVariables is a defensive copy of the serializable, non-transient final Board state.
	BoardVariables map[string]any
	// Interrupted reports that an unpulled suffix was discarded.
	Interrupted bool
}

// ObservationBuilder converts one delivered turn into memory extraction input.
type ObservationBuilder func(context.Context, ObservationInput) (memory.Observation, error)

// RecallRenderer converts ordered provider-neutral memory matches into a Board value.
type RecallRenderer func(context.Context, []memory.Match) (string, error)

// DefaultObservationBuilder preserves the user and delivered assistant text.
func DefaultObservationBuilder(_ context.Context, input ObservationInput) (memory.Observation, error) {
	turns := []memory.Turn{{ID: input.StreamID + ":user", Role: memory.RoleUser, Text: input.UserText}}
	if strings.TrimSpace(input.DeliveredAssistantText) != "" {
		turns = append(turns, memory.Turn{ID: input.StreamID + ":assistant", Role: memory.RoleAssistant, Text: input.DeliveredAssistantText})
	}
	return memory.Observation{ID: input.StreamID, Turns: turns}, nil
}

// DefaultRecallRenderer renders recalled facts as a short system context block.
func DefaultRecallRenderer(_ context.Context, matches []memory.Match) (string, error) {
	var result strings.Builder
	for _, match := range matches {
		text := strings.TrimSpace(match.Fact.Text)
		if text == "" {
			continue
		}
		if result.Len() == 0 {
			result.WriteString("Relevant memory:\n")
		}
		result.WriteString("- ")
		result.WriteString(text)
		result.WriteByte('\n')
	}
	return strings.TrimSpace(result.String()), nil
}

func normalizeConfig(source Config) (Config, error) {
	config := source
	config.ID = strings.TrimSpace(config.ID)
	config.Name = strings.TrimSpace(config.Name)
	config.MemoryScope = memory.Scope(strings.TrimSpace(string(config.MemoryScope)))
	if config.ID == "" {
		return Config{}, fmt.Errorf("flowcraft: ID is required")
	}
	if config.Models == nil {
		return Config{}, fmt.Errorf("flowcraft: Models is required")
	}
	if config.MaxIterations < 0 {
		return Config{}, fmt.Errorf("flowcraft: MaxIterations cannot be negative")
	}
	if err := config.Graph.Validate(); err != nil {
		return Config{}, fmt.Errorf("flowcraft: invalid Graph: %w", err)
	}
	data, err := json.Marshal(config.Graph)
	if err != nil {
		return Config{}, fmt.Errorf("flowcraft: clone Graph: %w", err)
	}
	if err := json.Unmarshal(data, &config.Graph); err != nil {
		return Config{}, fmt.Errorf("flowcraft: clone Graph: %w", err)
	}
	nodes := make(map[string]struct{}, len(config.Graph.Nodes))
	for _, node := range config.Graph.Nodes {
		nodes[node.ID] = struct{}{}
		switch node.Type {
		case "llm":
			modelAlias, _ := node.Config["model"].(string)
			modelAlias = strings.TrimSpace(modelAlias)
			if modelAlias == "" {
				return Config{}, fmt.Errorf("flowcraft: LLM node %q requires model alias", node.ID)
			}
			if !strings.Contains(modelAlias, "${") && strings.Contains(modelAlias, "/") {
				return Config{}, fmt.Errorf("flowcraft: LLM node %q model must be an alias, got %q", node.ID, modelAlias)
			}
		case "script":
			source, _ := node.Config["source"].(string)
			if strings.TrimSpace(source) == "" {
				return Config{}, fmt.Errorf("flowcraft: script node %q requires inline source", node.ID)
			}
		default:
			return Config{}, fmt.Errorf("flowcraft: unsupported node type %q for node %q", node.Type, node.ID)
		}
	}
	if len(config.PublishNodes) == 0 {
		return Config{}, fmt.Errorf("flowcraft: PublishNodes is required")
	}
	seen := make(map[string]struct{}, len(config.PublishNodes))
	config.PublishNodes = make([]string, 0, len(source.PublishNodes))
	for _, nodeID := range source.PublishNodes {
		nodeID = strings.TrimSpace(nodeID)
		if _, ok := nodes[nodeID]; !ok {
			return Config{}, fmt.Errorf("flowcraft: PublishNodes contains unknown node %q", nodeID)
		}
		if _, duplicate := seen[nodeID]; duplicate {
			continue
		}
		seen[nodeID] = struct{}{}
		config.PublishNodes = append(config.PublishNodes, nodeID)
	}
	if config.Memory == nil {
		if config.MemoryScope != "" || len(config.RecallProfiles) != 0 || config.ObserveEnabled || config.ObserveWaitForCompletion {
			return Config{}, fmt.Errorf("flowcraft: Memory settings require Memory")
		}
	} else if config.MemoryScope == "" {
		return Config{}, fmt.Errorf("flowcraft: MemoryScope is required when Memory is configured")
	}
	if config.ObserveWaitForCompletion {
		if !config.ObserveEnabled {
			return Config{}, fmt.Errorf("flowcraft: ObserveWaitForCompletion requires ObserveEnabled")
		}
		if _, ok := config.Memory.(memory.OperationWaiter); !ok {
			return Config{}, fmt.Errorf("flowcraft: ObserveWaitForCompletion requires memory.OperationWaiter")
		}
	}
	config.RecallProfiles = append([]MemoryRecallProfile(nil), source.RecallProfiles...)
	recallVariables := make(map[string]struct{}, len(config.RecallProfiles))
	for index := range config.RecallProfiles {
		profile := &config.RecallProfiles[index]
		profile.BoardVariable = strings.TrimSpace(profile.BoardVariable)
		profile.Filters = append([]memory.Filter(nil), profile.Filters...)
		for filterIndex := range profile.Filters {
			value, err := cloneConfigValue(profile.Filters[filterIndex].Value)
			if err != nil {
				return Config{}, fmt.Errorf("flowcraft: clone RecallProfiles[%d].Filters[%d]: %w", index, filterIndex, err)
			}
			profile.Filters[filterIndex].Value = value
		}
		if profile.BoardVariable == "" || profile.Limit <= 0 {
			return Config{}, fmt.Errorf("flowcraft: RecallProfiles[%d] requires BoardVariable and positive Limit", index)
		}
		if _, duplicate := recallVariables[profile.BoardVariable]; duplicate {
			return Config{}, fmt.Errorf("flowcraft: RecallProfiles contains duplicate BoardVariable %q", profile.BoardVariable)
		}
		recallVariables[profile.BoardVariable] = struct{}{}
		if err := memory.ValidateQuery(memory.Query{Scope: config.MemoryScope, Text: "validation", Limit: profile.Limit, Filters: profile.Filters}); err != nil {
			return Config{}, fmt.Errorf("flowcraft: invalid RecallProfiles[%d]: %w", index, err)
		}
	}
	if config.RecallRenderer == nil {
		config.RecallRenderer = DefaultRecallRenderer
	}
	if config.ObservationBuilder == nil {
		config.ObservationBuilder = DefaultObservationBuilder
	}
	return config, nil
}

func cloneConfigValue(source any) (any, error) {
	if source == nil {
		return nil, nil
	}
	cloned, err := cloneConfigReflect(reflect.ValueOf(source))
	if err != nil {
		return nil, err
	}
	return cloned.Interface(), nil
}

func cloneConfigReflect(source reflect.Value) (reflect.Value, error) {
	switch source.Kind() {
	case reflect.Interface:
		cloned, err := cloneConfigReflect(source.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		result := reflect.New(source.Type()).Elem()
		result.Set(cloned)
		return result, nil
	case reflect.Pointer:
		if source.IsNil() {
			return reflect.Zero(source.Type()), nil
		}
		cloned, err := cloneConfigReflect(source.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		result := reflect.New(source.Type().Elem())
		result.Elem().Set(cloned)
		return result, nil
	case reflect.Slice:
		if source.IsNil() {
			return reflect.Zero(source.Type()), nil
		}
		result := reflect.MakeSlice(source.Type(), source.Len(), source.Len())
		for index := range source.Len() {
			cloned, err := cloneConfigReflect(source.Index(index))
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(index).Set(cloned)
		}
		return result, nil
	case reflect.Map:
		if source.IsNil() {
			return reflect.Zero(source.Type()), nil
		}
		result := reflect.MakeMapWithSize(source.Type(), source.Len())
		iterator := source.MapRange()
		for iterator.Next() {
			cloned, err := cloneConfigReflect(iterator.Value())
			if err != nil {
				return reflect.Value{}, err
			}
			result.SetMapIndex(iterator.Key(), cloned)
		}
		return result, nil
	case reflect.Array:
		result := reflect.New(source.Type()).Elem()
		for index := range source.Len() {
			cloned, err := cloneConfigReflect(source.Index(index))
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(index).Set(cloned)
		}
		return result, nil
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return reflect.Value{}, fmt.Errorf("unsupported value type %s", source.Type())
	default:
		return source, nil
	}
}
