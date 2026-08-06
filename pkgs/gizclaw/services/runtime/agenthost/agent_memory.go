package agenthost

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// NewMemoryAgent exposes a borrowed Memory Store through the common workspace
// runtime surface. backend is the command-owned configured provider kind.
func NewMemoryAgent(agent Agent, store memory.Store, scope memory.Scope, backend string) Agent {
	if agent == nil {
		return nil
	}
	return &memoryAgent{Agent: agent, store: store, scope: scope, backend: strings.TrimSpace(backend)}
}

type memoryAgent struct {
	Agent
	store   memory.Store
	scope   memory.Scope
	backend string
}

func (a *memoryAgent) Status(ctx context.Context) (apitypes.PeerRunWorkspaceState, error) {
	status, err := a.Agent.Status(ctx)
	if err != nil {
		return status, err
	}
	available := a.store != nil
	status.MemoryStatsAvailable = &available
	status.RecallAvailable = &available
	return status, nil
}

func (a *memoryAgent) MemoryStats(ctx context.Context, _ apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	if a == nil || a.store == nil {
		message := "workspace memory is not enabled"
		return apitypes.PeerRunMemoryStatsResponse{Available: true, Enabled: false, Message: &message}, nil
	}
	backend := a.backend
	if backend == "" {
		backend = "unknown"
	}
	metadata := map[string]any{"scope": map[string]any{
		"app_id":   a.scope.AppID,
		"user_id":  a.scope.UserID,
		"agent_id": a.scope.AgentID,
		"run_id":   a.scope.RunID,
	}}
	response := apitypes.PeerRunMemoryStatsResponse{
		Available: true,
		Enabled:   true,
		Backend:   &backend,
		Metadata:  &metadata,
	}
	if provider, ok := a.store.(memory.StatisticsProvider); ok {
		stats, err := provider.Stats(ctx, a.scope)
		if err != nil {
			return apitypes.PeerRunMemoryStatsResponse{}, err
		}
		response.ItemCount = stats.ItemCount
		if !stats.LastUpdatedAt.IsZero() {
			response.LastUpdatedAt = &stats.LastUpdatedAt
		}
	}
	return response, nil
}

func (a *memoryAgent) Recall(ctx context.Context, req apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	if a == nil || a.store == nil {
		message := "workspace memory is not enabled"
		return apitypes.PeerRunRecallResponse{Available: true, Hits: []apitypes.PeerRunRecallHit{}, Message: &message}, nil
	}
	limit := 10
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}
	filters := make([]memory.Filter, 0)
	if req.Filters != nil {
		keys := make([]string, 0, len(*req.Filters))
		for key := range *req.Filters {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			filters = append(filters, memory.Filter{Field: key, Operator: memory.FilterEqual, Value: (*req.Filters)[key]})
		}
	}
	result, err := a.store.Recall(ctx, memory.Query{Scope: a.scope, Text: req.Query, Limit: limit, Filters: filters})
	if err != nil {
		return apitypes.PeerRunRecallResponse{}, err
	}
	hits := make([]apitypes.PeerRunRecallHit, 0, len(result.Matches))
	for _, match := range result.Matches {
		metadata := maps.Clone(match.Fact.Attributes)
		hit := apitypes.PeerRunRecallHit{
			Name:      match.Fact.ID,
			Score:     match.Score,
			Snippet:   match.Fact.Text,
			CreatedAt: &match.Fact.CreatedAt,
			Metadata:  &metadata,
		}
		if len(match.Fact.Sources) > 0 && strings.TrimSpace(match.Fact.Sources[0].ObservationID) != "" {
			hit.SourceName = &match.Fact.Sources[0].ObservationID
			sourceType := "observation"
			hit.SourceType = &sourceType
		}
		hits = append(hits, hit)
	}
	return apitypes.PeerRunRecallResponse{Available: true, Hits: hits}, nil
}
