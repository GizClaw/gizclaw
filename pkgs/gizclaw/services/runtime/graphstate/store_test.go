package graphstate

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	if dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"); dsn != "" {
		return postgresTestDB(t, dsn)
	}
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := Initialize(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestScopesIsolateStateAndRetirement(t *testing.T) {
	db := testDB(t)
	ctx := t.Context()
	scopes := [][3]string{{"owner", "workspace", "agent"}, {"other", "workspace", "agent"}, {"owner", "other", "agent"}, {"owner", "workspace", "other"}}
	stores := make([]*Store, 0, len(scopes))
	for _, scope := range scopes {
		s, err := OpenScope(ctx, db, scope[0], scope[1], scope[2])
		if err != nil {
			t.Fatal(err)
		}
		if value, err := s.Load(ctx, "context"); err != nil || len(value.Fields) != 0 || value.Version != "" {
			t.Fatalf("initial state = %#v, %v", value, err)
		}
		if _, err := s.CompareAndSwap(ctx, "context", "", map[string]any{"kept": "yes"}); err != nil {
			t.Fatal(err)
		}
		stores = append(stores, s)
	}
	if err := RetireWorkspace(ctx, db, "owner", "workspace"); err != nil {
		t.Fatal(err)
	}
	for i, s := range stores {
		value, err := s.Load(ctx, "context")
		if i == 0 || i == 3 {
			if !errors.Is(err, ErrRetired) {
				t.Fatalf("retired read = %#v, %v", value, err)
			}
			if _, err := s.CompareAndSwap(ctx, "context", "", map[string]any{}); !errors.Is(err, ErrRetired) {
				t.Fatalf("stale write = %v", err)
			}
		} else if err != nil || value.Fields["kept"] != "yes" || value.Version == "" {
			t.Fatalf("foreign state = %#v, %v", value, err)
		}
	}
	if _, err := OpenScope(ctx, db, "owner", "workspace", "new-agent"); !errors.Is(err, ErrRetired) {
		t.Fatalf("reopen = %v", err)
	}
	if err := RetireWorkspace(ctx, db, "owner", "workspace"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM graph_states WHERE owner_id='owner' AND workspace_id='workspace'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("retired rows = %d, %v", count, err)
	}
}

func TestConcurrentCASCannotResurrectRetiredWorkspace(t *testing.T) {
	db := testDB(t)
	ctx := t.Context()
	s, err := OpenScope(ctx, db, "owner", "workspace", "agent")
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	start := make(chan struct{})
	for range 20 {
		workers.Go(func() {
			<-start
			_, err := s.CompareAndSwap(ctx, "context", "", map[string]any{"value": int64(1)})
			if err != nil && !errors.Is(err, ErrRetired) && !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
		})
	}
	close(start)
	if err := RetireWorkspace(ctx, db, "owner", "workspace"); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if value, err := s.Load(ctx, "context"); !errors.Is(err, ErrRetired) {
		t.Fatalf("after retirement = %#v, %v", value, err)
	}
	if _, err := s.CompareAndSwap(ctx, "context", "", map[string]any{}); !errors.Is(err, ErrRetired) {
		t.Fatalf("late write = %v", err)
	}
}
