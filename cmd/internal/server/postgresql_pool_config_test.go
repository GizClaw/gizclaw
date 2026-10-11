package server

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestPostgreSQLPoolConfig(t *testing.T) {
	cfg, err := parseConfigData([]byte("storage:\n  pg:\n    kind: postgresql\n    dsn: postgres://local\n    max_open_conns: 8\n    conn_max_idle_time: 30s\n"))
	if err != nil {
		t.Fatal(err)
	}
	physical, err := runtimeStorageConfigs(cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := physical["pg"].(storage.PostgreSQLConfig)
	if !ok || got.MaxOpenConns != 8 || got.ConnMaxIdleTime != 30*time.Second || got.DSN != "postgres://local" {
		t.Fatalf("pool config: %+v", got)
	}
	for _, fields := range []string{"max_open_conns: -1", "conn_max_idle_time: -1s", "conn_max_idle_time: invalid", "max_open_conns: 1.5"} {
		invalid, err := parseConfigData([]byte("storage:\n  pg:\n    kind: postgresql\n    dsn: postgres://local\n    " + fields + "\n"))
		if err == nil {
			_, err = runtimeStorageConfigs(invalid.Storage)
		}
		if err == nil {
			t.Fatalf("accepted %s", fields)
		}
	}
	if _, err := parseConfigData([]byte("storage:\n  sqlite:\n    kind: sqlite\n    dsn: local.db\n    max_open_conns: 8\n")); err == nil {
		t.Fatal("PostgreSQL pool setting accepted for SQLite")
	}
}
