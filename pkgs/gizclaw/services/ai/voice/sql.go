package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const voiceSQLColumns = "id,source,provider_kind,provider_id,provider_voice_id,provider_data_json,display_name,description,created_at,updated_at,synced_at"

// Initialize creates Voice business tables and indexes once at startup.
func (s *Server) Initialize(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS voices(id TEXT PRIMARY KEY CHECK(length(id)>0),source TEXT NOT NULL,provider_kind TEXT NOT NULL,provider_id TEXT NOT NULL,provider_voice_id TEXT,provider_data_json TEXT NOT NULL,display_name TEXT,description TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,synced_at TEXT,sync_generation TEXT)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS voices_sync_identity ON voices(provider_kind,provider_id,provider_voice_id) WHERE source='sync' AND provider_voice_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS voice_provider_sync(provider_kind TEXT NOT NULL,provider_id TEXT NOT NULL,lock_value INTEGER NOT NULL,PRIMARY KEY(provider_kind,provider_id))`,
	}
	// Each supported combination of equality filters leads directly into ID order.
	fields := []string{"source", "provider_kind", "provider_id"}
	for mask := 1; mask < 8; mask++ {
		columns := []string{}
		for i, field := range fields {
			if mask&(1<<i) != 0 {
				columns = append(columns, field)
			}
		}
		columns = append(columns, "id")
		statements = append(statements, fmt.Sprintf("CREATE INDEX IF NOT EXISTS voices_filter_%d ON voices(%s)", mask, strings.Join(columns, ",")))
	}
	for _, query := range statements {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func lockVoiceProvider(ctx context.Context, tx *sqlx.Tx, kind apitypes.VoiceProviderKind, providerID string) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO voice_provider_sync(provider_kind,provider_id,lock_value) VALUES (?,?,0) ON CONFLICT(provider_kind,provider_id) DO NOTHING`), string(kind), providerID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE voice_provider_sync SET lock_value=lock_value WHERE provider_kind=? AND provider_id=?`), string(kind), providerID)
	return err
}

func voiceSQLValues(item apitypes.Voice) ([]any, error) {
	data, err := json.Marshal(item.ProviderData)
	if err != nil {
		return nil, err
	}
	var providerVoiceID, synced *string
	if value := ProviderDataString(item, "voice_id"); value != "" {
		providerVoiceID = &value
	}
	if item.SyncedAt != nil {
		synced = new(item.SyncedAt.UTC().Format(time.RFC3339Nano))
	}
	return []any{item.Id, string(item.Source), string(item.Provider.Kind), item.Provider.Id, providerVoiceID, string(data), item.DisplayName, item.Description, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano), synced}, nil
}

func scanVoice(row interface{ Scan(...any) error }) (apitypes.Voice, error) {
	var item apitypes.Voice
	var providerVoiceID, synced *string
	var data, created, updated string
	if err := row.Scan(&item.Id, &item.Source, &item.Provider.Kind, &item.Provider.Id, &providerVoiceID, &data, &item.DisplayName, &item.Description, &created, &updated, &synced); err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(data), &item.ProviderData); err != nil {
		return item, err
	}
	var err error
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return item, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return item, err
	}
	if synced != nil {
		value, err := time.Parse(time.RFC3339Nano, *synced)
		if err != nil {
			return item, err
		}
		item.SyncedAt = &value
	}
	return item, nil
}

func reconcileVoiceSQL(ctx context.Context, db *sqlx.DB, kind apitypes.VoiceProviderKind, providerID string, desired []apitypes.Voice) (int32, int32, int32, error) {
	seen := make(map[string]struct{}, len(desired))
	for _, candidate := range desired {
		if candidate.Provider.Kind != kind || candidate.Provider.Id != providerID || candidate.Source != apitypes.VoiceSourceSync {
			return 0, 0, 0, fmt.Errorf("provider voice %q does not belong to requested provider", candidate.Id)
		}
		if err := customid.ValidateResourceID(candidate.Id); err != nil {
			return 0, 0, 0, err
		}
		identity := ProviderDataString(candidate, "voice_id")
		if identity == "" {
			return 0, 0, 0, errors.New("provider voice is missing voice_id")
		}
		if _, ok := seen[identity]; ok {
			return 0, 0, 0, fmt.Errorf("duplicate provider voice_id %q", identity)
		}
		seen[identity] = struct{}{}
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()
	if err := lockVoiceProvider(ctx, tx, kind, providerID); err != nil {
		return 0, 0, 0, err
	}
	query := `SELECT ` + voiceSQLColumns + ` FROM voices WHERE source='sync' AND provider_kind=? AND provider_id=? ORDER BY id`
	if db.DriverName() == "pgx" || db.DriverName() == "postgres" {
		query += ` FOR UPDATE`
	}
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), string(kind), providerID)
	if err != nil {
		return 0, 0, 0, err
	}
	existing := map[string]apitypes.Voice{}
	for rows.Next() {
		item, err := scanVoice(rows)
		if err != nil {
			rows.Close()
			return 0, 0, 0, err
		}
		existing[ProviderDataString(item, "voice_id")] = item
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, 0, err
	}
	generation := uuid.NewString()
	var created, updated int32
	for start := 0; start < len(desired); start += 64 {
		end := min(start+64, len(desired))
		placeholders := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*12)
		for _, input := range desired[start:end] {
			candidate := input
			identity := ProviderDataString(candidate, "voice_id")
			if previous, ok := existing[identity]; ok {
				candidate.Id = previous.Id
				candidate.CreatedAt = previous.CreatedAt
				if SemanticEqual(previous, candidate) {
					candidate.UpdatedAt = previous.UpdatedAt
				} else {
					updated++
				}
			} else {
				created++
			}
			values, err := voiceSQLValues(candidate)
			if err != nil {
				return 0, 0, 0, err
			}
			args = append(args, values...)
			args = append(args, generation)
			placeholders = append(placeholders, "(?,?,?,?,?,?,?,?,?,?,?,?)")
		}
		query := `INSERT INTO voices(` + voiceSQLColumns + `,sync_generation) VALUES ` + strings.Join(placeholders, ",") + ` ON CONFLICT(id) DO UPDATE SET source=excluded.source,provider_kind=excluded.provider_kind,provider_id=excluded.provider_id,provider_voice_id=excluded.provider_voice_id,provider_data_json=excluded.provider_data_json,display_name=excluded.display_name,description=excluded.description,updated_at=excluded.updated_at,synced_at=excluded.synced_at,sync_generation=excluded.sync_generation WHERE voices.source='sync' AND voices.provider_kind=excluded.provider_kind AND voices.provider_id=excluded.provider_id AND voices.provider_voice_id=excluded.provider_voice_id`
		result, err := tx.ExecContext(ctx, tx.Rebind(query), args...)
		if err != nil {
			return 0, 0, 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, 0, 0, err
		}
		if count != int64(end-start) {
			return 0, 0, 0, errors.New("provider voice ID conflicts with another catalog record")
		}
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM voices WHERE source='sync' AND provider_kind=? AND provider_id=? AND (sync_generation IS NULL OR sync_generation<>?)`), string(kind), providerID, generation)
	if err != nil {
		return 0, 0, 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return created, updated, int32(deleted), nil
}

func deleteProviderVoiceSQL(ctx context.Context, db *sqlx.DB, kind apitypes.VoiceProviderKind, providerID string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := deleteProviderVoiceTx(ctx, tx, kind, providerID); err != nil {
		return err
	}
	return tx.Commit()
}

func deleteProviderVoiceTx(ctx context.Context, tx *sqlx.Tx, kind apitypes.VoiceProviderKind, providerID string) error {
	if err := lockVoiceProvider(ctx, tx, kind, providerID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM voices WHERE source='sync' AND provider_kind=? AND provider_id=?`), string(kind), providerID)
	return err
}

func getVoiceSQL(ctx context.Context, db *sqlx.DB, id string) (apitypes.Voice, error) {
	return scanVoice(db.QueryRowContext(ctx, db.Rebind(`SELECT `+voiceSQLColumns+` FROM voices WHERE id=?`), id))
}
func insertVoiceSQL(ctx context.Context, db *sqlx.DB, item apitypes.Voice) (bool, error) {
	args, err := voiceSQLValues(item)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO voices(`+voiceSQLColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`), args...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func deleteVoiceSQL(ctx context.Context, db *sqlx.DB, id string) (apitypes.Voice, error) {
	return scanVoice(db.QueryRowContext(ctx, db.Rebind(`DELETE FROM voices WHERE id=? RETURNING `+voiceSQLColumns), id))
}
func listVoiceSQL(ctx context.Context, db *sqlx.DB, filters Filters, cursor string, limit int) ([]apitypes.Voice, bool, *string, error) {
	var query strings.Builder
	query.WriteString(`SELECT ` + voiceSQLColumns + ` FROM voices WHERE id>?`)
	args := []any{cursor}
	for _, filter := range []struct {
		column string
		value  *string
	}{{"source", filters.Source}, {"provider_kind", filters.ProviderKind}, {"provider_id", filters.ProviderId}} {
		if filter.value != nil {
			query.WriteString(` AND ` + filter.column + `=?`)
			args = append(args, *filter.value)
		}
	}
	query.WriteString(` ORDER BY id LIMIT ?`)
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, db.Rebind(query.String()), args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.Voice, 0, limit+1)
	for rows.Next() {
		item, err := scanVoice(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	if len(items) <= limit {
		return items, false, nil, nil
	}
	items = items[:limit]
	next := items[len(items)-1].Id
	return items, true, &next, nil
}
