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

func TestRuntimeProfileSQLAppConfigPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.sqlite")
	open := func() *sqlx.DB {
		db, err := sqlx.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		return db
	}
	db := open()
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	if err := initializeProfileSQL(ctx, db); err != nil {
		t.Fatal(err)
	}
	config := apitypes.RuntimeProfileAppConfig{"ui.theme": "  {\n\"theme\":\"深色\"\n}  ", "empty": "", "text": "\x00\ttext\n"}
	item, err := normalizeProfile(appConfigUpsert(config), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := setProfileRevision(&item); err != nil {
		t.Fatal(err)
	}
	if created, err := insertRuntimeProfileSQL(ctx, db, item); err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	token := apitypes.RegistrationToken{Id: "token", Token: "test-token", RuntimeProfileId: item.Id}
	if _, err := insertRegistrationTokenSQL(ctx, db, token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := setOwnerProfileSQL(ctx, db, "owner", item.Id); err != nil {
		t.Fatal(err)
	}
	assertProfile := func(got apitypes.RuntimeProfile, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, item) {
			t.Fatalf("profile = %#v, %v; want %#v", got, err, item)
		}
		persistedRevision := got.Revision
		if err := setProfileRevision(&got); err != nil || got.Revision != persistedRevision {
			t.Fatalf("persisted revision does not match spec: %q -> %q, %v", persistedRevision, got.Revision, err)
		}
	}
	checkReads := func() profileRowVersion {
		t.Helper()
		got, version, err := getRuntimeProfileSQL(ctx, db, item.Id)
		assertProfile(got, err)
		items, hasNext, _, err := listRuntimeProfileSQL(ctx, db, "", 10)
		if err != nil || hasNext || len(items) != 1 {
			t.Fatalf("list = %#v, %v, %v", items, hasNext, err)
		}
		assertProfile(items[0], nil)
		_, _, got, err = resolveRegistrationSQL(ctx, db, token.Token)
		assertProfile(got, err)
		got, _, err = resolveOwnerProfileSQL(ctx, db, "owner")
		assertProfile(got, err)
		return version
	}
	version := checkReads()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = open()
	if err := initializeProfileSQL(ctx, db); err != nil {
		t.Fatal(err)
	}
	if got := checkReads(); got != version {
		t.Fatalf("reopen changed row version: %#v -> %#v", version, got)
	}
	for _, config := range []*apitypes.RuntimeProfileAppConfig{
		new(apitypes.RuntimeProfileAppConfig{"replacement": "新配置"}),
		new(apitypes.RuntimeProfileAppConfig{}),
		new(apitypes.RuntimeProfileAppConfig{"restored": "value"}),
		nil,
	} {
		item.Spec.AppConfig = config
		if err := setProfileRevision(&item); err != nil {
			t.Fatal(err)
		}
		updated, next, err := updateRuntimeProfileSQL(ctx, db, item, version)
		assertProfile(updated, err)
		version = next
		checkReads()
	}
	deleted, _, err := deleteRuntimeProfileSQL(ctx, db, item.Id, version)
	assertProfile(deleted, err)
}

func TestRuntimeProfileSQLAddsAppConfigToExistingTable(t *testing.T) {
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "existing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := t.Context()
	_, err = db.ExecContext(ctx, `CREATE TABLE runtime_profiles(id TEXT PRIMARY KEY CHECK(length(id)>0),revision TEXT NOT NULL,resources_json TEXT NOT NULL,workflows_json TEXT NOT NULL,gameplay_json TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,row_version BIGINT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO runtime_profiles VALUES ('existing','revision','{}','{}','null','2026-09-08T00:00:00Z','2026-09-08T00:00:00Z','original',7)`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := initializeProfileSQL(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	got, version, err := getRuntimeProfileSQL(ctx, db, "existing")
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.AppConfig != nil || got.Revision != "revision" || version.incarnation != "original" || version.revision != 7 || !got.CreatedAt.Equal(time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("existing profile changed: %#v, %#v", got, version)
	}
	got.Spec.AppConfig = new(apitypes.RuntimeProfileAppConfig{"ui.theme": "dark"})
	if _, _, err := updateRuntimeProfileSQL(ctx, db, got, version); err != nil {
		t.Fatal(err)
	}
	persisted, _, err := getRuntimeProfileSQL(ctx, db, got.Id)
	if err != nil || !reflect.DeepEqual(persisted, got) {
		t.Fatalf("updated existing profile = %#v, %v; want %#v", persisted, err, got)
	}
}
