package gameplay

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	_ "modernc.org/sqlite"
)

func TestRuntimeAdoptAndDrive(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, catalog)
	db := testDB(t)
	workspaces := &recordingWorkspaceService{}
	ids := sequentialIDs("pet-1", "adopt-txn", "drive-cost-txn", "game-result-1", "grant-1", "reward-txn")
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
		Workspaces: workspaces,
		Now: func() time.Time {
			return now
		},
		NewID: ids,
		PickWeight: func(total int64) int64 {
			if total != 10 {
				t.Fatalf("pick total = %d, want 10", total)
			}
			return 0
		},
	}

	adopted, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	if adopted.Pet.Id != "pet-1" || adopted.Pet.PetdefId != "petdef-basic" {
		t.Fatalf("adopted pet = %#v", adopted.Pet)
	}
	if adopted.Pet.DisplayName != "Spark" || adopted.Pet.WorkspaceName != "pet-pet-1" || valueOrZero(adopted.Pet.WorkflowName) != "pet-chat" {
		t.Fatalf("adopted pet display/workspace = %#v", adopted.Pet)
	}
	if got := workspaces.created; len(got) != 1 || got[0].Name != "pet-pet-1" || got[0].WorkflowName != "pet-chat" {
		t.Fatalf("created workspaces = %#v", got)
	}
	if adopted.Points.Balance != 35 {
		t.Fatalf("adopted points balance = %d, want 35", adopted.Points.Balance)
	}
	if adopted.Transaction.Id != "adopt-txn" || adopted.Transaction.Delta != -15 || adopted.Transaction.BalanceAfter != 35 {
		t.Fatalf("adopt transaction = %#v", adopted.Transaction)
	}

	now = now.Add(2 * time.Hour)
	score := int64(321)
	outcome := "win"
	drive, err := runtime.DrivePet(ctx, "peer-a", apitypes.PetDriveRequest{
		PetId:  adopted.Pet.Id,
		Action: stringPtr("bath"),
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId: "game-basic",
			Score:     &score,
			Outcome:   &outcome,
		},
	})
	if err != nil {
		t.Fatalf("DrivePet() error = %v", err)
	}
	if drive.Pet.Exp != 110 || drive.Pet.Level != 2 {
		t.Fatalf("pet exp/level = %d/%d, want 110/2", drive.Pet.Exp, drive.Pet.Level)
	}
	if drive.Pet.Life["hunger"] != 90 || drive.Pet.Life["clean"] != 110 {
		t.Fatalf("pet life = %#v", drive.Pet.Life)
	}
	if drive.Points.Balance != 55 {
		t.Fatalf("points balance = %d, want 55", drive.Points.Balance)
	}
	if drive.GameResult == nil || drive.GameResult.Id != "game-result-1" || drive.GameResult.GameDefId != "game-basic" || valueOrZero(drive.GameResult.Score) != 321 {
		t.Fatalf("game result = %#v", drive.GameResult)
	}
	if len(drive.RewardGrants) != 1 || drive.RewardGrants[0].Id != "grant-1" || drive.RewardGrants[0].BadgeExpDelta["badge-basic"] != 100 {
		t.Fatalf("reward grants = %#v", drive.RewardGrants)
	}
	if len(drive.Badges) != 1 || !drive.Badges[0].Active || drive.Badges[0].Level != 1 || drive.Badges[0].Progress != 0 {
		t.Fatalf("badges = %#v", drive.Badges)
	}
	if len(drive.Transactions) != 2 {
		t.Fatalf("transactions = %#v", drive.Transactions)
	}
	if drive.Transactions[0].Delta != -10 || drive.Transactions[1].Delta != 30 || drive.Transactions[1].BalanceAfter != 55 {
		t.Fatalf("transactions = %#v", drive.Transactions)
	}

	ruleset, err := runtime.GetGameRuleset(ctx, "default")
	if err != nil {
		t.Fatalf("GetGameRuleset() error = %v", err)
	}
	if ruleset.Name != "default" {
		t.Fatalf("GetGameRuleset() = %#v", ruleset)
	}
	petList, err := runtime.ListPets(ctx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil {
		t.Fatalf("ListPets() error = %v", err)
	}
	if len(petList.Items) != 1 || petList.Items[0].Id != adopted.Pet.Id {
		t.Fatalf("ListPets() = %#v", petList)
	}
	renamed, err := runtime.PutPet(ctx, "peer-a", apitypes.PetPutRequest{Id: adopted.Pet.Id, DisplayName: "Renamed"})
	if err != nil {
		t.Fatalf("PutPet() error = %v", err)
	}
	if renamed.DisplayName != "Renamed" {
		t.Fatalf("PutPet() = %#v", renamed)
	}
	points, err := runtime.GetPoints(ctx, "peer-a", "default")
	if err != nil {
		t.Fatalf("GetPoints() error = %v", err)
	}
	if points.Balance != 55 {
		t.Fatalf("GetPoints() = %#v", points)
	}
	txnList, err := runtime.ListPointsTransactions(ctx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil {
		t.Fatalf("ListPointsTransactions() error = %v", err)
	}
	if len(txnList.Items) != 3 {
		t.Fatalf("ListPointsTransactions() = %#v", txnList)
	}
	if got, err := runtime.GetPointsTransaction(ctx, "peer-a", drive.Transactions[1].Id); err != nil || got.Id != drive.Transactions[1].Id {
		t.Fatalf("GetPointsTransaction() = %#v, %v", got, err)
	}
	badgeList, err := runtime.ListBadges(ctx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil {
		t.Fatalf("ListBadges() error = %v", err)
	}
	if len(badgeList.Items) != 1 || badgeList.Items[0].Id != "badge-basic" {
		t.Fatalf("ListBadges() = %#v", badgeList)
	}
	if got, err := runtime.GetBadge(ctx, "peer-a", "badge-basic"); err != nil || got.Id != "badge-basic" {
		t.Fatalf("GetBadge() = %#v, %v", got, err)
	}
	resultList, err := runtime.ListGameResults(ctx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil {
		t.Fatalf("ListGameResults() error = %v", err)
	}
	if len(resultList.Items) != 1 || resultList.Items[0].Id != "game-result-1" {
		t.Fatalf("ListGameResults() = %#v", resultList)
	}
	if got, err := runtime.GetGameResult(ctx, "peer-a", "game-result-1"); err != nil || got.Id != "game-result-1" {
		t.Fatalf("GetGameResult() = %#v, %v", got, err)
	}
	grantList, err := runtime.ListRewardGrants(ctx, "peer-a", apitypes.GameplayListRequest{})
	if err != nil {
		t.Fatalf("ListRewardGrants() error = %v", err)
	}
	if len(grantList.Items) != 1 || grantList.Items[0].Id != "grant-1" {
		t.Fatalf("ListRewardGrants() = %#v", grantList)
	}
	if got, err := runtime.GetRewardGrant(ctx, "peer-a", "grant-1"); err != nil || got.Id != "grant-1" {
		t.Fatalf("GetRewardGrant() = %#v, %v", got, err)
	}
	deleted, err := runtime.DeletePet(ctx, "peer-a", adopted.Pet.Id)
	if err != nil {
		t.Fatalf("DeletePet() error = %v", err)
	}
	if deleted.Id != adopted.Pet.Id || len(workspaces.deleted) != 1 || workspaces.deleted[0] != "pet-pet-1" {
		t.Fatalf("DeletePet() = %#v deletedWorkspaces=%#v", deleted, workspaces.deleted)
	}
}

func TestRuntimeAdoptCompensatesWorkspaceOnSQLError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, catalog)
	db := testDB(t)
	workspaces := &recordingWorkspaceService{}
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
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
	if len(workspaces.deleted) != 1 || workspaces.deleted[0] != "pet-same-id" {
		t.Fatalf("deleted workspaces = %#v", workspaces.deleted)
	}
}

func TestRuntimeErrorsPaginationAndTimeDrive(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, catalog)
	db := testDB(t)
	runtime := &Runtime{
		DB:         db,
		Catalog:    catalog,
		Workspaces: &recordingWorkspaceService{},
		Now: func() time.Time {
			return now
		},
		NewID:      sequentialIDs("pet-1", "adopt-txn-1", "pet-2", "adopt-txn-2"),
		PickWeight: func(int64) int64 { return 999 },
	}

	if err := (&Runtime{}).Migration(ctx); err == nil {
		t.Fatal("Migration() without db should fail")
	}
	if _, err := runtime.AdoptPet(ctx, "", apitypes.PetAdoptRequest{}); err == nil {
		t.Fatal("AdoptPet() without owner should fail")
	}
	noWorkspace := *runtime
	noWorkspace.Workspaces = nil
	noWorkspace.NewID = sequentialIDs("no-workspace-pet")
	if _, err := noWorkspace.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{}); err == nil {
		t.Fatal("AdoptPet() without workspace service should fail")
	}

	first, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("first AdoptPet() error = %v", err)
	}
	second, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("second AdoptPet() error = %v", err)
	}
	if first.Pet.Id != "pet-1" || second.Pet.Id != "pet-2" {
		t.Fatalf("adopted pets = %#v %#v", first.Pet, second.Pet)
	}

	limit := 1
	page1, err := runtime.ListPets(ctx, "peer-a", apitypes.GameplayListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("ListPets() page1 error = %v", err)
	}
	if len(page1.Items) != 1 || !page1.HasNext || page1.NextCursor == nil || *page1.NextCursor != "pet-1" {
		t.Fatalf("ListPets() page1 = %#v", page1)
	}
	page2, err := runtime.ListPets(ctx, "peer-a", apitypes.GameplayListRequest{Limit: &limit, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("ListPets() page2 error = %v", err)
	}
	if len(page2.Items) != 1 || page2.HasNext || page2.Items[0].Id != "pet-2" {
		t.Fatalf("ListPets() page2 = %#v", page2)
	}

	now = now.Add(3 * time.Hour)
	timeDrive, err := runtime.DrivePet(ctx, "peer-a", apitypes.PetDriveRequest{PetId: first.Pet.Id})
	if err != nil {
		t.Fatalf("DrivePet() time drive error = %v", err)
	}
	if timeDrive.Pet.Life["hunger"] != 85 || len(timeDrive.Transactions) != 0 || len(timeDrive.RewardGrants) != 0 {
		t.Fatalf("time drive = %#v", timeDrive)
	}

	if _, err := runtime.DrivePet(ctx, "peer-a", apitypes.PetDriveRequest{
		PetId:      first.Pet.Id,
		GameResult: &apitypes.PetDriveGameResultInput{GameDefId: "missing-game"},
	}); err == nil {
		t.Fatal("DrivePet() with game outside ruleset should fail")
	}
	if _, err := runtime.PutPet(ctx, "peer-a", apitypes.PetPutRequest{Id: first.Pet.Id, DisplayName: "  "}); err == nil {
		t.Fatal("PutPet() with blank display name should fail")
	}

	poorCatalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, poorCatalog)
	zero := int64(0)
	cost := int64(99)
	_, err = poorCatalog.PutGameRuleset(ctx, adminservice.PutGameRulesetRequestObject{
		Name: "default",
		Body: &adminservice.GameRulesetUpsert{
			Spec: apitypes.GameRulesetSpec{
				Enabled: true,
				Points:  &apitypes.GameRulesetPointsSpec{InitialBalance: &zero},
				PetPool: []apitypes.GameRulesetPetPoolEntry{{
					PetdefId:     "petdef-basic",
					Weight:       1,
					AdoptionCost: &cost,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("PutGameRuleset() error = %v", err)
	}
	poorRuntime := &Runtime{
		DB:          testDB(t),
		Catalog:     poorCatalog,
		Workspaces:  &recordingWorkspaceService{},
		Now:         func() time.Time { return now },
		NewID:       sequentialIDs("poor-pet", "poor-txn"),
		PickWeight:  func(int64) int64 { return -1 },
		DecayPeriod: 30 * time.Minute,
	}
	if _, err := poorRuntime.AdoptPet(ctx, "peer-poor", apitypes.PetAdoptRequest{}); err == nil {
		t.Fatal("AdoptPet() with insufficient points should fail")
	}
	if _, err := poorRuntime.pickPetDef([]apitypes.GameRulesetPetPoolEntry{{PetdefId: "petdef-basic", Weight: 0}}); err == nil {
		t.Fatal("pickPetDef() with no positive weight should fail")
	}
}

func TestRuntimeHelperBranches(t *testing.T) {
	if got := (&Runtime{PickWeight: func(int64) int64 { return -5 }}).pickWeight(10); got != 0 {
		t.Fatalf("negative pickWeight = %d", got)
	}
	if got := (&Runtime{PickWeight: func(int64) int64 { return 99 }}).pickWeight(10); got != 9 {
		t.Fatalf("large pickWeight = %d", got)
	}
	if got := (&Runtime{}).pickWeight(1); got != 0 {
		t.Fatalf("default pickWeight = %d", got)
	}
	if got := selectedWorkflow(apitypes.GameRuleset{}, apitypes.PetDef{}, apitypes.GameRulesetPetPoolEntry{}); got != defaultPetWorkflowName {
		t.Fatalf("selectedWorkflow default = %q", got)
	}
	if got := selectedWorkflow(apitypes.GameRuleset{}, apitypes.PetDef{}, apitypes.GameRulesetPetPoolEntry{WorkflowName: stringPtr(" pool ")}); got != "pool" {
		t.Fatalf("selectedWorkflow pool = %q", got)
	}
	if got := petLevel(-100); got != 1 {
		t.Fatalf("petLevel(-100) = %d", got)
	}

	life := apitypes.StatMap{"hunger": 1}
	applyStatDelta(life, &apitypes.StatMap{"hunger": -5, "clean": 3})
	if life["hunger"] != 0 || life["clean"] != 3 {
		t.Fatalf("applyStatDelta() = %#v", life)
	}
	applyStatDelta(nil, &apitypes.StatMap{"hunger": 1})

	result := apitypes.GameResult{GameDefId: "game-a"}
	if got := rewardReason("", nil); got != "time" {
		t.Fatalf("rewardReason time = %q", got)
	}
	if got := rewardReason("bath", nil); got != "action.bath" {
		t.Fatalf("rewardReason action = %q", got)
	}
	if got := rewardReason("bath", &result); got != "game_result.game-a" {
		t.Fatalf("rewardReason result = %q", got)
	}

	for _, tc := range []struct {
		item any
		want string
	}{
		{apitypes.Pet{Id: "pet-a"}, "pet-a"},
		{apitypes.Badge{Id: "badge-a"}, "badge-a"},
		{apitypes.PointsTransaction{Id: "txn-a"}, "txn-a"},
		{apitypes.GameResult{Id: "result-a"}, "result-a"},
		{apitypes.RewardGrant{Id: "grant-a"}, "grant-a"},
		{struct{}{}, ""},
	} {
		if got := runtimeItemID(tc.item); got != tc.want {
			t.Fatalf("runtimeItemID(%T) = %q, want %q", tc.item, got, tc.want)
		}
	}

	var decoded map[string]int64
	if err := unmarshalJSON("", &decoded); err != nil || len(decoded) != 0 {
		t.Fatalf("unmarshalJSON empty = %#v, %v", decoded, err)
	}
	if nullableInt64(nil).Valid {
		t.Fatal("nullableInt64(nil) should be invalid")
	}
	if !nullableInt64(int64Ptr(7)).Valid {
		t.Fatal("nullableInt64(7) should be valid")
	}
	if boolInt(false) != 0 || boolInt(true) != 1 {
		t.Fatal("boolInt() returned unexpected values")
	}

	for _, resp := range []adminservice.CreateWorkspaceResponseObject{
		adminservice.CreateWorkspace400JSONResponse{Error: apitypes.NewErrorResponse("BAD", "bad request").Error},
		adminservice.CreateWorkspace409JSONResponse{Error: apitypes.NewErrorResponse("CONFLICT", "conflict").Error},
		adminservice.CreateWorkspace500JSONResponse{Error: apitypes.NewErrorResponse("ERROR", "server error").Error},
		nil,
	} {
		runtime := &Runtime{Workspaces: workspaceResponseService{resp: resp}}
		if err := runtime.createPetWorkspace(context.Background(), "pet-a", "chatroom"); err == nil {
			t.Fatalf("createPetWorkspace(%T) should fail", resp)
		}
	}
}

func testCatalog(t *testing.T, now time.Time) *Catalog {
	t.Helper()
	return &Catalog{
		GameRulesets: kv.NewMemory(nil),
		PetDefs:      kv.NewMemory(nil),
		BadgeDefs:    kv.NewMemory(nil),
		GameDefs:     kv.NewMemory(nil),
		Now: func() time.Time {
			return now
		},
	}
}

func seedGameplayCatalog(t *testing.T, ctx context.Context, catalog *Catalog) {
	t.Helper()
	life := apitypes.StatMap{"hunger": 100, "clean": 100}
	ability := apitypes.StatMap{"play": 1}
	petResp, err := catalog.CreatePetDef(ctx, adminservice.CreatePetDefRequestObject{
		Body: &adminservice.PetDefUpsert{
			Id: "petdef-basic",
			Spec: apitypes.PetDefSpec{
				DisplayName:    "Spark",
				WorkflowName:   stringPtr("pet-chat"),
				InitialLife:    &life,
				InitialAbility: &ability,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	if _, ok := petResp.(adminservice.CreatePetDef200JSONResponse); !ok {
		t.Fatalf("CreatePetDef() response = %#v", petResp)
	}
	badgeResp, err := catalog.CreateBadgeDef(ctx, adminservice.CreateBadgeDefRequestObject{
		Body: &adminservice.BadgeDefUpsert{Id: "badge-basic", Spec: apitypes.BadgeDefSpec{DisplayName: "First Win"}},
	})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	if _, ok := badgeResp.(adminservice.CreateBadgeDef200JSONResponse); !ok {
		t.Fatalf("CreateBadgeDef() response = %#v", badgeResp)
	}
	gameResp, err := catalog.CreateGameDef(ctx, adminservice.CreateGameDefRequestObject{
		Body: &adminservice.GameDefUpsert{Id: "game-basic", Spec: apitypes.GameDefSpec{DisplayName: "Puzzle"}},
	})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	if _, ok := gameResp.(adminservice.CreateGameDef200JSONResponse); !ok {
		t.Fatalf("CreateGameDef() response = %#v", gameResp)
	}
	initialBalance := int64(50)
	adoptionCost := int64(15)
	actionCosts := map[string]int64{"bath": 10}
	petExp := int64(90)
	points := int64(30)
	gameExp := int64(20)
	cleanDelta := apitypes.StatMap{"clean": 10}
	decay := apitypes.StatMap{"hunger": 5}
	badgeDelta := map[string]int64{"badge-basic": 100}
	gameIDs := []string{"game-basic"}
	rulesetResp, err := catalog.CreateGameRuleset(ctx, adminservice.CreateGameRulesetRequestObject{
		Body: &adminservice.GameRulesetUpsert{
			Name: "default",
			Spec: apitypes.GameRulesetSpec{
				Enabled: true,
				Points:  &apitypes.GameRulesetPointsSpec{InitialBalance: &initialBalance},
				PetPool: []apitypes.GameRulesetPetPoolEntry{{
					PetdefId:     "petdef-basic",
					Weight:       10,
					AdoptionCost: &adoptionCost,
				}},
				GameDefIds: &gameIDs,
				Drive: &apitypes.GameRulesetDriveSpec{
					ActionCosts:      &actionCosts,
					LifeDecayPerHour: &decay,
					ActionRewards: &map[string]apitypes.GameRewardSpec{
						"bath": {PetExpDelta: &petExp, LifeDelta: &cleanDelta},
					},
					GameRewards: &map[string]apitypes.GameRewardSpec{
						"game-basic": {PointsDelta: &points, PetExpDelta: &gameExp, BadgeExpDelta: &badgeDelta},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateGameRuleset() error = %v", err)
	}
	if _, ok := rulesetResp.(adminservice.CreateGameRuleset200JSONResponse); !ok {
		t.Fatalf("CreateGameRuleset() response = %#v", rulesetResp)
	}
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
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
	created []adminservice.WorkspaceUpsert
	deleted []string
}

func (s *recordingWorkspaceService) ListWorkspaces(context.Context, adminservice.ListWorkspacesRequestObject) (adminservice.ListWorkspacesResponseObject, error) {
	return adminservice.ListWorkspaces200JSONResponse(adminservice.WorkspaceList{}), nil
}

func (s *recordingWorkspaceService) CreateWorkspace(_ context.Context, req adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error) {
	if req.Body == nil {
		return adminservice.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	s.created = append(s.created, *req.Body)
	return adminservice.CreateWorkspace200JSONResponse(apitypes.Workspace{Name: req.Body.Name, WorkflowName: req.Body.WorkflowName}), nil
}

func (s *recordingWorkspaceService) DeleteWorkspace(_ context.Context, req adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error) {
	s.deleted = append(s.deleted, req.Name)
	return adminservice.DeleteWorkspace200JSONResponse(apitypes.Workspace{Name: req.Name}), nil
}

func (s *recordingWorkspaceService) GetWorkspace(context.Context, adminservice.GetWorkspaceRequestObject) (adminservice.GetWorkspaceResponseObject, error) {
	return adminservice.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
}

func (s *recordingWorkspaceService) PutWorkspace(context.Context, adminservice.PutWorkspaceRequestObject) (adminservice.PutWorkspaceResponseObject, error) {
	return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("UNIMPLEMENTED", "not implemented")), nil
}

type workspaceResponseService struct {
	resp adminservice.CreateWorkspaceResponseObject
}

func (s workspaceResponseService) ListWorkspaces(context.Context, adminservice.ListWorkspacesRequestObject) (adminservice.ListWorkspacesResponseObject, error) {
	return adminservice.ListWorkspaces200JSONResponse(adminservice.WorkspaceList{}), nil
}

func (s workspaceResponseService) CreateWorkspace(context.Context, adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error) {
	return s.resp, nil
}

func (s workspaceResponseService) DeleteWorkspace(context.Context, adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error) {
	return adminservice.DeleteWorkspace200JSONResponse(apitypes.Workspace{}), nil
}

func (s workspaceResponseService) GetWorkspace(context.Context, adminservice.GetWorkspaceRequestObject) (adminservice.GetWorkspaceResponseObject, error) {
	return adminservice.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
}

func (s workspaceResponseService) PutWorkspace(context.Context, adminservice.PutWorkspaceRequestObject) (adminservice.PutWorkspaceResponseObject, error) {
	return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("UNIMPLEMENTED", "not implemented")), nil
}
