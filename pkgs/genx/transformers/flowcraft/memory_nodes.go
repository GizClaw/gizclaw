package flowcraft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	flownode "github.com/GizClaw/flowcraft/sdk/graph/node"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type memoryRecallNodeConfig struct {
	Query struct {
		TextFrom string         `json:"text_from"`
		Kinds    []string       `json:"kinds"`
		Lanes    []string       `json:"lanes"`
		Filters  []memoryFilter `json:"filters"`
	} `json:"query"`
	Output string `json:"output"`
	Render *struct {
		Header     string `json:"header"`
		ItemPrefix string `json:"item_prefix"`
		MaxItems   int    `json:"max_items"`
	} `json:"render"`
	TopK int `json:"top_k"`
}

type memoryFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type memoryObserveNodeConfig struct {
	Observations []struct {
		TurnsFrom string `json:"turns_from"`
		TextFrom  string `json:"text_from"`
		Facts     []struct {
			TextFrom   string            `json:"text_from"`
			Attributes map[string]string `json:"attributes"`
		} `json:"facts"`
	} `json:"observations"`
	WaitForCompletion bool `json:"wait_for_completion"`
}

func registerMemoryNodes(factory *flownode.Factory, config Config) {
	factory.RegisterBuilder("memory_recall", func(def flowgraph.NodeDefinition) (flowgraph.Node, error) {
		var nodeConfig memoryRecallNodeConfig
		if err := decodeNodeConfig(def.Config, &nodeConfig); err != nil {
			return nil, fmt.Errorf("flowcraft: memory_recall node %q: %w", def.ID, err)
		}
		if config.Memory == nil {
			return nil, fmt.Errorf("flowcraft: memory_recall node %q requires Memory", def.ID)
		}
		if strings.TrimSpace(nodeConfig.Query.TextFrom) == "" || strings.TrimSpace(nodeConfig.Output) == "" || nodeConfig.TopK <= 0 {
			return nil, fmt.Errorf("flowcraft: memory_recall node %q requires query.text_from, output, and positive top_k", def.ID)
		}
		return &memoryRecallNode{
			id: def.ID, store: config.Memory, scope: config.MemoryScope, config: nodeConfig,
		}, nil
	})
	factory.RegisterBuilder("memory_observe", func(def flowgraph.NodeDefinition) (flowgraph.Node, error) {
		var nodeConfig memoryObserveNodeConfig
		if err := decodeNodeConfig(def.Config, &nodeConfig); err != nil {
			return nil, fmt.Errorf("flowcraft: memory_observe node %q: %w", def.ID, err)
		}
		if config.Memory == nil {
			return nil, fmt.Errorf("flowcraft: memory_observe node %q requires Memory", def.ID)
		}
		if len(nodeConfig.Observations) == 0 {
			return nil, fmt.Errorf("flowcraft: memory_observe node %q requires observations", def.ID)
		}
		if nodeConfig.WaitForCompletion {
			if _, ok := config.Memory.(memory.OperationWaiter); !ok {
				return nil, fmt.Errorf("flowcraft: memory_observe node %q wait_for_completion requires memory.OperationWaiter", def.ID)
			}
		}
		return &memoryObserveNode{
			id: def.ID, store: config.Memory, scope: config.MemoryScope, config: nodeConfig, tasks: config.asyncTasks,
		}, nil
	})
}

func decodeNodeConfig(source map[string]any, target any) error {
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	return nil
}

type memoryRecallNode struct {
	id     string
	store  memory.Store
	scope  memory.Scope
	config memoryRecallNodeConfig
}

func (n *memoryRecallNode) ID() string { return n.id }
func (*memoryRecallNode) Type() string { return "memory_recall" }
func (n *memoryRecallNode) ExecuteBoard(ctx flowgraph.ExecutionContext, board *flowgraph.Board) error {
	queryText := boardString(board, n.config.Query.TextFrom)
	filters := make([]memory.Filter, 0, len(n.config.Query.Filters))
	for _, filter := range n.config.Query.Filters {
		operator := memory.FilterEqual
		if filter.Operator != "" {
			operator = memory.FilterOperator(filter.Operator)
		}
		filters = append(filters, memory.Filter{
			Field: strings.TrimSpace(filter.Field), Operator: operator, Value: filter.Value,
		})
	}
	result, err := n.store.Recall(ctx.Context, memory.Query{
		Scope: n.scope, Text: queryText,
		Limit:   memoryRecallCandidateLimit(n.config.TopK, len(n.config.Query.Kinds) > 0 || len(n.config.Query.Lanes) > 0),
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("flowcraft: memory_recall node %q: %w", n.id, err)
	}
	matches := selectMemoryMatches(result.Matches, n.config.Query.Kinds, n.config.Query.Lanes, n.config.TopK)
	board.SetVar(n.config.Output, renderMemoryMatches(matches, n.config.Render))
	return nil
}

func memoryRecallCandidateLimit(topK int, selectsAttributes bool) int {
	if !selectsAttributes {
		return topK
	}
	const maximum = 1000
	limit := max(topK*10, 100)
	return min(limit, maximum)
}

func selectMemoryMatches(matches []memory.Match, kinds, lanes []string, topK int) []memory.Match {
	selected := make([]memory.Match, 0, min(len(matches), topK))
	for _, match := range matches {
		if len(kinds) > 0 && !memoryAttributeMatches(match.Fact.Attributes, "kind", "categories", kinds) {
			continue
		}
		if len(lanes) > 0 && !memoryAttributeMatches(match.Fact.Attributes, "lane", "", lanes) {
			continue
		}
		selected = append(selected, match)
		if len(selected) == topK {
			break
		}
	}
	return selected
}

func memoryAttributeMatches(attributes map[string]any, field, alternate string, accepted []string) bool {
	values := memoryAttributeStrings(attributes[field])
	if len(values) == 0 && alternate != "" {
		values = memoryAttributeStrings(attributes[alternate])
	}
	for _, value := range values {
		for _, candidate := range accepted {
			if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
				return true
			}
		}
	}
	return false
}

func memoryAttributeStrings(value any) []string {
	switch value := value.(type) {
	case string:
		return []string{value}
	case []string:
		return value
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func renderMemoryMatches(matches []memory.Match, config *struct {
	Header     string `json:"header"`
	ItemPrefix string `json:"item_prefix"`
	MaxItems   int    `json:"max_items"`
}) string {
	header := "Relevant memory:"
	itemPrefix := "- "
	maxItems := len(matches)
	if config != nil {
		header = config.Header
		itemPrefix = config.ItemPrefix
		if config.MaxItems > 0 && config.MaxItems < maxItems {
			maxItems = config.MaxItems
		}
	}
	var rendered strings.Builder
	if header != "" && maxItems > 0 {
		rendered.WriteString(header)
		rendered.WriteByte('\n')
	}
	written := 0
	for _, match := range matches {
		if written >= maxItems {
			break
		}
		text := strings.TrimSpace(match.Fact.Text)
		if text == "" {
			continue
		}
		rendered.WriteString(itemPrefix)
		rendered.WriteString(text)
		rendered.WriteByte('\n')
		written++
	}
	return strings.TrimSpace(rendered.String())
}

type memoryObserveNode struct {
	id     string
	store  memory.Store
	scope  memory.Scope
	config memoryObserveNodeConfig
	tasks  *taskOwner
}

func (n *memoryObserveNode) ID() string { return n.id }
func (*memoryObserveNode) Type() string { return "memory_observe" }
func (n *memoryObserveNode) ExecuteBoard(ctx flowgraph.ExecutionContext, board *flowgraph.Board) error {
	observation := memory.Observation{
		ID: ctx.RunID, Scope: n.scope, ObservedAt: time.Now(),
	}
	for _, source := range n.config.Observations {
		if source.TextFrom != "" {
			text := boardString(board, source.TextFrom)
			if text != "" {
				if observation.Text != "" {
					observation.Text += "\n"
				}
				observation.Text += text
			}
		}
		if source.TurnsFrom != "" {
			observation.Turns = append(observation.Turns, boardTurns(board, source.TurnsFrom)...)
		}
		for _, fact := range source.Facts {
			text := boardString(board, fact.TextFrom)
			if strings.TrimSpace(text) == "" {
				continue
			}
			attributes := make(map[string]any, len(fact.Attributes))
			for key, value := range fact.Attributes {
				attributes[key] = value
			}
			observation.Facts = append(observation.Facts, memory.FactCandidate{Text: text, Attributes: attributes})
		}
	}
	result, err := n.store.Observe(ctx.Context, observation)
	if err != nil {
		return fmt.Errorf("flowcraft: memory_observe node %q: %w", n.id, err)
	}
	if n.config.WaitForCompletion && result.Operation != nil && result.Operation.Status == memory.OperationPending {
		result, err = n.store.(memory.OperationWaiter).Wait(ctx.Context, memory.OperationRequest{
			Scope: n.scope, ID: result.Operation.ID,
		})
		if err != nil {
			return fmt.Errorf("flowcraft: memory_observe node %q wait: %w", n.id, err)
		}
	}
	if !n.config.WaitForCompletion && result.Operation != nil && result.Operation.Status == memory.OperationPending {
		if processor, ok := n.store.(memory.AsyncOperationProcessor); ok {
			operationID := result.Operation.ID
			if n.tasks == nil || !n.tasks.Run(func(taskContext context.Context) {
				_, _ = processor.ProcessAsync(taskContext, memory.OperationRequest{Scope: n.scope, ID: operationID})
			}) {
				return fmt.Errorf("flowcraft: memory_observe node %q has no generation task owner", n.id)
			}
		}
	}
	if result.Operation != nil && result.Operation.Status == memory.OperationFailed {
		return fmt.Errorf("flowcraft: memory_observe node %q: %s", n.id, result.Operation.Error)
	}
	return nil
}

func boardString(board *flowgraph.Board, key string) string {
	if board == nil {
		return ""
	}
	value, ok := board.GetVar(strings.TrimSpace(key))
	if !ok {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func boardTurns(board *flowgraph.Board, key string) []memory.Turn {
	if board == nil {
		return nil
	}
	if strings.TrimSpace(key) == "conversation" {
		messages := board.Channel("__main_channel")
		turns := make([]memory.Turn, 0, len(messages))
		for index, message := range messages {
			text := strings.TrimSpace(message.Content())
			if text == "" {
				continue
			}
			turns = append(turns, memory.Turn{
				ID: fmt.Sprintf("conversation:%d", index), Role: memory.Role(message.Role), Text: text,
			})
		}
		return turns
	}
	value, ok := board.GetVar(strings.TrimSpace(key))
	if !ok {
		return nil
	}
	switch turns := value.(type) {
	case []memory.Turn:
		return append([]memory.Turn(nil), turns...)
	case []flowmodel.Message:
		result := make([]memory.Turn, 0, len(turns))
		for index, message := range turns {
			result = append(result, memory.Turn{
				ID: fmt.Sprintf("%s:%d", key, index), Role: memory.Role(message.Role), Text: message.Content(),
			})
		}
		return result
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return []memory.Turn{{ID: key, Role: memory.RoleUser, Text: text}}
	}
}
