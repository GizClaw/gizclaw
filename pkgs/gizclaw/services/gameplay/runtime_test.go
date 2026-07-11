package gameplay

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
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
		ACL:        &recordingACLService{},
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
	if workspaces.created[0].Parameters == nil {
		t.Fatalf("created workspace parameters = nil")
	}
	workspaceParams, err := workspaces.created[0].Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatalf("created workspace parameters: %v", err)
	}
	if workspaceParams.Input == nil || *workspaceParams.Input != apitypes.WorkspaceInputModePushToTalk {
		t.Fatalf("created workspace input = %#v, want push-to-talk", workspaceParams.Input)
	}
	if adopted.Points.Balance != 35 {
		t.Fatalf("adopted points balance = %d, want 35", adopted.Points.Balance)
	}
	if adopted.Transaction.Id != "adopt-txn" || adopted.Transaction.Delta != -15 || adopted.Transaction.BalanceAfter != 35 {
		t.Fatalf("adopt transaction = %#v", adopted.Transaction)
	}

	now = now.Add(2 * time.Hour)
	score := int64(321)
	maxScore := int64(500)
	duration := int64(12345)
	occurredAt := now.Add(-5 * time.Minute)
	idempotencyKey := "result-key-1"
	difficulty := "normal"
	outcome := "win"
	payload := apitypes.GameplayMetadata{"round": float64(1)}
	drive, err := runtime.DrivePet(ctx, "peer-a", apitypes.PetDriveRequest{
		PetId:  adopted.Pet.Id,
		Action: stringPtr("bath"),
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId:      "game-basic",
			Score:          &score,
			MaxScore:       &maxScore,
			Difficulty:     &difficulty,
			Outcome:        &outcome,
			DurationMs:     &duration,
			IdempotencyKey: &idempotencyKey,
			Payload:        &payload,
			OccurredAt:     &occurredAt,
		},
	})
	if err != nil {
		t.Fatalf("DrivePet() error = %v", err)
	}
	if drive.Pet.Progression["xp"] != 110 {
		t.Fatalf("pet progression = %#v, want xp=110", drive.Pet.Progression)
	}
	if drive.Pet.Life["hunger"] != 100 || drive.Pet.Life["clean"] != 110 {
		t.Fatalf("pet life = %#v", drive.Pet.Life)
	}
	if drive.Points.Balance != 55 {
		t.Fatalf("points balance = %d, want 55", drive.Points.Balance)
	}
	if drive.GameResult == nil || drive.GameResult.Id != "game-result-1" || drive.GameResult.GameDefId != "game-basic" || valueOrZero(drive.GameResult.Score) != 321 {
		t.Fatalf("game result = %#v", drive.GameResult)
	}
	if valueOrZero(drive.GameResult.MaxScore) != 500 || valueOrZero(drive.GameResult.Difficulty) != "normal" || valueOrZero(drive.GameResult.DurationMs) != 12345 || valueOrZero(drive.GameResult.IdempotencyKey) != "result-key-1" || !drive.GameResult.OccurredAt.Equal(occurredAt) {
		t.Fatalf("game result details = %#v", drive.GameResult)
	}
	if len(drive.RewardGrants) != 1 || drive.RewardGrants[0].Id != "grant-1" || drive.RewardGrants[0].BadgeExpDelta["badge-basic"] != 100 {
		t.Fatalf("reward grants = %#v", drive.RewardGrants)
	}
	if drive.RewardGrants[0].SourceType != "game_result" || drive.RewardGrants[0].SourceId != "game-result-1" || drive.RewardGrants[0].PetExpDelta != 110 {
		t.Fatalf("reward grant details = %#v", drive.RewardGrants[0])
	}
	if len(drive.Badges) != 1 || !drive.Badges[0].Active || drive.Badges[0].Level != 1 || drive.Badges[0].Progress != 0 {
		t.Fatalf("badges = %#v", drive.Badges)
	}
	if ok, err := runtime.OwnerHasPetDef(ctx, "peer-a", "petdef-basic"); err != nil || !ok {
		t.Fatalf("OwnerHasPetDef(peer-a, petdef-basic) = %v, %v", ok, err)
	}
	if ok, err := runtime.OwnerHasPetDef(ctx, "peer-b", "petdef-basic"); err != nil || ok {
		t.Fatalf("OwnerHasPetDef(peer-b, petdef-basic) = %v, %v", ok, err)
	}
	if ok, err := runtime.OwnerHasBadgeDef(ctx, "peer-a", "badge-basic"); err != nil || !ok {
		t.Fatalf("OwnerHasBadgeDef(peer-a, badge-basic) = %v, %v", ok, err)
	}
	if ok, err := runtime.OwnerHasBadgeDef(ctx, "peer-b", "badge-basic"); err != nil || ok {
		t.Fatalf("OwnerHasBadgeDef(peer-b, badge-basic) = %v, %v", ok, err)
	}
	if len(drive.Transactions) != 2 {
		t.Fatalf("transactions = %#v", drive.Transactions)
	}
	if drive.Transactions[0].Delta != -10 || drive.Transactions[1].Delta != 30 || drive.Transactions[1].BalanceAfter != 55 {
		t.Fatalf("transactions = %#v", drive.Transactions)
	}
	if drive.Transactions[0].SourceType != "pet_action" || drive.Transactions[0].SourceId != "bath" || drive.Transactions[1].SourceType != "reward_grant" || drive.Transactions[1].SourceId != "grant-1" {
		t.Fatalf("transaction sources = %#v", drive.Transactions)
	}
	if _, err := runtime.DrivePet(ctx, "peer-a", apitypes.PetDriveRequest{
		PetId: adopted.Pet.Id,
		GameResult: &apitypes.PetDriveGameResultInput{
			GameDefId:      "game-basic",
			IdempotencyKey: &idempotencyKey,
		},
	}); err == nil {
		t.Fatal("duplicate game result idempotency key should fail")
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

func TestRuntimeAdoptGrantsAndDeleteRevokesPetWorkspace(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, catalog)
	workspaces := &recordingWorkspaceService{}
	acl := &recordingACLService{}
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workspaces: workspaces,
		ACL:        acl,
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("pet-1", "adopt-txn"),
	}

	adopted, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	bindingID := petWorkspaceACLBindingID(adopted.Pet.WorkspaceName, "peer-a")
	policy, ok := acl.bindings[bindingID]
	if !ok {
		t.Fatalf("workspace ACL binding %q was not created: %#v", bindingID, acl.bindings)
	}
	if policy.Subject.Kind != apitypes.ACLSubjectKindPk || policy.Subject.Id != "peer-a" || policy.Resource.Kind != apitypes.ACLResourceKindWorkspace || policy.Resource.Id != adopted.Pet.WorkspaceName {
		t.Fatalf("workspace ACL policy = %#v", policy)
	}

	if _, err := runtime.DeletePet(ctx, "peer-a", adopted.Pet.Id); err != nil {
		t.Fatalf("DeletePet() error = %v", err)
	}
	if _, ok := acl.bindings[bindingID]; ok {
		t.Fatalf("workspace ACL binding %q was not revoked", bindingID)
	}
	if got := workspaces.deleted; len(got) != 1 || got[0] != adopted.Pet.WorkspaceName {
		t.Fatalf("deleted workspaces = %#v", got)
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
	if timeDrive.Pet.Life["hunger"] != 100 || len(timeDrive.Transactions) != 0 || len(timeDrive.RewardGrants) != 0 {
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
	_, err = poorCatalog.PutGameRuleset(ctx, adminhttp.PutGameRulesetRequestObject{
		Name: "default",
		Body: &adminhttp.GameRulesetUpsert{
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

func TestScanPetIgnoresLegacyAbilityStats(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	runtime := &Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	_, err := db.ExecContext(ctx, `INSERT INTO gameplay_pets (owner_public_key, id, ruleset_name, petdef_id, display_name, workspace_name, workflow_name, life_json, ability_json, exp, level, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"peer-a", "pet-a", "default", "petdef-a", "Pet A", "pet-pet-a", "pet-chat", `{"hunger":100}`, `{"play":1}`, int64(42), int64(1), formatTime(now), formatTime(now), formatTime(now))
	if err != nil {
		t.Fatalf("insert legacy pet error = %v", err)
	}
	pet, err := scanPet(db.QueryRowContext(ctx, petSelectSQL()+` WHERE id = ?`, "pet-a"))
	if err != nil {
		t.Fatalf("scanPet() error = %v", err)
	}
	if _, ok := pet.Progression["play"]; ok {
		t.Fatalf("legacy ability stat leaked into progression: %#v", pet.Progression)
	}
	if got := pet.Progression["xp"]; got != 42 {
		t.Fatalf("progression xp = %d, want 42", got)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO gameplay_pets (owner_public_key, id, ruleset_name, petdef_id, display_name, workspace_name, workflow_name, life_json, ability_json, exp, level, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"peer-a", "pet-c", "default", "petdef-a", "Pet C", "pet-pet-c", "pet-chat", `{"hunger":100}`, `{"xp":1,"play":1}`, int64(42), int64(1), formatTime(now), formatTime(now), formatTime(now))
	if err != nil {
		t.Fatalf("insert legacy xp ability pet error = %v", err)
	}
	pet, err = scanPet(db.QueryRowContext(ctx, petSelectSQL()+` WHERE id = ?`, "pet-c"))
	if err != nil {
		t.Fatalf("scanPet() legacy xp ability error = %v", err)
	}
	if _, ok := pet.Progression["play"]; ok {
		t.Fatalf("legacy xp ability stat leaked into progression: %#v", pet.Progression)
	}
	if got := pet.Progression["xp"]; got != 42 {
		t.Fatalf("legacy xp ability progression xp = %d, want 42", got)
	}

	progressionJSON, err := marshalStoredPetProgression(apitypes.PetProgression{"xp": 7, "rank": 2})
	if err != nil {
		t.Fatalf("marshalStoredPetProgression() error = %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO gameplay_pets (owner_public_key, id, ruleset_name, petdef_id, display_name, workspace_name, workflow_name, life_json, ability_json, exp, level, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"peer-a", "pet-b", "default", "petdef-a", "Pet B", "pet-pet-b", "pet-chat", `{"hunger":100}`, progressionJSON, int64(7), int64(1), formatTime(now), formatTime(now), formatTime(now))
	if err != nil {
		t.Fatalf("insert progression pet error = %v", err)
	}
	pet, err = scanPet(db.QueryRowContext(ctx, petSelectSQL()+` WHERE id = ?`, "pet-b"))
	if err != nil {
		t.Fatalf("scanPet() progression error = %v", err)
	}
	if pet.Progression["xp"] != 7 || pet.Progression["rank"] != 2 {
		t.Fatalf("stored progression not preserved: %#v", pet.Progression)
	}
}

func TestRuntimeDrivesLegacyRulesetActionFallback(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	petStore, err := catalog.store(catalog.PetDefs, "pet defs")
	if err != nil {
		t.Fatalf("pet store error = %v", err)
	}
	rulesetStore, err := catalog.store(catalog.GameRulesets, "game rulesets")
	if err != nil {
		t.Fatalf("ruleset store error = %v", err)
	}
	if err := petStore.Set(ctx, petDefKey("legacy-pet"), []byte(`{
		"id":"legacy-pet",
		"spec":{
			"display_name":"Legacy Pet",
			"description":"Legacy description",
			"workflow_name":"pet-chat",
			"initial_life":{"hunger":100},
			"initial_ability":{"play":1}
		},
		"created_at":"2026-07-05T11:00:00Z",
		"updated_at":"2026-07-05T11:00:00Z"
	}`)); err != nil {
		t.Fatalf("seed legacy petdef: %v", err)
	}
	if err := rulesetStore.Set(ctx, rulesetKey("default"), []byte(`{
		"name":"default",
		"spec":{
			"enabled":true,
			"points":{"initial_balance":50},
				"pet_pool":[{"petdef_id":"legacy-pet","weight":1}],
				"drive":{
					"action_costs":{"idle":3},
					"action_rewards":{"idle":{"points_delta":4,"pet_exp_delta":5,"life_delta":{"hunger":10}}}
				}
		},
		"created_at":"2026-07-05T11:00:00Z",
		"updated_at":"2026-07-05T11:00:00Z"
	}`)); err != nil {
		t.Fatalf("seed legacy ruleset: %v", err)
	}
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workspaces: &recordingWorkspaceService{},
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("pet-1", "adopt-txn", "idle-cost-txn", "grant-1", "reward-txn"),
		PickWeight: func(int64) int64 { return 0 },
	}
	adopted, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	drive, err := runtime.DrivePet(ctx, "peer-a", apitypes.PetDriveRequest{PetId: adopted.Pet.Id, Action: stringPtr("idle")})
	if err != nil {
		t.Fatalf("DrivePet() legacy action error = %v", err)
	}
	if drive.Pet.Life["hunger"] != 110 || drive.Pet.Progression["xp"] != 5 {
		t.Fatalf("legacy action pet = %#v", drive.Pet)
	}
	if drive.Points.Balance != 51 {
		t.Fatalf("legacy action points = %d, want 51", drive.Points.Balance)
	}
	if len(drive.Transactions) != 2 || drive.Transactions[0].Delta != -3 || drive.Transactions[1].Delta != 4 {
		t.Fatalf("legacy action transactions = %#v", drive.Transactions)
	}
	if len(drive.RewardGrants) != 1 || drive.RewardGrants[0].PetExpDelta != 5 || drive.RewardGrants[0].PointsDelta != 4 {
		t.Fatalf("legacy action grants = %#v", drive.RewardGrants)
	}
}

func TestRuntimeDoesNotApplyLegacyRulesetActionWhenPetDefDefinesNoop(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	seedGameplayCatalog(t, ctx, catalog)
	rulesetStore, err := catalog.store(catalog.GameRulesets, "game rulesets")
	if err != nil {
		t.Fatalf("ruleset store error = %v", err)
	}
	if err := rulesetStore.Set(ctx, rulesetKey("default"), []byte(`{
		"name":"default",
		"spec":{
			"enabled":true,
			"points":{"initial_balance":50},
			"pet_pool":[{"petdef_id":"petdef-basic","weight":1}],
			"drive":{
				"action_costs":{"idle":3},
				"action_rewards":{"idle":{"points_delta":4,"pet_exp_delta":5,"life_delta":{"hunger":10}}}
			}
		},
		"created_at":"2026-07-05T11:00:00Z",
		"updated_at":"2026-07-05T11:00:00Z"
	}`)); err != nil {
		t.Fatalf("seed legacy ruleset: %v", err)
	}
	runtime := &Runtime{
		DB:         testDB(t),
		Catalog:    catalog,
		Workspaces: &recordingWorkspaceService{},
		Now:        func() time.Time { return now },
		NewID:      sequentialIDs("pet-1", "adopt-txn"),
		PickWeight: func(int64) int64 { return 0 },
	}
	adopted, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	drive, err := runtime.DrivePet(ctx, "peer-a", apitypes.PetDriveRequest{PetId: adopted.Pet.Id, Action: stringPtr("idle")})
	if err != nil {
		t.Fatalf("DrivePet() noop action error = %v", err)
	}
	if drive.Pet.Life["hunger"] != 100 || drive.Pet.Progression["xp"] != 0 {
		t.Fatalf("noop action pet = %#v", drive.Pet)
	}
	if drive.Points.Balance != 50 {
		t.Fatalf("noop action points = %d, want 50", drive.Points.Balance)
	}
	if len(drive.Transactions) != 0 || len(drive.RewardGrants) != 0 {
		t.Fatalf("noop action should not apply legacy data: txns=%#v grants=%#v", drive.Transactions, drive.RewardGrants)
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

	life := apitypes.PetLife{"hunger": 1}
	applyLifeDelta(life, &apitypes.PetLife{"hunger": -5, "clean": 3})
	if life["hunger"] != 0 || life["clean"] != 3 {
		t.Fatalf("applyLifeDelta() = %#v", life)
	}
	applyLifeDelta(nil, &apitypes.PetLife{"hunger": 1})

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

	for _, resp := range []adminhttp.CreateWorkspaceResponseObject{
		adminhttp.CreateWorkspace400JSONResponse{Error: apitypes.NewErrorResponse("BAD", "bad request").Error},
		adminhttp.CreateWorkspace409JSONResponse{Error: apitypes.NewErrorResponse("CONFLICT", "conflict").Error},
		adminhttp.CreateWorkspace500JSONResponse{Error: apitypes.NewErrorResponse("ERROR", "server error").Error},
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
	petResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{
		Body: &adminhttp.PetDefUpsert{
			Id:   "petdef-basic",
			Spec: testPetDefSpec("Spark"),
		},
	})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	if _, ok := petResp.(adminhttp.CreatePetDef200JSONResponse); !ok {
		t.Fatalf("CreatePetDef() response = %#v", petResp)
	}
	badgeResp, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{
		Body: &adminhttp.BadgeDefUpsert{Id: "badge-basic", Spec: apitypes.BadgeDefSpec{DisplayName: "First Win"}},
	})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	if _, ok := badgeResp.(adminhttp.CreateBadgeDef200JSONResponse); !ok {
		t.Fatalf("CreateBadgeDef() response = %#v", badgeResp)
	}
	gameResp, err := catalog.CreateGameDef(ctx, adminhttp.CreateGameDefRequestObject{
		Body: &adminhttp.GameDefUpsert{Id: "game-basic", Spec: apitypes.GameDefSpec{DisplayName: "Puzzle"}},
	})
	if err != nil {
		t.Fatalf("CreateGameDef() error = %v", err)
	}
	if _, ok := gameResp.(adminhttp.CreateGameDef200JSONResponse); !ok {
		t.Fatalf("CreateGameDef() response = %#v", gameResp)
	}
	initialBalance := int64(50)
	adoptionCost := int64(15)
	points := int64(30)
	gameExp := int64(20)
	badgeDelta := map[string]int64{"badge-basic": 100}
	gameIDs := []string{"game-basic"}
	rulesetResp, err := catalog.CreateGameRuleset(ctx, adminhttp.CreateGameRulesetRequestObject{
		Body: &adminhttp.GameRulesetUpsert{
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
	if _, ok := rulesetResp.(adminhttp.CreateGameRuleset200JSONResponse); !ok {
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
	created []adminhttp.WorkspaceUpsert
	deleted []string
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

type recordingACLService struct {
	roles    map[string]apitypes.ACLPermissionList
	bindings map[string]apitypes.ACLPolicy
}

func (s *recordingACLService) PutRole(_ context.Context, name string, permissions apitypes.ACLPermissionList) (apitypes.ACLRole, error) {
	if s.roles == nil {
		s.roles = map[string]apitypes.ACLPermissionList{}
	}
	s.roles[name] = permissions
	return apitypes.ACLRole{Name: name, Permissions: permissions}, nil
}

func (s *recordingACLService) PutPolicyBinding(_ context.Context, id string, _ float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if s.bindings == nil {
		s.bindings = map[string]apitypes.ACLPolicy{}
	}
	s.bindings[id] = policy
	return apitypes.ACLPolicyBinding{Id: id, Policy: policy}, nil
}

func (s *recordingACLService) DeletePolicyBinding(_ context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	if s.bindings == nil {
		return apitypes.ACLPolicyBinding{}, nil
	}
	policy := s.bindings[id]
	delete(s.bindings, id)
	return apitypes.ACLPolicyBinding{Id: id, Policy: policy}, nil
}
