package flowcraft

import (
	"context"
	"testing"
	"time"

	corememory "github.com/GizClaw/flowcraft/core/memory"
	flowmessage "github.com/GizClaw/flowcraft/core/message"
	storememory "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type assemblyMemoryStore struct {
	recall      storememory.RecallResult
	query       storememory.Query
	observation storememory.Observation
}

func (s *assemblyMemoryStore) Observe(_ context.Context, observation storememory.Observation) (storememory.ObserveResult, error) {
	s.observation = observation
	return storememory.ObserveResult{}, nil
}

func (s *assemblyMemoryStore) Recall(_ context.Context, query storememory.Query) (storememory.RecallResult, error) {
	s.query = query
	return s.recall, nil
}

func (*assemblyMemoryStore) Update(context.Context, storememory.UpdateRequest) (storememory.Fact, error) {
	return storememory.Fact{}, storememory.ErrUnsupported
}

func (*assemblyMemoryStore) Delete(context.Context, storememory.DeleteRequest) error {
	return storememory.ErrUnsupported
}

func TestMemoryAssemblyMapsContextAndTurn(t *testing.T) {
	now := time.Now().UTC()
	store := &assemblyMemoryStore{recall: storememory.RecallResult{Matches: []storememory.Match{{
		Fact: storememory.Fact{
			ID: "fact-1", Revision: "rev-1", Text: "blue lantern", Attributes: map[string]any{"lane": "profile"},
			Sources: []storememory.SourceRef{{ObservationID: "observation-1"}}, UpdatedAt: now,
		},
		Score: 0.8,
	}}}}
	assembly := &memoryAssembly{store: store}
	scope := corememory.Scope{RuntimeID: "runtime", UserID: "user", AgentID: "agent"}
	result, err := assembly.Context(t.Context(), corememory.ContextRequest{
		Scope: scope, ConversationID: "conversation", Query: "lantern", Budget: corememory.Budget{MaxItems: 2}, MinScore: 0.5,
	})
	if err != nil {
		t.Fatalf("Context() error = %v", err)
	}
	if store.query.Scope != (storememory.Scope{AppID: "runtime", UserID: "user", AgentID: "agent"}) || store.query.Limit != 2 {
		t.Fatalf("Recall query = %#v", store.query)
	}
	if len(result.Items) != 1 || result.Items[0].Content.Text() != "blue lantern" || result.Items[0].Address.ConversationID != "conversation" {
		t.Fatalf("Context result = %#v", result)
	}
	if err := assembly.CommitTurn(t.Context(), corememory.Turn{
		Scope: scope, ConversationID: "conversation", IdempotencyKey: "run-1",
		Messages: []flowmessage.Message{
			flowmessage.NewTextMessage(flowmessage.RoleUser, "remember this"),
			flowmessage.NewTextMessage(flowmessage.RoleAssistant, "saved"),
		},
	}); err != nil {
		t.Fatalf("CommitTurn() error = %v", err)
	}
	if store.observation.ID != "run-1" || len(store.observation.Turns) != 2 || store.observation.Turns[1].Role != storememory.RoleAssistant {
		t.Fatalf("Observation = %#v", store.observation)
	}
}
