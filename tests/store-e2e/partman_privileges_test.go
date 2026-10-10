//go:build store_e2e

package store_e2e_test

import (
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestPostgreSQLBackgroundPartitionsRetainApplicationOwnership(t *testing.T) {
	admin := openPostgreSQL(t)
	role := uniqueTable("partman_role")
	quotedRole := pq.QuoteIdentifier(role)
	if _, err := admin.Exec("CREATE ROLE " + quotedRole + " LOGIN PASSWORD 'fixture'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP OWNED BY " + quotedRole + " CASCADE"); err != nil {
			t.Error(err)
		}
		if _, err := admin.Exec("DROP ROLE " + quotedRole); err != nil {
			t.Error(err)
		}
	})
	var database string
	if err := admin.GetContext(t.Context(), &database, "SELECT current_database()"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"GRANT CONNECT ON DATABASE " + pq.QuoteIdentifier(database) + " TO " + quotedRole,
		"GRANT USAGE, CREATE ON SCHEMA public, partman TO " + quotedRole,
		"GRANT ALL ON ALL TABLES IN SCHEMA partman TO " + quotedRole,
		"GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA partman TO " + quotedRole,
	} {
		if _, err := admin.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	dsn := requiredEnvironment(t, "GIZCLAW_TEST_POSTGRES_DSN")
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		var err error
		dsn, err = pq.ParseURL(dsn)
		if err != nil {
			t.Fatal(err)
		}
	}
	application, err := sqlx.Open("postgres", dsn+" user="+role+" password=fixture")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Close() })
	var superuser bool
	if err := application.GetContext(t.Context(), &superuser, "SELECT rolsuper FROM pg_roles WHERE rolname=current_user"); err != nil || superuser {
		t.Fatalf("expected a real non-superuser application connection: superuser=%v error=%v", superuser, err)
	}
	table := uniqueTable("partman_owner")
	cleanupPostgreSQLTables(t, admin, table, table+"_keys")
	store, err := logstore.NewSQLStoreWithDBAndTTL(application, table, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Append(t.Context(), []logstore.Record{{Stream: "stream", ID: "record", Kind: "message", Time: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	parent := "public." + table
	// Match the administrative background worker's partition creation identity.
	if _, err := admin.ExecContext(t.Context(), "SELECT partman.create_partition_time($1,ARRAY[$2]::timestamptz[])", parent, time.Now().UTC().AddDate(0, 0, 100)); err != nil {
		t.Fatal(err)
	}
	var mismatched int
	if err := admin.GetContext(t.Context(), &mismatched, `
		SELECT count(*) FROM pg_inherits AS inheritance
		JOIN pg_class AS child ON child.oid=inheritance.inhrelid
		JOIN pg_class AS parent ON parent.oid=inheritance.inhparent
		WHERE parent.oid=to_regclass($1) AND child.relowner<>parent.relowner`, parent); err != nil || mismatched != 0 {
		t.Fatalf("background worker changed child ownership: mismatched=%d error=%v", mismatched, err)
	}
	tx, err := application.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), "SELECT pg_advisory_xact_lock(hashtext('pg_partman run_maintenance'))"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), "SELECT partman.run_maintenance($1,p_analyze:=false,p_jobmon:=false)", parent); err != nil {
		t.Fatal(err)
	}
	var maintained bool
	if err := tx.GetContext(t.Context(), &maintained, "SELECT maintenance_last_run IS NOT NULL FROM partman.part_config WHERE parent_table=$1", parent); err != nil || !maintained {
		t.Fatalf("application could not maintain background-created children: maintained=%v error=%v", maintained, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
