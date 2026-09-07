package gameplay

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/sqltest"
)

func TestCatalogLookupBoundsQueriesAndReads(t *testing.T) {
	db, observer := sqltest.New(t)
	ctx := t.Context()
	if err := initializeCatalogSQL(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 10000 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO pet_definitions(id,character_prompt,voice_prompt,visual_json,created_at,updated_at,incarnation,revision) VALUES (?,'character','voice','{}','2026-09-06T00:00:00Z','2026-09-06T00:00:00Z','initial',1)`, fmt.Sprintf("pet-%05d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2001)
	for i := range 1000 {
		id := fmt.Sprintf("pet-%05d", i)
		ids = append(ids, id, id)
	}
	ids = append(ids, "missing")
	observer.Reset()
	observer.Delay.Store(int64(20 * time.Millisecond))
	found, err := existingCatalogIDs(ctx, db, "pet_definitions", ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1001 || found["missing"] {
		t.Fatalf("membership size=%d missing=%v", len(found), found["missing"])
	}
	for i := range 1000 {
		if !found[fmt.Sprintf("pet-%05d", i)] {
			t.Fatalf("missing requested record %d", i)
		}
	}
	if statements, rows := observer.Statements.Load(), observer.Rows.Load(); statements != 4 || rows != 1000 {
		t.Fatalf("statements=%d rows=%d; want 4 and 1000", statements, rows)
	}
	observer.Reset()
	if _, err := existingCatalogIDs(ctx, db, "pet_definitions", nil); err != nil {
		t.Fatal(err)
	}
	if observer.Statements.Load() != 0 {
		t.Fatal("empty membership query accessed database")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := existingCatalogIDs(canceled, db, "pet_definitions", ids); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query = %v", err)
	}
}
