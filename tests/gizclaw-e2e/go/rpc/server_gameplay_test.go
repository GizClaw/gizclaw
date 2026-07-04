//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestRPCGameplayAdoptAndDrive(t *testing.T) {
	env := newGameplayRPCHarness(t)

	ruleset, err := env.peer.GetGameRuleset(env.ctx, "shared.game_ruleset.get", rpcapi.ServerGameRulesetGetRequest{Name: testStringPtr("default-gameplay")})
	if err != nil {
		t.Fatalf("game_ruleset.get default-gameplay: %v", err)
	}
	if ruleset.Name != "default-gameplay" || !ruleset.Spec.Enabled {
		t.Fatalf("game_ruleset.get = %#v", ruleset)
	}

	adopted, err := env.peer.AdoptPet(env.ctx, "shared.pet.adopt", rpcapi.ServerPetAdoptRequest{
		RulesetName: testStringPtr("default-gameplay"),
		DisplayName: testStringPtr("E2E Pet"),
	})
	if err != nil {
		t.Fatalf("pet.adopt: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeletePet(env.ctx, "shared.pet.delete.cleanup", rpcapi.ServerPetDeleteRequest{Id: adopted.Pet.Id})
	})
	if adopted.Pet.PetdefId != "petdef-starter" || adopted.Pet.DisplayName != "E2E Pet" {
		t.Fatalf("pet.adopt pet = %#v", adopted.Pet)
	}
	if adopted.Points.Balance != 90 || adopted.Transaction.Delta != -10 {
		t.Fatalf("pet.adopt points/transaction = %#v %#v", adopted.Points, adopted.Transaction)
	}

	score := int64(42)
	maxScore := int64(100)
	durationMs := int64(2345)
	difficulty := "normal"
	idempotencyKey := "rpc-result-1"
	drive, err := env.peer.DrivePet(env.ctx, "shared.pet.drive", rpcapi.ServerPetDriveRequest{
		PetId:  adopted.Pet.Id,
		Action: testStringPtr("bath"),
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameDefId:      "game-starter",
			Score:          &score,
			MaxScore:       &maxScore,
			Difficulty:     &difficulty,
			Outcome:        testStringPtr("win"),
			DurationMs:     &durationMs,
			IdempotencyKey: &idempotencyKey,
		},
	})
	if err != nil {
		t.Fatalf("pet.drive: %v", err)
	}
	if drive.Pet.Level != 2 || drive.Pet.Exp != 105 {
		t.Fatalf("pet.drive pet = %#v", drive.Pet)
	}
	if drive.Points.Balance != 105 {
		t.Fatalf("pet.drive points = %#v", drive.Points)
	}
	if drive.GameResult == nil || drive.GameResult.GameDefId != "game-starter" || drive.GameResult.Score == nil || *drive.GameResult.Score != score {
		t.Fatalf("pet.drive game result = %#v", drive.GameResult)
	}
	if drive.GameResult.MaxScore == nil || *drive.GameResult.MaxScore != maxScore || drive.GameResult.DurationMs == nil || *drive.GameResult.DurationMs != durationMs || drive.GameResult.IdempotencyKey == nil || *drive.GameResult.IdempotencyKey != idempotencyKey {
		t.Fatalf("pet.drive game result details = %#v", drive.GameResult)
	}
	if len(drive.Badges) != 1 || drive.Badges[0].BadgeDefId != "badge-starter" || !drive.Badges[0].Active || drive.Badges[0].Level != 1 {
		t.Fatalf("pet.drive badges = %#v", drive.Badges)
	}
	if len(drive.RewardGrants) != 1 || drive.RewardGrants[0].PointsDelta != 20 || drive.RewardGrants[0].PetExpDelta != 105 {
		t.Fatalf("pet.drive reward grants = %#v", drive.RewardGrants)
	}
	if len(drive.Transactions) != 2 || drive.Transactions[0].Delta != -5 || drive.Transactions[1].Delta != 20 {
		t.Fatalf("pet.drive transactions = %#v", drive.Transactions)
	}
	if _, err := env.peer.DrivePet(env.ctx, "shared.pet.drive.duplicate", rpcapi.ServerPetDriveRequest{
		PetId: adopted.Pet.Id,
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameDefId:      "game-starter",
			IdempotencyKey: &idempotencyKey,
		},
	}); err == nil {
		t.Fatal("duplicate game result idempotency key should fail")
	}

	pets, err := env.peer.ListPets(env.ctx, "shared.pet.list", rpcapi.ServerPetListRequest{})
	if err != nil {
		t.Fatalf("pet.list: %v", err)
	}
	requirePetID(t, pets.Items, adopted.Pet.Id)

	pointsTransactions, err := env.peer.ListPointsTransactions(env.ctx, "shared.points.transactions.list", rpcapi.ServerPointsTransactionListRequest{})
	if err != nil {
		t.Fatalf("points.transactions.list: %v", err)
	}
	requirePointsTransactionID(t, pointsTransactions.Items, adopted.Transaction.Id)

	results, err := env.peer.ListGameResults(env.ctx, "shared.game_result.list", rpcapi.ServerGameResultListRequest{})
	if err != nil {
		t.Fatalf("game_result.list: %v", err)
	}
	requireGameResultID(t, results.Items, drive.GameResult.Id)

	grants, err := env.peer.ListRewardGrants(env.ctx, "shared.reward_grant.list", rpcapi.ServerRewardGrantListRequest{})
	if err != nil {
		t.Fatalf("reward_grant.list: %v", err)
	}
	requireRewardGrantID(t, grants.Items, drive.RewardGrants[0].Id)
}

type gameplayRPCHarness struct {
	ctx  context.Context
	h    *clitest.Harness
	peer *gizcli.Client
}

func newGameplayRPCHarness(t *testing.T) *gameplayRPCHarness {
	t.Helper()

	h := clitest.NewHarnessForRoot(t, "tests/gizclaw-e2e/go/rpc", "client-rpc-gameplay")
	h.StartServerFromFixture("server_config.yaml")
	h.InstallFixedAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "client-rpc-gameplay-peer-a-sn").MustSucceed(t)
	applyGameplayCatalog(t, h)
	applyGameplayACL(t, h, "peer-a")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })
	return &gameplayRPCHarness{ctx: ctx, h: h, peer: peer}
}

func applyGameplayCatalog(t *testing.T, h *clitest.Harness) {
	t.Helper()

	for _, fixture := range []string{
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "04-workflows", "22-chatroom-direct.yaml"),
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "07-gameplay", "00-starter-gameplay.yaml"),
	} {
		result := h.RunCLI("admin", "apply", "--context", "admin-a", "-f", fixture)
		result.MustSucceed(t)
	}
}

func applyGameplayACL(t *testing.T, h *clitest.Harness, contextName string) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	roleResp, err := api.PutACLRoleWithResponse(ctx, "default-client", adminservice.ACLRoleUpsert{
		Name: "default-client",
		Permissions: apitypes.ACLPermissionList{
			apitypes.ACLPermissionGamerulesetRead,
			apitypes.ACLPermissionGamerulesetUse,
		},
	})
	if err != nil {
		t.Fatalf("put gameplay ACL role: %v", err)
	}
	if roleResp.JSON200 == nil {
		t.Fatalf("put gameplay ACL role status %d: %s", roleResp.StatusCode(), strings.TrimSpace(string(roleResp.Body)))
	}
	view := "default-client"
	configResp, err := api.PutPeerConfigWithResponse(ctx, h.ContextPublicKey(contextName), apitypes.Configuration{View: &view})
	if err != nil {
		t.Fatalf("put gameplay peer config: %v", err)
	}
	if configResp.JSON200 == nil {
		t.Fatalf("put gameplay peer config status %d: %s", configResp.StatusCode(), strings.TrimSpace(string(configResp.Body)))
	}
	bindingID := "gameplay-default-ruleset-" + h.ContextPublicKey(contextName)
	bindingResp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
		Id: &bindingID,
		Policy: apitypes.ACLPolicy{
			Subject: apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindView, Id: "default-client"},
			Resource: apitypes.ACLResource{
				Kind: apitypes.ACLResourceKindGameruleset,
				Id:   "default-gameplay",
			},
			Role: "default-client",
		},
	})
	if err != nil {
		t.Fatalf("create gameplay ACL binding: %v", err)
	}
	if bindingResp.JSON200 == nil && bindingResp.JSON409 == nil {
		t.Fatalf("create gameplay ACL binding status %d: %s", bindingResp.StatusCode(), strings.TrimSpace(string(bindingResp.Body)))
	}
}

func requirePetID(t *testing.T, items []rpcapi.Pet, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("pet %q not found in %#v", id, items)
}

func requirePointsTransactionID(t *testing.T, items []rpcapi.PointsTransaction, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("points transaction %q not found in %#v", id, items)
}

func requireGameResultID(t *testing.T, items []rpcapi.GameResult, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("game result %q not found in %#v", id, items)
}

func requireRewardGrantID(t *testing.T, items []rpcapi.RewardGrant, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("reward grant %q not found in %#v", id, items)
}
