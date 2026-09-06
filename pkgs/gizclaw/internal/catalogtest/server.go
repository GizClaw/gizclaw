// Package catalogtest supplies initialized SQL Gameplay catalogs for tests.
package catalogtest

import (
	"encoding/json"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// New creates an isolated catalog, closed at test cleanup.
func New(t testing.TB) *gameplay.Catalog {
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
	s := &gameplay.Catalog{DB: db}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}

// SeedGame creates a game definition with owner-managed icon metadata for tests.
func SeedGame(t testing.TB, catalog *gameplay.Catalog, item apitypes.GameDef) {
	t.Helper()
	response, err := catalog.CreateGameDef(t.Context(), adminhttp.CreateGameDefRequestObject{Body: &adminhttp.GameDefUpsert{Id: item.Id, Spec: item.Spec}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateGameDef200JSONResponse); !ok {
		t.Fatalf("CreateGameDef = %#v", response)
	}
	icon, err := json.Marshal(item.Icon)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.DB.ExecContext(t.Context(), catalog.DB.Rebind("UPDATE game_definitions SET icon_json=? WHERE id=?"), string(icon), item.Id); err != nil {
		t.Fatal(err)
	}
}
