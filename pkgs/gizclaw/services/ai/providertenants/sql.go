package providertenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type tenantObject interface {
	apitypes.OpenAITenant | apitypes.GeminiTenant | apitypes.DashScopeTenant | apitypes.DeepSeekTenant | apitypes.MiniMaxTenant | apitypes.VolcTenant
}

type tenantFields struct {
	ID           string     `json:"id"`
	CredentialID string     `json:"credential_id"`
	Description  *string    `json:"description,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
}

const tenantColumns = "id,credential_id,description,created_at,updated_at,last_synced_at,config_json,incarnation"

// Initialize creates the typed Provider Tenant catalog once at startup.
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
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS provider_tenants(provider_kind TEXT NOT NULL CHECK(provider_kind IN ('openai','gemini','dashscope','deepseek','minimax','volc')),id TEXT NOT NULL CHECK(length(id)>0),credential_id TEXT NOT NULL,description TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,last_synced_at TEXT,config_json TEXT NOT NULL,incarnation TEXT NOT NULL,PRIMARY KEY(provider_kind,id))`,
		`CREATE INDEX IF NOT EXISTS provider_tenants_credential ON provider_tenants(credential_id,provider_kind,id)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func tenantValues[T tenantObject](item T) (tenantFields, string, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return tenantFields{}, "", err
	}
	var fields tenantFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fields, "", err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		return fields, "", err
	}
	for _, column := range []string{"id", "credential_id", "description", "created_at", "updated_at", "last_synced_at"} {
		delete(config, column)
	}
	raw, err = json.Marshal(config)
	return fields, string(raw), err
}

func createSQLTenant[T tenantObject](ctx context.Context, db *sqlx.DB, kind string, item T) (bool, error) {
	fields, config, err := tenantValues(item)
	if err != nil {
		return false, err
	}
	var synced *string
	if fields.LastSyncedAt != nil {
		synced = new(fields.LastSyncedAt.UTC().Format(time.RFC3339Nano))
	}
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO provider_tenants(provider_kind,`+tenantColumns+`) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(provider_kind,id) DO NOTHING`), kind, fields.ID, fields.CredentialID, fields.Description, fields.CreatedAt.UTC().Format(time.RFC3339Nano), fields.UpdatedAt.UTC().Format(time.RFC3339Nano), synced, config, uuid.NewString())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func scanTenant[T tenantObject](row interface{ Scan(...any) error }) (T, string, error) {
	var item T
	var id, credentialID, created, updated, config, incarnation string
	var description, synced *string
	if err := row.Scan(&id, &credentialID, &description, &created, &updated, &synced, &config, &incarnation); err != nil {
		return item, "", err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(config), &values); err != nil {
		return item, "", err
	}
	if values == nil {
		return item, "", errors.New("provider tenant: null configuration")
	}
	for _, field := range []struct {
		name  string
		value any
	}{{"id", id}, {"credential_id", credentialID}, {"description", description}, {"created_at", created}, {"updated_at", updated}, {"last_synced_at", synced}} {
		raw, err := json.Marshal(field.value)
		if err != nil {
			return item, "", err
		}
		values[field.name] = raw
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return item, "", err
	}
	err = json.Unmarshal(raw, &item)
	return item, incarnation, err
}

func getSQLTenant[T tenantObject](ctx context.Context, db *sqlx.DB, kind, id string) (T, error) {
	item, _, err := scanTenant[T](db.QueryRowContext(ctx, db.Rebind(`SELECT `+tenantColumns+` FROM provider_tenants WHERE provider_kind=? AND id=?`), kind, id))
	return item, err
}

func updateSQLTenant[T tenantObject](ctx context.Context, db *sqlx.DB, kind string, item T) (T, error) {
	fields, config, err := tenantValues(item)
	if err != nil {
		var zero T
		return zero, err
	}
	// A configuration replacement invalidates snapshots held by in-flight sync
	// and deletion operations, even when the caller reuses the same timestamp.
	result, _, err := scanTenant[T](db.QueryRowContext(ctx, db.Rebind(`UPDATE provider_tenants SET credential_id=?,description=?,updated_at=?,config_json=?,incarnation=? WHERE provider_kind=? AND id=? RETURNING `+tenantColumns), fields.CredentialID, fields.Description, fields.UpdatedAt.UTC().Format(time.RFC3339Nano), config, uuid.NewString(), kind, fields.ID))
	return result, err
}

func deleteSQLTenant[T tenantObject](ctx context.Context, db *sqlx.DB, kind, id string) (T, error) {
	item, _, err := scanTenant[T](db.QueryRowContext(ctx, db.Rebind(`DELETE FROM provider_tenants WHERE provider_kind=? AND id=? RETURNING `+tenantColumns), kind, id))
	return item, err
}

// deleteSQLTenantIncarnation locks and removes the observed tenant before
// dependent cleanup. The uncommitted deletion prevents a replacement with the
// same identity from being published until cleanup finishes.
func deleteSQLTenantIncarnation[T tenantObject](ctx context.Context, db *sqlx.DB, kind, id, incarnation string, cleanup func(*sqlx.Tx) error) (T, error) {
	var zero T
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	item, _, err := scanTenant[T](tx.QueryRowContext(ctx, tx.Rebind(`DELETE FROM provider_tenants WHERE provider_kind=? AND id=? AND incarnation=? RETURNING `+tenantColumns), kind, id, incarnation))
	if err != nil {
		return zero, err
	}
	if cleanup != nil {
		if err := cleanup(tx); err != nil {
			return zero, err
		}
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return item, nil
}

func listSQLTenants[T tenantObject](ctx context.Context, db *sqlx.DB, kind, cursor string, limit int) ([]T, bool, *string, error) {
	rows, err := db.QueryContext(ctx, db.Rebind(`SELECT `+tenantColumns+` FROM provider_tenants WHERE provider_kind=? AND id>? ORDER BY id LIMIT ?`), kind, cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]T, 0, limit+1)
	var lastID string
	for rows.Next() {
		item, _, err := scanTenant[T](rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
		if len(items) == limit {
			fields, _, err := tenantValues(item)
			if err != nil {
				return nil, false, nil, err
			}
			lastID = fields.ID
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	if len(items) <= limit {
		return items, false, nil, nil
	}
	return items[:limit], true, &lastID, nil
}

// recordTenantSync updates synchronization metadata without replacing configuration.
func recordTenantSync(ctx context.Context, db *sqlx.DB, kind, id, incarnation string, at time.Time, reconcile func(*sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_tenants SET last_synced_at=?,updated_at=? WHERE provider_kind=? AND id=? AND incarnation=?`), at.UTC().Format(time.RFC3339Nano), at.UTC().Format(time.RFC3339Nano), kind, id, incarnation)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("provider tenant %q changed during synchronization", id)
	}
	if reconcile != nil {
		if err := reconcile(tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
