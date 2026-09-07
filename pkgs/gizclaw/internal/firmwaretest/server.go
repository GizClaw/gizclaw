// Package firmwaretest supplies initialized SQL Firmware catalogs for tests.
package firmwaretest

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// New creates an isolated catalog and closes its database after the test.
func New(t testing.TB) *firmware.Server {
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
	server := &firmware.Server{DB: db}
	if err := server.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return server
}
