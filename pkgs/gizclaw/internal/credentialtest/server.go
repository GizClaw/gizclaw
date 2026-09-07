// Package credentialtest supplies initialized SQL Credential catalogs for tests.
package credentialtest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// New creates an isolated catalog, closed at test cleanup.
func New(t testing.TB) *credential.Server {
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
	s := &credential.Server{DB: db}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}

// Seed installs a Credential fixture, including deliberately incomplete credentials.
func Seed(t testing.TB, s *credential.Server, item apitypes.Credential) {
	t.Helper()
	body, err := json.Marshal(item.Body)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.ExecContext(t.Context(), s.DB.Rebind(`INSERT INTO credentials(id,provider,body_json,description,created_at,updated_at,revision,incarnation) VALUES (?,?,?,?,?,?,1,?) ON CONFLICT(id) DO UPDATE SET provider=excluded.provider,body_json=excluded.body_json,description=excluded.description,created_at=excluded.created_at,updated_at=excluded.updated_at,revision=credentials.revision+1`), item.Id, item.Provider, string(body), item.Description, item.CreatedAt.Format(time.RFC3339Nano), item.UpdatedAt.Format(time.RFC3339Nano), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
}
