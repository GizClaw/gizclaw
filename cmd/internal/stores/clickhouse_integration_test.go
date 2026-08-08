//go:build integration

package stores

import (
	"context"
	"os"
	"strings"
	"testing"

	physicalstorage "github.com/GizClaw/gizclaw-go/cmd/internal/storage"
)

func TestClickHousePhysicalPoolSupportsScopedMetricsAndLogs(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_CLICKHOUSE_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_CLICKHOUSE_DSN is not set")
	}
	physical, err := physicalstorage.New(map[string]physicalstorage.Config{
		"analytics": {Kind: physicalstorage.KindSQL, ClickHouse: &physicalstorage.SQLConfig{DSN: dsn}},
	})
	if err != nil {
		t.Fatalf("storage.New() error = %v", err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	db, err := physical.SQL("analytics")
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{"gizclaw_metrics_shared_test", "gizclaw_log_immutable_shared_test", "gizclaw_log_mutable_shared_test"}
	for _, table := range tables {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
	}

	registry, err := NewWithStorage(physical, map[string]Config{
		"metrics": {
			Kind: KindMetrics, Storage: "analytics",
			ClickHouse: &ClickHouseConfig{Table: "gizclaw_metrics_shared_test"},
		},
		"audit": {
			Kind: KindLogImmutable, Storage: "analytics",
			ClickHouse: &ClickHouseConfig{Table: "gizclaw_log_immutable_shared_test"},
		},
		"history": {
			Kind: KindLogMutable, Storage: "analytics",
			ClickHouse: &ClickHouseConfig{Table: "gizclaw_log_mutable_shared_test"},
		},
	})
	if err != nil {
		t.Fatalf("NewWithStorage() error = %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if _, err := registry.Metrics("metrics"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Log("audit"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.MutableLog("audit"); err == nil {
		t.Fatal("immutable Log declaration exposed mutable access")
	}
	if _, err := registry.MutableLog("history"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("closing logical Stores closed shared physical pool: %v", err)
	}
	for _, table := range tables {
		if _, err := db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table); err != nil {
			t.Fatalf("drop %s: %v", table, err)
		}
	}
	if err := physical.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("closing physical registry left ClickHouse pool open")
	}
}
