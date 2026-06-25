//go:build gizclaw_e2e

package rpc_test

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
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
	"github.com/goccy/go-yaml"
	_ "modernc.org/sqlite"
)

const serverResourceRole = "server-resource-rpc-admin"

type serverResourceHarness struct {
	h    *clitest.Harness
	ctx  context.Context
	peer *gizcli.Client
}

type socialRPCHarness struct {
	h    *clitest.Harness
	ctx  context.Context
	a    *gizcli.Client
	b    *gizcli.Client
	c    *gizcli.Client
	d    *gizcli.Client
	peer map[string]string
}

type businessRPCHarness struct {
	ctx context.Context
	a   *gizcli.Client
	b   *gizcli.Client
}

func newServerResourceHarness(t *testing.T) *serverResourceHarness {
	t.Helper()

	h := clitest.NewHarness(t, "client-rpc-server-resource")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "peer-a-sn").MustSucceed(t)
	h.CreateContext("peer-denied").MustSucceed(t)
	h.RegisterContext("peer-denied", "--sn", "peer-denied-sn").MustSucceed(t)
	seedPeerResources(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })
	return &serverResourceHarness{h: h, ctx: ctx, peer: peer}
}

func newSocialRPCHarness(t *testing.T) *socialRPCHarness {
	t.Helper()

	h := clitest.NewSetupHarness(t, "client-rpc-social")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "client-rpc-social-admin-sn").MustSucceed(t)
	chatroomWorkflow := filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "testdata", "resources", "04-workflows", "03-chatroom.yaml")
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	applyRPCResourceFile(t, api, chatroomWorkflow)
	for _, peer := range []string{"peer-a", "peer-b", "peer-c", "peer-d"} {
		h.CreateContext(peer).MustSucceed(t)
		h.RegisterContext(peer, "--sn", "client-rpc-social-"+peer+"-sn").MustSucceed(t)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	a := h.ConnectClientFromContext("peer-a")
	b := h.ConnectClientFromContext("peer-b")
	c := h.ConnectClientFromContext("peer-c")
	d := h.ConnectClientFromContext("peer-d")
	t.Cleanup(func() { a.Close() })
	t.Cleanup(func() { b.Close() })
	t.Cleanup(func() { c.Close() })
	t.Cleanup(func() { d.Close() })
	return &socialRPCHarness{
		h:   h,
		ctx: ctx,
		a:   a,
		b:   b,
		c:   c,
		d:   d,
		peer: map[string]string{
			"peer-a": h.ContextPublicKey("peer-a"),
			"peer-b": h.ContextPublicKey("peer-b"),
			"peer-c": h.ContextPublicKey("peer-c"),
			"peer-d": h.ContextPublicKey("peer-d"),
		},
	}
}

func newBusinessHarness(t *testing.T) *businessRPCHarness {
	t.Helper()

	openAI := newBusinessOpenAIServer(t)
	h := clitest.NewHarness(t, "client-rpc-business")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "peer-a-sn").MustSucceed(t)
	h.CreateContext("peer-b").MustSucceed(t)
	h.RegisterContext("peer-b", "--sn", "peer-b-sn").MustSucceed(t)
	seedBusinessResources(t, h, openAI.URL+"/v1")
	seedWalletBalance(t, h, h.ContextPublicKey("peer-a"), 250)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	a := h.ConnectClientFromContext("peer-a")
	b := h.ConnectClientFromContext("peer-b")
	t.Cleanup(func() { a.Close() })
	t.Cleanup(func() { b.Close() })
	return &businessRPCHarness{ctx: ctx, a: a, b: b}
}

func createRPCFriendByInviteToken(t *testing.T, env *socialRPCHarness, from, to *gizcli.Client, toPeerID string) rpcapi.FriendObject {
	t.Helper()

	empty, err := to.GetFriendInviteToken(env.ctx, "friend.invite_token.get.empty", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("friend.invite_token.get empty: %v", err)
	}
	if empty.InviteToken != nil || empty.ExpiresAt != nil {
		t.Fatalf("friend invite token empty get = %#v, want no token", empty)
	}
	token, err := to.CreateFriendInviteToken(env.ctx, "friend.invite_token.create", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("friend.invite_token.create: %v", err)
	}
	if token.InviteToken == "" || token.ExpiresAt.IsZero() {
		t.Fatalf("friend invite token create = %#v", token)
	}
	got, err := to.GetFriendInviteToken(env.ctx, "friend.invite_token.get", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("friend.invite_token.get: %v", err)
	}
	if got.InviteToken == nil || *got.InviteToken != token.InviteToken {
		t.Fatalf("friend invite token get = %#v, want %q", got, token.InviteToken)
	}
	added, err := from.AddFriend(env.ctx, "friend.add", rpcapi.FriendAddRequest{InviteToken: token.InviteToken})
	if err != nil {
		t.Fatalf("friend.add: %v", err)
	}
	if added.PeerPublicKey != nil && *added.PeerPublicKey == toPeerID {
		return *added
	}
	friends, err := from.ListFriends(env.ctx, "friend.list", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("friend.list: %v", err)
	}
	for _, friend := range friends.Items {
		if friend.PeerPublicKey != nil && *friend.PeerPublicKey == toPeerID {
			return friend
		}
	}
	t.Fatalf("friend relation with %s not found in %#v", toPeerID, friends.Items)
	return rpcapi.FriendObject{}
}

func testStringPtr(value string) *string { return &value }

func hasString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func testOpenAICredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{ApiKey: testStringPtr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testMiniMaxCredentialBody(apiKey string) apitypes.CredentialBody {
	return testMiniMaxCredentialBodyFromStrings(map[string]string{"api_key": apiKey})
}

func testMiniMaxCredentialBodyFromStrings(values map[string]string) apitypes.CredentialBody {
	typed := apitypes.MiniMaxCredentialBody{}
	for key, value := range values {
		value := value
		switch key {
		case "api_key":
			typed.ApiKey = &value
		case "token":
			typed.Token = &value
		case "base_url":
			typed.BaseUrl = &value
		case "voice_base_url":
			typed.VoiceBaseUrl = &value
		case "minimax_voice_base_url":
			typed.MinimaxVoiceBaseUrl = &value
		default:
			panic("unsupported minimax credential field: " + key)
		}
	}
	var body apitypes.CredentialBody
	if err := body.FromMiniMaxCredentialBody(typed); err != nil {
		panic(err)
	}
	return body
}

func testGeminiCredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromGeminiCredentialBody(apitypes.GeminiCredentialBody{ApiKey: testStringPtr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testVolcCredentialBodyFromStrings(values map[string]string) apitypes.CredentialBody {
	typed := apitypes.VolcCredentialBody{}
	for key, value := range values {
		value := value
		switch key {
		case "openapi_access_key_id":
			typed.OpenapiAccessKeyId = &value
		case "app_id":
			typed.AppId = &value
		case "ark_api_key":
			typed.ArkApiKey = &value
		case "secret_access_key":
			typed.SecretAccessKey = &value
		case "session_token":
			typed.SessionToken = &value
		case "speech_token":
			typed.SpeechToken = &value
		case "websearch_api_key":
			typed.WebsearchApiKey = &value
		default:
			panic("unsupported volc credential field: " + key)
		}
	}
	var body apitypes.CredentialBody
	if err := body.FromVolcCredentialBody(typed); err != nil {
		panic(err)
	}
	return body
}

func testCredentialBodyString(body apitypes.CredentialBody, key string) string {
	data, err := body.MarshalJSON()
	if err != nil {
		return ""
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return ""
	}
	return values[key]
}

func testRPCOpenAICredentialBody(apiKey string) rpcapi.CredentialBody {
	var body rpcapi.CredentialBody
	if err := body.FromOpenAICredentialBody(rpcapi.OpenAICredentialBody{ApiKey: testStringPtr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testRPCCredentialBodyString(body rpcapi.CredentialBody, key string) string {
	data, err := body.MarshalJSON()
	if err != nil {
		return ""
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return ""
	}
	return values[key]
}

func seedPeerResources(t *testing.T, h *clitest.Harness) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	roleResp, err := api.CreateACLRoleWithResponse(ctx, adminservice.ACLRoleUpsert{
		Name: serverResourceRole,
		Permissions: apitypes.ACLPermissionList{
			apitypes.ACLPermissionWorkspaceAdmin,
			apitypes.ACLPermissionWorkspaceRead,
			apitypes.ACLPermissionWorkspaceUse,
			apitypes.ACLPermissionWorkflowAdmin,
			apitypes.ACLPermissionWorkflowRead,
			apitypes.ACLPermissionWorkflowUse,
			apitypes.ACLPermissionModelAdmin,
			apitypes.ACLPermissionModelRead,
			apitypes.ACLPermissionCredentialAdmin,
			apitypes.ACLPermissionCredentialRead,
			apitypes.ACLPermissionFirmwareRead,
		},
	})
	if err != nil {
		t.Fatalf("create ACL role: %v", err)
	}
	if roleResp.JSON200 == nil {
		t.Fatalf("create ACL role status %d: %s", roleResp.StatusCode(), strings.TrimSpace(string(roleResp.Body)))
	}

	subject := apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindPk, Id: h.ContextPublicKey("peer-a")}
	for _, resource := range []apitypes.ACLResource{
		{Kind: apitypes.ACLResourceKindWorkflow, Id: "seed-flow"},
		{Kind: apitypes.ACLResourceKindWorkflow, Id: "peer-flow"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "seed-workspace"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "peer-workspace"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "social-direct-visible"},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: "social-group-visible"},
		{Kind: apitypes.ACLResourceKindModel, Id: "seed-model"},
		{Kind: apitypes.ACLResourceKindModel, Id: "peer-model"},
		{Kind: apitypes.ACLResourceKindCredential, Id: "seed-credential"},
		{Kind: apitypes.ACLResourceKindCredential, Id: "peer-credential"},
		{Kind: apitypes.ACLResourceKindFirmware, Id: "seed-firmware"},
	} {
		id := fmt.Sprintf("peer-resource-rpc-%s-%s", resource.Kind, resource.Id)
		resp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
			Id: &id,
			Policy: apitypes.ACLPolicy{
				Subject:  subject,
				Resource: resource,
				Role:     serverResourceRole,
			},
		})
		if err != nil {
			t.Fatalf("create ACL policy binding %s: %v", id, err)
		}
		if resp.JSON200 == nil {
			t.Fatalf("create ACL policy binding %s status %d: %s", id, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}

	if resp, err := api.CreateWorkflowWithResponse(ctx, apiWorkflow("seed-flow", "seeded workflow")); err != nil {
		t.Fatalf("seed workflow: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed workflow status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	var params apitypes.WorkspaceParameters
	if err := params.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{}); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters(seed) error = %v", err)
	}
	if resp, err := api.CreateWorkspaceWithResponse(ctx, adminservice.WorkspaceUpsert{
		Name:         "seed-workspace",
		WorkflowName: "seed-flow",
		Parameters:   &params,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed workspace status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	for _, name := range []string{"social-direct-visible", "social-direct-hidden", "social-group-visible"} {
		if resp, err := api.CreateWorkspaceWithResponse(ctx, adminservice.WorkspaceUpsert{
			Name:         name,
			WorkflowName: "seed-flow",
			Parameters:   &params,
		}); err != nil {
			t.Fatalf("seed workspace %s: %v", name, err)
		} else if resp.JSON200 == nil {
			t.Fatalf("seed workspace %s status %d: %s", name, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}
	if resp, err := api.CreateModelWithResponse(ctx, adminservice.ModelUpsert{
		Id:     "seed-model",
		Kind:   apitypes.ModelKindLlm,
		Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKindOpenaiTenant,
			Name: "seed-provider",
		},
	}); err != nil {
		t.Fatalf("seed model: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed model status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp, err := api.CreateCredentialWithResponse(ctx, adminservice.CredentialUpsert{
		Name:     "seed-credential",
		Provider: "openai",
		Body:     testOpenAICredentialBody("sk-seed"),
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed credential status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp, err := api.CreateFirmwareWithResponse(ctx, apiFirmware("seed-firmware", "1.0.0")); err != nil {
		t.Fatalf("seed firmware: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("seed firmware status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp, err := api.UploadFirmwareBinWithBodyWithResponse(ctx, "seed-firmware", adminservice.Stable, "main", "application/octet-stream", strings.NewReader("firmware-payload")); err != nil {
		t.Fatalf("upload firmware bin: %v", err)
	} else if resp.JSON200 == nil {
		t.Fatalf("upload firmware bin status %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
}

func seedBusinessResources(t *testing.T, h *clitest.Harness, openAIBaseURL string) {
	t.Helper()

	resourceJSON := fmt.Sprintf(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ResourceList",
		"metadata": {"name": "business-resources"},
		"spec": {
			"items": [
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Credential","metadata":{"name":"openai-key"},"spec":{"provider":"openai","body":{"api_key":"sk-e2e"}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"OpenAITenant","metadata":{"name":"openai-e2e"},"spec":{"kind":"compatible","credential_name":"openai-key","base_url":%q,"api_mode":"chat_completions"}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"name":"reward-claim"},"spec":{"kind":"llm","source":"manual","provider":{"kind":"openai-tenant","name":"openai-e2e"},"provider_data":{"upstream_model":"reward-e2e","support_json_output":true,"use_system_role":true}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"name":"pet-action"},"spec":{"kind":"llm","source":"manual","provider":{"kind":"openai-tenant","name":"openai-e2e"},"provider_data":{"upstream_model":"pet-e2e","support_json_output":true,"use_system_role":true}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Voice","metadata":{"name":"voice-a"},"spec":{"name":"Voice A","source":"manual","provider":{"kind":"openai-tenant","name":"openai-e2e"},"provider_data":{"voice_id":"alloy"}}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"PetSpecies","metadata":{"name":"rabbit"},"spec":{"name":"Rabbit"}},
				{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Badge","metadata":{"name":"founder"},"spec":{"name":"Founder","description":"first reward badge"}}
			]
		}
	}`, openAIBaseURL)
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	applyRPCResourceData(t, ctx, api, "business-resources", []byte(resourceJSON))
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

func applyRPCResourceFile(t *testing.T, api *adminservice.ClientWithResponses, resourcePath string) {
	t.Helper()

	data, err := os.ReadFile(resourcePath)
	if err != nil {
		t.Fatalf("read rpc resource %s: %v", resourcePath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	applyRPCResourceData(t, ctx, api, resourcePath, data)
}

func applyRPCResourceData(t *testing.T, ctx context.Context, api *adminservice.ClientWithResponses, label string, data []byte) {
	t.Helper()

	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		t.Fatalf("decode rpc resource YAML %s: %v", label, err)
	}
	var resource apitypes.Resource
	if err := json.Unmarshal(jsonData, &resource); err != nil {
		t.Fatalf("decode rpc resource %s: %v", label, err)
	}
	resp, err := api.ApplyResourceWithResponse(ctx, resource)
	if err != nil {
		t.Fatalf("apply rpc resource %s: %v", label, err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("apply rpc resource %s status %d: %s", label, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
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

func apiWorkflow(name, description string) apitypes.WorkflowDocument {
	spec := apitypes.FlowcraftWorkflowSpec{
		"entry_agent": "",
	}
	return apitypes.WorkflowDocument{
		Metadata: apitypes.WorkflowMetadata{
			Name:        name,
			Description: &description,
		},
		Spec: apitypes.WorkflowSpec{
			Driver:    apitypes.WorkflowDriverFlowcraft,
			Flowcraft: &spec,
		},
	}
}

func apiFirmware(name, version string) adminservice.FirmwareUpsert {
	artifacts := []apitypes.FirmwareArtifact{{
		Name: "main",
		Kind: apitypes.FirmwareArtifactKindApp,
	}}
	description := "seeded firmware"
	return adminservice.FirmwareUpsert{
		Name:        name,
		Description: &description,
		Slots: apitypes.FirmwareSlots{
			Stable: apitypes.FirmwareSlot{
				Version:   &version,
				Artifacts: &artifacts,
			},
		},
	}
}

func rpcWorkflow(name, description string) rpcapi.WorkflowDocument {
	spec := rpcapi.FlowcraftWorkflowSpec{
		"entry_agent": "",
	}
	return rpcapi.WorkflowDocument{
		Metadata: rpcapi.WorkflowMetadata{
			Name:        name,
			Description: &description,
		},
		Spec: rpcapi.WorkflowSpec{
			Driver:    rpcapi.WorkflowDriverFlowcraft,
			Flowcraft: &spec,
		},
	}
}

func rpcChatRoomWorkflow(name, description string) rpcapi.WorkflowDocument {
	return rpcapi.WorkflowDocument{
		Metadata: rpcapi.WorkflowMetadata{
			Name:        name,
			Description: &description,
		},
		Spec: rpcapi.WorkflowSpec{
			Driver:   rpcapi.WorkflowDriverChatroom,
			Chatroom: &rpcapi.ChatRoomWorkflowSpec{History: rpcapi.ChatRoomWorkflowHistorySpec{}},
		},
	}
}

func rpcModel(id, providerName string) rpcapi.Model {
	return rpcapi.Model{
		Id:     id,
		Kind:   rpcapi.ModelKindLlm,
		Source: rpcapi.ModelSourceManual,
		Provider: rpcapi.ModelProvider{
			Kind: rpcapi.ModelProviderKindOpenaiTenant,
			Name: providerName,
		},
	}
}

func rpcCredential(name, apiKey string) rpcapi.Credential {
	return rpcapi.Credential{
		Name:     name,
		Provider: "openai",
		Body:     testRPCOpenAICredentialBody(apiKey),
	}
}

func assertWorkflowPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListWorkflows(ctx, "workflow.list.page1", rpcapi.WorkflowListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("workflow.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("workflow.list page1 = %#v", first)
	}
	second, err := peer.ListWorkflows(ctx, "workflow.list.page2", rpcapi.WorkflowListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("workflow.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Metadata.Name] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("workflow list pagination got names %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertWorkspacePagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListWorkspaces(ctx, "workspace.list.page1", rpcapi.WorkspaceListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("workspace.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("workspace.list page1 = %#v", first)
	}
	second, err := peer.ListWorkspaces(ctx, "workspace.list.page2", rpcapi.WorkspaceListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("workspace.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Name] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("workspace list pagination got names %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertWorkspacePrefixList(t *testing.T, ctx context.Context, peer *gizcli.Client) {
	t.Helper()

	limit := 10
	prefix := "social-direct-"
	list, err := peer.ListWorkspaces(ctx, "workspace.list.prefix", rpcapi.WorkspaceListRequest{Prefix: &prefix, Limit: &limit})
	if err != nil {
		t.Fatalf("workspace.list prefix: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "social-direct-visible" {
		t.Fatalf("workspace.list prefix items = %#v", list.Items)
	}
	if hasWorkspace(list.Items, "social-direct-hidden") || hasWorkspace(list.Items, "social-group-visible") {
		t.Fatalf("workspace.list prefix leaked workspace = %#v", list.Items)
	}
}

func assertModelPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListModels(ctx, "model.list.page1", rpcapi.ModelListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("model.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("model.list page1 = %#v", first)
	}
	second, err := peer.ListModels(ctx, "model.list.page2", rpcapi.ModelListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("model.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Id] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("model list pagination got ids %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertCredentialPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	first, err := peer.ListCredentials(ctx, "credential.list.page1", rpcapi.CredentialListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("credential.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("credential.list page1 = %#v", first)
	}
	second, err := peer.ListCredentials(ctx, "credential.list.page2", rpcapi.CredentialListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("credential.list page2: %v", err)
	}
	got := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		got[item.Name] = true
	}
	if !got[wantA] || !got[wantB] {
		t.Fatalf("credential list pagination got names %#v, want %q and %q", got, wantA, wantB)
	}
}

func assertDeniedListsAreEmpty(t *testing.T, ctx context.Context, denied *gizcli.Client) {
	t.Helper()

	workflows, err := denied.ListWorkflows(ctx, "workflow.list.denied", rpcapi.WorkflowListRequest{})
	if err != nil {
		t.Fatalf("denied workflow.list: %v", err)
	}
	if len(workflows.Items) != 0 {
		t.Fatalf("denied workflow.list items = %#v", workflows.Items)
	}
	workspaces, err := denied.ListWorkspaces(ctx, "workspace.list.denied", rpcapi.WorkspaceListRequest{})
	if err != nil {
		t.Fatalf("denied workspace.list: %v", err)
	}
	if len(workspaces.Items) != 0 {
		t.Fatalf("denied workspace.list items = %#v", workspaces.Items)
	}
	models, err := denied.ListModels(ctx, "model.list.denied", rpcapi.ModelListRequest{})
	if err != nil {
		t.Fatalf("denied model.list: %v", err)
	}
	if len(models.Items) != 0 {
		t.Fatalf("denied model.list items = %#v", models.Items)
	}
	credentials, err := denied.ListCredentials(ctx, "credential.list.denied", rpcapi.CredentialListRequest{})
	if err != nil {
		t.Fatalf("denied credential.list: %v", err)
	}
	if len(credentials.Items) != 0 {
		t.Fatalf("denied credential.list items = %#v", credentials.Items)
	}
}

func assertPetPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantIDs []string) {
	t.Helper()

	first, err := peer.ListPets(ctx, "pet.list.page1", rpcapi.PetListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("pet.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("pet.list page1 = %#v", first)
	}
	second, err := peer.ListPets(ctx, "pet.list.page2", rpcapi.PetListRequest{Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("pet.list page2: %v", err)
	}
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("pet.list page2 = %#v", second)
	}
	got := []string{first.Items[0].Id, second.Items[0].Id}
	if !sameStringSet(got, wantIDs) {
		t.Fatalf("pet.list pages ids = %#v, want %#v", got, wantIDs)
	}
}

func assertRewardPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantIDs []string) {
	t.Helper()

	first, err := peer.ListRewards(ctx, "reward.list.page1", rpcapi.RewardListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("reward.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("reward.list page1 = %#v", first)
	}
	second, err := peer.ListRewards(ctx, "reward.list.page2", rpcapi.RewardListRequest{Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("reward.list page2: %v", err)
	}
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("reward.list page2 = %#v", second)
	}
	got := []string{first.Items[0].Id, second.Items[0].Id}
	if !sameStringSet(got, wantIDs) {
		t.Fatalf("reward.list pages ids = %#v, want %#v", got, wantIDs)
	}
}

func assertWalletTransactionPagination(t *testing.T, ctx context.Context, peer *gizcli.Client) string {
	t.Helper()

	first, err := peer.ListWalletTransactions(ctx, "wallet.transactions.list.page1", rpcapi.WalletTransactionsListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("wallet.transactions.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("wallet.transactions.list page1 = %#v", first)
	}
	second, err := peer.ListWalletTransactions(ctx, "wallet.transactions.list.page2", rpcapi.WalletTransactionsListRequest{Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("wallet.transactions.list page2: %v", err)
	}
	if len(second.Items) != 1 {
		t.Fatalf("wallet.transactions.list page2 = %#v", second)
	}
	got, err := peer.GetWalletTransaction(ctx, "wallet.transactions.get", rpcapi.WalletTransactionsGetRequest{Id: first.Items[0].Id})
	if err != nil {
		t.Fatalf("wallet.transactions.get: %v", err)
	}
	if got.Id != first.Items[0].Id {
		t.Fatalf("wallet.transactions.get id = %q, want %q", got.Id, first.Items[0].Id)
	}
	return got.Id
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

func hasWorkflow(items []rpcapi.WorkflowDocument, name string) bool {
	for _, item := range items {
		if item.Metadata.Name == name {
			return true
		}
	}
	return false
}

func hasWorkspace(items []rpcapi.Workspace, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasModel(items []rpcapi.Model, id string) bool {
	for _, item := range items {
		if item.Id == id {
			return true
		}
	}
	return false
}

func hasCredential(items []rpcapi.Credential, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
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
