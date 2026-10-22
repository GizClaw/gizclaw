package peerrun

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// KVUpdate is an already timestamped telemetry entry. Nil Value is deletion;
// JSON null is the nonempty RawMessage "null".
type KVUpdate struct {
	Key        string
	Value      json.RawMessage
	ObservedAt time.Time
}

type kvRecord struct {
	Value      json.RawMessage `json:"value,omitempty"`
	ObservedAt time.Time       `json:"observed_at"`
}

func kvSnapshot(data string) (*apitypes.PeerReportedKV, error) {
	if data == "" {
		return nil, nil
	}
	var records map[string]kvRecord
	if err := json.Unmarshal([]byte(data), &records); err != nil {
		return nil, fmt.Errorf("peerrun: decode reported KV state")
	}
	values := make(apitypes.PeerReportedKV)
	for key, record := range records {
		if len(record.Value) > 0 {
			values[key] = apitypes.PeerKVEntry{Value: record.Value, JsonValue: string(record.Value), ObservedAt: record.ObservedAt}
		}
	}
	return &values, nil
}

// GetReportedKV reads the durable device-owned snapshot, including offline
// devices. Nil means never reported; an empty map can mean all keys deleted.
func (s *Server) GetReportedKV(ctx context.Context, peer giznet.PublicKey) (*apitypes.PeerReportedKV, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	if peer.IsZero() {
		return nil, ErrInvalidPublicKey
	}
	var data string
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT state_json FROM peer_reported_kv WHERE public_key=?`), peer.String()).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("peerrun: read reported KV: %w", err)
	}
	return kvSnapshot(data)
}

// PutReportedKV atomically applies an entire report to an independent runtime
// record. Bounded compare-and-swap prevents concurrent reports, status control
// replies and reconnects from overwriting newer observations. Equal timestamps
// are immutable, so replays cannot replace a value or resurrect a tombstone.
func (s *Server) PutReportedKV(ctx context.Context, peer giznet.PublicKey, updates []KVUpdate) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	if peer.IsZero() {
		return ErrInvalidPublicKey
	}
	if len(updates) == 0 || len(updates) > telemetrypb.MaxKVFrameEntries {
		return telemetrypb.ErrInvalidKV
	}
	for _, update := range updates {
		kv := &telemetrypb.KVObservation{Key: update.Key}
		if update.Value == nil {
			kv.Delete = new(true)
		} else {
			kv.JsonValue = new(string(update.Value))
		}
		if err := telemetrypb.ValidateKV(kv); err != nil {
			return err
		}
		if update.ObservedAt.IsZero() || update.ObservedAt.UnixMilli() <= 0 {
			return telemetrypb.ErrInvalidKV
		}
	}
	for range 16 {
		if err := ctx.Err(); err != nil {
			return err
		}
		var previous string
		err := db.QueryRowContext(ctx, db.Rebind(`SELECT state_json FROM peer_reported_kv WHERE public_key=?`), peer.String()).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("peerrun: read reported KV: %w", err)
		}
		records := make(map[string]kvRecord)
		if previous != "" {
			if err := json.Unmarshal([]byte(previous), &records); err != nil {
				return fmt.Errorf("peerrun: decode reported KV state")
			}
		}
		changed := false
		for _, update := range updates {
			if current, ok := records[update.Key]; ok && !update.ObservedAt.After(current.ObservedAt) {
				continue
			}
			records[update.Key] = kvRecord{Value: update.Value, ObservedAt: update.ObservedAt.UTC()}
			changed = true
		}
		if !changed {
			return nil
		}
		if len(records) > telemetrypb.MaxKVStoredKeys {
			return telemetrypb.ErrInvalidKV
		}
		var encoded bytes.Buffer
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		err = encoder.Encode(records)
		data := bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))
		if err != nil {
			return telemetrypb.ErrInvalidKV
		}
		if len(data) > telemetrypb.MaxKVStoredBytes {
			return telemetrypb.ErrInvalidKV
		}
		var result sql.Result
		if previous == "" {
			result, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO peer_reported_kv(public_key,state_json) VALUES (?,?) ON CONFLICT(public_key) DO NOTHING`), peer.String(), string(data))
		} else {
			result, err = db.ExecContext(ctx, db.Rebind(`UPDATE peer_reported_kv SET state_json=? WHERE public_key=? AND state_json=?`), string(data), peer.String(), previous)
		}
		if err != nil {
			return fmt.Errorf("peerrun: update reported KV: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 1 {
			return nil
		}
	}
	return fmt.Errorf("peerrun: reported KV concurrent update retry limit reached")
}
