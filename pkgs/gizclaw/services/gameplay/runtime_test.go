package gameplay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

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
	if _, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{}); err == nil {
		t.Fatal("second AdoptPet() should fail")
	}
	if len(workspaces.deleted) != 0 {
		t.Fatalf("deleted workspaces = %#v, want existing workspace preserved", workspaces.deleted)
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
			Life:               apitypes.PetLife{"hunger": 100},
			Progression:        apitypes.PetProgression{"xp": 0},
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
			Life:               apitypes.PetLife{"hunger": 100},
			Progression:        apitypes.PetProgression{"xp": 0},
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
	created   []adminhttp.WorkspaceUpsert
	deleted   []string
	deleteErr error
}

func (s *recordingWorkspaceService) CreateSystemWorkspace(_ context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	for _, existing := range s.created {
		if existing.Name == body.Name {
			system := true
			return apitypes.Workspace{Name: body.Name, WorkflowName: body.WorkflowName, Parameters: body.Parameters, System: &system}, false, nil
		}
	}
	s.created = append(s.created, body)
	system := true
	return apitypes.Workspace{Name: body.Name, WorkflowName: body.WorkflowName, Parameters: body.Parameters, System: &system}, true, nil
}

func (s *recordingWorkspaceService) DeleteSystemWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	if s.deleteErr != nil {
		return apitypes.Workspace{}, s.deleteErr
	}
	s.deleted = append(s.deleted, name)
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
