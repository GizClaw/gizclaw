package gizclaw

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestMigratorMigrateRunsACL(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open error = %v", err)
	}
	defer db.Close()

	migrator := &Migrator{ACL: &acl.Server{DB: db}}
	for range 2 {
		if err := migrator.Migrate(context.Background()); err != nil {
			t.Fatalf("Migrate() error = %v", err)
		}
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO acl_views (name, created_at, updated_at) VALUES ('default', 'now', 'now')`); err != nil {
		t.Fatalf("insert acl view after migration: %v", err)
	}
}

func TestMigratorMigrateValidation(t *testing.T) {
	if err := (*Migrator)(nil).Migrate(context.Background()); err == nil {
		t.Fatal("nil migrator Migrate() error = nil")
	}
	if err := (&Migrator{}).Migrate(context.Background()); err != nil {
		t.Fatalf("empty migrator Migrate() error = %v", err)
	}
	if err := (&Migrator{ACL: &acl.Server{}}).Migrate(context.Background()); err == nil || !strings.Contains(err.Error(), "acl: sql db not configured") {
		t.Fatalf("missing ACL DB Migrate() error = %v", err)
	}
}
