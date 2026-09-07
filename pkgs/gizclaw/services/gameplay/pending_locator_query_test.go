package gameplay

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/sqltest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
)

func TestPendingDeletionAbsentLocatorUsesOneIndexedQuery(t *testing.T) {
	db, observer := sqltest.New(t)
	ctx := t.Context()
	if err := (&Runtime{DB: db}).Migration(ctx); err != nil {
		t.Fatal(err)
	}
	// Retired formats have no read fallback: an absent locator must not read
	// the deletion payload table, regardless of how many payloads it contains.
	observer.Reset()
	observer.Delay.Store(int64(20 * time.Millisecond))
	owner := "peer-a"
	exists, err := (PendingDeletionSource{DB: db}).HasLocator(ctx, pendingdeletion.Locator{Kind: pendingdeletion.KindPet, ResourceID: "absent", OwnerPublicKey: &owner})
	if err != nil || exists {
		t.Fatalf("absent locator = %v, %v", exists, err)
	}
	if statements, rows := observer.Statements.Load(), observer.Rows.Load(); statements != 1 || rows != 0 {
		t.Fatalf("statements=%d rows=%d; want 1 and 0", statements, rows)
	}
}
