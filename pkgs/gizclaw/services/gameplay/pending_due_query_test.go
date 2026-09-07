package gameplay

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/sqltest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
)

func TestPendingDeletionDueQueryBoundsReadsAndUsesTimeIndexes(t *testing.T) {
	db, observer := sqltest.New(t)
	ctx := t.Context()
	if err := (&Runtime{DB: db}).Migration(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	past, future := formatPendingDeletionTime(now.Add(-time.Hour)), formatPendingDeletionTime(now.Add(time.Hour))
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	want := []string{}
	for i := range 3000 {
		id := fmt.Sprintf("task-%05d", i)
		status, kind, next, lease := "queued", "pet", future, future
		// Due records alternate between retry deadlines and expired running leases.
		if i%100 == 0 {
			if i%200 == 0 {
				status = "running"
				lease = past
			} else {
				status = "retry_wait"
				next = past
			}
			want = append(want, id)
		}
		if i%100 == 1 {
			status = "failed"
			next = past
			lease = past
		}
		if i%100 == 2 {
			kind = "workspace"
			next = past
			lease = past
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO gameplay_pending_deletions
   (deletion_id,kind,owner_public_key,resource_id,reason,deleted_at,descriptor_version,descriptor_json,marker_fingerprint,task_status,next_attempt_at,lease_deadline)
   VALUES (?,?,'owner',?,'resource_delete',?,1,?,'fingerprint',?,?,?)`, id, kind, id, past, strings.Repeat("x", 2048), status, next, lease); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	observer.Reset()
	observer.Delay.Store(int64(10 * time.Millisecond))
	source := PendingDeletionSource{DB: db}
	var got []string
	cursor := ""
	for {
		before := observer.Rows.Load()
		refs, next, err := source.ScanDue(ctx, now, 7, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if read := observer.Rows.Load() - before; read != int64(len(refs)) || read > 7 {
			t.Fatalf("page read %d rows for %d refs", read, len(refs))
		}
		for _, ref := range refs {
			if ref.Source != source.Name() || ref.MarkerFingerprint != "fingerprint" {
				t.Fatalf("reference=%+v", ref)
			}
			got = append(got, ref.DeletionID)
		}
		if next == "" {
			break
		}
		if next <= cursor {
			t.Fatalf("cursor failed to advance: %q", next)
		}
		cursor = next
	}
	if !slices.Equal(got, want) {
		t.Fatalf("due IDs=%v, want %v", got, want)
	}
	if observer.Statements.Load() != 5 || observer.Rows.Load() != 30 {
		t.Fatalf("queries=%d rows=%d", observer.Statements.Load(), observer.Rows.Load())
	}
	observer.Delay.Store(0)
	rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+pendingDeletionDueSQL, pendingdeletion.KindPet, pendingdeletion.StatusQueued, pendingdeletion.StatusRetryWait, formatPendingDeletionTime(now), "", pendingdeletion.KindPet, pendingdeletion.StatusRunning, formatPendingDeletionTime(now), "", 7)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"gameplay_pending_deletions_due_idx", "gameplay_pending_deletions_lease_idx"} {
		if !strings.Contains(plan.String(), "USING INDEX "+index) {
			t.Fatalf("missing indexed lookup %s:\n%s", index, plan.String())
		}
	}
}
