// Package modeltest supplies initialized SQL Model catalogs for tests.
package modeltest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// New creates an isolated catalog, closed at test cleanup.
func New(t testing.TB) *model.Server {
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
	s := &model.Server{DB: db}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}

// Seed installs a Model fixture, including intentionally incomplete runtime references.
func Seed(t testing.TB, s *model.Server, item apitypes.Model) {
	t.Helper()
	data, err := json.Marshal(item.ProviderData)
	if err != nil {
		t.Fatal(err)
	}
	var synced *string
	if item.SyncedAt != nil {
		synced = new(item.SyncedAt.Format(time.RFC3339Nano))
	}
	_, err = s.DB.ExecContext(t.Context(), s.DB.Rebind(`INSERT INTO models(id,kind,source,provider_kind,provider_id,provider_data_json,display_name,description,created_at,updated_at,synced_at) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET kind=excluded.kind,source=excluded.source,provider_kind=excluded.provider_kind,provider_id=excluded.provider_id,provider_data_json=excluded.provider_data_json,display_name=excluded.display_name,description=excluded.description,created_at=excluded.created_at,updated_at=excluded.updated_at,synced_at=excluded.synced_at`), item.Id, string(item.Kind), string(item.Source), string(item.Provider.Kind), item.Provider.Id, string(data), item.DisplayName, item.Description, item.CreatedAt.Format(time.RFC3339Nano), item.UpdatedAt.Format(time.RFC3339Nano), synced)
	if err != nil {
		t.Fatal(err)
	}
}
