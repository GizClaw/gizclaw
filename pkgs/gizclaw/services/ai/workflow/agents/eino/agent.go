package eino

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	einoagent "github.com/GizClaw/gizclaw-go/pkgs/agent/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

type agent struct {
	*einoagent.Agent
	workspace string
	memory    memory.Store
}

func (a *agent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	historyAvailable := a != nil && a.Agent != nil
	memoryAvailable := a != nil && a.memory != nil
	return apitypes.PeerRunWorkspaceState{
		RuntimeState:         apitypes.PeerRunStatusStateRunning,
		HistoryAvailable:     &historyAvailable,
		MemoryStatsAvailable: &memoryAvailable,
		RecallAvailable:      &memoryAvailable,
	}, nil
}

func (a *agent) ListHistory(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	if a == nil || a.Agent == nil {
		message := "eino history is unavailable"
		return apitypes.PeerRunHistoryListResponse{Available: false, Items: []apitypes.PeerRunHistoryEntry{}, Message: &message}, nil
	}
	messages, err := a.History(ctx)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, fmt.Errorf("eino: list history: %w", err)
	}
	offset, err := historyOffset(req.Cursor)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, err
	}
	if offset > len(messages) {
		offset = len(messages)
	}
	limit := 50
	if req.Limit != nil && *req.Limit > 0 {
		limit = min(*req.Limit, 200)
	}
	count := min(limit, len(messages)-offset)
	end := offset + count
	items := make([]apitypes.PeerRunHistoryEntry, 0, count)
	createdAt := time.Now().UTC()
	descending := req.Order != nil && *req.Order == apitypes.PeerRunHistoryListRequestOrderDesc
	for position := offset; position < end; position++ {
		index := position
		if descending {
			index = len(messages) - 1 - position
		}
		items = append(items, historyEntry(a.workspace, index, createdAt, messages[index]))
	}
	response := apitypes.PeerRunHistoryListResponse{Available: true, Items: items, HasNext: end < len(messages)}
	if response.HasNext {
		next := strconv.Itoa(end)
		response.NextCursor = &next
	}
	if len(messages) == 0 {
		message := "eino history is empty"
		response.Message = &message
	}
	return response, nil
}

func (a *agent) PlayHistory(_ context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	message := "eino text history replay is not supported"
	return apitypes.PeerRunHistoryPlayResponse{
		Accepted: false, HistoryId: req.HistoryId, State: "unsupported", Message: &message,
	}, nil
}

func (a *agent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	if a == nil || a.memory == nil {
		message := "eino memory store is not configured"
		return apitypes.PeerRunMemoryStatsResponse{Available: false, Enabled: false, Message: &message}, nil
	}
	backend := "memory.Store"
	indexStatus := "available"
	metadata := map[string]any{"capabilities": []string{"observe", "recall", "update", "delete"}}
	return apitypes.PeerRunMemoryStatsResponse{
		Available: true, Enabled: true, Backend: &backend, IndexStatus: &indexStatus, Metadata: &metadata,
	}, nil
}

func (a *agent) Recall(ctx context.Context, req apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	if a == nil || a.memory == nil {
		message := "eino memory store is not configured"
		return apitypes.PeerRunRecallResponse{Available: false, Hits: []apitypes.PeerRunRecallHit{}, Message: &message}, nil
	}
	limit := 10
	if req.Limit != nil {
		limit = *req.Limit
	}
	filters := make([]memory.Filter, 0)
	if req.Filters != nil {
		for key, value := range *req.Filters {
			filters = append(filters, memory.Filter{Field: key, Operator: memory.FilterEqual, Value: value})
		}
	}
	slices.SortFunc(filters, func(left, right memory.Filter) int { return strings.Compare(left.Field, right.Field) })
	result, err := a.memory.Recall(ctx, memory.Query{Text: req.Query, Limit: limit, Filters: filters})
	if err != nil {
		return apitypes.PeerRunRecallResponse{}, fmt.Errorf("eino: recall memory: %w", err)
	}
	hits := make([]apitypes.PeerRunRecallHit, 0, len(result.Matches))
	for index, match := range result.Matches {
		hits = append(hits, recallHit(index, match))
	}
	return apitypes.PeerRunRecallResponse{Available: true, Hits: hits}, nil
}

func historyOffset(cursor *string) (int, error) {
	if cursor == nil || strings.TrimSpace(*cursor) == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(strings.TrimSpace(*cursor))
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("eino: invalid history cursor %q", *cursor)
	}
	return offset, nil
}

func historyEntry(workspace string, index int, createdAt time.Time, message *schema.Message) apitypes.PeerRunHistoryEntry {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "default"
	}
	entry := apitypes.PeerRunHistoryEntry{
		CreatedAt: createdAt, Id: fmt.Sprintf("%s:%06d", workspace, index), Name: "agent",
		ReplayAvailable: false, Type: apitypes.PeerRunHistoryEntryTypeAgent,
	}
	if message == nil {
		return entry
	}
	entry.Text = strings.TrimSpace(message.Content)
	if message.Role == schema.User {
		gearID := Type
		entry.GearId = &gearID
		entry.Name = "gear"
		entry.Type = apitypes.PeerRunHistoryEntryTypeGear
	}
	return entry
}

func recallHit(index int, match memory.Match) apitypes.PeerRunRecallHit {
	id := strings.TrimSpace(match.Fact.ID)
	if id == "" {
		id = fmt.Sprintf("hit-%06d", index)
	}
	metadata := make(map[string]any, len(match.Fact.Attributes)+1)
	maps.Copy(metadata, match.Fact.Attributes)
	if match.Fact.Revision != "" {
		metadata["revision"] = match.Fact.Revision
	}
	var sourceID *string
	if len(match.Fact.Sources) > 0 {
		value := strings.TrimSpace(match.Fact.Sources[0].ObservationID)
		if value != "" {
			sourceID = &value
		}
	}
	sourceType := "memory"
	var createdAt *time.Time
	if !match.Fact.CreatedAt.IsZero() {
		value := match.Fact.CreatedAt
		createdAt = &value
	}
	return apitypes.PeerRunRecallHit{
		Id: id, Score: match.Score, Snippet: match.Fact.Text, SourceId: sourceID,
		SourceType: &sourceType, CreatedAt: createdAt, Metadata: &metadata,
	}
}
