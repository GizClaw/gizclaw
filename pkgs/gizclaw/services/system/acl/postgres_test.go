package acl

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestPostgresACLContract(t *testing.T) {
	db := openPostgresTestDB(t)
	ctx := context.Background()
	dropACLPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropACLPostgresTables(t, context.Background(), db) })

	server := &Server{DB: db}
	if err := server.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if err := server.Migration(ctx); err != nil {
		t.Fatalf("Migration() second run error = %v", err)
	}

	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if role, err := server.PutRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"read", "use"}); err != nil || len(role.Permissions) != 2 {
		t.Fatalf("PutRole() = %#v, %v", role, err)
	}
	roles, _, _, err := server.ListRoles(ctx, ListRolesRequest{})
	if err != nil || len(roles) != 1 || roles[0].Name != "workspace-reader" {
		t.Fatalf("ListRoles() = %#v, %v", roles, err)
	}
	description := "PostgreSQL view"
	if _, err := server.PutView(ctx, "postgres-view", apitypes.ACLViewSpec{Description: &description}); err != nil {
		t.Fatalf("PutView() error = %v", err)
	}
	views, _, _, err := server.ListViews(ctx, ListViewsRequest{})
	if err != nil || len(views) != 1 || views[0].Name != "postgres-view" {
		t.Fatalf("ListViews() = %#v, %v", views, err)
	}
	expiresAt := time.Now().UTC().Add(-time.Minute)
	for _, binding := range []struct {
		id        string
		workspace string
		expiresAt *time.Time
	}{
		{id: "binding-active", workspace: "workspace-a"},
		{id: "binding-expired", workspace: "workspace-old", expiresAt: &expiresAt},
	} {
		if _, err := server.CreatePolicyBinding(ctx, binding.id, 0, apitypes.ACLPolicy{
			Subject:   PublicKeySubject("subject-a"),
			Resource:  WorkspaceResource(binding.workspace),
			Role:      "workspace-reader",
			ExpiresAt: binding.expiresAt,
		}); err != nil {
			t.Fatalf("CreatePolicyBinding(%s) error = %v", binding.id, err)
		}
	}
	if err := server.Authorize(ctx, AuthorizeRequest{
		Subject:    PublicKeySubject("subject-a"),
		Resource:   WorkspaceResource("workspace-a"),
		Permission: "read",
	}); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	items, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, ListPolicyBindingsRequest{
		SubjectID:        "subject-a",
		ResourceIDPrefix: "workspace-",
		Permission:       "read",
		Limit:            1,
	})
	if err != nil {
		t.Fatalf("ListPolicyBindings() error = %v", err)
	}
	if len(items) != 1 || !hasNext || nextCursor == nil {
		t.Fatalf("ListPolicyBindings() = items:%#v hasNext:%v next:%v", items, hasNext, nextCursor)
	}
	if cleaned, err := server.CleanupExpired(ctx, 10); err != nil || cleaned != 1 {
		t.Fatalf("CleanupExpired() = %d, %v; want 1, nil", cleaned, err)
	}
	if _, err := server.GetPolicyBinding(ctx, "binding-expired"); !errors.Is(err, ErrPolicyBindingNotFound) {
		t.Fatalf("GetPolicyBinding(expired) error = %v, want %v", err, ErrPolicyBindingNotFound)
	}
	updated, err := server.PutPolicyBinding(ctx, "binding-active", 1, apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-b"),
		Role:     "workspace-reader",
	})
	if err != nil || updated.Policy.Resource.Id != "workspace-b" {
		t.Fatalf("PutPolicyBinding() = %#v, %v", updated, err)
	}
	if _, err := server.DeletePolicyBinding(ctx, "binding-active"); err != nil {
		t.Fatalf("DeletePolicyBinding() error = %v", err)
	}
	if _, err := server.DeleteRole(ctx, "workspace-reader"); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
	if _, err := server.DeleteView(ctx, "postgres-view"); err != nil {
		t.Fatalf("DeleteView() error = %v", err)
	}
}

func TestPostgresACLLegacyDisplayOrderMigration(t *testing.T) {
	db := openPostgresTestDB(t)
	ctx := context.Background()
	dropACLPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropACLPostgresTables(t, context.Background(), db) })

	if _, err := db.ExecContext(ctx, `CREATE TABLE acl_policy_bindings (
		id TEXT PRIMARY KEY,
		subject_kind TEXT NOT NULL,
		subject_id TEXT NOT NULL,
		resource_kind TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		role TEXT NOT NULL,
		not_before TEXT,
		expires_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy acl_policy_bindings: %v", err)
	}
	server := &Server{DB: db}
	if err := server.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'acl_policy_bindings'
			  AND column_name = 'display_order'
		)`).Scan(&exists); err != nil {
		t.Fatalf("inspect display_order: %v", err)
	}
	if !exists {
		t.Fatal("Migration() did not add acl_policy_bindings.display_order")
	}
}

func TestPostgresACLConcurrentLegacyMigration(t *testing.T) {
	db := openPostgresTestDB(t)
	ctx := context.Background()
	dropACLPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropACLPostgresTables(t, context.Background(), db) })
	server := &Server{DB: db}
	if err := server.Migration(ctx); err != nil {
		t.Fatalf("initial Migration() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE acl_policy_bindings DROP COLUMN display_order CASCADE`); err != nil {
		t.Fatalf("prepare legacy schema: %v", err)
	}

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- server.Migration(ctx)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Migration() error = %v", err)
		}
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'acl_policy_bindings'
			  AND column_name = 'display_order'
		)`).Scan(&exists); err != nil {
		t.Fatalf("inspect display_order: %v", err)
	}
	if !exists {
		t.Fatal("concurrent Migration() did not add display_order")
	}
}

func openPostgresTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sqlx.Open(postgres) error = %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("PingContext() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func dropACLPostgresTables(t *testing.T, ctx context.Context, db *sqlx.DB) {
	t.Helper()
	for _, table := range []string{"acl_binding_permissions", "acl_policy_bindings", "acl_views", "acl_roles"} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table+" CASCADE"); err != nil {
			t.Errorf("drop %s: %v", table, err)
		}
	}
}
