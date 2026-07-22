package gameplay

import (
	"context"
	"errors"
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
	profile := seedGameplayCatalog(t, ctx, catalog)
	ctx = WithRuntimeProfile(ctx, profile)
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: &recordingWorkspaceService{},
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("pet-postgres", "adopt-txn", "game-result", "reward-grant", "drive-txn", "reward-txn"),
		PickWeight: func(int64) int64 { return 0 },
	}
	ctx = WithRewardEvaluator(ctx, rewardEvaluatorFunc(func(context.Context, RewardEvaluationRequest) (apitypes.GameRewardSpec, error) {
		return apitypes.GameRewardSpec{PetExpDelta: 5, BadgeExpDelta: map[string]int64{"basic": 5}, Reason: "completed"}, nil
	}))
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
	tickKey := "postgres-empty-tick"
	now = now.Add(time.Hour)
	tick, err := runtime.DrivePet(ctx, "peer-postgres", apitypes.PetDriveRequest{PetId: adopted.Pet.Id, IdempotencyKey: &tickKey})
	if err != nil {
		t.Fatalf("DrivePet(empty) error = %v", err)
	}
	now = now.Add(2 * time.Hour)
	tickReplay, err := runtime.DrivePet(ctx, "peer-postgres", apitypes.PetDriveRequest{PetId: adopted.Pet.Id, IdempotencyKey: &tickKey})
	if err != nil || tickReplay.Pet.StateSettledAt != tick.Pet.StateSettledAt {
		t.Fatalf("DrivePet(empty replay) = %#v, %v", tickReplay, err)
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
	if drive.GameResult == nil || len(drive.RewardGrants) != 1 || drive.Points.Balance != 25 {
		t.Fatalf("DrivePet() = %#v", drive)
	}
	if duplicate, err := runtime.DrivePet(ctx, "peer-postgres", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id,
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId:      "game-basic",
			IdempotencyKey: &idempotencyKey,
		},
	}); err != nil || duplicate.GameResult == nil || duplicate.GameResult.Id != drive.GameResult.Id {
		t.Fatalf("DrivePet() duplicate = %#v, %v", duplicate, err)
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
	if points.Balance != 25 {
		t.Fatalf("GetPoints() balance = %d, want 25", points.Balance)
	}
	pet, err := runtime.GetPet(ctx, "peer-postgres", adopted.Pet.Id)
	if err != nil || pet.Id != adopted.Pet.Id {
		t.Fatalf("GetPet() = %#v, %v", pet, err)
	}
	badge, err := runtime.GetBadge(ctx, "peer-postgres", "badge-basic")
	if err != nil || badge.Exp != 5 {
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
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_points_accounts (owner_public_key, runtime_profile_name, balance, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`),
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
			t.Fatalf("concurrent Migration() lost %s", column)
		}
	}
}

func TestPostgresCallerAssignedAdoptionIsConcurrent(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })
	now := time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	voices := *profile.Spec.Resources.Voices
	voices["pet-voice-alt"] = gameplayTestBinding("voice-alt")
	pool := *profile.Spec.Gameplay.Adoption.Pool
	alternate := pool[0]
	alternate.Voice = "pet-voice-alt"
	pool = append(pool, alternate)
	profile.Spec.Gameplay.Adoption.Pool = &pool
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	newRuntime := func(pickWeight func(int64) int64) *Runtime {
		return &Runtime{
			DB:         db,
			Catalog:    catalog,
			Workflows:  petWorkflowService{},
			Workspaces: workspaces,
			Now:        func() time.Time { return now },
			PickWeight: pickWeight,
		}
	}
	runtimes := []*Runtime{
		newRuntime(func(int64) int64 { return 0 }),
		newRuntime(func(total int64) int64 { return total - 1 }),
	}
	if err := runtimes[0].Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	petID := "postgres-pet-01"
	const workers = 8
	start := make(chan struct{})
	responses := make(chan apitypes.PetAdoptResponse, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		runtime := runtimes[i%len(runtimes)]
		wg.Go(func() {
			<-start
			response, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Id: &petID})
			responses <- response
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(responses)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AdoptPet(concurrent) error = %v", err)
		}
	}
	var transactionID string
	for response := range responses {
		if response.Pet.Id != petID || response.Points.Balance != 35 {
			t.Fatalf("AdoptPet(concurrent) = %#v", response)
		}
		if transactionID == "" {
			transactionID = response.Transaction.Id
		} else if response.Transaction.Id != transactionID {
			t.Fatalf("transaction ID = %q, want %q", response.Transaction.Id, transactionID)
		}
	}
	var pets, transactions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pets WHERE owner_public_key = $1 AND id = $2`, "peer-postgres", petID).Scan(&pets); err != nil {
		t.Fatalf("count Pets: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_points_transactions WHERE owner_public_key = $1 AND source_type = 'pet' AND source_id = $2 AND reason = 'pet.adopt'`, "peer-postgres", petID).Scan(&transactions); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if pets != 1 || transactions != 1 {
		t.Fatalf("persisted Pets=%d transactions=%d, want 1 and 1", pets, transactions)
	}
	if len(workspaces.created) != 1 || len(workspaces.deleted) != 0 {
		t.Fatalf("workspace mutations: created=%d deleted=%d, want 1 and 0", len(workspaces.created), len(workspaces.deleted))
	}
	var reservedVoice string
	if err := db.QueryRowContext(ctx, `SELECT voice_alias FROM gameplay_pet_adoption_reservations WHERE owner_public_key = $1 AND pet_id = $2`, "peer-postgres", petID).Scan(&reservedVoice); err != nil {
		t.Fatalf("load adoption reservation voice: %v", err)
	}
	parameters, err := workspaces.created[0].Parameters.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("decode winning Pet Workspace parameters: %v", err)
	}
	if parameters.Voice.VoiceId != reservedVoice {
		t.Fatalf("winning Pet Workspace voice = %q, want reserved voice %q", parameters.Voice.VoiceId, reservedVoice)
	}
}

func TestPostgresDifferentPetAdoptionsDebitPointsAtomically(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })
	now := time.Date(2026, 7, 22, 11, 30, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	newRuntime := func() *Runtime {
		return &Runtime{
			DB:         db,
			Catalog:    catalog,
			Workflows:  petWorkflowService{},
			Workspaces: workspaces,
			Now:        func() time.Time { return now },
			PickWeight: func(int64) int64 { return 0 },
		}
	}
	runtimes := []*Runtime{newRuntime(), newRuntime()}
	if err := runtimes[0].Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	petIDs := []string{"postgres-pet-a", "postgres-pet-b"}
	start := make(chan struct{})
	responses := make(chan apitypes.PetAdoptResponse, len(petIDs))
	errs := make(chan error, len(petIDs))
	var wg sync.WaitGroup
	for i, petID := range petIDs {
		runtime := runtimes[i]
		wg.Go(func() {
			<-start
			response, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Id: &petID})
			responses <- response
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(responses)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AdoptPet(concurrent different IDs) error = %v", err)
		}
	}
	balances := map[int64]int{}
	for response := range responses {
		balances[response.Points.Balance]++
	}
	if balances[35] != 1 || balances[20] != 1 {
		t.Fatalf("response balances = %v, want one 35 and one 20", balances)
	}
	var pets, transactions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pets WHERE owner_public_key = $1`, "peer-postgres").Scan(&pets); err != nil {
		t.Fatalf("count Pets: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_points_transactions WHERE owner_public_key = $1 AND source_type = 'pet' AND reason = 'pet.adopt'`, "peer-postgres").Scan(&transactions); err != nil {
		t.Fatalf("count adoption transactions: %v", err)
	}
	var balance int64
	if err := db.QueryRowContext(ctx, `SELECT balance FROM gameplay_points_accounts WHERE owner_public_key = $1 AND runtime_profile_name = $2`, "peer-postgres", profile.Name).Scan(&balance); err != nil {
		t.Fatalf("load final Points balance: %v", err)
	}
	if pets != 2 || transactions != 2 || balance != 20 {
		t.Fatalf("persisted Pets=%d transactions=%d balance=%d, want 2, 2, 20", pets, transactions, balance)
	}
	if len(workspaces.created) != 2 {
		t.Fatalf("created workspaces = %d, want 2", len(workspaces.created))
	}
}

func TestPostgresDifferentPetAdoptionsReleaseFailedReservation(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	initialBalance := int64(15)
	profile.Spec.Gameplay.Points.InitialBalance = &initialBalance
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	newRuntime := func() *Runtime {
		return &Runtime{
			DB: db, Catalog: catalog, Workflows: petWorkflowService{}, Workspaces: workspaces,
			Now: func() time.Time { return now }, PickWeight: func(int64) int64 { return 0 },
		}
	}
	runtimes := []*Runtime{newRuntime(), newRuntime()}
	if err := runtimes[0].Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	petIDs := []string{"postgres-pet-funded", "postgres-pet-unfunded"}
	errs := make(chan error, len(petIDs))
	for i, petID := range petIDs {
		runtime := runtimes[i]
		go func() {
			_, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Id: &petID})
			errs <- err
		}()
	}
	var succeeded, insufficient int
	for range petIDs {
		err := <-errs
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, errInsufficientPoints):
			insufficient++
		default:
			t.Fatalf("AdoptPet(concurrent different IDs) error = %v", err)
		}
	}
	if succeeded != 1 || insufficient != 1 {
		t.Fatalf("concurrent results: succeeded=%d insufficient=%d, want 1 and 1", succeeded, insufficient)
	}
	var reservations, pets, transactions int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM gameplay_pet_adoption_reservations WHERE owner_public_key = $1),
		(SELECT count(*) FROM gameplay_pets WHERE owner_public_key = $1),
		(SELECT count(*) FROM gameplay_points_transactions WHERE owner_public_key = $1 AND source_type = 'pet' AND reason = 'pet.adopt')`,
		"peer-postgres").Scan(&reservations, &pets, &transactions); err != nil {
		t.Fatalf("count adoption rows: %v", err)
	}
	if reservations != 1 || pets != 1 || transactions != 1 {
		t.Fatalf("persisted reservations=%d Pets=%d transactions=%d, want 1, 1, 1", reservations, pets, transactions)
	}
	if len(workspaces.created) != 1 {
		t.Fatalf("created workspaces = %d, want 1", len(workspaces.created))
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
		"gameplay_pet_drive_ticks",
		"gameplay_pet_adoption_reservations",
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
