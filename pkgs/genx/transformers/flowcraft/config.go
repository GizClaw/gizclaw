package flowcraft

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"

	flowgraph "github.com/GizClaw/flowcraft/core/graph"
	memoryhook "github.com/GizClaw/flowcraft/core/memory/hook"
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
	// ContextID is the stable conversation identity used by Flowcraft, History,
	// and State. Empty creates an invocation-lifetime identity for standalone use.
	ContextID string

	// Graph is the required Flowcraft Graph definition.
	Graph flowgraph.GraphDefinition
	// MaxIterations overrides the SDK default when positive.
	MaxIterations int
	// PublishNodes is the required allow-list of node IDs exposed as output.
	PublishNodes []string
	// Models resolves every LLM node alias through model/<alias>.
	Models genx.Generator
	// ToolInvoker resolves and executes function tools inside model turns. The
	// caller owns its lifecycle.
	ToolInvoker genx.ToolInvoker
	// MaxToolCalls bounds ToolCalls across one Transform invocation. Zero uses
	// genx.DefaultMaxToolCalls.
	MaxToolCalls int

	// History stores ordered conversation messages. The caller owns its lifecycle.
	History logstore.MutableStore
	// HistoryScope is the durable Workspace/Agent partition recorded on History.
	HistoryScope string
	// Memory stores provider-neutral long-term facts. The caller owns its lifecycle.
	Memory memory.Store
	// State stores serializable Board variables. The caller owns its lifecycle.
	State kv.Store

	// MemoryScope is the fixed opaque scope used by every Recall and Observe.
	MemoryScope memory.Scope
	// MemoryLaneRecall maps portable Layout lane names to guidance rendered
	// when a Graph memory_recall node explicitly selects those lanes.
	MemoryLaneRecall map[string]string

	// MemoryContext configures the official Flowcraft memory.context prepare
	// hook. Scope is runtime-owned and must match MemoryScope.
	MemoryContext *memoryhook.ContextSettings
	// MemoryTurn configures the official Flowcraft memory.turn commit hook.
	// Scope is runtime-owned and must match MemoryScope.
	MemoryTurn *memoryhook.TurnSettings

	// BoardInputs resolves transient variables immediately before every Graph
	// turn. Returned values are copied into the Board after durable State loads.
	BoardInputs func(context.Context) (map[string]any, error)

	// Initiative controls the optional empty-input Graph turn.
	Initiative InitiativePolicy

	asyncTasks *taskOwner
	matchNodes map[string]matchNodeRuntime
}

// InitiativePolicy controls when an Agent may run without user input.
type InitiativePolicy string

const (
	InitiativeDisabled      InitiativePolicy = ""
	InitiativeOnceWhenEmpty InitiativePolicy = "once_when_empty"
	InitiativeOnReload      InitiativePolicy = "on_reload"
)

func normalizeConfig(source Config) (Config, error) {
	config := source
	config.ID = strings.TrimSpace(config.ID)
	config.Name = strings.TrimSpace(config.Name)
	config.ContextID = strings.TrimSpace(config.ContextID)
	config.HistoryScope = strings.TrimSpace(config.HistoryScope)
	config.MemoryScope = normalizeMemoryScope(config.MemoryScope)
	config.MemoryLaneRecall = maps.Clone(config.MemoryLaneRecall)
	for lane, guidance := range config.MemoryLaneRecall {
		normalizedLane := strings.TrimSpace(lane)
		normalizedGuidance := strings.TrimSpace(guidance)
		if normalizedLane == "" || normalizedGuidance == "" {
			delete(config.MemoryLaneRecall, lane)
			continue
		}
		if normalizedLane != lane {
			delete(config.MemoryLaneRecall, lane)
			config.MemoryLaneRecall[normalizedLane] = normalizedGuidance
			continue
		}
		config.MemoryLaneRecall[lane] = normalizedGuidance
	}
	if config.ID == "" {
		return Config{}, fmt.Errorf("flowcraft: ID is required")
	}
	if config.Models == nil {
		return Config{}, fmt.Errorf("flowcraft: Models is required")
	}
	if config.MaxIterations < 0 {
		return Config{}, fmt.Errorf("flowcraft: MaxIterations cannot be negative")
	}
	if config.MaxToolCalls < 0 {
		return Config{}, fmt.Errorf("flowcraft: MaxToolCalls cannot be negative")
	}
	if config.ToolInvoker == nil && config.MaxToolCalls > 0 {
		return Config{}, fmt.Errorf("flowcraft: MaxToolCalls requires ToolInvoker")
	}
	switch config.Initiative {
	case InitiativeDisabled, InitiativeOnceWhenEmpty, InitiativeOnReload:
	default:
		return Config{}, fmt.Errorf("flowcraft: unsupported Initiative %q", config.Initiative)
	}
	if err := config.Graph.Validate(); err != nil {
		return Config{}, fmt.Errorf("flowcraft: invalid Graph: %w", err)
	}
	data, err := json.Marshal(config.Graph)
	if err != nil {
		return Config{}, fmt.Errorf("flowcraft: clone Graph: %w", err)
	}
	var ownedGraph flowgraph.GraphDefinition
	if err := json.Unmarshal(data, &ownedGraph); err != nil {
		return Config{}, fmt.Errorf("flowcraft: clone Graph: %w", err)
	}
	config.Graph = ownedGraph
	config.matchNodes = make(map[string]matchNodeRuntime)
	nodes := make(map[string]struct{}, len(config.Graph.Nodes))
	for _, node := range config.Graph.Nodes {
		nodes[node.ID] = struct{}{}
		var rawConfig map[string]any
		if len(node.Config) > 0 {
			if err := json.Unmarshal(node.Config, &rawConfig); err != nil {
				return Config{}, fmt.Errorf("flowcraft: node %q config: %w", node.ID, err)
			}
		}
		switch node.Type {
		case "inference":
			model, _ := rawConfig["model"].(map[string]any)
			modelID, _ := model["id"].(map[string]any)
			provider, _ := modelID["provider"].(string)
			modelAlias, _ := modelID["name"].(string)
			modelAlias = strings.TrimSpace(modelAlias)
			if provider != genXInferenceProvider || modelAlias == "" {
				return Config{}, fmt.Errorf("flowcraft: inference node %q requires model.id {provider: %q, name: <alias>}", node.ID, genXInferenceProvider)
			}
			if !strings.Contains(modelAlias, "${") && strings.Contains(modelAlias, "/") {
				return Config{}, fmt.Errorf("flowcraft: inference node %q model name must be an alias, got %q", node.ID, modelAlias)
			}
		case "script":
			source, _ := rawConfig["source"].(string)
			if strings.TrimSpace(source) == "" {
				return Config{}, fmt.Errorf("flowcraft: script node %q requires inline source", node.ID)
			}
		case "passthrough":
			if len(rawConfig) != 0 {
				return Config{}, fmt.Errorf("flowcraft: passthrough node %q does not accept config", node.ID)
			}
		case "memory_recall":
			if config.Memory == nil {
				return Config{}, fmt.Errorf("flowcraft: %s node %q requires Memory", node.Type, node.ID)
			}
		case "memory_observe":
			if config.Memory == nil {
				return Config{}, fmt.Errorf("flowcraft: %s node %q requires Memory", node.Type, node.ID)
			}
			var nodeConfig memoryObserveNodeConfig
			if err := decodeNodeConfig(node.Config, &nodeConfig); err != nil {
				return Config{}, fmt.Errorf("flowcraft: memory_observe node %q: %w", node.ID, err)
			}
			for _, observation := range nodeConfig.Observations {
				if len(observation.Facts) > 0 && !memory.SupportsDirectFactObservation(config.Memory) {
					return Config{}, fmt.Errorf(
						"flowcraft: memory_observe node %q requires direct Fact observation support",
						node.ID,
					)
				}
			}
		case "match":
			matchNode, err := compileMatchNode(node.ID, node.Config)
			if err != nil {
				return Config{}, err
			}
			config.matchNodes[node.ID] = matchNode
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
		if _, matchNode := config.matchNodes[nodeID]; matchNode {
			return Config{}, fmt.Errorf("flowcraft: Match node %q cannot be a PublishNodes target", nodeID)
		}
		if _, duplicate := seen[nodeID]; duplicate {
			continue
		}
		seen[nodeID] = struct{}{}
		config.PublishNodes = append(config.PublishNodes, nodeID)
	}
	if config.Memory == nil {
		if config.MemoryScope != (memory.Scope{}) || config.MemoryContext != nil || config.MemoryTurn != nil {
			return Config{}, fmt.Errorf("flowcraft: Memory settings require Memory")
		}
	} else if config.MemoryScope == (memory.Scope{}) {
		return Config{}, fmt.Errorf("flowcraft: MemoryScope is required when Memory is configured")
	}
	if config.MemoryContext != nil {
		if config.MemoryContext.Query.RecentOnly {
			return Config{}, fmt.Errorf("flowcraft: memory.context recent_only is not supported by memory.Store")
		}
		if len(config.MemoryContext.DatasetIDs) > 0 {
			return Config{}, fmt.Errorf("flowcraft: memory.context dataset_ids are not supported by memory.Store")
		}
	}
	return config, nil
}

func normalizeMemoryScope(scope memory.Scope) memory.Scope {
	scope.AppID = strings.TrimSpace(scope.AppID)
	scope.UserID = strings.TrimSpace(scope.UserID)
	scope.AgentID = strings.TrimSpace(scope.AgentID)
	scope.RunID = strings.TrimSpace(scope.RunID)
	return scope
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
		if source.IsNil() {
			return reflect.Zero(source.Type()), nil
		}
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
