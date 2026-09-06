package gameplay

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestCatalogRewardUsesExistingSingleConnectionTransaction(t *testing.T) {
	db := catalogSQLTestDB(t)
	runtime := &Runtime{DB: db, Catalog: &Catalog{DB: db}}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if created, err := insertBadgeDefSQL(ctx, db, apitypes.BadgeDef{Id: "badge", Spec: apitypes.BadgeDefSpec{DisplayName: "Badge"}, CreatedAt: now, UpdatedAt: now}); err != nil || !created {
		t.Fatalf("create = %v, %v", created, err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	badge, err := runtime.applyBadgeExp(ctx, tx, "owner", "badge", 20, now)
	if err != nil {
		t.Fatal(err)
	}
	if badge.Exp != 20 {
		t.Fatalf("exp = %v", badge.Exp)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
