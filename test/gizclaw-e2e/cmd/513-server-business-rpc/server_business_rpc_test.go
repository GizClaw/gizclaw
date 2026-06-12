package peerbusinessrpc_test

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
	_ "modernc.org/sqlite"
)

func TestServerBusinessRPCUserStory(t *testing.T) {
	assertRemovedBusinessRPCSurfaces(t)

	openAI := newBusinessOpenAIServer(t)

	h := clitest.NewHarness(t, "513-server-business-rpc")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "peer-a-sn").MustSucceed(t)
	h.CreateContext("peer-b").MustSucceed(t)
	h.RegisterContext("peer-b", "--sn", "peer-b-sn").MustSucceed(t)

	seedBusinessResources(t, h, openAI.URL+"/v1")
	seedWalletBalance(t, h, h.ContextPublicKey("peer-a"), 250)

	walletBefore := mustRunCLIJSON[rpcapi.WalletGetResponse](t, h, "connect", "wallet", "get", "--context", "peer-a")
	if walletBefore.PointBalance != 250 {
		t.Fatalf("wallet before point balance = %d, want 250", walletBefore.PointBalance)
	}

	pet := mustRunCLIJSON[rpcapi.PetAdoptResponse](t, h, "connect", "pet", "adopt", "--id", "pet-a", "--name", "Momo", "--context", "peer-a")
	if pet.Id != "pet-a" || pet.SpeciesId != "rabbit" || pet.VoiceId != "voice-a" {
		t.Fatalf("adopted pet = %#v", pet)
	}

	pet = mustRunCLIJSON[rpcapi.PetFeedResponse](t, h, "connect", "pet", "feed", "pet-a", "--prompt", "ate lunch", "--context", "peer-a")
	if pet.Life.Satiety < 60 {
		t.Fatalf("pet.feed satiety = %d, want increased", pet.Life.Satiety)
	}
	pet = mustRunCLIJSON[rpcapi.PetWashResponse](t, h, "connect", "pet", "wash", "pet-a", "--prompt", "had a bath", "--context", "peer-a")
	if pet.Life.Cleanliness < 60 {
		t.Fatalf("pet.wash cleanliness = %d, want increased", pet.Life.Cleanliness)
	}
	pet = mustRunCLIJSON[rpcapi.PetPlayResponse](t, h, "connect", "pet", "play", "pet-a", "--prompt", "played a game", "--context", "peer-a")
	if pet.Life.Mood < 57 || pet.Ability.Exp == 0 {
		t.Fatalf("pet.play result = %#v", pet)
	}

	reward := mustRunCLIJSON[rpcapi.RewardClaimResponse](t, h, "connect", "reward", "claim", "--prompt", "finished the tutorial", "--context", "peer-a")
	if reward.BadgeId != "founder" || reward.PointAmount != 9 {
		t.Fatalf("reward = %#v", reward)
	}
	gotReward := mustRunCLIJSON[rpcapi.RewardGetResponse](t, h, "connect", "reward", "get", reward.Id, "--context", "peer-a")
	if gotReward.Id != reward.Id {
		t.Fatalf("reward.get id = %q, want %q", gotReward.Id, reward.Id)
	}

	walletAfter := mustRunCLIJSON[rpcapi.WalletGetResponse](t, h, "connect", "wallet", "get", "--context", "peer-a")
	if walletAfter.PointBalance != 156 {
		t.Fatalf("wallet after point balance = %d, want 156", walletAfter.PointBalance)
	}

	renamedPet := mustRunCLIJSON[rpcapi.PetPutResponse](t, h, "connect", "pet", "put", "pet-a", "--name", "Momo II", "--context", "peer-a")
	if renamedPet.Name != "Momo II" {
		t.Fatalf("pet.put name = %q, want Momo II", renamedPet.Name)
	}
	gotPet := mustRunCLIJSON[rpcapi.PetGetResponse](t, h, "connect", "pet", "get", "pet-a", "--context", "peer-a")
	if gotPet.Id != "pet-a" || gotPet.Name != "Momo II" {
		t.Fatalf("pet.get = %#v", gotPet)
	}

	secondPet := mustRunCLIJSON[rpcapi.PetAdoptResponse](t, h, "connect", "pet", "adopt", "--id", "pet-b", "--name", "Nono", "--context", "peer-a")
	time.Sleep(2 * time.Millisecond)
	secondReward := mustRunCLIJSON[rpcapi.RewardClaimResponse](t, h, "connect", "reward", "claim", "--prompt", "helped a friend", "--context", "peer-a")

	assertPetPagination(t, h, []string{"pet-a", secondPet.Id})
	assertRewardPagination(t, h, []string{reward.Id, secondReward.Id})
	firstTransactionID := assertWalletTransactionPagination(t, h)
	assertPeerIsolation(t, h, "pet-a", reward.Id, firstTransactionID)

	deletedPet := mustRunCLIJSON[rpcapi.PetDeleteResponse](t, h, "connect", "pet", "delete", secondPet.Id, "--context", "peer-a")
	if deletedPet.Id != secondPet.Id {
		t.Fatalf("pet.delete id = %q, want %q", deletedPet.Id, secondPet.Id)
	}
}

func assertRemovedBusinessRPCSurfaces(t *testing.T) {
	t.Helper()

	for _, method := range []rpcapi.RPCMethod{
		"server." + "game." + "results." + "create",
		"server.reward." + "create",
		"server.pet." + "create",
		"server.pet." + "level-up",
	} {
		if method.Valid() {
			t.Fatalf("removed business RPC method %q is still generated as valid", method)
		}
	}
}

func seedBusinessResources(t *testing.T, h *clitest.Harness, openAIBaseURL string) {
	t.Helper()

	resourcePath := filepath.Join(h.SandboxDir, "business-resources.json")
	resourceJSON := fmt.Sprintf(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ResourceList",
		"metadata": {"name": "business-resources"},
		"spec": {
			"items": [
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Credential","metadata":{"name":"openai-key"},"spec":{"provider":"openai","method":"api_key","body":{"api_key":"sk-e2e"}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"OpenAITenant","metadata":{"name":"openai-e2e"},"spec":{"kind":"compatible","credential_name":"openai-key","base_url":%q,"api_mode":"chat_completions"}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"name":"reward-claim"},"spec":{"kind":"llm","source":"manual","provider":{"kind":"openai-tenant","name":"openai-e2e"},"provider_data":{"openai-tenant":{"upstream_model":"reward-e2e","support_json_output":true,"use_system_role":true}}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"name":"pet-action"},"spec":{"kind":"llm","source":"manual","provider":{"kind":"openai-tenant","name":"openai-e2e"},"provider_data":{"openai-tenant":{"upstream_model":"pet-e2e","support_json_output":true,"use_system_role":true}}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Voice","metadata":{"name":"voice-a"},"spec":{"name":"Voice A","source":"manual","provider":{"kind":"openai-tenant","name":"openai-e2e"},"provider_data":{"openai-tenant":{"voice_id":"alloy"}}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"PetSpecies","metadata":{"name":"rabbit"},"spec":{"name":"Rabbit"}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Badge","metadata":{"name":"founder"},"spec":{"name":"Founder","description":"first reward badge"}}
			]
		}
	}`, openAIBaseURL)
	if err := os.WriteFile(resourcePath, []byte(resourceJSON), 0o644); err != nil {
		t.Fatalf("write resource file: %v", err)
	}
	apply := h.RunCLI("admin", "apply", "-f", resourcePath, "--context", "admin-a")
	apply.MustSucceed(t)

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	seedBusinessACL(t, ctx, api)

	pixa := testPixa(240, 240, []string{"idle"})
	if resp, err := api.UploadPetSpeciesPixaWithBodyWithResponse(ctx, "rabbit", "application/octet-stream", strings.NewReader(string(pixa))); err != nil {
		t.Fatalf("upload pixa: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("upload pixa status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp, err := api.UploadBadgeIconWithBodyWithResponse(ctx, "founder", "application/octet-stream", strings.NewReader("icon")); err != nil {
		t.Fatalf("upload badge icon: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("upload badge icon status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
}

func testPixa(width, height uint16, clips []string) []byte {
	const (
		headerSize     = 40
		clipEntrySize  = 56
		frameEntrySize = 16
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + 2
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payloadOffset := frameOffset + frameEntrySize
	data := make([]byte, payloadOffset)
	copy(data[0:4], "PIXA")
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
	for i, name := range clips {
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+32], name)
		binary.LittleEndian.PutUint32(data[base+40:base+44], 1)
	}
	return data
}

func seedBusinessACL(t *testing.T, ctx context.Context, api *adminservice.ClientWithResponses) {
	t.Helper()

	roleResp, err := api.CreateACLRoleWithResponse(ctx, adminservice.ACLRoleUpsert{
		Name: "business-user",
		Permissions: apitypes.ACLPermissionList{
			apitypes.ACLPermissionPetSpeciesUse,
			apitypes.ACLPermissionBadgeUse,
		},
	})
	if err != nil {
		t.Fatalf("create business ACL role: %v", err)
	}
	if roleResp.JSON200 == nil {
		t.Fatalf("create business ACL role status %d: %s", roleResp.StatusCode(), strings.TrimSpace(string(roleResp.Body)))
	}

	subject := apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindAllPeers}
	for _, resource := range []apitypes.ACLResource{
		{Kind: apitypes.ACLResourceKindPetSpecies, Id: "rabbit"},
		{Kind: apitypes.ACLResourceKindBadge, Id: "founder"},
	} {
		id := fmt.Sprintf("business-user-%s-%s", resource.Kind, resource.Id)
		resp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
			Id: &id,
			Policy: apitypes.ACLPolicy{
				Subject:  subject,
				Resource: resource,
				Role:     "business-user",
			},
		})
		if err != nil {
			t.Fatalf("create business ACL binding %s: %v", id, err)
		}
		if resp.JSON200 == nil {
			t.Fatalf("create business ACL binding %s status %d: %s", id, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}
}

func seedWalletBalance(t *testing.T, h *clitest.Harness, peerID string, points int64) {
	t.Helper()

	dbPath := filepath.Join(h.ServerWorkspace, "data", "history.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open wallet db: %v", err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS wallets (peer_id TEXT PRIMARY KEY, id TEXT NOT NULL UNIQUE, token_balance INTEGER NOT NULL, point_balance INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS wallet_transactions (peer_id TEXT NOT NULL, id TEXT NOT NULL, token_delta INTEGER NOT NULL, point_delta INTEGER NOT NULL, reason TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY (peer_id, id), FOREIGN KEY (peer_id) REFERENCES wallets(peer_id))`,
		`CREATE INDEX IF NOT EXISTS wallet_transactions_peer_created_desc ON wallet_transactions(peer_id, created_at DESC, id DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("wallet schema: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO wallets (peer_id, id, token_balance, point_balance, created_at, updated_at) VALUES (?, ?, 0, ?, ?, ?)`, peerID, "wallet-"+peerID, points, now, now); err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO wallet_transactions (peer_id, id, token_delta, point_delta, reason, created_at) VALUES (?, 'seed-credit', 0, ?, 'reward_claim', ?)`, peerID, points, now); err != nil {
		t.Fatalf("seed wallet transaction: %v", err)
	}
}

func assertPetPagination(t *testing.T, h *clitest.Harness, wantIDs []string) {
	t.Helper()
	first := mustRunCLIJSON[rpcapi.PetListResponse](t, h, "connect", "pet", "list", "--limit", "1", "--context", "peer-a")
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("pet.list page1 = %#v", first)
	}
	second := mustRunCLIJSON[rpcapi.PetListResponse](t, h, "connect", "pet", "list", "--limit", "1", "--cursor", *first.NextCursor, "--context", "peer-a")
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("pet.list page2 = %#v", second)
	}
	got := []string{first.Items[0].Id, second.Items[0].Id}
	if !sameStringSet(got, wantIDs) {
		t.Fatalf("pet.list pages ids = %#v, want %#v", got, wantIDs)
	}
}

func assertRewardPagination(t *testing.T, h *clitest.Harness, wantIDs []string) {
	t.Helper()
	first := mustRunCLIJSON[rpcapi.RewardListResponse](t, h, "connect", "reward", "list", "--limit", "1", "--context", "peer-a")
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("reward.list page1 = %#v", first)
	}
	second := mustRunCLIJSON[rpcapi.RewardListResponse](t, h, "connect", "reward", "list", "--limit", "1", "--cursor", *first.NextCursor, "--context", "peer-a")
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("reward.list page2 = %#v", second)
	}
	got := []string{first.Items[0].Id, second.Items[0].Id}
	if !sameStringSet(got, wantIDs) {
		t.Fatalf("reward.list pages ids = %#v, want %#v", got, wantIDs)
	}
}

func assertWalletTransactionPagination(t *testing.T, h *clitest.Harness) string {
	t.Helper()
	first := mustRunCLIJSON[rpcapi.WalletTransactionsListResponse](t, h, "connect", "wallet", "transactions", "list", "--limit", "1", "--context", "peer-a")
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("wallet.transactions.list page1 = %#v", first)
	}
	second := mustRunCLIJSON[rpcapi.WalletTransactionsListResponse](t, h, "connect", "wallet", "transactions", "list", "--limit", "1", "--cursor", *first.NextCursor, "--context", "peer-a")
	if len(second.Items) != 1 {
		t.Fatalf("wallet.transactions.list page2 = %#v", second)
	}
	got := mustRunCLIJSON[rpcapi.WalletTransactionsGetResponse](t, h, "connect", "wallet", "transactions", "get", first.Items[0].Id, "--context", "peer-a")
	if got.Id != first.Items[0].Id {
		t.Fatalf("wallet.transactions.get id = %q, want %q", got.Id, first.Items[0].Id)
	}
	return got.Id
}

func assertPeerIsolation(t *testing.T, h *clitest.Harness, petID string, rewardID string, transactionID string) {
	t.Helper()

	for _, args := range [][]string{
		{"connect", "pet", "get", petID, "--context", "peer-b"},
		{"connect", "reward", "get", rewardID, "--context", "peer-b"},
		{"connect", "wallet", "transactions", "get", transactionID, "--context", "peer-b"},
	} {
		result := h.RunCLI(args...)
		if result.Err == nil {
			t.Fatalf("%v should fail for another peer:\nstdout:\n%s", args, result.Stdout)
		}
	}
}

func mustRunCLIJSON[T any](t *testing.T, h *clitest.Harness, args ...string) T {
	t.Helper()

	result, err := h.RunCLIUntilSuccess(args...)
	if err != nil {
		t.Fatalf("%v failed: %v", args, err)
	}
	var out T
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("%v returned invalid JSON: %v\nstdout:\n%s\nstderr:\n%s", args, err, result.Stdout, result.Stderr)
	}
	return out
}

func newBusinessOpenAIServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		content := `{"point_delta":3,"life_delta":{"mood":7},"ability_delta":{"exp":4}}`
		switch req.Model {
		case "reward-e2e":
			content = `{"badge_id":"founder","point_amount":9}`
		case "pet-e2e":
			// A single deterministic pet-action model is enough here; the test
			// checks that every action traverses the configured generator path.
			content = `{"point_delta":-1,"life_delta":{"satiety":10,"cleanliness":10,"mood":7},"ability_delta":{"exp":4}}`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-e2e",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   req.Model,
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     1,
				"completion_tokens": 1,
				"total_tokens":      2,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, value := range got {
		seen[value]++
	}
	for _, value := range want {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	return true
}
