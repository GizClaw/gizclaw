package memory

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func newHistoryTestStore(t *testing.T) *SQLHistoryStore {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLHistoryStore(t.Context(), db, "history", 0)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSQLHistoryIsolationCursorAndAssets(t *testing.T) {
	store := newHistoryTestStore(t)
	base := time.Now().UTC().Truncate(time.Second)
	entries := []HistoryEntry{
		{ConversationID: "a", ID: "one", Role: RoleUser, Text: "hello", CreatedAt: base},
		{ConversationID: "a", ID: "two", Role: RoleAssistant, Text: "answer", CreatedAt: base.Add(time.Second), Interrupted: true, Assets: []HistoryAsset{{Name: "audio/one.ogg", MIMEType: "audio/ogg", Bytes: 42}}},
		{ConversationID: "a", ID: "three", Role: RoleUser, Text: "current", TurnID: "current-turn", CreatedAt: base.Add(2 * time.Second)},
		{ConversationID: "b", ID: "one", Role: RoleUser, Text: "private", CreatedAt: base},
	}
	if err := store.Append(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	page, err := store.Query(t.Context(), HistoryQuery{ConversationID: "a", Limit: 1, Descending: true, ExcludeTurnID: "current-turn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].ID != "two" || page.Next == nil || len(page.Entries[0].Assets) != 1 || !page.Entries[0].Interrupted {
		t.Fatalf("page=%+v", page)
	}
	if err := store.Delete(t.Context(), entries[1].Key()); err != nil {
		t.Fatal(err)
	}
	next, err := store.Query(t.Context(), HistoryQuery{ConversationID: "a", Limit: 2, Descending: true, ExcludeTurnID: "current-turn", Cursor: page.Next})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Entries) != 1 || next.Entries[0].ID != "one" {
		t.Fatalf("next=%+v", next)
	}
	private, err := store.Get(t.Context(), HistoryKey{ConversationID: "b", ID: "one"})
	if err != nil || private.Text != "private" {
		t.Fatalf("private=%+v err=%v", private, err)
	}
	if _, err := store.Get(t.Context(), entries[1].Key()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted Get=%v", err)
	}
}

func TestSQLHistoryBatchConflictRollsBack(t *testing.T) {
	store := newHistoryTestStore(t)
	entry := HistoryEntry{ConversationID: "a", ID: "one", Role: RoleUser, Text: "original", CreatedAt: time.Now().UTC()}
	if err := store.Append(t.Context(), []HistoryEntry{entry}); err != nil {
		t.Fatal(err)
	}
	other := entry
	other.ID = "other"
	if err := store.Append(t.Context(), []HistoryEntry{other, entry}); !errors.Is(err, ErrConflict) {
		t.Fatalf("Append=%v", err)
	}
	if _, err := store.Get(t.Context(), other.Key()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partial batch committed: %v", err)
	}
	got, err := store.Get(t.Context(), entry.Key())
	if err != nil || !reflect.DeepEqual(got, entry) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestSQLHistoryTransactionRollback(t *testing.T) {
	store := newHistoryTestStore(t)
	tx, err := store.db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	bound, err := store.WithTx(tx)
	if err != nil {
		t.Fatal(err)
	}
	entry := HistoryEntry{ConversationID: "a", ID: "one", Role: RoleUser, Text: "uncommitted", CreatedAt: time.Now().UTC()}
	if err := bound.Append(t.Context(), []HistoryEntry{entry}); err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Get(t.Context(), entry.Key()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), entry.Key()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rolled back Get=%v", err)
	}
}
