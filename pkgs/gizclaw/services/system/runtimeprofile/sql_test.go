package runtimeprofile

import (
	"database/sql"
	"errors"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func profileSQLTestDB(t testing.TB) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "profiles.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := initializeProfileSQL(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := initializeProfileSQL(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRuntimeProfileSQLRoundTripAndConflict(t *testing.T) {
	db := profileSQLTestDB(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 6, 1, 2, 3, 4, time.UTC)
	item := apitypes.RuntimeProfile{Id: "opaque/id", CreatedAt: now, UpdatedAt: now, Revision: "revision", Spec: apitypes.RuntimeProfileSpec{}}
	if created, err := insertRuntimeProfileSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	if created, err := insertRuntimeProfileSQL(ctx, db, item); err != nil || created {
		t.Fatalf("duplicate = %v, %v", created, err)
	}
	got, version, err := getRuntimeProfileSQL(ctx, db, item.Id)
	if err != nil || !reflect.DeepEqual(got, item) {
		t.Fatalf("read = %#v, %v; want %#v", got, err, item)
	}
	item.UpdatedAt = now.Add(time.Second)
	updated, next, err := updateRuntimeProfileSQL(ctx, db, item, version)
	if err != nil || !reflect.DeepEqual(updated, item) || next.revision != version.revision+1 {
		t.Fatalf("update = %#v, %#v, %v", updated, next, err)
	}
	if _, _, err := updateRuntimeProfileSQL(ctx, db, item, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale update = %v", err)
	}
	if _, _, err := deleteRuntimeProfileSQL(ctx, db, item.Id, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale delete = %v", err)
	}
	if _, _, err := deleteRuntimeProfileSQL(ctx, db, item.Id, next); err != nil {
		t.Fatal(err)
	}
	if created, err := insertRuntimeProfileSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("recreate = %v, %v", created, err)
	}
	if _, _, err := updateRuntimeProfileSQL(ctx, db, item, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement update = %v", err)
	}
	if _, _, err := deleteRuntimeProfileSQL(ctx, db, item.Id, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement delete = %v", err)
	}
}

func TestRegistrationTokenSQLRoundTripAndConflict(t *testing.T) {
	db := profileSQLTestDB(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 6, 1, 2, 3, 4, time.UTC)
	item := apitypes.RegistrationToken{Id: "opaque/id", CreatedAt: now, UpdatedAt: now, Token: "secret-token", RuntimeProfileId: "profile", FirmwareId: new("firmware")}
	if created, err := insertRegistrationTokenSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	if created, err := insertRegistrationTokenSQL(ctx, db, item); err != nil || created {
		t.Fatalf("duplicate = %v, %v", created, err)
	}
	got, version, err := getRegistrationTokenSQL(ctx, db, item.Id)
	if err != nil || !reflect.DeepEqual(got, item) {
		t.Fatalf("read = %#v, %v; want %#v", got, err, item)
	}
	item.UpdatedAt = now.Add(time.Second)
	updated, next, err := updateRegistrationTokenSQL(ctx, db, item, version)
	if err != nil || !reflect.DeepEqual(updated, item) || next.revision != version.revision+1 {
		t.Fatalf("update = %#v, %#v, %v", updated, next, err)
	}
	if _, _, err := updateRegistrationTokenSQL(ctx, db, item, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale update = %v", err)
	}
	if _, _, err := deleteRegistrationTokenSQL(ctx, db, item.Id, version); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale delete = %v", err)
	}
	if _, _, err := deleteRegistrationTokenSQL(ctx, db, item.Id, next); err != nil {
		t.Fatal(err)
	}
	if created, err := insertRegistrationTokenSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("recreate = %v, %v", created, err)
	}
	if _, _, err := updateRegistrationTokenSQL(ctx, db, item, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement update = %v", err)
	}
	if _, _, err := deleteRegistrationTokenSQL(ctx, db, item.Id, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replacement delete = %v", err)
	}
}

func TestProfileSQLTokenUniquenessAndJoinedResolution(t *testing.T) {
	db := profileSQLTestDB(t)
	ctx := t.Context()
	profile := apitypes.RuntimeProfile{Id: "profile", Revision: "current"}
	if created, err := insertRuntimeProfileSQL(ctx, db, profile); err != nil || !created {
		t.Fatalf("create profile = %v, %v", created, err)
	}
	token := apitypes.RegistrationToken{Id: "token-a", Token: "shared-token", RuntimeProfileId: profile.Id, FirmwareId: new("firmware")}
	if created, err := insertRegistrationTokenSQL(ctx, db, token); err != nil || !created {
		t.Fatalf("create token = %v, %v", created, err)
	}
	token.Id = "token-b"
	if created, err := insertRegistrationTokenSQL(ctx, db, token); err == nil || created {
		t.Fatal("duplicate token accepted")
	}
	id, firmware, got, err := resolveRegistrationSQL(ctx, db, token.Token)
	if err != nil || id != "token-a" || firmware == nil || *firmware != "firmware" || got.Id != profile.Id {
		t.Fatalf("registration id=%q profile=%q error=%v", id, got.Id, err)
	}
	if _, _, _, err := resolveRegistrationSQL(ctx, db, "unknown"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown token = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime_profile_owners(owner_public_key,runtime_profile_id,binding_id) VALUES ('owner',?,'initial')`, profile.Id); err != nil {
		t.Fatal(err)
	}
	got, _, err = resolveOwnerProfileSQL(ctx, db, "owner")
	if err != nil || got.Revision != "current" {
		t.Fatalf("owner profile = %#v, %v", got, err)
	}
}

func TestOwnerRollbackCannotOverwriteLaterBinding(t *testing.T) {
	db := profileSQLTestDB(t)
	ctx := t.Context()
	for _, id := range []string{"old", "first", "later"} {
		if _, err := insertRuntimeProfileSQL(ctx, db, apitypes.RuntimeProfile{Id: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := setOwnerProfileSQL(ctx, db, "owner", "old"); err != nil {
		t.Fatal(err)
	}
	previous, stamp, err := setOwnerProfileSQL(ctx, db, "owner", "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := setOwnerProfileSQL(ctx, db, "owner", "later"); err != nil {
		t.Fatal(err)
	}
	if err := restoreOwnerProfileSQL(ctx, db, "owner", stamp, previous); err == nil {
		t.Fatal("stale rollback succeeded")
	}
	got, _, err := resolveOwnerProfileSQL(ctx, db, "owner")
	if err != nil || got.Id != "later" {
		t.Fatalf("later binding = %#v, %v", got, err)
	}
}

// TestInitializeProfileSQLUpgradesLegacyGameplayColumn pins the in-place upgrade
// path: a table created by an earlier release carries a NOT NULL gameplay_json
// column that CREATE TABLE IF NOT EXISTS cannot remove, and every current insert
// omits it.
func TestInitializeProfileSQLUpgradesLegacyGameplayColumn(t *testing.T) {
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "legacy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `CREATE TABLE runtime_profiles(id TEXT PRIMARY KEY CHECK(length(id)>0),revision TEXT NOT NULL,resources_json TEXT NOT NULL,workflows_json TEXT NOT NULL,gameplay_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,row_version BIGINT NOT NULL)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	now := time.Date(2026, 9, 8, 1, 2, 3, 4, time.UTC)
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime_profiles(id,revision,resources_json,workflows_json,gameplay_json,created_at,updated_at,incarnation,row_version) VALUES ('legacy','revision','{}','{}','{"points":{}}',?,?,'incarnation',1)`,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := initializeProfileSQL(ctx, db); err != nil {
		t.Fatalf("initializeProfileSQL(legacy) error = %v", err)
	}
	if err := initializeProfileSQL(ctx, db); err != nil {
		t.Fatalf("initializeProfileSQL(repeat) error = %v", err)
	}
	var columns int
	if err := db.GetContext(ctx, &columns, `SELECT COUNT(*) FROM pragma_table_info('runtime_profiles') WHERE name = 'gameplay_json'`); err != nil {
		t.Fatalf("inspect columns: %v", err)
	}
	if columns != 0 {
		t.Fatalf("gameplay_json columns = %d, want 0", columns)
	}
	item := apitypes.RuntimeProfile{Id: "upgraded", CreatedAt: now, UpdatedAt: now, Revision: "revision", Spec: apitypes.RuntimeProfileSpec{}}
	if created, err := insertRuntimeProfileSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("insert after upgrade = %v, %v", created, err)
	}
	retained, _, err := getRuntimeProfileSQL(ctx, db, "legacy")
	if err != nil || retained.Id != "legacy" || retained.Revision != "revision" {
		t.Fatalf("retained legacy profile = %#v, %v", retained, err)
	}
}
