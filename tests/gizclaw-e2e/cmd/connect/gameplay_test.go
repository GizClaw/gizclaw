//go:build gizclaw_e2e

package connect_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestConnectGameplayUserStory(t *testing.T) {
	h := clitest.NewHarnessForRoot(t, "tests/gizclaw-e2e/cmd/connect", "305-gameplay-cli")
	h.StartServerFromFixture("server_config.yaml")
	h.InstallFixedAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "connect-gameplay-peer-a-sn").MustSucceed(t)
	applyGameplayCLIResources(t, h)
	applyGameplayCLIACL(t, h, "peer-a")

	ruleset := mustRunCLIJSON[rpcapi.GameRuleset](t, h, "connect", "gameplay", "ruleset", "--name", "default-gameplay", "--context", "peer-a")
	if ruleset.Name != "default-gameplay" || !ruleset.Spec.Enabled {
		t.Fatalf("ruleset = %#v", ruleset)
	}

	adopted := mustRunCLIJSON[rpcapi.PetAdoptResponse](t, h, "connect", "gameplay", "pet", "adopt", "--ruleset", "default-gameplay", "--name", "CLI Pet", "--context", "peer-a")
	t.Cleanup(func() {
		_ = h.RunCLI("connect", "gameplay", "pet", "delete", adopted.Pet.Id, "--context", "peer-a")
	})
	if adopted.Pet.DisplayName != "CLI Pet" || adopted.Pet.PetdefId != "petdef-starter" || adopted.Transaction.Delta != -10 {
		t.Fatalf("adopted = %#v", adopted)
	}

	drive := mustRunCLIJSON[rpcapi.PetDriveResponse](t, h,
		"connect", "gameplay", "pet", "drive", adopted.Pet.Id,
		"--action", "bath",
		"--game", "game-starter",
		"--score", "42",
		"--max-score", "100",
		"--difficulty", "normal",
		"--outcome", "win",
		"--duration-ms", "1234",
		"--idempotency-key", "cli-result-1",
		"--context", "peer-a",
	)
	if drive.GameResult == nil || drive.GameResult.IdempotencyKey == nil || *drive.GameResult.IdempotencyKey != "cli-result-1" || drive.GameResult.MaxScore == nil || *drive.GameResult.MaxScore != 100 {
		t.Fatalf("drive game result = %#v", drive.GameResult)
	}
	if len(drive.Badges) != 1 || !drive.Badges[0].Active || len(drive.RewardGrants) != 1 || len(drive.Transactions) != 2 {
		t.Fatalf("drive = %#v", drive)
	}

	duplicate := h.RunCLI(
		"connect", "gameplay", "pet", "drive", adopted.Pet.Id,
		"--game", "game-starter",
		"--idempotency-key", "cli-result-1",
		"--context", "peer-a",
	)
	if duplicate.Err == nil {
		t.Fatalf("duplicate idempotency key should fail:\nstdout:\n%s\nstderr:\n%s", duplicate.Stdout, duplicate.Stderr)
	}

	petList := mustRunCLIJSON[rpcapi.PetListResponse](t, h, "connect", "gameplay", "pet", "list", "--context", "peer-a")
	requireCLIPetID(t, petList.Items, adopted.Pet.Id)
	petGet := mustRunCLIJSON[rpcapi.Pet](t, h, "connect", "gameplay", "pet", "get", adopted.Pet.Id, "--context", "peer-a")
	if petGet.Id != adopted.Pet.Id {
		t.Fatalf("pet get = %#v", petGet)
	}
	points := mustRunCLIJSON[rpcapi.PointsAccount](t, h, "connect", "gameplay", "points", "get", "--ruleset", "default-gameplay", "--context", "peer-a")
	if points.Balance != drive.Points.Balance {
		t.Fatalf("points = %#v drive=%#v", points, drive.Points)
	}
	txnList := mustRunCLIJSON[rpcapi.PointsTransactionListResponse](t, h, "connect", "gameplay", "points", "transactions", "list", "--context", "peer-a")
	requireCLIPointsTransactionID(t, txnList.Items, adopted.Transaction.Id)
	txnGet := mustRunCLIJSON[rpcapi.PointsTransaction](t, h, "connect", "gameplay", "points", "transactions", "get", adopted.Transaction.Id, "--context", "peer-a")
	if txnGet.Id != adopted.Transaction.Id || txnGet.SourceType == "" {
		t.Fatalf("transaction get = %#v", txnGet)
	}
	badgeList := mustRunCLIJSON[rpcapi.BadgeListResponse](t, h, "connect", "gameplay", "badge", "list", "--context", "peer-a")
	requireCLIBadgeID(t, badgeList.Items, "badge-starter")
	badgeGet := mustRunCLIJSON[rpcapi.Badge](t, h, "connect", "gameplay", "badge", "get", "badge-starter", "--context", "peer-a")
	if !badgeGet.Active {
		t.Fatalf("badge get = %#v", badgeGet)
	}
	resultList := mustRunCLIJSON[rpcapi.GameResultListResponse](t, h, "connect", "gameplay", "game-result", "list", "--context", "peer-a")
	requireCLIGameResultID(t, resultList.Items, drive.GameResult.Id)
	resultGet := mustRunCLIJSON[rpcapi.GameResult](t, h, "connect", "gameplay", "game-result", "get", drive.GameResult.Id, "--context", "peer-a")
	if resultGet.Id != drive.GameResult.Id || resultGet.DurationMs == nil || *resultGet.DurationMs != 1234 {
		t.Fatalf("game result get = %#v", resultGet)
	}
	grantList := mustRunCLIJSON[rpcapi.RewardGrantListResponse](t, h, "connect", "gameplay", "reward-grant", "list", "--context", "peer-a")
	requireCLIRewardGrantID(t, grantList.Items, drive.RewardGrants[0].Id)
	grantGet := mustRunCLIJSON[rpcapi.RewardGrant](t, h, "connect", "gameplay", "reward-grant", "get", drive.RewardGrants[0].Id, "--context", "peer-a")
	if grantGet.Id != drive.RewardGrants[0].Id || grantGet.SourceType != "game_result" {
		t.Fatalf("reward grant get = %#v", grantGet)
	}
}

func applyGameplayCLIResources(t *testing.T, h *clitest.Harness) {
	t.Helper()
	for _, fixture := range []string{
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "04-workflows", "22-chatroom-direct.yaml"),
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "07-gameplay", "00-starter-gameplay.yaml"),
	} {
		h.RunCLI("admin", "apply", "--context", "admin-a", "-f", fixture).MustSucceed(t)
	}
}

func applyGameplayCLIACL(t *testing.T, h *clitest.Harness, contextName string) {
	t.Helper()
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
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

func requireCLIPetID(t *testing.T, items []rpcapi.Pet, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("pet %q not found in %#v", id, items)
}

func requireCLIPointsTransactionID(t *testing.T, items []rpcapi.PointsTransaction, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("points transaction %q not found in %#v", id, items)
}

func requireCLIBadgeID(t *testing.T, items []rpcapi.Badge, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("badge %q not found in %#v", id, items)
}

func requireCLIGameResultID(t *testing.T, items []rpcapi.GameResult, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("game result %q not found in %#v", id, items)
}

func requireCLIRewardGrantID(t *testing.T, items []rpcapi.RewardGrant, id string) {
	t.Helper()
	for _, item := range items {
		if item.Id == id {
			return
		}
	}
	t.Fatalf("reward grant %q not found in %#v", id, items)
}
