package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	corememory "github.com/GizClaw/flowcraft/core/memory"
	flowmessage "github.com/GizClaw/flowcraft/core/message"
	storememory "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const defaultMemoryContextItems = 8

// memoryAssembly adapts GizClaw's provider-neutral Store to the Flowcraft
// Core 0.2 memory capability consumed by the official agent memory hooks.
// The Store can therefore be backed by Mem0 or Flowcraft Memory without the
// Agent lifecycle knowing which implementation was selected.
type memoryAssembly struct {
	store storememory.Store
	tasks *taskOwner
}

var _ corememory.Assembly = (*memoryAssembly)(nil)

func (a *memoryAssembly) Context(ctx context.Context, request corememory.ContextRequest) (corememory.ContextResult, error) {
	if err := request.Validate(); err != nil {
		return corememory.ContextResult{}, err
	}
	if len(request.DatasetIDs) > 0 {
		return corememory.ContextResult{}, corememory.NewError(
			corememory.KindNotConfigured,
			"context",
			errors.New("GizClaw memory stores do not expose document datasets"),
		)
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return corememory.ContextResult{}, corememory.NewError(
			corememory.KindNotConfigured,
			"context",
			errors.New("GizClaw memory stores do not expose recent conversation messages"),
		)
	}
	limit := request.Budget.MaxItems
	if limit == 0 {
		limit = defaultMemoryContextItems
	}
	recalled, err := a.store.Recall(ctx, storememory.Query{
		Scope: storememory.Scope{
			AppID: request.Scope.RuntimeID, UserID: request.Scope.UserID, AgentID: request.Scope.AgentID,
		},
		Text: query, Limit: limit,
	})
	if err != nil {
		return corememory.ContextResult{}, classifyMemoryError("context", err)
	}

	result := corememory.ContextResult{RecallEventID: request.RecallEventID}
	chars := 0
	for _, match := range recalled.Matches {
		if len(result.Items) >= limit {
			result.Truncated = true
			break
		}
		text := strings.TrimSpace(match.Fact.Text)
		if text == "" || match.Score < request.MinScore {
			continue
		}
		itemChars := utf8.RuneCountInString(text)
		itemTokens := conservativeTokenCount(text)
		if (request.Budget.MaxChars > 0 && chars+itemChars > request.Budget.MaxChars) ||
			(request.Budget.MaxTokens > 0 && result.TokenCount+itemTokens > request.Budget.MaxTokens) {
			result.Truncated = true
			break
		}
		address := corememory.ContextAddress{}
		if request.ConversationID != "" {
			address = corememory.ContextAddress{
				Kind: corememory.ContextFact, ConversationID: request.ConversationID, ItemID: match.Fact.ID,
			}
		}
		item := corememory.ContextItem{
			ID:      match.Fact.ID,
			Address: address,
			Kind:    corememory.ContextFact,
			Content: flowmessage.Content{Parts: []flowmessage.Part{flowmessage.TextPart{Text: text}}},
			Score:   match.Score, Sources: memorySources(match.Fact),
			Metadata: memoryMetadata(match.Fact.Attributes), TokenCount: itemTokens,
			SourceClass: corememory.ContextSourceLongTerm,
			Timestamp:   match.Fact.UpdatedAt,
		}
		if item.Timestamp.IsZero() {
			item.Timestamp = match.Fact.CreatedAt
		}
		if err := item.Validate(); err != nil {
			return corememory.ContextResult{}, corememory.NewError(corememory.KindInternal, "context", err)
		}
		result.Items = append(result.Items, item)
		result.TokenCount += itemTokens
		chars += itemChars
	}
	return result, nil
}

func (a *memoryAssembly) CommitTurn(ctx context.Context, turn corememory.Turn) error {
	if err := turn.Validate(); err != nil {
		return err
	}
	observation := storememory.Observation{
		Scope: storememory.Scope{
			AppID: turn.Scope.RuntimeID, UserID: turn.Scope.UserID, AgentID: turn.Scope.AgentID,
		},
		ID: turn.IdempotencyKey,
	}
	for index, item := range turn.Messages {
		text := strings.TrimSpace(item.Content.Text())
		if text == "" {
			continue
		}
		observation.Turns = append(observation.Turns, storememory.Turn{
			ID: fmt.Sprintf("%s:%d", turn.IdempotencyKey, index), Role: storeMemoryRole(item.Role), Text: text,
		})
	}
	if len(observation.Turns) == 0 {
		return nil
	}
	observed, err := a.store.Observe(ctx, observation)
	if err != nil {
		return classifyMemoryError("turn", err)
	}
	if observed.Operation == nil {
		return nil
	}
	switch observed.Operation.Status {
	case storememory.OperationSucceeded:
		return nil
	case storememory.OperationFailed:
		return corememory.NewError(corememory.KindProviderFailure, "turn", fmt.Errorf(
			"operation %q failed: %s", observed.Operation.ID, observed.Operation.Error,
		))
	case storememory.OperationPending:
		operationID := strings.TrimSpace(observed.Operation.ID)
		if operationID == "" {
			return corememory.NewError(corememory.KindInternal, "turn", errors.New("pending operation has no ID"))
		}
		processor, ok := a.store.(storememory.AsyncOperationProcessor)
		if !ok {
			// Remote providers may durably accept work and expose only a waiter.
			return nil
		}
		request := storememory.OperationRequest{Scope: observation.Scope, ID: operationID}
		if a.tasks == nil || !a.tasks.Run(func(taskCtx context.Context) { _, _ = processor.ProcessAsync(taskCtx, request) }) {
			return corememory.NewError(corememory.KindInternal, "turn", errors.New("asynchronous operation has no task owner"))
		}
		return nil
	default:
		return corememory.NewError(corememory.KindInternal, "turn", fmt.Errorf("unknown operation status %q", observed.Operation.Status))
	}
}

func (*memoryAssembly) PutDocument(_ context.Context, document corememory.Document) error {
	if err := document.Validate(); err != nil {
		return err
	}
	return corememory.NewError(
		corememory.KindNotConfigured,
		"document",
		errors.New("GizClaw memory.Store does not implement document ingestion"),
	)
}

func memorySources(fact storememory.Fact) []corememory.SourceRef {
	result := make([]corememory.SourceRef, 0, len(fact.Sources))
	for _, source := range fact.Sources {
		id := strings.TrimSpace(source.ObservationID)
		if id == "" && len(source.TurnIDs) > 0 {
			id = strings.TrimSpace(source.TurnIDs[0])
		}
		if id != "" {
			result = append(result, corememory.SourceRef{Kind: corememory.SourceMessage, ID: id, Revision: fact.Revision})
		}
	}
	if len(result) == 0 {
		result = append(result, corememory.SourceRef{Kind: corememory.SourceExternal, ID: fact.ID, Revision: fact.Revision})
	}
	return result
}

func memoryMetadata(attributes map[string]any) corememory.Metadata {
	if len(attributes) == 0 {
		return nil
	}
	result := make(corememory.Metadata)
	for key, value := range attributes {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		switch typed := value.(type) {
		case string:
			result[key] = typed
		case fmt.Stringer:
			result[key] = typed.String()
		case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			result[key] = fmt.Sprint(typed)
		default:
			if encoded, err := json.Marshal(typed); err == nil {
				result[key] = string(encoded)
			}
		}
	}
	return result
}

func storeMemoryRole(role flowmessage.Role) storememory.Role {
	switch role {
	case flowmessage.RoleSystem:
		return storememory.RoleSystem
	case flowmessage.RoleAssistant:
		return storememory.RoleAssistant
	case flowmessage.RoleTool:
		return storememory.RoleTool
	default:
		return storememory.RoleUser
	}
}

func conservativeTokenCount(text string) int {
	if text == "" {
		return 0
	}
	// Without the selected model's tokenizer, UTF-8 byte count is a safe
	// upper bound for text token budgets and never over-packs context.
	return len(text)
}

func classifyMemoryError(capability string, err error) error {
	switch {
	case errors.Is(err, storememory.ErrInvalidInput):
		return corememory.NewError(corememory.KindInvalidRequest, capability, err)
	case errors.Is(err, storememory.ErrConflict):
		return corememory.NewError(corememory.KindConflict, capability, err)
	case errors.Is(err, storememory.ErrUnsupported):
		return corememory.NewError(corememory.KindNotConfigured, capability, err)
	case errors.Is(err, storememory.ErrUnavailable):
		return corememory.NewError(corememory.KindProviderFailure, capability, err)
	case errors.Is(err, storememory.ErrNotFound):
		return corememory.NewError(corememory.KindProviderFailure, capability, err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return corememory.NewError(corememory.KindOperationInterrupted, capability, err)
	default:
		return corememory.NewError(corememory.KindInternal, capability, err)
	}
}
