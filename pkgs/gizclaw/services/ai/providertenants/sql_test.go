package providertenants

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func tenantTestDB(t *testing.T) *sqlx.DB {
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
	if err := (&Server{DB: db}).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func checkTenant[T tenantObject](t *testing.T, db *sqlx.DB, kind string, item T) {
	t.Helper()
	ctx := t.Context()
	created, err := createSQLTenant(ctx, db, kind, item)
	if err != nil || !created {
		t.Fatalf("create %s: %v/%v", kind, created, err)
	}
	again, err := createSQLTenant(ctx, db, kind, item)
	if err != nil || again {
		t.Fatalf("duplicate %s: %v/%v", kind, again, err)
	}
	fields, _, err := tenantValues(item)
	if err != nil {
		t.Fatal(err)
	}
	got, err := getSQLTenant[T](ctx, db, kind, fields.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, item) {
		t.Fatalf("round trip changed %s", kind)
	}
	page, more, _, err := listSQLTenants[T](ctx, db, kind, "", 1)
	if err != nil || len(page) != 1 || more {
		t.Fatalf("list %s: count=%d more=%v error=%v", kind, len(page), more, err)
	}
}
func TestProviderKindsHaveIndependentIdentities(t *testing.T) {
	db := tenantTestDB(t)
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	checkTenant(t, db, "openai", apitypes.OpenAITenant{Id: "main", CredentialId: "secret", CreatedAt: now, UpdatedAt: now})
	checkTenant(t, db, "gemini", apitypes.GeminiTenant{Id: "main", CredentialId: "secret", CreatedAt: now, UpdatedAt: now, ProjectId: new("project")})
	checkTenant(t, db, "dashscope", apitypes.DashScopeTenant{Id: "main", CredentialId: "secret", CreatedAt: now, UpdatedAt: now})
	checkTenant(t, db, "deepseek", apitypes.DeepSeekTenant{Id: "main", CredentialId: "secret", CreatedAt: now, UpdatedAt: now})
	checkTenant(t, db, "minimax", apitypes.MiniMaxTenant{Id: "main", CredentialId: "secret", CreatedAt: now, UpdatedAt: now, GroupId: new("group")})
	checkTenant(t, db, "volc", apitypes.VolcTenant{Id: "main", CredentialId: "secret", CreatedAt: now, UpdatedAt: now, ResourceIds: new([]string{"resource"})})
	if _, err := deleteSQLTenant[apitypes.OpenAITenant](t.Context(), db, "openai", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := getSQLTenant[apitypes.GeminiTenant](t.Context(), db, "gemini", "main"); err != nil {
		t.Fatal("deleting one Provider affected another")
	}
	if _, err := updateSQLTenant(t.Context(), db, "openai", apitypes.OpenAITenant{Id: "main", CredentialId: "secret", UpdatedAt: now}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update resurrected missing record: %v", err)
	}
}
func TestProviderUpdatePreservesSyncMetadata(t *testing.T) {
	db := tenantTestDB(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	item := apitypes.MiniMaxTenant{Id: "main", CredentialId: "secret", CreatedAt: now, UpdatedAt: now}
	if _, err := createSQLTenant(ctx, db, "minimax", item); err != nil {
		t.Fatal(err)
	}
	_, incarnation, err := scanTenant[apitypes.MiniMaxTenant](db.QueryRowContext(ctx, `SELECT `+tenantColumns+` FROM provider_tenants WHERE provider_kind='minimax' AND id='main'`))
	if err != nil {
		t.Fatal(err)
	}
	synced := now.Add(time.Minute)
	if err := recordTenantSync(ctx, db, "minimax", "main", incarnation, synced); err != nil {
		t.Fatal(err)
	}
	item.CreatedAt = now.Add(time.Hour)
	item.UpdatedAt = now.Add(2 * time.Minute)
	item.Description = new("changed")
	updated, err := updateSQLTenant(ctx, db, "minimax", item)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.CreatedAt.Equal(now) || updated.LastSyncedAt == nil || !updated.LastSyncedAt.Equal(synced) {
		t.Fatal("configuration update overwrote independent timestamps")
	}
	if _, err := deleteSQLTenant[apitypes.MiniMaxTenant](ctx, db, "minimax", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := createSQLTenant(ctx, db, "minimax", item); err != nil {
		t.Fatal(err)
	}
	if err := recordTenantSync(ctx, db, "minimax", "main", incarnation, synced); err == nil {
		t.Fatal("old sync modified recreated tenant")
	}
}

func TestTenantDeleteRejectsReplacementAfterCleanup(t *testing.T) {
	db := tenantTestDB(t)
	ctx := t.Context()
	item := apitypes.MiniMaxTenant{Id: "tenant", CredentialId: "original"}
	if created, err := createSQLTenant(ctx, db, "minimax", item); err != nil || !created {
		t.Fatalf("create = %v, %v", created, err)
	}
	_, incarnation, err := scanTenant[apitypes.MiniMaxTenant](db.QueryRowContext(ctx, `SELECT `+tenantColumns+` FROM provider_tenants WHERE provider_kind='minimax' AND id='tenant'`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deleteSQLTenant[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id); err != nil {
		t.Fatal(err)
	}
	item.CredentialId = "replacement"
	if created, err := createSQLTenant(ctx, db, "minimax", item); err != nil || !created {
		t.Fatalf("recreate = %v, %v", created, err)
	}
	if _, err := deleteSQLTenantIncarnation[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id, incarnation); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale delete = %v", err)
	}
	got, err := getSQLTenant[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id)
	if err != nil || got.CredentialId != "replacement" {
		t.Fatalf("replacement = %#v, %v", got, err)
	}
}
