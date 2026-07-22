package gameplay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestGetPointsAllowsProfileWithoutPetGameplay(t *testing.T) {
	initialBalance := int64(25)
	profile := apitypes.RuntimeProfile{
		Name: "points-only",
		Spec: apitypes.RuntimeProfileSpec{Gameplay: &apitypes.RuntimeProfileGameplaySpec{
			Points: &apitypes.RuntimeProfilePointsSpec{InitialBalance: &initialBalance},
		}},
	}
	runtime := &Runtime{DB: testDB(t)}
	account, err := runtime.GetPoints(WithRuntimeProfile(context.Background(), profile), "peer-points", profile.Name)
	if err != nil {
		t.Fatalf("GetPoints() error = %v", err)
	}
	if account.Balance != initialBalance || account.RuntimeProfileName != profile.Name {
		t.Fatalf("GetPoints() = %#v, want points-only profile account", account)
	}
}

func TestListPetWorkspaceNamesMigratesFreshDatabase(t *testing.T) {
	runtime := &Runtime{DB: testDB(t)}
	ctx := WithRuntimeProfile(context.Background(), apitypes.RuntimeProfile{Name: "profile-a"})
	names, err := runtime.ListPetWorkspaceNames(ctx, "peer-a")
	if err != nil {
		t.Fatalf("ListPetWorkspaceNames() error = %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("ListPetWorkspaceNames() = %#v, want empty", names)
	}
}

func TestRuntimeAdoptDoesNotDeleteExistingSystemWorkspaceOnIDCollision(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	ctx = WithRuntimeProfile(ctx, profile)
	db := testDB(t)
	workspaces := &recordingWorkspaceService{}
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: workspaces,
		Now: func() time.Time {
			return now
		},
		NewID: func() string {
			return "same-id"
		},
		PickWeight: func(int64) int64 { return 0 },
	}
	if _, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{}); err != nil {
		t.Fatalf("first AdoptPet() error = %v", err)
	}
	if len(workspaces.created) != 1 || workspaces.created[0].Parameters == nil {
		t.Fatalf("created workspaces = %#v, want one Pet Workspace with parameters", workspaces.created)
	}
	parameters, err := workspaces.created[0].Parameters.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("decode Pet Workspace parameters: %v", err)
	}
	if parameters.Voice.VoiceId != "pet-voice" {
		t.Fatalf("Pet Workspace voice alias = %q, want pet-voice from RuntimeProfile adoption pool", parameters.Voice.VoiceId)
	}
	if _, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{}); err == nil {
		t.Fatal("second AdoptPet() should fail")
	}
	if len(workspaces.deleted) != 0 {
		t.Fatalf("deleted workspaces = %#v, want existing workspace preserved", workspaces.deleted)
	}
}

func TestRuntimeAdoptWithCallerIDIsIdempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	pickCount := 0
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: workspaces,
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("adopt-txn"),
		PickWeight: func(int64) int64 {
			pickCount++
			return 0
		},
	}
	petID := "device-pet-01"
	displayName := "Miso"
	first, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID, DisplayName: &displayName})
	if err != nil {
		t.Fatalf("AdoptPet(first) error = %v", err)
	}
	if first.Pet.Id != petID || first.Pet.WorkspaceName != petWorkspaceName("peer-a", petID) || first.Transaction.Id != "adopt-txn" {
		t.Fatalf("AdoptPet(first) = %#v", first)
	}
	if _, err := runtime.DB.Exec(`UPDATE gameplay_points_accounts SET balance = 0 WHERE owner_public_key = ?`, "peer-a"); err != nil {
		t.Fatalf("set current Points balance: %v", err)
	}
	changedName := "Changed"
	retry, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID, DisplayName: &changedName})
	if err != nil {
		t.Fatalf("AdoptPet(retry) error = %v", err)
	}
	if retry.Pet.Id != first.Pet.Id || retry.Pet.WorkspaceName != first.Pet.WorkspaceName || retry.Transaction.Id != first.Transaction.Id {
		t.Fatalf("AdoptPet(retry) = %#v, want %#v", retry, first)
	}
	if retry.Points.Balance != 0 || retry.Transaction.BalanceAfter != first.Transaction.BalanceAfter {
		t.Fatalf("AdoptPet(retry) Points = %#v, transaction = %#v; want current balance and original transaction", retry.Points, retry.Transaction)
	}
	if retry.Pet.DisplayName != displayName || pickCount != 1 || len(workspaces.created) != 1 {
		t.Fatalf("retry mutated adoption: name=%q picks=%d workspaces=%d", retry.Pet.DisplayName, pickCount, len(workspaces.created))
	}
	var pets, transactions int
	if err := runtime.DB.QueryRow(`SELECT count(*) FROM gameplay_pets WHERE owner_public_key = ? AND id = ?`, "peer-a", petID).Scan(&pets); err != nil {
		t.Fatalf("count Pets: %v", err)
	}
	if err := runtime.DB.QueryRow(`SELECT count(*) FROM gameplay_points_transactions WHERE owner_public_key = ? AND source_type = 'pet' AND source_id = ? AND reason = 'pet.adopt'`, "peer-a", petID).Scan(&transactions); err != nil {
		t.Fatalf("count adoption transactions: %v", err)
	}
	if pets != 1 || transactions != 1 {
		t.Fatalf("persisted Pets=%d transactions=%d, want 1 and 1", pets, transactions)
	}
}

func TestRuntimeAdoptCallerIDScopesIdentityToPeer(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 9, 30, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: workspaces,
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("txn-a", "txn-b", "txn-c"),
		PickWeight: func(int64) int64 { return 0 },
	}
	petID := "shared-pet-01"
	first, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID})
	if err != nil {
		t.Fatalf("AdoptPet(peer-a) error = %v", err)
	}
	second, err := runtime.AdoptPet(ctx, "peer-b", apitypes.PetAdoptRequest{Id: &petID})
	if err != nil {
		t.Fatalf("AdoptPet(peer-b) error = %v", err)
	}
	if first.Pet.Id != second.Pet.Id || first.Pet.OwnerPublicKey == second.Pet.OwnerPublicKey || first.Pet.WorkspaceName == second.Pet.WorkspaceName {
		t.Fatalf("peer-scoped Pets = %#v and %#v", first.Pet, second.Pet)
	}
	got, err := runtime.GetPet(ctx, "peer-a", second.Pet.Id)
	if err != nil {
		t.Fatalf("GetPet(peer-a own textual ID) error = %v", err)
	}
	if got.OwnerPublicKey != "peer-a" || got.WorkspaceName != first.Pet.WorkspaceName {
		t.Fatalf("GetPet(peer-a own textual ID) = %#v, want peer-a Pet", got)
	}
	secondPetID := "shared-pet-02"
	third, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &secondPetID})
	if err != nil {
		t.Fatalf("AdoptPet(peer-a second ID) error = %v", err)
	}
	if third.Pet.Id != secondPetID || third.Pet.OwnerPublicKey != "peer-a" || third.Pet.WorkspaceName == first.Pet.WorkspaceName {
		t.Fatalf("AdoptPet(peer-a second ID) = %#v", third.Pet)
	}
	if len(workspaces.created) != 3 {
		t.Fatalf("created workspaces = %d, want 3", len(workspaces.created))
	}
}

func TestRuntimeAdoptCallerIDDoesNotReserveUnaffordableID(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 9, 45, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	initialBalance := int64(0)
	profile.Spec.Gameplay.Points.InitialBalance = &initialBalance
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	pickCount := 0
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: workspaces,
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("adopt-txn"),
		PickWeight: func(int64) int64 {
			pickCount++
			return 0
		},
	}
	petID := "device-pet-cleanup"
	if _, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID}); err == nil {
		t.Fatal("AdoptPet(insufficient Points) error = nil")
	}
	if len(workspaces.created) != 0 || len(workspaces.deleted) != 0 {
		t.Fatalf("workspace mutations after unaffordable adoption: created=%d deleted=%d, want 0 and 0", len(workspaces.created), len(workspaces.deleted))
	}
	var reservations, pets, transactions int
	if err := runtime.DB.QueryRow(`SELECT
		(SELECT count(*) FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ? AND pet_id = ?),
		(SELECT count(*) FROM gameplay_pets WHERE owner_public_key = ? AND id = ?),
		(SELECT count(*) FROM gameplay_points_transactions WHERE owner_public_key = ? AND source_type = 'pet' AND source_id = ? AND reason = 'pet.adopt')`,
		"peer-a", petID, "peer-a", petID, "peer-a", petID).Scan(&reservations, &pets, &transactions); err != nil {
		t.Fatalf("count unaffordable adoption rows: %v", err)
	}
	if reservations != 0 || pets != 0 || transactions != 0 {
		t.Fatalf("rows after unaffordable adoption: reservations=%d Pets=%d transactions=%d, want all zero", reservations, pets, transactions)
	}
	fundedBalance := int64(50)
	profile.Name = "funded"
	profile.Spec.Gameplay.Points.InitialBalance = &fundedBalance
	response, err := runtime.AdoptPet(WithRuntimeProfile(context.Background(), profile), "peer-a", apitypes.PetAdoptRequest{Id: &petID})
	if err != nil {
		t.Fatalf("AdoptPet(funded profile) error = %v", err)
	}
	if response.Pet.Id != petID || response.Pet.RuntimeProfileName != profile.Name || response.Points.Balance != 35 || pickCount != 2 {
		t.Fatalf("AdoptPet(funded profile) = %#v, picks=%d; want successful reuse under funded profile", response, pickCount)
	}
	if len(workspaces.created) != 1 {
		t.Fatalf("created workspaces after funded adoption = %d, want 1", len(workspaces.created))
	}
}

func TestRuntimeAdoptCallerIDPreservesExistingReservationWhenUnfunded(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 9, 50, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	initialBalance := int64(0)
	profile.Spec.Gameplay.Points.InitialBalance = &initialBalance
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	runtime := &Runtime{
		DB: testDB(t), Catalog: catalog, Workflows: petWorkflowService{}, Workspaces: workspaces,
		Now: func() time.Time { return now }, PickWeight: func(int64) int64 { return 0 },
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	petID := "device-pet-existing-reservation"
	reservation := petAdoptionReservation{
		OwnerPublicKey: "peer-a", PetID: petID, RuntimeProfileName: profile.Name,
		PetDefID: "petdef-basic", DisplayName: "Spark", WorkspaceName: petWorkspaceName("peer-a", petID),
		WorkflowName: defaultPetWorkflowName, VoiceAlias: "voice-basic", AdoptionCost: 15, CreatedAt: now,
	}
	tx, err := runtime.DB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin reservation: %v", err)
	}
	if err := insertPetAdoptionReservation(ctx, tx, reservation); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit reservation: %v", err)
	}
	if _, err := runtime.createPetWorkspace(ctx, reservation.WorkspaceName, reservation.WorkflowName, reservation.VoiceAlias); err != nil {
		t.Fatalf("create reserved Workspace: %v", err)
	}
	if _, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID}); !errors.Is(err, errInsufficientPoints) {
		t.Fatalf("AdoptPet(existing unfunded reservation) error = %v, want insufficient Points", err)
	}
	var reservations, pets, transactions int
	if err := runtime.DB.QueryRow(`SELECT
		(SELECT count(*) FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ? AND pet_id = ?),
		(SELECT count(*) FROM gameplay_pets WHERE owner_public_key = ? AND id = ?),
		(SELECT count(*) FROM gameplay_points_transactions WHERE owner_public_key = ? AND source_type = 'pet' AND source_id = ? AND reason = 'pet.adopt')`,
		"peer-a", petID, "peer-a", petID, "peer-a", petID).Scan(&reservations, &pets, &transactions); err != nil {
		t.Fatalf("count preserved adoption rows: %v", err)
	}
	if reservations != 1 || pets != 0 || transactions != 0 || len(workspaces.created) != 1 {
		t.Fatalf("preserved state: reservations=%d Pets=%d transactions=%d Workspaces=%d, want 1, 0, 0, 1", reservations, pets, transactions, len(workspaces.created))
	}
}

func TestRuntimeAdoptCallerIDUsesAuthoritativeReservationCost(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 9, 55, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	pool := append([]apitypes.RuntimeProfilePetPoolEntry(nil), (*profile.Spec.Gameplay.Adoption.Pool)...)
	tooExpensive := int64(60)
	expensiveEntry := pool[0]
	expensiveEntry.AdoptionCost = &tooExpensive
	pool = append(pool, expensiveEntry)
	profile.Spec.Gameplay.Adoption.Pool = &pool
	ctx = WithRuntimeProfile(ctx, profile)
	db := testDB(t)
	petID := "device-pet-reservation-race"
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: &recordingWorkspaceService{},
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("adopt-txn"),
		PickWeight: func(total int64) int64 {
			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("begin concurrent reservation: %v", err)
			}
			defer tx.Rollback()
			if err := insertPetAdoptionReservation(ctx, tx, petAdoptionReservation{
				OwnerPublicKey: "peer-a", PetID: petID, RuntimeProfileName: profile.Name,
				PetDefID: "petdef-basic", DisplayName: "Spark", WorkspaceName: petWorkspaceName("peer-a", petID),
				WorkflowName: defaultPetWorkflowName, VoiceAlias: "voice-basic", AdoptionCost: 15, CreatedAt: now,
			}); err != nil {
				t.Fatalf("insert concurrent reservation: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit concurrent reservation: %v", err)
			}
			return total - 1
		},
	}
	response, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	if response.Pet.Id != petID || response.Points.Balance != 35 {
		t.Fatalf("AdoptPet() = %#v, want authoritative affordable reservation", response)
	}
	var adoptionCost int64
	if err := db.QueryRow(`SELECT adoption_cost FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ? AND pet_id = ?`, "peer-a", petID).Scan(&adoptionCost); err != nil {
		t.Fatalf("load authoritative reservation cost: %v", err)
	}
	if adoptionCost != 15 {
		t.Fatalf("authoritative reservation cost = %d, want 15", adoptionCost)
	}
}

func TestRuntimeAdoptCallerIDRejectsInvalidProfileAndDeletedReuse(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	workspaces := &recordingWorkspaceService{}
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: workspaces,
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("adopt-txn"),
		PickWeight: func(int64) int64 { return 0 },
	}
	invalidID := "short"
	if _, err := runtime.AdoptPet(WithRuntimeProfile(ctx, profile), "peer-a", apitypes.PetAdoptRequest{Id: &invalidID}); err == nil {
		t.Fatal("AdoptPet(invalid ID) error = nil")
	}
	if len(workspaces.created) != 0 {
		t.Fatalf("invalid ID created %d workspaces", len(workspaces.created))
	}
	petID := "device-pet-02"
	profileCtx := WithRuntimeProfile(ctx, profile)
	adopted, err := runtime.AdoptPet(profileCtx, "peer-a", apitypes.PetAdoptRequest{Id: &petID})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	otherProfile := profile
	otherProfile.Name = "other"
	if _, err := runtime.AdoptPet(WithRuntimeProfile(ctx, otherProfile), "peer-a", apitypes.PetAdoptRequest{Id: &petID}); !errors.Is(err, ErrPetIDConflict) {
		t.Fatalf("AdoptPet(cross-profile) error = %v, want conflict", err)
	}
	if _, err := runtime.DeletePet(profileCtx, "peer-a", petID); err != nil {
		t.Fatalf("DeletePet() error = %v", err)
	}
	if len(workspaces.deleted) != 0 {
		t.Fatalf("DeletePet() deleted bound Workspace: %#v", workspaces.deleted)
	}
	workspaceName := adopted.Pet.WorkspaceName
	allowed, err := runtime.OwnerHasPetWorkspace(profileCtx, "peer-a", workspaceName)
	if err != nil || !allowed {
		t.Fatalf("OwnerHasPetWorkspace() after delete = %v, %v", allowed, err)
	}
	workspaceNames, err := runtime.ListPetWorkspaceNames(profileCtx, "peer-a")
	if err != nil || !slices.Contains(workspaceNames, workspaceName) {
		t.Fatalf("ListPetWorkspaceNames() after delete = %#v, %v", workspaceNames, err)
	}
	var bindingCount int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ? AND pet_id = ?`, "peer-a", petID).Scan(&bindingCount); err != nil {
		t.Fatalf("query Pet Workspace binding: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("Pet Workspace binding count = %d, want 1", bindingCount)
	}
	var pendingCount int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_pending_deletions WHERE kind = 'pet' AND owner_public_key = ? AND resource_id = ?`, "peer-a", petID).Scan(&pendingCount); err != nil {
		t.Fatalf("query Pet pending deletion: %v", err)
	}
	if pendingCount != 1 {
		t.Fatalf("Pet pending deletion count = %d, want 1", pendingCount)
	}
	var deletionID string
	if err := runtime.DB.QueryRowContext(ctx, `SELECT deletion_id FROM gameplay_pending_deletions WHERE owner_public_key = ? AND resource_id = ?`, "peer-a", petID).Scan(&deletionID); err != nil {
		t.Fatalf("query Pet pending deletion ID: %v", err)
	}
	source := PendingDeletionSource{DB: runtime.DB}
	record, err := source.Get(ctx, deletionID)
	if err != nil || record.Kind != pendingdeletion.KindPet || record.ResourceID != petID {
		t.Fatalf("PendingDeletionSource.Get() = %#v, error = %v", record, err)
	}
	owner := "peer-a"
	if exists, err := source.HasLocator(ctx, pendingdeletion.Locator{Kind: pendingdeletion.KindPet, ResourceID: petID, OwnerPublicKey: &owner}); err != nil || !exists {
		t.Fatalf("PendingDeletionSource.HasLocator() = %v, error = %v", exists, err)
	}
	if _, err := runtime.AdoptPet(profileCtx, "peer-a", apitypes.PetAdoptRequest{Id: &petID}); !errors.Is(err, ErrPetIDConflict) {
		t.Fatalf("AdoptPet(deleted ID) error = %v, want conflict", err)
	}
}

func TestRuntimeDeletePetRollsBackWhenPendingInsertFails(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	runtime := &Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	now := time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO gameplay_pets (
		owner_public_key, id, runtime_profile_name, petdef_id, display_name, workspace_name,
		stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"peer-a", "pet-rollback", "default", "petdef-a", "Pet", "pet-pet-rollback",
		`{"life":100,"health":100,"satiety":100,"hygiene":100,"mood":100,"energy":100}`, `{"experience":0,"level":1}`, "alive", nil, now, now, now, now,
	); err != nil {
		t.Fatalf("insert Pet: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_gameplay_pending_insert BEFORE INSERT ON gameplay_pending_deletions BEGIN SELECT RAISE(ABORT, 'injected pending failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, err := runtime.DeletePet(ctx, "peer-a", "pet-rollback"); err == nil {
		t.Fatal("DeletePet() error = nil, want pending insert failure")
	}
	var pets, pending, bindings int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pets WHERE owner_public_key = ? AND id = ?`, "peer-a", "pet-rollback").Scan(&pets); err != nil {
		t.Fatalf("count Pets: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pending_deletions WHERE owner_public_key = ? AND resource_id = ?`, "peer-a", "pet-rollback").Scan(&pending); err != nil {
		t.Fatalf("count pending deletions: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ? AND pet_id = ?`, "peer-a", "pet-rollback").Scan(&bindings); err != nil {
		t.Fatalf("count Pet Workspace bindings: %v", err)
	}
	if pets != 1 || pending != 0 || bindings != 0 {
		t.Fatalf("after rollback Pets=%d pending=%d bindings=%d, want 1, 0 and 0", pets, pending, bindings)
	}
}

func TestRuntimeDeletePetRejectsConflictingWorkspaceBinding(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	runtime := &Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	now := time.Date(2026, 7, 22, 11, 15, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO gameplay_pets (
		owner_public_key, id, runtime_profile_name, petdef_id, display_name, workspace_name,
		stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"peer-a", "pet-conflict", "default", "petdef-a", "Pet", "pet-pet-conflict",
		`{"life":100,"health":100,"satiety":100,"hygiene":100,"mood":100,"energy":100}`, `{"experience":0,"level":1}`, "alive", nil, now, now, now, now,
	); err != nil {
		t.Fatalf("insert Pet: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gameplay_pet_workspace_bindings (owner_public_key, pet_id, runtime_profile_name, workspace_name, created_at) VALUES (?, ?, ?, ?, ?)`,
		"peer-a", "pet-conflict", "other-profile", "other-workspace", now); err != nil {
		t.Fatalf("insert conflicting binding: %v", err)
	}
	if _, err := runtime.DeletePet(ctx, "peer-a", "pet-conflict"); err == nil || !strings.Contains(err.Error(), "binding conflicts") {
		t.Fatalf("DeletePet() error = %v, want binding conflict", err)
	}
	var pets, pending int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pets WHERE owner_public_key = ? AND id = ?`, "peer-a", "pet-conflict").Scan(&pets); err != nil {
		t.Fatalf("count Pets: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM gameplay_pending_deletions WHERE owner_public_key = ? AND resource_id = ?`, "peer-a", "pet-conflict").Scan(&pending); err != nil {
		t.Fatalf("count pending deletions: %v", err)
	}
	if pets != 1 || pending != 0 {
		t.Fatalf("after conflict Pets=%d pending=%d, want 1 and 0", pets, pending)
	}
}

func TestRuntimeAdoptCallerIDSerializesConcurrentRetries(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	profile := seedGameplayCatalog(t, ctx, catalog)
	ctx = WithRuntimeProfile(ctx, profile)
	workspaces := &recordingWorkspaceService{}
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workflows:  petWorkflowService{},
		Workspaces: workspaces,
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("adopt-txn"),
		PickWeight: func(int64) int64 { return 0 },
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	petID := "device-pet-03"
	const workers = 8
	start := make(chan struct{})
	responses := make(chan apitypes.PetAdoptResponse, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			response, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID})
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
	for response := range responses {
		if response.Pet.Id != petID || response.Transaction.Id != "adopt-txn" {
			t.Fatalf("AdoptPet(concurrent) = %#v", response)
		}
	}
	if len(workspaces.created) != 1 {
		t.Fatalf("created workspaces = %d, want 1", len(workspaces.created))
	}
}

func TestRuntimeAdoptCallerIDConvergesAcrossRuntimeInstances(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 10, 45, 0, 0, time.UTC)
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
	db := testDB(t)
	workspaces := &recordingWorkspaceService{}
	newRuntime := func(transactionID string, pickWeight func(int64) int64) *Runtime {
		return &Runtime{
			DB:         db,
			Catalog:    catalog,
			Workflows:  petWorkflowService{},
			Workspaces: workspaces,
			Now:        func() time.Time { return now },
			NewID:      sequentialIDs(transactionID),
			PickWeight: pickWeight,
		}
	}
	runtimes := []*Runtime{
		newRuntime("txn-runtime-a", func(int64) int64 { return 0 }),
		newRuntime("txn-runtime-b", func(total int64) int64 { return total - 1 }),
	}
	if err := runtimes[0].Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	petID := "device-pet-04"
	const workers = 8
	start := make(chan struct{})
	responses := make(chan apitypes.PetAdoptResponse, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		runtime := runtimes[i%len(runtimes)]
		wg.Go(func() {
			<-start
			response, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Id: &petID})
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
			t.Fatalf("AdoptPet(cross-runtime) error = %v", err)
		}
	}
	var transactionID string
	for response := range responses {
		if response.Pet.Id != petID {
			t.Fatalf("AdoptPet(cross-runtime) = %#v", response)
		}
		if transactionID == "" {
			transactionID = response.Transaction.Id
		} else if response.Transaction.Id != transactionID {
			t.Fatalf("transaction ID = %q, want %q", response.Transaction.Id, transactionID)
		}
	}
	if len(workspaces.created) != 1 || len(workspaces.deleted) != 0 {
		t.Fatalf("workspace mutations: created=%d deleted=%d, want 1 and 0", len(workspaces.created), len(workspaces.deleted))
	}
	parameters, err := workspaces.created[0].Parameters.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("decode winning Pet Workspace parameters: %v", err)
	}
	var reservedVoice string
	if err := db.QueryRow(`SELECT voice_alias FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ? AND pet_id = ?`, "peer-a", petID).Scan(&reservedVoice); err != nil {
		t.Fatalf("load adoption reservation voice: %v", err)
	}
	if parameters.Voice.VoiceId != reservedVoice {
		t.Fatalf("winning Pet Workspace voice = %q, want reserved voice %q", parameters.Voice.VoiceId, reservedVoice)
	}
}

func TestRuntimeProfileScopesGameplayLists(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 19, 6, 0, 0, 0, time.UTC)
	db := testDB(t)
	runtime := &Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx() error = %v", err)
	}
	defer tx.Rollback()
	for _, profileName := range []string{"profile-a", "profile-b"} {
		petID := profileName + "-pet"
		if err := insertPet(ctx, tx, apitypes.Pet{
			OwnerPublicKey:     "peer-a",
			Id:                 petID,
			RuntimeProfileName: profileName,
			PetdefId:           "petdef-basic",
			DisplayName:        petID,
			WorkspaceName:      profileName + "-workspace",
			Stats:              initialPetStats(),
			Progression:        initialPetProgression(),
			Lifecycle:          apitypes.PetLifecycleAlive,
			StateSettledAt:     now,
			LastActiveAt:       now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}); err != nil {
			t.Fatalf("insertPet(%s) error = %v", profileName, err)
		}
		if err := insertPointsTransaction(ctx, tx, apitypes.PointsTransaction{
			OwnerPublicKey:     "peer-a",
			Id:                 profileName + "-transaction",
			RuntimeProfileName: profileName,
			PetId:              &petID,
			Reason:             "test",
			SourceType:         "test",
			SourceId:           profileName,
			CreatedAt:          now,
		}); err != nil {
			t.Fatalf("insertPointsTransaction(%s) error = %v", profileName, err)
		}
		if err := insertGameResult(ctx, tx, apitypes.GameResult{
			OwnerPublicKey:     "peer-a",
			Id:                 profileName + "-result",
			RuntimeProfileName: profileName,
			PetId:              petID,
			GameDefId:          "game-basic",
			OccurredAt:         now,
			CreatedAt:          now,
		}); err != nil {
			t.Fatalf("insertGameResult(%s) error = %v", profileName, err)
		}
		if err := insertRewardGrant(ctx, tx, apitypes.RewardGrant{
			OwnerPublicKey:     "peer-a",
			Id:                 profileName + "-grant",
			RuntimeProfileName: profileName,
			PetId:              &petID,
			BadgeExpDelta:      map[string]int64{},
			SourceType:         "test",
			SourceId:           profileName,
			CreatedAt:          now,
		}); err != nil {
			t.Fatalf("insertRewardGrant(%s) error = %v", profileName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	profileCtx := WithRuntimeProfile(ctx, apitypes.RuntimeProfile{Name: "profile-a"})
	pets, err := runtime.ListPets(profileCtx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil || len(pets.Items) != 1 || pets.Items[0].RuntimeProfileName != "profile-a" {
		t.Fatalf("ListPets(profile-a) = %#v, %v", pets, err)
	}
	transactions, err := runtime.ListPointsTransactions(profileCtx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil || len(transactions.Items) != 1 || transactions.Items[0].RuntimeProfileName != "profile-a" {
		t.Fatalf("ListPointsTransactions(profile-a) = %#v, %v", transactions, err)
	}
	results, err := runtime.ListGameResults(profileCtx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil || len(results.Items) != 1 || results.Items[0].RuntimeProfileName != "profile-a" {
		t.Fatalf("ListGameResults(profile-a) = %#v, %v", results, err)
	}
	grants, err := runtime.ListRewardGrants(profileCtx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil || len(grants.Items) != 1 || grants.Items[0].RuntimeProfileName != "profile-a" {
		t.Fatalf("ListRewardGrants(profile-a) = %#v, %v", grants, err)
	}
	if _, err := runtime.GetPet(profileCtx, "peer-a", "profile-b-pet"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetPet(cross-profile) error = %v, want not found", err)
	}
	if _, err := runtime.PutPet(profileCtx, "peer-a", apitypes.PetPutRequest{Id: "profile-b-pet", DisplayName: "renamed"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("PutPet(cross-profile) error = %v, want not found", err)
	}
	if _, err := runtime.DeletePet(profileCtx, "peer-a", "profile-b-pet"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("DeletePet(cross-profile) error = %v, want not found", err)
	}
	if _, err := runtime.GetPointsTransaction(profileCtx, "peer-a", "profile-b-transaction"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetPointsTransaction(cross-profile) error = %v, want not found", err)
	}
	if _, err := runtime.GetGameResult(profileCtx, "peer-a", "profile-b-result"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetGameResult(cross-profile) error = %v, want not found", err)
	}
	if _, err := runtime.GetRewardGrant(profileCtx, "peer-a", "profile-b-grant"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetRewardGrant(cross-profile) error = %v, want not found", err)
	}
	allPets, err := runtime.ListPets(ctx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil || len(allPets.Items) != 2 {
		t.Fatalf("ListPets(admin owner view) = %#v, %v", allPets, err)
	}
	allowed, err := runtime.OwnerHasPetWorkspace(profileCtx, "peer-a", "profile-a-workspace")
	if err != nil || !allowed {
		t.Fatalf("OwnerHasPetWorkspace(profile-a) = %v, %v", allowed, err)
	}
	allowed, err = runtime.OwnerHasPetWorkspace(profileCtx, "peer-a", "profile-b-workspace")
	if err != nil || allowed {
		t.Fatalf("OwnerHasPetWorkspace(cross-profile) = %v, %v", allowed, err)
	}
	allowed, err = runtime.OwnerHasPetWorkspace(ctx, "peer-a", "profile-a-workspace")
	if err != nil || allowed {
		t.Fatalf("OwnerHasPetWorkspace(without profile) = %v, %v", allowed, err)
	}
}

func TestResolvePetContextRequiresExactlyOneWorkspaceBinding(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	db := testDB(t)
	catalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, catalog)
	runtime := &Runtime{DB: db, Catalog: catalog}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if _, _, err := runtime.ResolvePetContext(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) || !errors.Is(err, errPetWorkspaceNotFound) {
		t.Fatalf("ResolvePetContext(missing) error = %v, want sql.ErrNoRows and errPetWorkspaceNotFound", err)
	}
	insert := func(owner, id string) {
		t.Helper()
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx() error = %v", err)
		}
		defer tx.Rollback()
		if err := insertPet(ctx, tx, apitypes.Pet{
			OwnerPublicKey:     owner,
			Id:                 id,
			RuntimeProfileName: "default",
			PetdefId:           "petdef-basic",
			DisplayName:        id,
			WorkspaceName:      "pet-shared",
			Stats:              initialPetStats(),
			Progression:        initialPetProgression(),
			Lifecycle:          apitypes.PetLifecycleAlive,
			StateSettledAt:     now,
			LastActiveAt:       now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}); err != nil {
			t.Fatalf("insertPet() error = %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
	}
	insert("peer-a", "pet-a")
	pet, petDef, err := runtime.ResolvePetContext(ctx, "pet-shared")
	if err != nil {
		t.Fatalf("ResolvePetContext() error = %v", err)
	}
	if pet.Id != "pet-a" || petDef.Id != "petdef-basic" {
		t.Fatalf("ResolvePetContext() = %#v, %#v", pet, petDef)
	}
	insert("peer-b", "pet-b")
	if _, _, err := runtime.ResolvePetContext(ctx, "pet-shared"); !errors.Is(err, errPetWorkspaceAmbiguous) {
		t.Fatalf("ResolvePetContext(ambiguous) error = %v, want errPetWorkspaceAmbiguous", err)
	}
}

func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sequentialIDs(ids ...string) func() string {
	var i int
	return func() string {
		if i >= len(ids) {
			return fmt.Sprintf("extra-%d", i)
		}
		id := ids[i]
		i++
		return id
	}
}

type recordingWorkspaceService struct {
	mu        sync.Mutex
	created   []adminhttp.WorkspaceUpsert
	deleted   []string
	deleteErr error
}

func (s *recordingWorkspaceService) CreateSystemWorkspace(_ context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.created {
		if existing.Name == body.Name {
			system := true
			return apitypes.Workspace{Name: existing.Name, WorkflowName: existing.WorkflowName, Parameters: existing.Parameters, System: &system}, false, nil
		}
	}
	s.created = append(s.created, body)
	system := true
	return apitypes.Workspace{Name: body.Name, WorkflowName: body.WorkflowName, Parameters: body.Parameters, System: &system}, true, nil
}

func (s *recordingWorkspaceService) DeleteSystemWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return apitypes.Workspace{}, s.deleteErr
	}
	s.deleted = append(s.deleted, name)
	for _, existing := range s.created {
		if existing.Name == name {
			system := true
			return apitypes.Workspace{
				Labels:       existing.Labels,
				Name:         existing.Name,
				Parameters:   existing.Parameters,
				System:       &system,
				Toolkit:      existing.Toolkit,
				WorkflowName: existing.WorkflowName,
			}, nil
		}
	}
	return apitypes.Workspace{Name: name}, nil
}

type petWorkflowService struct {
	driver apitypes.WorkflowDriver
}

func (s petWorkflowService) GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	driver := s.driver
	if driver == "" {
		driver = apitypes.WorkflowDriverPet
	}
	return adminhttp.GetWorkflow200JSONResponse(apitypes.Workflow{
		Spec: apitypes.WorkflowSpec{Driver: driver},
	}), nil
}

func (s *recordingWorkspaceService) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	return adminhttp.ListWorkspaces200JSONResponse(adminhttp.WorkspaceList{}), nil
}

func (s *recordingWorkspaceService) CreateWorkspace(_ context.Context, req adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	if req.Body == nil {
		return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	s.created = append(s.created, *req.Body)
	return adminhttp.CreateWorkspace200JSONResponse(apitypes.Workspace{Name: req.Body.Name, WorkflowName: req.Body.WorkflowName}), nil
}

func (s *recordingWorkspaceService) DeleteWorkspace(_ context.Context, req adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	s.deleted = append(s.deleted, req.Name)
	return adminhttp.DeleteWorkspace200JSONResponse(apitypes.Workspace{Name: req.Name}), nil
}

func (s *recordingWorkspaceService) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
}

func (s *recordingWorkspaceService) PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("UNIMPLEMENTED", "not implemented")), nil
}

type workspaceResponseService struct {
	resp adminhttp.CreateWorkspaceResponseObject
}

func (s workspaceResponseService) CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	if response, ok := s.resp.(adminhttp.CreateWorkspace200JSONResponse); ok {
		return apitypes.Workspace(response), true, nil
	}
	return apitypes.Workspace{}, false, fmt.Errorf("create system workspace failed: %T", s.resp)
}

func (s workspaceResponseService) DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, nil
}

func (s workspaceResponseService) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	return adminhttp.ListWorkspaces200JSONResponse(adminhttp.WorkspaceList{}), nil
}

func (s workspaceResponseService) CreateWorkspace(context.Context, adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	return s.resp, nil
}

func (s workspaceResponseService) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return adminhttp.DeleteWorkspace200JSONResponse(apitypes.Workspace{}), nil
}

func (s workspaceResponseService) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
}

func (s workspaceResponseService) PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("UNIMPLEMENTED", "not implemented")), nil
}
