package runtimeprofile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type profileRowVersion struct {
	incarnation string
	revision    int64
}
type profileScanner interface{ Scan(...any) error }

func initializeProfileSQL(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS runtime_profile_owners(owner_public_key TEXT PRIMARY KEY, runtime_profile_id TEXT,binding_id TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS runtime_profile_owners_profile ON runtime_profile_owners(runtime_profile_id,owner_public_key)`,

		`CREATE TABLE IF NOT EXISTS runtime_profiles(id TEXT PRIMARY KEY CHECK(length(id)>0),revision TEXT NOT NULL,resources_json TEXT NOT NULL,workflows_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,row_version BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS registration_tokens(id TEXT PRIMARY KEY CHECK(length(id)>0),token TEXT NOT NULL,runtime_profile_id TEXT NOT NULL,firmware_id TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,row_version BIGINT NOT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS registration_tokens_token ON registration_tokens(token)`,
		`CREATE INDEX IF NOT EXISTS registration_tokens_profile ON registration_tokens(runtime_profile_id,id)`,
		`CREATE INDEX IF NOT EXISTS registration_tokens_firmware ON registration_tokens(firmware_id,id)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const runtimeProfileColumns = "id,revision,resources_json,workflows_json,created_at,updated_at,incarnation,row_version"

func encodeRuntimeProfileSQL(item apitypes.RuntimeProfile) ([]any, error) {
	j1, err := json.Marshal(item.Spec.Resources)
	if err != nil {
		return nil, err
	}
	j2, err := json.Marshal(item.Spec.Workflows)
	if err != nil {
		return nil, err
	}
	return []any{item.Id, item.Revision, string(j1), string(j2), item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil
}
func scanRuntimeProfileSQL(row profileScanner) (apitypes.RuntimeProfile, profileRowVersion, error) {
	var item apitypes.RuntimeProfile
	var version profileRowVersion
	var created, updated string
	var j1 string
	var j2 string
	if err := row.Scan(&item.Id, &item.Revision, &j1, &j2, &created, &updated, &version.incarnation, &version.revision); err != nil {
		return item, version, err
	}
	var err error
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return item, version, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return item, version, err
	}
	if err := json.Unmarshal([]byte(j1), &item.Spec.Resources); err != nil {
		return item, version, err
	}
	if err := json.Unmarshal([]byte(j2), &item.Spec.Workflows); err != nil {
		return item, version, err
	}
	return item, version, nil
}
func insertRuntimeProfileSQL(ctx context.Context, db *sqlx.DB, item apitypes.RuntimeProfile) (bool, error) {
	values, err := encodeRuntimeProfileSQL(item)
	if err != nil {
		return false, err
	}
	values = append(values, uuid.NewString(), int64(1))
	result, err := db.ExecContext(ctx, db.Rebind("INSERT INTO runtime_profiles("+runtimeProfileColumns+") VALUES ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+") ON CONFLICT(id) DO NOTHING"), values...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func getRuntimeProfileSQL(ctx context.Context, db *sqlx.DB, id string) (apitypes.RuntimeProfile, profileRowVersion, error) {
	return scanRuntimeProfileSQL(db.QueryRowContext(ctx, db.Rebind("SELECT "+runtimeProfileColumns+" FROM runtime_profiles WHERE id=?"), id))
}
func updateRuntimeProfileSQL(ctx context.Context, db *sqlx.DB, item apitypes.RuntimeProfile, version profileRowVersion) (apitypes.RuntimeProfile, profileRowVersion, error) {
	values, err := encodeRuntimeProfileSQL(item)
	if err != nil {
		return item, version, err
	}
	values = append(values[1:len(values)-2], values[len(values)-1], item.Id, version.incarnation, version.revision)
	return scanRuntimeProfileSQL(db.QueryRowContext(ctx, db.Rebind("UPDATE runtime_profiles SET revision=?,resources_json=?,workflows_json=?,updated_at=?,row_version=row_version+1 WHERE id=? AND incarnation=? AND row_version=? RETURNING "+runtimeProfileColumns), values...))
}
func deleteRuntimeProfileSQL(ctx context.Context, db *sqlx.DB, id string, version profileRowVersion) (apitypes.RuntimeProfile, profileRowVersion, error) {
	return scanRuntimeProfileSQL(db.QueryRowContext(ctx, db.Rebind("DELETE FROM runtime_profiles WHERE id=? AND incarnation=? AND row_version=? RETURNING "+runtimeProfileColumns), id, version.incarnation, version.revision))
}
func listRuntimeProfileSQL(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.RuntimeProfile, bool, *string, error) {
	if limit <= 0 {
		return nil, false, nil, fmt.Errorf("profile page limit must be positive")
	}
	rows, err := db.QueryContext(ctx, db.Rebind("SELECT "+runtimeProfileColumns+" FROM runtime_profiles WHERE id>? ORDER BY id LIMIT ?"), cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.RuntimeProfile, 0, limit)
	for rows.Next() {
		if len(items) == limit {
			next := items[len(items)-1].Id
			return items, true, &next, nil
		}
		item, _, err := scanRuntimeProfileSQL(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, false, nil, rows.Err()
}

const registrationTokenColumns = "id,token,runtime_profile_id,firmware_id,created_at,updated_at,incarnation,row_version"

func encodeRegistrationTokenSQL(item apitypes.RegistrationToken) ([]any, error) {
	return []any{item.Id, item.Token, item.RuntimeProfileId, item.FirmwareId, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil
}
func scanRegistrationTokenSQL(row profileScanner) (apitypes.RegistrationToken, profileRowVersion, error) {
	var item apitypes.RegistrationToken
	var version profileRowVersion
	var created, updated string
	if err := row.Scan(&item.Id, &item.Token, &item.RuntimeProfileId, &item.FirmwareId, &created, &updated, &version.incarnation, &version.revision); err != nil {
		return item, version, err
	}
	var err error
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return item, version, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return item, version, err
	}
	return item, version, nil
}
func insertRegistrationTokenSQL(ctx context.Context, db *sqlx.DB, item apitypes.RegistrationToken) (bool, error) {
	values, err := encodeRegistrationTokenSQL(item)
	if err != nil {
		return false, err
	}
	values = append(values, uuid.NewString(), int64(1))
	result, err := db.ExecContext(ctx, db.Rebind("INSERT INTO registration_tokens("+registrationTokenColumns+") VALUES ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+") ON CONFLICT(id) DO NOTHING"), values...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func getRegistrationTokenSQL(ctx context.Context, db *sqlx.DB, id string) (apitypes.RegistrationToken, profileRowVersion, error) {
	return scanRegistrationTokenSQL(db.QueryRowContext(ctx, db.Rebind("SELECT "+registrationTokenColumns+" FROM registration_tokens WHERE id=?"), id))
}
func updateRegistrationTokenSQL(ctx context.Context, db *sqlx.DB, item apitypes.RegistrationToken, version profileRowVersion) (apitypes.RegistrationToken, profileRowVersion, error) {
	values, err := encodeRegistrationTokenSQL(item)
	if err != nil {
		return item, version, err
	}
	values = append(values[1:len(values)-2], values[len(values)-1], item.Id, version.incarnation, version.revision)
	return scanRegistrationTokenSQL(db.QueryRowContext(ctx, db.Rebind("UPDATE registration_tokens SET token=?,runtime_profile_id=?,firmware_id=?,updated_at=?,row_version=row_version+1 WHERE id=? AND incarnation=? AND row_version=? RETURNING "+registrationTokenColumns), values...))
}
func deleteRegistrationTokenSQL(ctx context.Context, db *sqlx.DB, id string, version profileRowVersion) (apitypes.RegistrationToken, profileRowVersion, error) {
	return scanRegistrationTokenSQL(db.QueryRowContext(ctx, db.Rebind("DELETE FROM registration_tokens WHERE id=? AND incarnation=? AND row_version=? RETURNING "+registrationTokenColumns), id, version.incarnation, version.revision))
}
func listRegistrationTokenSQL(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.RegistrationToken, bool, *string, error) {
	if limit <= 0 {
		return nil, false, nil, fmt.Errorf("profile page limit must be positive")
	}
	rows, err := db.QueryContext(ctx, db.Rebind("SELECT "+registrationTokenColumns+" FROM registration_tokens WHERE id>? ORDER BY id LIMIT ?"), cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.RegistrationToken, 0, limit)
	for rows.Next() {
		if len(items) == limit {
			next := items[len(items)-1].Id
			return items, true, &next, nil
		}
		item, _, err := scanRegistrationTokenSQL(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, false, nil, rows.Err()
}

func resolveOwnerProfileSQL(ctx context.Context, db *sqlx.DB, owner string) (apitypes.RuntimeProfile, profileRowVersion, error) {
	columns := "p." + strings.ReplaceAll(runtimeProfileColumns, ",", ",p.")
	return scanRuntimeProfileSQL(db.QueryRowContext(ctx, db.Rebind("SELECT "+columns+" FROM runtime_profiles p JOIN runtime_profile_owners o ON o.runtime_profile_id=p.id WHERE o.owner_public_key=?"), owner))
}

type profileExtraRow struct {
	row   profileScanner
	extra []any
}

func (r profileExtraRow) Scan(dest ...any) error { return r.row.Scan(append(dest, r.extra...)...) }

func resolveRegistrationSQL(ctx context.Context, db *sqlx.DB, token string) (string, *string, apitypes.RuntimeProfile, error) {
	columns := "p." + strings.ReplaceAll(runtimeProfileColumns, ",", ",p.")
	var id string
	var firmware *string
	row := db.QueryRowContext(ctx, db.Rebind("SELECT "+columns+",t.id,t.firmware_id FROM registration_tokens t JOIN runtime_profiles p ON p.id=t.runtime_profile_id WHERE t.token=?"), token)
	profile, _, err := scanRuntimeProfileSQL(profileExtraRow{row: row, extra: []any{&id, &firmware}})
	return id, firmware, profile, err
}

func setOwnerProfileSQL(ctx context.Context, db *sqlx.DB, owner, profileID string) (*string, string, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	stamp := uuid.NewString()
	if _, err := tx.ExecContext(ctx, tx.Rebind("INSERT INTO runtime_profile_owners(owner_public_key,runtime_profile_id,binding_id) VALUES (?,NULL,?) ON CONFLICT(owner_public_key) DO NOTHING"), owner, stamp); err != nil {
		return nil, "", err
	}
	// Acquire this owner's row before reading the value that rollback may restore.
	if _, err := tx.ExecContext(ctx, tx.Rebind("UPDATE runtime_profile_owners SET binding_id=binding_id WHERE owner_public_key=?"), owner); err != nil {
		return nil, "", err
	}
	var previous *string
	if err := tx.QueryRowContext(ctx, tx.Rebind("SELECT runtime_profile_id FROM runtime_profile_owners WHERE owner_public_key=?"), owner).Scan(&previous); err != nil {
		return nil, "", err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind("UPDATE runtime_profile_owners SET runtime_profile_id=?,binding_id=? WHERE owner_public_key=? AND EXISTS(SELECT 1 FROM runtime_profiles WHERE id=?)"), profileID, stamp, owner, profileID)
	if err != nil {
		return nil, "", err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, "", err
	}
	if count == 0 {
		return nil, "", sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	return previous, stamp, nil
}

func restoreOwnerProfileSQL(ctx context.Context, db *sqlx.DB, owner, stamp string, previous *string) error {
	var result sql.Result
	var err error
	if previous == nil {
		result, err = db.ExecContext(ctx, db.Rebind("DELETE FROM runtime_profile_owners WHERE owner_public_key=? AND binding_id=?"), owner, stamp)
	} else {
		result, err = db.ExecContext(ctx, db.Rebind("UPDATE runtime_profile_owners SET runtime_profile_id=?,binding_id=? WHERE owner_public_key=? AND binding_id=?"), *previous, uuid.NewString(), owner, stamp)
	}
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("runtime profile owner binding changed before rollback")
	}
	return nil
}
