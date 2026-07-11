//go:build gizclaw_e2e

package connect_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
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
	uploadGameplayCLIPixa(t, h)
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
	if drive.Pet.Progression["xp"] != 105 {
		t.Fatalf("drive pet progression = %#v", drive.Pet)
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
	presentation := mustRunCLIJSON[rpcapi.PetPresentation](t, h, "connect", "gameplay", "pet", "presentation", adopted.Pet.Id, "--context", "peer-a")
	if presentation.PetId != adopted.Pet.Id || presentation.PetdefId != adopted.Pet.PetdefId || presentation.DefaultLocale != "en" {
		t.Fatalf("pet presentation identity = %#v", presentation)
	}
	if presentation.Attr.Life["hunger"].Initial != 100 || presentation.Attr.Progression["xp"].Initial != 0 {
		t.Fatalf("pet presentation attr = %#v", presentation.Attr)
	}
	if !hasCLIPresentationAction(presentation.Drive.Actions, "bath", 5, "bath") {
		t.Fatalf("pet presentation actions = %#v", presentation.Drive.Actions)
	}
	if presentation.PixaMetadata.Canvas.Width != 60 || presentation.PixaMetadata.Canvas.Height != 60 || !hasCLIPresentationClip(presentation.PixaMetadata.Clips, "idle", "idle") {
		t.Fatalf("pet presentation pixa metadata = %#v", presentation.PixaMetadata)
	}
	if presentation.I18n["en"].DisplayName == nil || *presentation.I18n["en"].DisplayName != "Starter Pet" {
		t.Fatalf("pet presentation i18n = %#v", presentation.I18n)
	}
	petPixaPath := filepath.Join(t.TempDir(), "pet.pixa")
	pixaDownload := mustRunCLIJSON[struct {
		Metadata rpcapi.PetPixaDownloadResponse `json:"metadata"`
		Output   string                         `json:"output"`
		Bytes    int64                          `json:"bytes"`
	}](t, h, "connect", "gameplay", "pet", "pixa", adopted.Pet.Id, "--output", petPixaPath, "--context", "peer-a")
	if pixaDownload.Metadata.PetId != adopted.Pet.Id || pixaDownload.Metadata.PetdefId != adopted.Pet.PetdefId || pixaDownload.Bytes <= 0 {
		t.Fatalf("pet pixa download = %#v", pixaDownload)
	}
	if info, err := os.Stat(petPixaPath); err != nil || info.Size() != pixaDownload.Bytes {
		t.Fatalf("pet pixa file stat size=%d err=%v download=%#v", fileSize(info), err, pixaDownload)
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
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "04-workflows", "23-flowcraft-pet-care.yaml"),
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "07-gameplay", "00-starter-gameplay.yaml"),
	} {
		h.RunCLI("admin", "apply", "--context", "admin-a", "-f", fixture).MustSucceed(t)
	}
}

func uploadGameplayCLIPixa(t *testing.T, h *clitest.Harness) {
	t.Helper()
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pixa := makeGameplayCLITestPixa(t, []string{"idle", "bath"}, 60, 60)
	resp, err := api.UploadPetDefPixaWithBodyWithResponse(ctx, "petdef-starter", "application/octet-stream", bytes.NewReader(pixa))
	if err != nil {
		t.Fatalf("upload gameplay pet pixa: %v", err)
	}
	if resp.JSON200 == nil || resp.JSON200.PixaPath == nil {
		t.Fatalf("upload gameplay pet pixa status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
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
	roleResp, err := api.PutACLRoleWithResponse(ctx, "default-client", adminhttp.ACLRoleUpsert{
		Name: "default-client",
		Permissions: apitypes.ACLPermissionList{
			apitypes.ACLPermissionRead,
			apitypes.ACLPermissionUse,
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
	bindingResp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminhttp.ACLPolicyBindingUpsert{
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

func makeGameplayCLITestPixa(t *testing.T, clips []string, width uint16, height uint16) []byte {
	t.Helper()
	if len(clips) == 0 {
		t.Fatal("makeGameplayCLITestPixa requires at least one clip")
	}
	const (
		headerSize       = 40
		clipEntrySize    = 56
		frameEntrySize   = 16
		clipNameSize     = 32
		paletteByteCount = 2
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + paletteByteCount
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payload := []byte{0x00, 0xf8, 0xe0, 0x07}
	payloadOffset := frameOffset + frameEntrySize
	data := make([]byte, payloadOffset+len(payload))
	copy(data[:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], width)
	binary.LittleEndian.PutUint16(data[10:12], height)
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], uint16(len(clips)))
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], uint32(len(payload)))
	for i, clip := range clips {
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+clipNameSize], []byte(clip))
		binary.LittleEndian.PutUint32(data[base+36:base+40], 0)
		binary.LittleEndian.PutUint32(data[base+40:base+44], 1)
		binary.LittleEndian.PutUint32(data[base+44:base+48], 120)
		binary.LittleEndian.PutUint16(data[base+48:base+50], 1)
	}
	binary.LittleEndian.PutUint16(data[frameOffset:frameOffset+2], 120)
	data[frameOffset+2] = 0
	binary.LittleEndian.PutUint32(data[frameOffset+4:frameOffset+8], 0)
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], uint32(len(payload)))
	copy(data[payloadOffset:], payload)
	return data
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

func hasCLIPresentationAction(items []rpcapi.PetPresentationActionSpec, id string, cost int64, visualClipID string) bool {
	for _, item := range items {
		if item.Id == id && item.Cost == cost && item.VisualClipId != nil && *item.VisualClipId == visualClipID {
			return true
		}
	}
	return false
}

func hasCLIPresentationClip(items []rpcapi.PetPresentationPixaClipMetadata, id string, pixaClipName string) bool {
	for _, item := range items {
		if item.Id == id && item.PixaClipName == pixaClipName {
			return true
		}
	}
	return false
}

func fileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
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
