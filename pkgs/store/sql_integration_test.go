package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	physicalstorage "github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestPostgreSQLPhysicalPoolSupportsSQLLogicalStores(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	suffix := time.Now().UnixNano()
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"database": physicalstorage.PostgreSQLConfig{DSN: dsn},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	db, err := physical.SQL("database")
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]string{
		"kv":      fmt.Sprintf("gzc_root_kv_%d", suffix),
		"metrics": fmt.Sprintf("gzc_root_metrics_%d", suffix),
		"logs":    fmt.Sprintf("gzc_root_logs_%d", suffix),
		"history": fmt.Sprintf("gzc_root_history_%d", suffix),
	}
	t.Cleanup(func() {
		for _, table := range tables {
			_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `"`)
		}
	})
	registry, err := New(map[string]Config{
		"kv":      {Kind: KindKeyValue, Storage: "database", Table: tables["kv"]},
		"metrics": {Kind: KindMetrics, Storage: "database", Table: tables["metrics"]},
		"logs":    {Kind: KindLogImmutable, Storage: "database", Table: tables["logs"]},
		"history": {Kind: KindLogMutable, Storage: "database", Table: tables["history"]},
		"raw":     {Kind: KindSQL, Storage: "database"},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	store, err := registry.KV("kv")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), kv.Key{"key"}, []byte("value")); err != nil {
		t.Fatal(err)
	}
	raw, err := registry.SQL("raw")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if err := raw.Ping(); err != nil {
		t.Fatalf("logical Close closed physical PostgreSQL pool: %v", err)
	}
}
