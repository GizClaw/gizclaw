package gameplay

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
	"reflect"
	"testing"
	"time"
)

func catalogSQLTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := initializeCatalogSQL(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := initializeCatalogSQL(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPetDefSQLRoundTripAndConflict(t *testing.T) {
	db := catalogSQLTestDB(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 6, 1, 2, 3, 4, time.UTC)
	item := apitypes.PetDef{Id: "opaque/id", CreatedAt: now, UpdatedAt: now, Spec: apitypes.PetDefSpec{Character: apitypes.PetDefCharacterSpec{Prompt: "character"}, Voice: apitypes.PetDefVoiceSpec{Prompt: "voice"}}, PixaPath: new("asset")}
	if created, err := insertPetDefSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	if created, err := insertPetDefSQL(ctx, db, item); err != nil || created {
		t.Fatalf("duplicate = %v, %v", created, err)
	}
	got, version, err := getPetDefSQL(ctx, db, item.Id)
	if err != nil || !reflect.DeepEqual(got, item) {
		t.Fatalf("read = %#v, %v; want %#v", got, err, item)
	}
	item.UpdatedAt = now.Add(time.Second)
	updated, next, err := updatePetDefSQL(ctx, db, item, version)
	if err != nil || !reflect.DeepEqual(updated, item) || next.revision != version.revision+1 {
		t.Fatalf("update = %#v, %#v, %v", updated, next, err)
	}
	if _, _, err := updatePetDefSQL(ctx, db, item, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale update = %v", err)
	}
	if _, _, err := deletePetDefSQL(ctx, db, item.Id, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale delete = %v", err)
	}
	if _, _, err := deletePetDefSQL(ctx, db, item.Id, next); err != nil {
		t.Fatal(err)
	}
	if created, err := insertPetDefSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("recreate = %v, %v", created, err)
	}
	if _, _, err := updatePetDefSQL(ctx, db, item, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement update = %v", err)
	}
	if _, _, err := deletePetDefSQL(ctx, db, item.Id, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement delete = %v", err)
	}
}

func TestBadgeDefSQLRoundTripAndConflict(t *testing.T) {
	db := catalogSQLTestDB(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 6, 1, 2, 3, 4, time.UTC)
	item := apitypes.BadgeDef{Id: "opaque/id", CreatedAt: now, UpdatedAt: now, Spec: apitypes.BadgeDefSpec{DisplayName: "badge", RewardPrompt: new("reward"), Tags: new([]string{"tag"})}, PixaPath: new("asset")}
	if created, err := insertBadgeDefSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	if created, err := insertBadgeDefSQL(ctx, db, item); err != nil || created {
		t.Fatalf("duplicate = %v, %v", created, err)
	}
	got, version, err := getBadgeDefSQL(ctx, db, item.Id)
	if err != nil || !reflect.DeepEqual(got, item) {
		t.Fatalf("read = %#v, %v; want %#v", got, err, item)
	}
	item.UpdatedAt = now.Add(time.Second)
	updated, next, err := updateBadgeDefSQL(ctx, db, item, version)
	if err != nil || !reflect.DeepEqual(updated, item) || next.revision != version.revision+1 {
		t.Fatalf("update = %#v, %#v, %v", updated, next, err)
	}
	if _, _, err := updateBadgeDefSQL(ctx, db, item, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale update = %v", err)
	}
	if _, _, err := deleteBadgeDefSQL(ctx, db, item.Id, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale delete = %v", err)
	}
	if _, _, err := deleteBadgeDefSQL(ctx, db, item.Id, next); err != nil {
		t.Fatal(err)
	}
	if created, err := insertBadgeDefSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("recreate = %v, %v", created, err)
	}
	if _, _, err := updateBadgeDefSQL(ctx, db, item, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement update = %v", err)
	}
	if _, _, err := deleteBadgeDefSQL(ctx, db, item.Id, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement delete = %v", err)
	}
}

func TestGameDefSQLRoundTripAndConflict(t *testing.T) {
	db := catalogSQLTestDB(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 6, 1, 2, 3, 4, time.UTC)
	item := apitypes.GameDef{Id: "opaque/id", CreatedAt: now, UpdatedAt: now, Spec: apitypes.GameDefSpec{DisplayName: "game", Outcomes: new([]string{"win"}), Tags: new([]string{"tag"})}}
	if created, err := insertGameDefSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	if created, err := insertGameDefSQL(ctx, db, item); err != nil || created {
		t.Fatalf("duplicate = %v, %v", created, err)
	}
	got, version, err := getGameDefSQL(ctx, db, item.Id)
	if err != nil || !reflect.DeepEqual(got, item) {
		t.Fatalf("read = %#v, %v; want %#v", got, err, item)
	}
	item.UpdatedAt = now.Add(time.Second)
	updated, next, err := updateGameDefSQL(ctx, db, item, version)
	if err != nil || !reflect.DeepEqual(updated, item) || next.revision != version.revision+1 {
		t.Fatalf("update = %#v, %#v, %v", updated, next, err)
	}
	if _, _, err := updateGameDefSQL(ctx, db, item, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale update = %v", err)
	}
	if _, _, err := deleteGameDefSQL(ctx, db, item.Id, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale delete = %v", err)
	}
	if _, _, err := deleteGameDefSQL(ctx, db, item.Id, next); err != nil {
		t.Fatal(err)
	}
	if created, err := insertGameDefSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("recreate = %v, %v", created, err)
	}
	if _, _, err := updateGameDefSQL(ctx, db, item, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement update = %v", err)
	}
	if _, _, err := deleteGameDefSQL(ctx, db, item.Id, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement delete = %v", err)
	}
}

func TestCatalogSQLPaginationDoesNotDecodeOutsidePage(t *testing.T) {
	db := catalogSQLTestDB(t)
	ctx := t.Context()
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 2000 {
		visual := `{}`
		if i < 1000 || i >= 1010 {
			visual = `invalid json`
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pet_definitions(id,character_prompt,voice_prompt,visual_json,created_at,updated_at,incarnation,revision) VALUES (?,?,?,?,'2026-09-06T00:00:00Z','2026-09-06T00:00:00Z','initial',1)`, fmt.Sprintf("pet-%04d", i), "character", "voice", visual); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	items, more, cursor, err := listPetDefSQL(ctx, db, "pet-0999", 10)
	if err != nil || len(items) != 10 || !more || cursor == nil || *cursor != "pet-1009" {
		t.Fatalf("page = %#v, %v, %v, %v", items, more, cursor, err)
	}
}
