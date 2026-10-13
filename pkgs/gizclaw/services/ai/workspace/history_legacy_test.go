package workspace

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

func historyRecord(entry HistoryEntry, stream string) (logstore.Record, error) {
	payload, err := json.Marshal(historyPayload{
		Version: historyRecordVersion, GearID: entry.GearID, Origin: entry.Origin,
		Name: entry.Name, ReplayAvailable: entry.ReplayAvailable,
		Assets: entry.Assets, ExpiresAt: entry.ExpiresAt,
	})
	if err != nil {
		return logstore.Record{}, fmt.Errorf("workspace history: encode record: %w", err)
	}
	attributes := map[string]string{
		"actor_name":     entry.Name,
		"schema_version": strconv.Itoa(historyRecordVersion),
	}
	if entry.GearID != "" {
		attributes["gear_id"] = entry.GearID
	}
	if entry.Origin != "" {
		attributes["origin"] = entry.Origin
	}
	record := logstore.Record{
		ID: entry.ID, Time: entry.CreatedAt, Stream: stream, Kind: entry.Type,
		Message: entry.Text, Attributes: attributes, Payload: payload,
	}
	if err := logstore.ValidateRecord(record); err != nil {
		return logstore.Record{}, fmt.Errorf("workspace history: encode record: %w", err)
	}
	return record, nil
}
