package eino

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

func loadPersistentState(ctx context.Context, config *StatePersistenceConfig, fields map[string]StateField) (map[string]any, string, error) {
	if config == nil {
		return nil, "", nil
	}
	snapshot, err := config.Store.Load(ctx, config.Scope)
	if err != nil {
		return nil, "", fmt.Errorf("eino: load State: %w", err)
	}
	values := make(map[string]any, len(config.Fields))
	allowed := make(map[string]struct{}, len(config.Fields))
	for _, field := range config.Fields {
		allowed[field] = struct{}{}
	}
	for name, value := range snapshot.Fields {
		if _, ok := allowed[name]; !ok {
			continue
		}
		normalized, err := normalizeStateValue(value, fields[name].Type)
		if err != nil {
			return nil, "", fmt.Errorf("eino: load State field %q: %w", name, err)
		}
		values[name] = normalized
	}
	return values, snapshot.Version, nil
}

func commitPersistentState(ctx context.Context, config *StatePersistenceConfig, state *runState, version string) error {
	if config == nil {
		return nil
	}
	values := make(map[string]any, len(config.Fields))
	for _, name := range config.Fields {
		value, err := state.value(name)
		if err != nil {
			continue
		}
		values[name] = value
	}
	if _, err := config.Store.CompareAndSwap(ctx, config.Scope, version, values); err != nil {
		return fmt.Errorf("eino: commit State: %w", err)
	}
	return nil
}

func recallMemory(ctx context.Context, config *MemoryConfig, state *runState) error {
	if config == nil {
		return nil
	}
	var recalled strings.Builder
	for index, definition := range config.Recall {
		queryValue, err := state.binding(Binding{From: definition.QueryFrom})
		if err != nil {
			return fmt.Errorf("eino: Recall[%d] query: %w", index, err)
		}
		query, ok := queryValue.(string)
		if !ok || strings.TrimSpace(query) == "" {
			return fmt.Errorf("eino: Recall[%d] query is empty or not text", index)
		}
		result, err := config.Store.Recall(ctx, memory.Query{
			Scope: config.Scope, Text: query, Limit: definition.TopK,
		})
		if err != nil {
			return fmt.Errorf("eino: Recall[%d]: %w", index, err)
		}
		var rendered strings.Builder
		for _, match := range result.Matches {
			text := strings.TrimSpace(match.Fact.Text)
			if text == "" {
				continue
			}
			if rendered.Len() > 0 {
				rendered.WriteByte('\n')
			}
			rendered.WriteString("- ")
			rendered.WriteString(text)
		}
		if err := state.set(definition.Output, rendered.String()); err != nil {
			return err
		}
		if rendered.Len() > 0 {
			if recalled.Len() > 0 {
				recalled.WriteByte('\n')
			}
			recalled.WriteString(rendered.String())
		}
	}
	state.mu.Lock()
	state.input.Memory = recalled.String()
	state.mu.Unlock()
	return nil
}

func observeMemory(
	ctx context.Context,
	config *MemoryConfig,
	state *runState,
	streamID, user, delivered string,
	interrupted bool,
) error {
	if config == nil || !config.Observe.Enabled || interrupted && delivered == "" {
		return nil
	}
	observation := memory.Observation{Scope: config.Scope, ID: streamID}
	if strings.TrimSpace(user) != "" {
		observation.Turns = append(observation.Turns, memory.Turn{
			ID: streamID + ":user", Role: memory.RoleUser, Text: user,
		})
	}
	if delivered != "" {
		observation.Turns = append(observation.Turns, memory.Turn{
			ID: streamID + ":assistant", Role: memory.RoleAssistant, Text: delivered,
		})
		if interrupted {
			observation.Context = map[string]any{"interrupted": true}
		}
	}
	for index, definition := range config.Observe.Facts {
		value, err := state.value(definition.TextFrom)
		if err != nil {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		fact := memory.FactCandidate{Text: text, Attributes: make(map[string]any, len(definition.Attributes))}
		for attribute, source := range definition.Attributes {
			value, err := state.value(source)
			if err != nil {
				return fmt.Errorf("eino: Observe Fact[%d] attribute %q: %w", index, attribute, err)
			}
			fact.Attributes[attribute] = value
		}
		observation.Facts = append(observation.Facts, fact)
	}
	if len(observation.Turns) == 0 && len(observation.Facts) == 0 {
		return nil
	}
	if err := memory.ValidateObservation(observation); err != nil {
		return fmt.Errorf("eino: validate Memory observation: %w", err)
	}
	result, err := config.Store.Observe(ctx, observation)
	if err != nil {
		return fmt.Errorf("eino: observe Memory: %w", err)
	}
	if result.Operation == nil {
		return nil
	}
	switch result.Operation.Status {
	case memory.OperationSucceeded:
		return nil
	case memory.OperationFailed:
		return fmt.Errorf("eino: Memory operation %q failed: %s", result.Operation.ID, result.Operation.Error)
	case memory.OperationPending:
		if strings.TrimSpace(result.Operation.ID) == "" {
			return fmt.Errorf("eino: pending Memory operation has no ID")
		}
		if !config.Observe.WaitForCompletion {
			if processor, ok := config.Store.(memory.AsyncOperationProcessor); ok {
				go func(operationID string) {
					_, _ = processor.ProcessAsync(context.WithoutCancel(ctx), operationID)
				}(result.Operation.ID)
			}
			return nil
		}
		waiter := config.Store.(memory.OperationWaiter)
		completed, err := waiter.Wait(ctx, result.Operation.ID)
		if err != nil {
			return fmt.Errorf("eino: wait Memory operation %q: %w", result.Operation.ID, err)
		}
		if completed.Operation == nil {
			return fmt.Errorf("eino: Memory operation %q wait returned no operation", result.Operation.ID)
		}
		switch completed.Operation.Status {
		case memory.OperationSucceeded:
			return nil
		case memory.OperationFailed:
			return fmt.Errorf("eino: Memory operation %q failed: %s", completed.Operation.ID, completed.Operation.Error)
		case memory.OperationPending:
			return fmt.Errorf("eino: Memory operation %q remained pending after wait", completed.Operation.ID)
		default:
			return fmt.Errorf(
				"eino: Memory operation %q wait returned unsupported status %q",
				completed.Operation.ID,
				completed.Operation.Status,
			)
		}
	default:
		return fmt.Errorf("eino: unsupported Memory operation status %q", result.Operation.Status)
	}
}

func historyMessages(user, delivered string) []*schema.Message {
	var messages []*schema.Message
	if strings.TrimSpace(user) != "" {
		messages = append(messages, schema.UserMessage(user))
	}
	if delivered != "" {
		messages = append(messages, schema.AssistantMessage(delivered, nil))
	}
	return messages
}
