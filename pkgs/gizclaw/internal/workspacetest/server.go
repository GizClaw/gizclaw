// Package workspacetest provides initialized Workspace SQL fixtures.
package workspacetest

import (
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// New creates an initialized Workspace service with a test-owned SQL pool.
func New(t testing.TB) *workspace.Server {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "workspaces.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := workspace.Initialize(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return &workspace.Server{DB: db}
}

// Seed inserts a complete fixture without invoking resource dependencies.
func Seed(t testing.TB, s *workspace.Server, item apitypes.Workspace) {
	t.Helper()
	var owner any
	if item.OwnerPublicKey != nil {
		owner = *item.OwnerPublicKey
	}
	system := 0
	if item.System != nil && *item.System {
		system = 1
	}
	args := []any{item.Id, owner, item.Name, item.WorkflowId, system, item.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), item.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), item.LastActiveAt.UTC().Format("2006-01-02T15:04:05.000000000Z")}
	for _, v := range []any{item.Parameters, item.Toolkit, item.Icon, item.Labels} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, string(data))
	}
	args = append(args, rand.Text())
	tx, err := s.DB.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), tx.Rebind(`INSERT INTO workspaces(id,owner_public_key,name,workflow_id,system,created_at,updated_at,last_active_at,parameters_json,toolkit_json,icon_json,labels_json,incarnation,revision) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,1)`), args...); err != nil {
		t.Fatal(err)
	}
	if item.Labels != nil {
		for key, value := range *item.Labels {
			if _, err := tx.ExecContext(t.Context(), tx.Rebind(`INSERT INTO workspace_labels(workspace_id,label_key,label_value) VALUES (?,?,?)`), item.Id, key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
