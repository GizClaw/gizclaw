// Package voicetest supplies initialized SQL Voice catalogs for tests.
package voicetest

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// New creates an isolated catalog, closed at test cleanup.
func New(t testing.TB) *voice.Server {
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
	s := &voice.Server{DB: db}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}
