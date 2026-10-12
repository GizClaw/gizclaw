package workspace

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type recordedHistoryInputKey struct{}

type historyReadBarrierKey struct{}

// WithHistoryReadBarrier lets a host finish previously delivered messages before
// a model reads its next context. The callback runs outside all Store locks.
func WithHistoryReadBarrier(ctx context.Context, wait func(context.Context) error) context.Context {
	return context.WithValue(ctx, historyReadBarrierKey{}, wait)
}

type recordedHistoryInput struct {
	workspaceID string
	entry       HistoryEntry
}

// WithRecordedHistoryInput marks an input already committed by an API ingress.
// The host may publish its transcript but must not persist a second user row.
func WithRecordedHistoryInput(ctx context.Context, workspaceID string, entry HistoryEntry) context.Context {
	return context.WithValue(ctx, recordedHistoryInputKey{}, recordedHistoryInput{workspaceID: workspaceID, entry: entry})
}

// RecordedHistoryInput returns an ingress-owned input for the selected Workspace.
func RecordedHistoryInput(ctx context.Context, workspaceID string) (HistoryEntry, bool) {
	value, ok := ctx.Value(recordedHistoryInputKey{}).(recordedHistoryInput)
	if !ok || value.workspaceID != workspaceID || value.entry.Type != historyEntryTypeGear {
		return HistoryEntry{}, false
	}
	return value.entry, true
}

// Query exposes the same retained messages to a read-only model adapter. The
// Workspace binding remains authoritative even if a caller supplies a scope.
func (s *HistoryStore) Query(ctx context.Context, query memory.HistoryQuery) (memory.HistoryPage, error) {
	if s == nil || s.Records == nil || query.ConversationID != s.Workspace {
		return memory.HistoryPage{}, fmt.Errorf("%w: history conversation does not match workspace", memory.ErrInvalidInput)
	}
	if query.ExcludeTurnID != "" {
		if wait, ok := ctx.Value(historyReadBarrierKey{}).(func(context.Context) error); ok {
			if err := wait(ctx); err != nil {
				return memory.HistoryPage{}, err
			}
		}
	}
	return s.Records.Query(ctx, query)
}

func (s *HistoryStore) memoryEntry(entry HistoryEntry) memory.HistoryEntry {
	role := memory.RoleAssistant
	if entry.Type == historyEntryTypeGear {
		role = memory.RoleUser
	}
	result := memory.HistoryEntry{
		ConversationID: s.Workspace, ID: entry.ID, TurnID: entry.TurnID,
		Role: role, ActorID: entry.GearID, Origin: entry.Origin, Name: entry.Name,
		Text: entry.Text, CreatedAt: entry.CreatedAt, ExpiresAt: entry.ExpiresAt,
		Interrupted: entry.Interrupted,
	}
	for _, asset := range entry.Assets {
		result.Assets = append(result.Assets, memory.HistoryAsset{
			Name: asset.Name, MIMEType: asset.MIMEType, Bytes: asset.Bytes, ExpiresAt: asset.ExpiresAt,
		})
	}
	return result
}

func historyEntryFromMemory(entry memory.HistoryEntry) HistoryEntry {
	typ := historyEntryTypeAgent
	if entry.Role == memory.RoleUser {
		typ = historyEntryTypeGear
	}
	result := HistoryEntry{
		ID: entry.ID, Type: typ, GearID: entry.ActorID, Origin: entry.Origin,
		Name: entry.Name, Text: entry.Text, CreatedAt: entry.CreatedAt,
		ExpiresAt: entry.ExpiresAt, TurnID: entry.TurnID, Interrupted: entry.Interrupted,
		ReplayAvailable: strings.TrimSpace(entry.Text) != "" || len(entry.Assets) > 0,
	}
	for _, asset := range entry.Assets {
		result.Assets = append(result.Assets, HistoryAsset{
			Name: asset.Name, MIMEType: asset.MIMEType, Bytes: asset.Bytes, ExpiresAt: asset.ExpiresAt,
		})
	}
	return result
}
