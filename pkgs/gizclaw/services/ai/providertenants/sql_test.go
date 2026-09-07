package providertenants

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	voicecatalog "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
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
	if err := recordTenantSync(ctx, db, "minimax", "main", incarnation, synced, nil); err != nil {
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
	if err := recordTenantSync(ctx, db, "minimax", "main", incarnation, synced, nil); err == nil {
		t.Fatal("old sync modified recreated tenant")
	}
}

func TestTenantDeleteRejectsReplacementBeforeCleanup(t *testing.T) {
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
	if _, err := deleteSQLTenantIncarnation[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id, incarnation, func(*sqlx.Tx) error { t.Fatal("stale delete reached dependent cleanup"); return nil }); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale delete = %v", err)
	}
	got, err := getSQLTenant[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id)
	if err != nil || got.CredentialId != "replacement" {
		t.Fatalf("replacement = %#v, %v", got, err)
	}
}

func TestTenantRetirementSharesSingleConnectionAndRollsBackVoices(t *testing.T) {
	db := tenantTestDB(t)
	voices := &voicecatalog.Server{DB: db}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := voices.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	item := apitypes.MiniMaxTenant{Id: "tenant", CredentialId: "credential"}
	if _, err := createSQLTenant(ctx, db, "minimax", item); err != nil {
		t.Fatal(err)
	}
	_, incarnation, err := scanTenant[apitypes.MiniMaxTenant](db.QueryRowContext(ctx, `SELECT `+tenantColumns+` FROM provider_tenants WHERE provider_kind='minimax' AND id='tenant'`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO voices(id,source,provider_kind,provider_id,provider_data_json,created_at,updated_at) VALUES ('synced','sync','minimax-tenant','tenant','{}','2026-09-07T00:00:00Z','2026-09-07T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	stopped := errors.New("abort after voice cleanup")
	_, err = deleteSQLTenantIncarnation[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id, incarnation, func(tx *sqlx.Tx) error {
		if err := voices.DeleteProviderVoicesInTransaction(ctx, db, tx, miniMaxProviderKind, item.Id); err != nil {
			return err
		}
		return stopped
	})
	if !errors.Is(err, stopped) {
		t.Fatalf("retirement=%v", err)
	}
	if _, err := getSQLTenant[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id); err != nil {
		t.Fatalf("rollback lost tenant: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM voices WHERE id='synced'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback voices=%d err=%v", count, err)
	}
	server := &Server{DB: db, Voices: voices}
	response, err := server.DeleteMiniMaxTenant(ctx, adminhttp.DeleteMiniMaxTenantRequestObject{Id: item.Id})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.DeleteMiniMaxTenant200JSONResponse); !ok {
		t.Fatalf("handler=%#v", response)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM voices WHERE id='synced'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("retired voices=%d err=%v", count, err)
	}
	if _, err := getSQLTenant[apitypes.MiniMaxTenant](ctx, db, "minimax", item.Id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("retired tenant=%v", err)
	}
}

type blockedRetirementVoices struct {
	voicecatalog.ProviderVoiceService
	entered chan struct{}
	release chan struct{}
}

func (v *blockedRetirementVoices) DeleteProviderVoicesInTransaction(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, kind apitypes.VoiceProviderKind, id string) error {
	close(v.entered)
	select {
	case <-v.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return v.ProviderVoiceService.DeleteProviderVoicesInTransaction(ctx, db, tx, kind, id)
}

func TestTenantDeleteHandlerFencesConcurrentReplacement(t *testing.T) {
	db := tenantTestDB(t)
	voices := &voicecatalog.Server{DB: db}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := voices.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	item := apitypes.MiniMaxTenant{Id: "tenant", CredentialId: "original"}
	if _, err := createSQLTenant(ctx, db, "minimax", item); err != nil {
		t.Fatal(err)
	}
	blocked := &blockedRetirementVoices{ProviderVoiceService: voices, entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(blocked.release) })
	old := &Server{DB: db, Voices: blocked}
	first := make(chan error, 1)
	go func() {
		response, err := old.DeleteMiniMaxTenant(ctx, adminhttp.DeleteMiniMaxTenantRequestObject{Id: item.Id})
		if err == nil {
			if _, ok := response.(adminhttp.DeleteMiniMaxTenant200JSONResponse); !ok {
				err = fmt.Errorf("old delete=%#v", response)
			}
		}
		first <- err
	}()
	select {
	case <-blocked.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	replacement := make(chan error, 1)
	go func() {
		other := &Server{DB: db, Voices: voices}
		if _, err := other.DeleteMiniMaxTenant(ctx, adminhttp.DeleteMiniMaxTenantRequestObject{Id: item.Id}); err != nil {
			replacement <- err
			return
		}
		item.CredentialId = "replacement"
		created, err := createSQLTenant(ctx, db, "minimax", item)
		if err != nil || !created {
			replacement <- fmt.Errorf("create replacement=%v/%v", created, err)
			return
		}
		_, err = db.ExecContext(ctx, `INSERT INTO voices(id,source,provider_kind,provider_id,provider_data_json,created_at,updated_at) VALUES ('replacement','sync','minimax-tenant','tenant','{}','2026-09-07T00:00:00Z','2026-09-07T00:00:00Z')`)
		replacement <- err
	}()
	select {
	case err := <-replacement:
		t.Fatalf("replacement overtook dependent cleanup: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release.Do(func() { close(blocked.release) })
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-replacement; err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM voices WHERE id='replacement'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("replacement voices=%d/%v", count, err)
	}
}

func TestTenantSyncRollsBackSharedVoiceReconciliation(t *testing.T) {
	db := tenantTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	voices := &voicecatalog.Server{DB: db}
	if err := voices.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	item := apitypes.MiniMaxTenant{Id: "tenant", CredentialId: "credential"}
	if _, err := createSQLTenant(ctx, db, "minimax", item); err != nil {
		t.Fatal(err)
	}
	_, incarnation, err := scanTenant[apitypes.MiniMaxTenant](db.QueryRowContext(ctx, `SELECT `+tenantColumns+` FROM provider_tenants WHERE provider_kind='minimax' AND id='tenant'`))
	if err != nil {
		t.Fatal(err)
	}
	desired := []apitypes.Voice{{Id: "voice", Source: apitypes.VoiceSourceSync, Provider: apitypes.VoiceProvider{Kind: miniMaxProviderKind, Id: "tenant"}, ProviderData: voicecatalog.ProviderData(miniMaxProviderKind, map[string]any{"voice_id": "voice"}), CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	aborted := errors.New("abort sync")
	err = recordTenantSync(ctx, db, "minimax", "tenant", incarnation, time.Now(), func(tx *sqlx.Tx) error {
		if _, _, _, err := voices.ReconcileProviderVoicesInTransaction(ctx, db, tx, miniMaxProviderKind, "tenant", desired); err != nil {
			return err
		}
		return aborted
	})
	if !errors.Is(err, aborted) {
		t.Fatalf("sync=%v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM voices`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback voices=%d %v", count, err)
	}
	tenant, err := getSQLTenant[apitypes.MiniMaxTenant](ctx, db, "minimax", "tenant")
	if err != nil || tenant.LastSyncedAt != nil {
		t.Fatalf("rollback metadata=%+v %v", tenant, err)
	}
}
