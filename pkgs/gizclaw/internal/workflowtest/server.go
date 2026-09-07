// Package workflowtest supplies initialized SQL Workflow catalogs for tests.
package workflowtest

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// New creates an isolated catalog, closed at test cleanup.
func New(t testing.TB) *workflow.Server {
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
	s := &workflow.Server{DB: db}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}
