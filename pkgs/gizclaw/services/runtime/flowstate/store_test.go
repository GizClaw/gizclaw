package flowstate

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
		if value, err := s.LoadState(ctx, "context"); err != nil || value != nil {
			t.Fatalf("initial state = %s, %v", value, err)
		}
		if err := s.SaveState(ctx, "context", []byte(`{"kept":"yes"}`)); err != nil {
			t.Fatal(err)
		}
		stores = append(stores, s)
	}
	if err := RetireWorkspace(ctx, db, "owner", "workspace"); err != nil {
		t.Fatal(err)
	}
	for i, s := range stores {
		value, err := s.LoadState(ctx, "context")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 || i == 3 {
			if value != nil {
				t.Fatalf("retired state = %s", value)
			}
			if err := s.SaveState(ctx, "context", []byte(`{}`)); !errors.Is(err, ErrRetired) {
				t.Fatalf("stale write = %v", err)
			}
		} else if string(value) != `{"kept":"yes"}` {
			t.Fatalf("foreign state = %s", value)
		}
	}
	if _, err := OpenScope(ctx, db, "owner", "workspace", "new-agent"); !errors.Is(err, ErrRetired) {
		t.Fatalf("reopen = %v", err)
	}
	if err := RetireWorkspace(ctx, db, "owner", "workspace"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM flowcraft_board_states WHERE owner_id='owner' AND workspace_id='workspace'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("retired rows = %d, %v", count, err)
	}
}

func TestConcurrentSaveCannotResurrectRetiredWorkspace(t *testing.T) {
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
			err := s.SaveState(ctx, "context", []byte(`{"value":1}`))
			if err != nil && !errors.Is(err, ErrRetired) {
				t.Error(err)
			}
		})
	}
	close(start)
	if err := RetireWorkspace(ctx, db, "owner", "workspace"); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if value, err := s.LoadState(ctx, "context"); err != nil || value != nil {
		t.Fatalf("after retirement = %s, %v", value, err)
	}
	if err := s.SaveState(ctx, "context", []byte(`{}`)); !errors.Is(err, ErrRetired) {
		t.Fatalf("late write = %v", err)
	}
}
