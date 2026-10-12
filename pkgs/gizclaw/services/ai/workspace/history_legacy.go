package workspace

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type historyPayload struct {
	Version         int            `json:"version"`
	GearID          string         `json:"gear_id,omitempty"`
	Origin          string         `json:"origin,omitempty"`
	Name            string         `json:"name"`
	ReplayAvailable bool           `json:"replay_available"`
	Assets          []HistoryAsset `json:"assets,omitempty"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
}

func historyEntryFromRecord(record logstore.Record, stream string) (HistoryEntry, error) {
	if record.Stream != stream {
		return HistoryEntry{}, fmt.Errorf("workspace history: record %q belongs to stream %q", record.ID, record.Stream)
	}
	var payload historyPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return HistoryEntry{}, fmt.Errorf("workspace history: decode record %q: %w", record.ID, err)
	}
	if payload.Version != historyRecordVersion {
		return HistoryEntry{}, fmt.Errorf("workspace history: record %q has unsupported version %d", record.ID, payload.Version)
	}
	entry := HistoryEntry{
		ID: record.ID, Type: record.Kind, GearID: payload.GearID,
		Origin: payload.Origin, Name: payload.Name, Text: record.Message,
		CreatedAt: record.Time.UTC(), ReplayAvailable: payload.ReplayAvailable,
		Assets: payload.Assets, ExpiresAt: payload.ExpiresAt,
	}
	if err := validateHistoryEntry(entry); err != nil {
		return HistoryEntry{}, fmt.Errorf("workspace history: decode record %q: %w", record.ID, err)
	}
	return entry, nil
}
