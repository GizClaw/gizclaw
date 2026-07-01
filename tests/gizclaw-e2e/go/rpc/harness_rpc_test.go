//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"encoding/json"
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

const (
	sharedWorkflow          = "flowcraft-support"
	sharedChatroomWorkflow  = "chatroom-direct"
	sharedWorkspace         = "support-desk-workspace"
	sharedChatroomWorkspace = "direct-chatroom-workspace"
	sharedModel             = "fake-openai-chat-000"
	sharedCredential        = "fake-openai-credential-000"
	sharedFirmware          = "devkit-firmware-main"
	mutationWorkflow        = "mutation-rpc-workflow"
	mutationWorkspace       = "mutation-rpc-workspace"
	mutationModel           = "mutation-openai-model"
	mutationCredential      = "mutation-openai-credential"
)

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

	h := clitest.NewSetupHarness(t, "client-rpc-server-resource")
	aliasSetupAdminContext(t, h)
	registerSetupPeer(t, h, "peer-a", "peer-a-sn", true)
	registerSetupPeer(t, h, "peer-denied", "peer-denied-sn", false)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })
	return &serverResourceHarness{h: h, ctx: ctx, peer: peer}
}

func newSocialRPCHarness(t *testing.T) *socialRPCHarness {
	t.Helper()

	h := clitest.NewSetupHarness(t, "client-rpc-social")
	aliasSetupAdminContext(t, h)
	for _, peer := range []string{"peer-a", "peer-b", "peer-c", "peer-d"} {
		registerSetupPeer(t, h, peer, "client-rpc-social-"+peer+"-sn", true)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	h := clitest.NewSetupHarness(t, "client-rpc-business")
	aliasSetupAdminContext(t, h)
	registerSetupPeer(t, h, "peer-a", "client-rpc-business-peer-a-sn", true)
	registerSetupPeer(t, h, "peer-b", "client-rpc-business-peer-b-sn", true)
	requireBusinessCatalog(t, h)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	a := h.ConnectClientFromContext("peer-a")
	b := h.ConnectClientFromContext("peer-b")
	t.Cleanup(func() { a.Close() })
	t.Cleanup(func() { b.Close() })
	return &businessRPCHarness{ctx: ctx, a: a, b: b}
}

func aliasSetupAdminContext(t *testing.T, h *clitest.Harness) {
	t.Helper()

	identitiesHome := getenvDefault("GIZCLAW_E2E_IDENTITIES_HOME", filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities"))
	contextName := getenvDefault("GIZCLAW_E2E_ADMIN_IDENTITY", "admin")
	h.SetContextDirAlias("admin-a", filepath.Join(identitiesHome, contextName))
}

func registerSetupPeer(t *testing.T, h *clitest.Harness, contextName, serial string, defaultClientView bool) {
	t.Helper()

	h.CreateContext(contextName).MustSucceed(t)
	h.RegisterContext(contextName, "--sn", serial).MustSucceed(t)
	if defaultClientView {
		applyDefaultClientView(t, h, contextName)
	}
}

func applyDefaultClientView(t *testing.T, h *clitest.Harness, contextName string) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	view := "default-client"
	resp, err := api.PutPeerConfigWithResponse(ctx, h.ContextPublicKey(contextName), apitypes.Configuration{
		View: &view,
	})
	if err != nil {
		t.Fatalf("put peer config for %s: %v", contextName, err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("put peer config for %s status %d: %s", contextName, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	applyPeerWorkspacePrefixBinding(t, h, api, contextName)
}

func applyPeerWorkspacePrefixBinding(t *testing.T, h *clitest.Harness, api *adminservice.ClientWithResponses, contextName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	peerPublicKey := h.ContextPublicKey(contextName)
	id := "e2e-rpc-workspace-prefix-" + peerPublicKey + "-" + sharedChatroomWorkspace
	resp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
		Id: &id,
		Policy: apitypes.ACLPolicy{
			Subject: apitypes.ACLSubject{
				Kind: apitypes.ACLSubjectKindPk,
				Id:   peerPublicKey,
			},
			Resource: apitypes.ACLResource{
				Kind: apitypes.ACLResourceKindWorkspace,
				Id:   sharedChatroomWorkspace,
			},
			Role: "standard-client",
		},
	})
	if err != nil {
		t.Fatalf("create workspace prefix ACL binding for %s: %v", contextName, err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("create workspace prefix ACL binding for %s status %d: %s", contextName, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
}

func requireBusinessCatalog(t *testing.T, h *clitest.Harness) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range []string{"reward-claim", "pet-action"} {
		resp, err := api.GetModelWithResponse(ctx, id)
		if err != nil {
			t.Fatalf("business model.get %s: %v", id, err)
		}
		if resp.JSON200 == nil {
			t.Skipf("business RPC e2e requires reset_data to apply OpenAI system task model %q; status=%d body=%s", id, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}
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
	got := map[string]bool{}
	var cursor *string
	for page := 0; page < 300; page++ {
		list, err := peer.ListWorkflows(ctx, "workflow.list.page", rpcapi.WorkflowListRequest{Limit: &limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("workflow.list page %d: %v", page, err)
		}
		if len(list.Items) > limit {
			t.Fatalf("workflow.list page %d len = %d, want <= %d", page, len(list.Items), limit)
		}
		for _, item := range list.Items {
			got[item.Metadata.Name] = true
		}
		if got[wantA] && got[wantB] {
			return
		}
		if !list.HasNext {
			break
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("workflow.list page %d has_next without cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatalf("workflow list pagination got names %#v, want %q and %q", got, wantA, wantB)
}

func assertWorkspacePagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	got := map[string]bool{}
	var cursor *string
	for page := 0; page < 300; page++ {
		list, err := peer.ListWorkspaces(ctx, "workspace.list.page", rpcapi.WorkspaceListRequest{Limit: &limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("workspace.list page %d: %v", page, err)
		}
		if len(list.Items) > limit {
			t.Fatalf("workspace.list page %d len = %d, want <= %d", page, len(list.Items), limit)
		}
		for _, item := range list.Items {
			got[item.Name] = true
		}
		if got[wantA] && got[wantB] {
			return
		}
		if !list.HasNext {
			break
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("workspace.list page %d has_next without cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatalf("workspace list pagination got names %#v, want %q and %q", got, wantA, wantB)
}

func assertWorkspacePrefixList(t *testing.T, ctx context.Context, peer *gizcli.Client) {
	t.Helper()

	limit := 10
	prefix := "direct-chatroom-"
	list, err := peer.ListWorkspaces(ctx, "workspace.list.prefix", rpcapi.WorkspaceListRequest{Prefix: &prefix, Limit: &limit})
	if err != nil {
		t.Fatalf("workspace.list prefix: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != sharedChatroomWorkspace {
		t.Fatalf("workspace.list prefix items = %#v", list.Items)
	}
}

func assertModelPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	got := map[string]bool{}
	var cursor *string
	for page := 0; page < 300; page++ {
		list, err := peer.ListModels(ctx, "model.list.page", rpcapi.ModelListRequest{Limit: &limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("model.list page %d: %v", page, err)
		}
		if len(list.Items) > limit {
			t.Fatalf("model.list page %d len = %d, want <= %d", page, len(list.Items), limit)
		}
		for _, item := range list.Items {
			got[item.Id] = true
		}
		if got[wantA] && got[wantB] {
			return
		}
		if !list.HasNext {
			break
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("model.list page %d has_next without cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatalf("model list pagination got ids %#v, want %q and %q", got, wantA, wantB)
}

func assertCredentialPagination(t *testing.T, ctx context.Context, peer *gizcli.Client, wantA, wantB string) {
	t.Helper()

	limit := 1
	got := map[string]bool{}
	var cursor *string
	for page := 0; page < 300; page++ {
		list, err := peer.ListCredentials(ctx, "credential.list.page", rpcapi.CredentialListRequest{Limit: &limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("credential.list page %d: %v", page, err)
		}
		if len(list.Items) > limit {
			t.Fatalf("credential.list page %d len = %d, want <= %d", page, len(list.Items), limit)
		}
		for _, item := range list.Items {
			got[item.Name] = true
		}
		if got[wantA] && got[wantB] {
			return
		}
		if !list.HasNext {
			break
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("credential.list page %d has_next without cursor: %#v", page, list)
		}
		cursor = list.NextCursor
	}
	t.Fatalf("credential list pagination got names %#v, want %q and %q", got, wantA, wantB)
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

func assertWalletTransaction(t *testing.T, ctx context.Context, peer *gizcli.Client) string {
	t.Helper()

	first, err := peer.ListWalletTransactions(ctx, "wallet.transactions.list", rpcapi.WalletTransactionsListRequest{Limit: 1})
	if err != nil {
		t.Fatalf("wallet.transactions.list: %v", err)
	}
	if len(first.Items) != 1 || first.HasNext {
		t.Fatalf("wallet.transactions.list = %#v", first)
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
