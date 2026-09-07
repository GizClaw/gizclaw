package gameplay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type catalogVersion struct {
	incarnation string
	revision    int64
}
type catalogScanner interface{ Scan(...any) error }

func initializeCatalogSQL(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS pet_definitions(id TEXT PRIMARY KEY CHECK(length(id)>0),character_prompt TEXT NOT NULL,voice_prompt TEXT NOT NULL,visual_json TEXT NOT NULL,pixa_path TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,revision BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS badge_definitions(id TEXT PRIMARY KEY CHECK(length(id)>0),display_name TEXT NOT NULL,description TEXT,reward_prompt TEXT,metadata_json TEXT NOT NULL,tags_json TEXT NOT NULL,pixa_path TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,revision BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS game_definitions(id TEXT PRIMARY KEY CHECK(length(id)>0),display_name TEXT NOT NULL,description TEXT,metadata_json TEXT NOT NULL,outcomes_json TEXT NOT NULL,score_schema_json TEXT NOT NULL,tags_json TEXT NOT NULL,icon_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,revision BIGINT NOT NULL)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const petDefColumns = "id,character_prompt,voice_prompt,visual_json,pixa_path,created_at,updated_at,incarnation,revision"

func encodePetDefSQL(item apitypes.PetDef) ([]any, error) {
	j2, err := json.Marshal(item.Spec.Visual)
	if err != nil {
		return nil, err
	}
	return []any{item.Id, item.Spec.Character.Prompt, item.Spec.Voice.Prompt, string(j2), item.PixaPath, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil
}
func scanPetDefSQL(row catalogScanner) (apitypes.PetDef, catalogVersion, error) {
	var item apitypes.PetDef
	var version catalogVersion
	var created, updated string
	var j2 string
	if err := row.Scan(&item.Id, &item.Spec.Character.Prompt, &item.Spec.Voice.Prompt, &j2, &item.PixaPath, &created, &updated, &version.incarnation, &version.revision); err != nil {
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
	if err := json.Unmarshal([]byte(j2), &item.Spec.Visual); err != nil {
		return item, version, err
	}
	return item, version, nil
}
func insertPetDefSQL(ctx context.Context, db *sqlx.DB, item apitypes.PetDef) (bool, error) {
	values, err := encodePetDefSQL(item)
	if err != nil {
		return false, err
	}
	values = append(values, uuid.NewString(), int64(1))
	result, err := db.ExecContext(ctx, db.Rebind("INSERT INTO pet_definitions("+petDefColumns+") VALUES ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+") ON CONFLICT(id) DO NOTHING"), values...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func getPetDefSQL(ctx context.Context, db *sqlx.DB, id string) (apitypes.PetDef, catalogVersion, error) {
	return scanPetDefSQL(db.QueryRowContext(ctx, db.Rebind("SELECT "+petDefColumns+" FROM pet_definitions WHERE id=?"), id))
}
func updatePetDefSQL(ctx context.Context, db *sqlx.DB, item apitypes.PetDef, version catalogVersion) (apitypes.PetDef, catalogVersion, error) {
	values, err := encodePetDefSQL(item)
	if err != nil {
		return item, version, err
	}
	values = append(values[1:len(values)-2], values[len(values)-1], item.Id, version.incarnation, version.revision)
	return scanPetDefSQL(db.QueryRowContext(ctx, db.Rebind("UPDATE pet_definitions SET character_prompt=?,voice_prompt=?,visual_json=?,pixa_path=?,updated_at=?,revision=revision+1 WHERE id=? AND incarnation=? AND revision=? RETURNING "+petDefColumns), values...))
}
func deletePetDefSQL(ctx context.Context, db *sqlx.DB, id string, version catalogVersion) (apitypes.PetDef, catalogVersion, error) {
	return scanPetDefSQL(db.QueryRowContext(ctx, db.Rebind("DELETE FROM pet_definitions WHERE id=? AND incarnation=? AND revision=? RETURNING "+petDefColumns), id, version.incarnation, version.revision))
}
func listPetDefSQL(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.PetDef, bool, *string, error) {
	if limit <= 0 {
		return nil, false, nil, fmt.Errorf("catalog page limit must be positive")
	}
	rows, err := db.QueryContext(ctx, db.Rebind("SELECT "+petDefColumns+" FROM pet_definitions WHERE id>? ORDER BY id LIMIT ?"), cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.PetDef, 0, limit)
	for rows.Next() {
		if len(items) == limit {
			next := items[len(items)-1].Id
			return items, true, &next, nil
		}
		item, _, err := scanPetDefSQL(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, false, nil, rows.Err()
}

const badgeDefColumns = "id,display_name,description,reward_prompt,metadata_json,tags_json,pixa_path,created_at,updated_at,incarnation,revision"

func encodeBadgeDefSQL(item apitypes.BadgeDef) ([]any, error) {
	j3, err := json.Marshal(item.Spec.Metadata)
	if err != nil {
		return nil, err
	}
	j4, err := json.Marshal(item.Spec.Tags)
	if err != nil {
		return nil, err
	}
	return []any{item.Id, item.Spec.DisplayName, item.Spec.Description, item.Spec.RewardPrompt, string(j3), string(j4), item.PixaPath, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil
}
func scanBadgeDefSQL(row catalogScanner) (apitypes.BadgeDef, catalogVersion, error) {
	var item apitypes.BadgeDef
	var version catalogVersion
	var created, updated string
	var j3 string
	var j4 string
	if err := row.Scan(&item.Id, &item.Spec.DisplayName, &item.Spec.Description, &item.Spec.RewardPrompt, &j3, &j4, &item.PixaPath, &created, &updated, &version.incarnation, &version.revision); err != nil {
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
	if err := json.Unmarshal([]byte(j3), &item.Spec.Metadata); err != nil {
		return item, version, err
	}
	if err := json.Unmarshal([]byte(j4), &item.Spec.Tags); err != nil {
		return item, version, err
	}
	return item, version, nil
}
func insertBadgeDefSQL(ctx context.Context, db *sqlx.DB, item apitypes.BadgeDef) (bool, error) {
	values, err := encodeBadgeDefSQL(item)
	if err != nil {
		return false, err
	}
	values = append(values, uuid.NewString(), int64(1))
	result, err := db.ExecContext(ctx, db.Rebind("INSERT INTO badge_definitions("+badgeDefColumns+") VALUES ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+") ON CONFLICT(id) DO NOTHING"), values...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func getBadgeDefSQL(ctx context.Context, db *sqlx.DB, id string) (apitypes.BadgeDef, catalogVersion, error) {
	return scanBadgeDefSQL(db.QueryRowContext(ctx, db.Rebind("SELECT "+badgeDefColumns+" FROM badge_definitions WHERE id=?"), id))
}
func updateBadgeDefSQL(ctx context.Context, db *sqlx.DB, item apitypes.BadgeDef, version catalogVersion) (apitypes.BadgeDef, catalogVersion, error) {
	values, err := encodeBadgeDefSQL(item)
	if err != nil {
		return item, version, err
	}
	values = append(values[1:len(values)-2], values[len(values)-1], item.Id, version.incarnation, version.revision)
	return scanBadgeDefSQL(db.QueryRowContext(ctx, db.Rebind("UPDATE badge_definitions SET display_name=?,description=?,reward_prompt=?,metadata_json=?,tags_json=?,pixa_path=?,updated_at=?,revision=revision+1 WHERE id=? AND incarnation=? AND revision=? RETURNING "+badgeDefColumns), values...))
}
func deleteBadgeDefSQL(ctx context.Context, db *sqlx.DB, id string, version catalogVersion) (apitypes.BadgeDef, catalogVersion, error) {
	return scanBadgeDefSQL(db.QueryRowContext(ctx, db.Rebind("DELETE FROM badge_definitions WHERE id=? AND incarnation=? AND revision=? RETURNING "+badgeDefColumns), id, version.incarnation, version.revision))
}
func listBadgeDefSQL(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.BadgeDef, bool, *string, error) {
	if limit <= 0 {
		return nil, false, nil, fmt.Errorf("catalog page limit must be positive")
	}
	rows, err := db.QueryContext(ctx, db.Rebind("SELECT "+badgeDefColumns+" FROM badge_definitions WHERE id>? ORDER BY id LIMIT ?"), cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.BadgeDef, 0, limit)
	for rows.Next() {
		if len(items) == limit {
			next := items[len(items)-1].Id
			return items, true, &next, nil
		}
		item, _, err := scanBadgeDefSQL(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, false, nil, rows.Err()
}

const gameDefColumns = "id,display_name,description,metadata_json,outcomes_json,score_schema_json,tags_json,icon_json,created_at,updated_at,incarnation,revision"

func encodeGameDefSQL(item apitypes.GameDef) ([]any, error) {
	j2, err := json.Marshal(item.Spec.Metadata)
	if err != nil {
		return nil, err
	}
	j3, err := json.Marshal(item.Spec.Outcomes)
	if err != nil {
		return nil, err
	}
	j4, err := json.Marshal(item.Spec.ScoreSchema)
	if err != nil {
		return nil, err
	}
	j5, err := json.Marshal(item.Spec.Tags)
	if err != nil {
		return nil, err
	}
	j6, err := json.Marshal(item.Icon)
	if err != nil {
		return nil, err
	}
	return []any{item.Id, item.Spec.DisplayName, item.Spec.Description, string(j2), string(j3), string(j4), string(j5), string(j6), item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil
}
func scanGameDefSQL(row catalogScanner) (apitypes.GameDef, catalogVersion, error) {
	var item apitypes.GameDef
	var version catalogVersion
	var created, updated string
	var j2 string
	var j3 string
	var j4 string
	var j5 string
	var j6 string
	if err := row.Scan(&item.Id, &item.Spec.DisplayName, &item.Spec.Description, &j2, &j3, &j4, &j5, &j6, &created, &updated, &version.incarnation, &version.revision); err != nil {
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
	if err := json.Unmarshal([]byte(j2), &item.Spec.Metadata); err != nil {
		return item, version, err
	}
	if err := json.Unmarshal([]byte(j3), &item.Spec.Outcomes); err != nil {
		return item, version, err
	}
	if err := json.Unmarshal([]byte(j4), &item.Spec.ScoreSchema); err != nil {
		return item, version, err
	}
	if err := json.Unmarshal([]byte(j5), &item.Spec.Tags); err != nil {
		return item, version, err
	}
	if err := json.Unmarshal([]byte(j6), &item.Icon); err != nil {
		return item, version, err
	}
	return item, version, nil
}
func insertGameDefSQL(ctx context.Context, db *sqlx.DB, item apitypes.GameDef) (bool, error) {
	values, err := encodeGameDefSQL(item)
	if err != nil {
		return false, err
	}
	values = append(values, uuid.NewString(), int64(1))
	result, err := db.ExecContext(ctx, db.Rebind("INSERT INTO game_definitions("+gameDefColumns+") VALUES ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+") ON CONFLICT(id) DO NOTHING"), values...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func getGameDefSQL(ctx context.Context, db *sqlx.DB, id string) (apitypes.GameDef, catalogVersion, error) {
	return scanGameDefSQL(db.QueryRowContext(ctx, db.Rebind("SELECT "+gameDefColumns+" FROM game_definitions WHERE id=?"), id))
}
func updateGameDefSQL(ctx context.Context, db *sqlx.DB, item apitypes.GameDef, version catalogVersion) (apitypes.GameDef, catalogVersion, error) {
	values, err := encodeGameDefSQL(item)
	if err != nil {
		return item, version, err
	}
	values = append(values[1:len(values)-2], values[len(values)-1], item.Id, version.incarnation, version.revision)
	return scanGameDefSQL(db.QueryRowContext(ctx, db.Rebind("UPDATE game_definitions SET display_name=?,description=?,metadata_json=?,outcomes_json=?,score_schema_json=?,tags_json=?,icon_json=?,updated_at=?,revision=revision+1 WHERE id=? AND incarnation=? AND revision=? RETURNING "+gameDefColumns), values...))
}
func deleteGameDefSQL(ctx context.Context, db *sqlx.DB, id string, version catalogVersion) (apitypes.GameDef, catalogVersion, error) {
	return scanGameDefSQL(db.QueryRowContext(ctx, db.Rebind("DELETE FROM game_definitions WHERE id=? AND incarnation=? AND revision=? RETURNING "+gameDefColumns), id, version.incarnation, version.revision))
}
func listGameDefSQL(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.GameDef, bool, *string, error) {
	if limit <= 0 {
		return nil, false, nil, fmt.Errorf("catalog page limit must be positive")
	}
	rows, err := db.QueryContext(ctx, db.Rebind("SELECT "+gameDefColumns+" FROM game_definitions WHERE id>? ORDER BY id LIMIT ?"), cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.GameDef, 0, limit)
	for rows.Next() {
		if len(items) == limit {
			next := items[len(items)-1].Id
			return items, true, &next, nil
		}
		item, _, err := scanGameDefSQL(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, false, nil, rows.Err()
}
