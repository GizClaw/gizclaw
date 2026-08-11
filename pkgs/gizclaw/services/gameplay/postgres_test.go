package gameplay

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
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
	workspaces := &recordingWorkspaceService{}
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: workspaces,
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

	adopted, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Name: "pet-main", DisplayName: "Pet"})
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
	var outboxRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_drive_fact_outbox`).Scan(&outboxRows); err != nil || outboxRows != 1 {
		t.Fatalf("Drive Fact outbox rows = %d, %v", outboxRows, err)
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
	points, err := runtime.GetPoints(ctx, "peer-postgres", profile.Id)
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
	secondAdoption, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Name: "pet-second", DisplayName: "Pet"})
	if err != nil {
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
	if _, err := runtime.DeletePet(ctx, "peer-postgres", adopted.Pet.Id); err != nil {
		t.Fatalf("DeletePet() error = %v", err)
	}
	if len(workspaces.deleted) != 0 {
		t.Fatalf("DeletePet() deleted bound Workspace: %#v", workspaces.deleted)
	}
	workspaceName := petWorkspaceName("peer-postgres", adopted.Pet.Id)
	allowed, err := runtime.OwnerHasPetWorkspace(ctx, "peer-postgres", workspaceName)
	if err != nil || !allowed {
		t.Fatalf("OwnerHasPetWorkspace() after delete = %v, %v", allowed, err)
	}
	workspaceNames, err := runtime.ListPetWorkspaceNames(ctx, "peer-postgres")
	if err != nil || !slices.Contains(workspaceNames, workspaceName) {
		t.Fatalf("ListPetWorkspaceNames() after delete = %#v, %v", workspaceNames, err)
	}
	var pendingRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pending_deletions WHERE kind = 'pet' AND owner_public_key = $1 AND resource_id = $2`, "peer-postgres", adopted.Pet.Id).Scan(&pendingRows); err != nil {
		t.Fatalf("count pending Pet deletions: %v", err)
	}
	if pendingRows != 1 {
		t.Fatalf("pending Pet deletions = %d, want 1", pendingRows)
	}
	source := PendingDeletionSource{DB: db}
	refs, _, err := source.ScanDue(ctx, now.Add(time.Second), 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("PendingDeletionSource.ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now.Add(time.Second), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("PendingDeletionSource.Claim() = %#v, %v, %v", claim, claimed, err)
	}
	if err := (PetDeletionHandler{DB: db, Now: func() time.Time { return now.Add(time.Second) }}).Handle(ctx, claim); err != nil {
		t.Fatalf("PetDeletionHandler.Handle() error = %v", err)
	}
	if _, err := runtime.GetPet(ctx, "peer-postgres", adopted.Pet.Id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetPet() after cleanup error = %v, want sql.ErrNoRows", err)
	}
	if _, err := source.GetTask(ctx, claim.Record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("GetTask() after cleanup error = %v, want ErrNotFound", err)
	}
	if points, err := runtime.GetPoints(ctx, "peer-postgres", profile.Id); err != nil || points.Balance != secondAdoption.Points.Balance {
		t.Fatalf("GetPoints() after cleanup = %#v, %v", points, err)
	}
	if result, err := runtime.GetGameResult(ctx, "peer-postgres", drive.GameResult.Id); err != nil || result.Id != drive.GameResult.Id {
		t.Fatalf("GetGameResult() after cleanup = %#v, %v", result, err)
	}
	if allowed, err := runtime.OwnerHasPetWorkspace(ctx, "peer-postgres", workspaceName); err != nil || !allowed {
		t.Fatalf("OwnerHasPetWorkspace() after cleanup = %v, %v", allowed, err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx() error = %v", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO gameplay_points_accounts (owner_public_key, runtime_profile_id, balance, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`),
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
	now := time.Date(2026, 7, 29, 3, 45, 0, 0, time.UTC)
	runtime := &Runtime{DB: db, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("initial Migration() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP INDEX gameplay_workspace_reward_windows_active_v2_idx`); err != nil {
		t.Fatalf("drop v2 active index: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX gameplay_workspace_reward_windows_active_idx
		ON gameplay_workspace_reward_windows(workspace_id)
		WHERE state IN ('pending', 'claimed', 'retry', 'blocked')`); err != nil {
		t.Fatalf("create legacy active index: %v", err)
	}
	source := workspaceRewardSource{
		WorkspaceID: "workflow-upgrade", ScheduledCheckpoint: "001",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
		t.Fatalf("insert upgrade source: %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	window := workspaceRewardWindow{
		ID: "window-blocked", WorkspaceID: source.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-a",
		RuntimeProfileId: "runtime-profile-a", RuntimeProfileRevision: "revision-a",
		Policy: policy, PolicyDigest: policy.Digest,
		StartHistoryID: "001", HighWaterHistoryID: "001",
		StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now,
		LastActivityAt: now, EvaluateAfter: now, State: workspaceRewardBlocked,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert legacy blocked window: %v", err)
	}
	const workers = 8
	runtimes := make([]*Runtime, workers)
	for i := range runtimes {
		runtimes[i] = &Runtime{DB: db, Now: func() time.Time { return now }}
	}
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for _, runtime := range runtimes {
		wg.Go(func() {
			<-start
			errs <- runtime.Migration(ctx)
		})
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
	var v2IndexExists, legacyIndexExists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_indexes
		WHERE schemaname = current_schema()
			AND tablename = 'gameplay_workspace_reward_windows'
			AND indexname = 'gameplay_workspace_reward_windows_active_v2_idx'
	)`).Scan(&v2IndexExists); err != nil {
		t.Fatalf("inspect v2 active index: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_indexes
		WHERE schemaname = current_schema()
			AND tablename = 'gameplay_workspace_reward_windows'
			AND indexname = 'gameplay_workspace_reward_windows_active_idx'
	)`).Scan(&legacyIndexExists); err != nil {
		t.Fatalf("inspect legacy active index: %v", err)
	}
	if !v2IndexExists || legacyIndexExists {
		t.Fatalf("active indexes: v2=%v legacy=%v, want true and false", v2IndexExists, legacyIndexExists)
	}
	var preservedState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM gameplay_workspace_reward_windows WHERE id = $1`, window.ID).Scan(&preservedState); err != nil {
		t.Fatalf("load legacy blocked window: %v", err)
	}
	if preservedState != workspaceRewardBlocked {
		t.Fatalf("legacy window state = %q, want %q", preservedState, workspaceRewardBlocked)
	}
	window.ID = "window-pending"
	window.BeneficiaryPublicKey = "peer-b"
	window.StartHistoryID = "002"
	window.HighWaterHistoryID = "002"
	window.State = workspaceRewardPending
	source.ScheduledCheckpoint = "002"
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert pending window after concurrent upgrade: %v", err)
	}
}

func TestPostgresGameplayMigrationLockCancellation(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })

	runtime := &Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("initial Migration() error = %v", err)
	}
	lockHolder, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx() error = %v", err)
	}
	defer lockHolder.Rollback()
	if _, err := lockHolder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, gameplayMigrationLockID); err != nil {
		t.Fatalf("acquire blocking migration lock: %v", err)
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- (&Runtime{DB: db}).Migration(waitCtx)
	}()
	waitForGameplayPostgresConnections(t, db, 2)
	cancel()
	select {
	case err = <-waitErr:
	case <-time.After(5 * time.Second):
		t.Fatal("waiting Migration() did not return after context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting Migration() error = %v, want context cancellation", err)
	}
	if !strings.Contains(err.Error(), "gameplay: acquire migration lock") {
		t.Fatalf("waiting Migration() error = %v, want lock operation context", err)
	}
	if err := lockHolder.Rollback(); err != nil {
		t.Fatalf("release blocking migration lock: %v", err)
	}
	waitForGameplayPostgresConnections(t, db, 0)
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() after cancellation error = %v", err)
	}
}

func TestPostgresWorkspaceRewardConcurrentSettlementHonorsBudget(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })
	testConcurrentWorkspaceRewardSettlement(t, db)
}

func TestPostgresCallerAssignedAdoptionIsConcurrent(t *testing.T) {
	db := openGameplayPostgresTestDB(t)
	ctx := context.Background()
	dropGameplayPostgresTables(t, ctx, db)
	t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })
	now := time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	pool := *profile.Spec.Gameplay.Adoption.Pool
	alternate := pool[0]
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
	petName := "postgres-pet-01"
	const workers = 8
	start := make(chan struct{})
	responses := make(chan apitypes.PetAdoptResponse, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		runtime := runtimes[i%len(runtimes)]
		wg.Go(func() {
			<-start
			response, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Name: petName, DisplayName: "Pet"})
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
	var petID string
	for response := range responses {
		if response.Pet.Name != petName || response.Points.Balance != 35 {
			t.Fatalf("AdoptPet(concurrent) = %#v", response)
		}
		if petID == "" {
			petID = response.Pet.Id
		} else if response.Pet.Id != petID {
			t.Fatalf("Pet ID = %q, want %q", response.Pet.Id, petID)
		}
		if transactionID == "" {
			transactionID = response.Transaction.Id
		} else if response.Transaction.Id != transactionID {
			t.Fatalf("transaction ID = %q, want %q", response.Transaction.Id, transactionID)
		}
	}
	var pets, transactions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pets WHERE owner_public_key = $1 AND name = $2`, "peer-postgres", petName).Scan(&pets); err != nil {
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
	if workspaces.created[0].Parameters != nil || workspaces.created[0].WorkflowId != profile.Spec.Workflows.System.Pet {
		t.Fatalf("winning Pet Workspace = %#v", workspaces.created[0])
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
			response, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Name: petID, DisplayName: "Pet"})
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
	if err := db.QueryRowContext(ctx, `SELECT balance FROM gameplay_points_accounts WHERE owner_public_key = $1 AND runtime_profile_id = $2`, "peer-postgres", profile.Id).Scan(&balance); err != nil {
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
			_, err := runtime.AdoptPet(ctx, "peer-postgres", apitypes.PetAdoptRequest{Name: petID, DisplayName: "Pet"})
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

func TestPostgresWorkspaceRewardIsolationContract(t *testing.T) {
	t.Run("blocks exact corrupt policy row", func(t *testing.T) {
		db := openGameplayPostgresTestDB(t)
		ctx := context.Background()
		dropGameplayPostgresTables(t, ctx, db)
		t.Cleanup(func() { dropGameplayPostgresTables(t, context.Background(), db) })

		now := time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)
		environment := &workspaceRewardTestEnvironment{
			ids:     []string{"workspace-corrupt", "workspace-z-active"},
			entries: map[string][]workspace.HistoryEntry{"workspace-corrupt": nil, "workspace-z-active": nil},
		}
		runtime := &Runtime{DB: db, WorkspaceRewards: environment, Now: func() time.Time { return now }}
		if err := runtime.Migration(ctx); err != nil {
			t.Fatalf("Migration() error = %v", err)
		}
		source := workspaceRewardSource{WorkspaceID: "workspace-corrupt", CreatedAt: now, UpdatedAt: now}
		if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
			t.Fatalf("insert source: %v", err)
		}
		policy := workspaceRewardTestPolicy(t)
		window := workspaceRewardWindow{
			ID: "window-corrupt", WorkspaceID: source.WorkspaceID,
			WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-corrupt",
			RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
			Policy: policy, PolicyDigest: policy.Digest,
			StartHistoryID: "history-corrupt", HighWaterHistoryID: "history-corrupt",
			StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
			EvaluateAfter: now, State: workspaceRewardPending, CreatedAt: now, UpdatedAt: now,
		}
		if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
			t.Fatalf("insert window: %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE gameplay_workspace_reward_windows SET policy_json = '{' WHERE id = $1`, window.ID); err != nil {
			t.Fatalf("corrupt policy JSON: %v", err)
		}
		stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
		if err != nil {
			t.Fatalf("StartWorkspaceRewardDispatcher() error = %v", err)
		}
		stop()
		<-done
		var state, lastError string
		if err := db.QueryRowContext(ctx, `SELECT state, last_error FROM gameplay_workspace_reward_windows WHERE id = $1`, window.ID).Scan(&state, &lastError); err != nil {
			t.Fatalf("read corrupt window: %v", err)
		}
		if state != workspaceRewardBlocked || lastError != "reward_policy_invalid" {
			t.Fatalf("corrupt window state/error = %q/%q, want blocked/reward_policy_invalid", state, lastError)
		}
		if _, err := runtime.getWorkspaceRewardSource(ctx, "workspace-z-active"); err != nil {
			t.Fatalf("active neighbor source error = %v", err)
		}
	})

	t.Run("rolls back exact local fence failure", func(t *testing.T) {
		db := openGameplayPostgresTestDB(t)
		ctx := context.Background()
		dropGameplayPostgresTables(t, ctx, db)
		_, _ = db.ExecContext(ctx, `DROP FUNCTION IF EXISTS fail_workspace_reward_fence()`)
		t.Cleanup(func() {
			dropGameplayPostgresTables(t, context.Background(), db)
			_, _ = db.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS fail_workspace_reward_fence()`)
		})

		now := time.Date(2026, 8, 12, 11, 15, 0, 0, time.UTC)
		environment := &workspaceRewardTestEnvironment{
			ids:          []string{"workspace-broken"},
			availability: map[string]error{"workspace-broken": workspace.ErrPeerDeleted},
			entries:      map[string][]workspace.HistoryEntry{"workspace-broken": nil},
		}
		runtime := &Runtime{DB: db, WorkspaceRewards: environment, Now: func() time.Time { return now }}
		if err := runtime.Migration(ctx); err != nil {
			t.Fatalf("Migration() error = %v", err)
		}
		source := workspaceRewardSource{WorkspaceID: "workspace-broken", CreatedAt: now, UpdatedAt: now}
		if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
			t.Fatalf("insert source: %v", err)
		}
		policy := workspaceRewardTestPolicy(t)
		window := workspaceRewardWindow{
			ID: "window-broken", WorkspaceID: source.WorkspaceID,
			WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-broken",
			RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
			Policy: policy, PolicyDigest: policy.Digest,
			StartHistoryID: "history-broken", HighWaterHistoryID: "history-broken",
			StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
			EvaluateAfter: now, State: workspaceRewardPending, CreatedAt: now, UpdatedAt: now,
		}
		if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
			t.Fatalf("insert window: %v", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE FUNCTION fail_workspace_reward_fence() RETURNS trigger
			LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced Workspace reward fence failure'; END $$`); err != nil {
			t.Fatalf("create failure function: %v", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_workspace_reward_fence
			BEFORE DELETE ON gameplay_workspace_reward_sources FOR EACH ROW
			WHEN (OLD.workspace_id = 'workspace-broken') EXECUTE FUNCTION fail_workspace_reward_fence()`); err != nil {
			t.Fatalf("create failure trigger: %v", err)
		}
		if _, _, err := runtime.StartWorkspaceRewardDispatcher(ctx); err == nil || !strings.Contains(err.Error(), "forced Workspace reward fence failure") {
			t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want fence failure", err)
		}
		if _, err := runtime.getWorkspaceRewardSource(ctx, source.WorkspaceID); err != nil {
			t.Fatalf("source changed after rollback: %v", err)
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE id = $1`, window.ID).Scan(&count); err != nil {
			t.Fatalf("count window after rollback: %v", err)
		}
		if count != 1 {
			t.Fatalf("window count after rollback = %d, want 1", count)
		}
	})
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

func waitForGameplayPostgresConnections(t *testing.T, db *sqlx.DB, inUse int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for db.Stats().InUse != inUse {
		if time.Now().After(deadline) {
			t.Fatalf("PostgreSQL connections in use = %d, want %d", db.Stats().InUse, inUse)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func dropGameplayPostgresTables(t *testing.T, ctx context.Context, db *sqlx.DB) {
	t.Helper()
	for _, table := range []string{
		"gameplay_workspace_reward_windows",
		"gameplay_workspace_reward_sources",
		"gameplay_workspace_reward_activation",
		"gameplay_drive_fact_outbox",
		"gameplay_pending_deletion_locators",
		"gameplay_pending_deletions",
		"gameplay_pet_drive_ticks",
		"gameplay_pet_workspace_bindings",
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
