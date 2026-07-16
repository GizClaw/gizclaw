package gameplay

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func TestPostgresGameplayContract(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })

	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, catalog)
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: &recordingWorkspaceService{},
		ACL:        &recordingACLService{},
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("pet-postgres", "adopt-txn", "game-result", "reward-grant", "drive-txn", "reward-txn"),
		PickWeight: func(int64) int64 { return 0 },
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() second run error = %v", err)
	}

	adopted, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	if adopted.Pet.Id != "pet-postgres" || adopted.Points.Balance != 35 {
		t.Fatalf("AdoptPet() = %#v", adopted)
	}
	idempotencyKey := "postgres-result-key"
	drive, err := runtime.DrivePet(ctx, "peer-postgres", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id,
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId:      "game-basic",
			IdempotencyKey: &idempotencyKey,
		},
	})
	if err != nil {
		t.Fatalf("DrivePet() error = %v", err)
	}
	if drive.GameResult == nil || len(drive.RewardGrants) != 1 || drive.Points.Balance != 65 {
		t.Fatalf("DrivePet() = %#v", drive)
	}
	if _, err := runtime.DrivePet(ctx, "peer-postgres", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id,
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId:      "game-basic",
			IdempotencyKey: &idempotencyKey,
		},
	}); err == nil {
		t.Fatal("DrivePet() duplicate idempotency key error = nil")
	}
	results, err := runtime.ListGameResults(ctx, "peer-postgres", apitypes.GameplayListRequest{})
	if err != nil {
		t.Fatalf("ListGameResults() error = %v", err)
	}
	if len(results.Items) != 1 {
		t.Fatalf("ListGameResults() count = %d, want 1", len(results.Items))
	}
	points, err := runtime.GetPoints(ctx, "peer-postgres", "default")
	if err != nil {
		t.Fatalf("GetPoints() error = %v", err)
	}
	if points.Balance != 65 {
		t.Fatalf("GetPoints() balance = %d, want 65", points.Balance)
	}
	pet, err := runtime.GetPet(ctx, "peer-postgres", adopted.Pet.Id)
	if err != nil || pet.Id != adopted.Pet.Id {
		t.Fatalf("GetPet() = %#v, %v", pet, err)
	}
	badge, err := runtime.GetBadge(ctx, "peer-postgres", "badge-basic")
	if err != nil || !badge.Active {
		t.Fatalf("GetBadge() = %#v, %v", badge, err)
	}
	badges, err := runtime.ListBadges(ctx, "peer-postgres", apitypes.GameplayListRequest{})
	if err != nil || len(badges.Items) != 1 {
		t.Fatalf("ListBadges() = %#v, %v", badges, err)
	}
	if result, err := runtime.GetGameResult(ctx, "peer-postgres", drive.GameResult.Id); err != nil || result.Id != drive.GameResult.Id {
		t.Fatalf("GetGameResult() = %#v, %v", result, err)
	}
	grants, err := runtime.ListRewardGrants(ctx, "peer-postgres", apitypes.GameplayListRequest{})
	if err != nil || len(grants.Items) != 1 {
		t.Fatalf("ListRewardGrants() = %#v, %v", grants, err)
	}
	transactions, err := runtime.ListPointsTransactions(ctx, "peer-postgres", apitypes.GameplayListRequest{})
	if err != nil || len(transactions.Items) != 2 {
		t.Fatalf("ListPointsTransactions() = %#v, %v", transactions, err)
	}

	runtime.NewID = sequentialIDs("pet-postgres-2", "adopt-txn-2")
	if _, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{}); err != nil {
		t.Fatalf("AdoptPet(second) error = %v", err)
	}
	limit := 1
	firstPage, err := runtime.ListPets(ctx, "peer-postgres", apitypes.GameplayListRequest{Limit: &limit})
	if err != nil || len(firstPage.Items) != 1 || !firstPage.HasNext || firstPage.NextCursor == nil {
		t.Fatalf("ListPets(first page) = %#v, %v", firstPage, err)
	}
	secondPage, err := runtime.ListPets(ctx, "peer-postgres", apitypes.GameplayListRequest{Limit: &limit, Cursor: firstPage.NextCursor})
	if err != nil || len(secondPage.Items) != 1 || secondPage.HasNext {
		t.Fatalf("ListPets(second page) = %#v, %v", secondPage, err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx() error = %v", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_points_accounts (owner_public_key, ruleset_name, balance, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`),
		"rollback-peer", "default", 1, formatTime(now), formatTime(now)); err != nil {
		_ = tx.Rollback()
		t.Fatalf("transactional insert error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	var rollbackRows int
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT count(*) FROM gameplay_points_accounts WHERE owner_public_key = ?`), "rollback-peer").Scan(&rollbackRows); err != nil {
		t.Fatalf("count rolled-back rows: %v", err)
	}
	if rollbackRows != 0 {
		t.Fatalf("rolled-back account rows = %d, want 0", rollbackRows)
	}
}

func TestPostgresGameplayConcurrentMigration(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })
	runtime := &Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("initial Migration() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE gameplay_points_transactions DROP COLUMN source_type, DROP COLUMN source_id`); err != nil {
		t.Fatalf("prepare legacy schema: %v", err)
	}

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- runtime.Migration(ctx)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Migration() error = %v", err)
		}
	}
	for _, column := range []string{"source_type", "source_id"} {
		exists, err := sqlColumnExists(ctx, db, "gameplay_points_transactions", column)
		if err != nil {
			t.Fatalf("inspect %s: %v", column, err)
		}
		if !exists {
			t.Fatalf("concurrent Migration() did not add %s", column)
		}
	}
}

func openGameplayPostgresTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sqlx.Open(postgres) error = %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("PingContext() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func dropGameplayPostgresTables(t *testing.T, ctx context.Context, db *sqlx.DB) {
	t.Helper()
	for _, table := range []string{
		"gameplay_reward_grants",
		"gameplay_game_results",
		"gameplay_badges",
		"gameplay_points_transactions",
		"gameplay_points_accounts",
		"gameplay_pets",
	} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table+" CASCADE"); err != nil {
			t.Errorf("drop %s: %v", table, err)
		}
	}
}
