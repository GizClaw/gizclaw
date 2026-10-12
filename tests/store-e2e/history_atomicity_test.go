//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

func historyStoreForTest(t *testing.T, db *sqlx.DB, table string) *memory.SQLHistoryStore {
	t.Helper()
	store, err := memory.NewSQLHistoryStore(t.Context(), db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := storage.PrepareSQLTable(db, "sql", table)
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := storage.SQLIndexName(parent, "history_keys")
	assets, _ := storage.SQLIndexName(parent, "history_assets")
	t.Cleanup(func() {
		var schema string
		_ = db.Get(&schema, "SELECT current_schema()")
		_, _ = db.Exec("DELETE FROM partman.part_config WHERE parent_table=$1", schema+"."+table)
		for _, name := range []string{table, keys, assets} {
			_, _ = db.Exec("DROP TABLE IF EXISTS \"" + name + "\" CASCADE")
		}
	})
	return store
}

func TestPostgreSQLHistoryConflictingBatchesRemainAtomic(t *testing.T) {
	db := openPostgreSQL(t)
	store := historyStoreForTest(t, db, uniqueTable("history_atomic"))
	const writers = 16
	start := make(chan struct{})
	errs := make([]error, writers)
	var workers sync.WaitGroup
	for writer := range writers {
		workers.Go(func() {
			<-start
			at := time.Now().UTC()
			entries := []memory.HistoryEntry{{ConversationID: "shared", ID: "one", Role: memory.RoleUser, Text: "one", CreatedAt: at}, {ConversationID: "shared", ID: "two", Role: memory.RoleAssistant, Text: "two", CreatedAt: at}, {ConversationID: "shared", ID: fmt.Sprintf("unique-%d", writer), Role: memory.RoleAssistant, Text: "unique", CreatedAt: at}}
			if writer%2 == 0 {
				entries[0], entries[1] = entries[1], entries[0]
			}
			errs[writer] = store.Append(t.Context(), entries)
		})
	}
	close(start)
	workers.Wait()
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		} else if !errors.Is(err, memory.ErrConflict) {
			t.Fatalf("conflict returned %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
	page, err := store.Query(t.Context(), memory.HistoryQuery{ConversationID: "shared", Limit: 100})
	if err != nil || len(page.Entries) != 3 {
		t.Fatalf("partial batches=%+v err=%v", page, err)
	}
}

func TestPostgreSQLHistoryIndependentRecordProgress(t *testing.T) {
	db := openPostgreSQL(t)
	table := uniqueTable("history_progress")
	first := historyStoreForTest(t, db, table)
	second, err := memory.NewSQLHistoryStore(t.Context(), db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := storage.PrepareSQLTable(db, "sql", table)
	keys, _ := storage.SQLIndexName(parent, "history_keys")
	tx, err := db.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var blocker int
	if err := tx.Get(&blocker, "SELECT pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO \""+keys+"\" VALUES('held','same',$1)", time.Now().Add(30*24*time.Hour).UnixNano()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- first.Append(ctx, []memory.HistoryEntry{{ConversationID: "held", ID: "same", Role: memory.RoleUser, Text: "held", CreatedAt: time.Now().UTC()}})
	}()
	waitForPostgresBlocker(t, ctx, db, blocker)
	freeCtx, freeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer freeCancel()
	if err := second.Append(freeCtx, []memory.HistoryEntry{{ConversationID: "held", ID: "other", Role: memory.RoleUser, Text: "free", CreatedAt: time.Now().UTC()}, {ConversationID: "other", ID: "same", Role: memory.RoleUser, Text: "free", CreatedAt: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatalf("same key did not remain blocked: %v", err)
	default:
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
