//go:build gizclaw_e2e

package gameplay_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

type isolatedGameplayHarness struct {
	ctx  context.Context
	h    *clitest.Harness
	peer *gizcli.Client
}

func newIsolatedGameplayHarness(t *testing.T) *isolatedGameplayHarness {
	t.Helper()

	h := clitest.NewHarnessForRoot(t, "tests/gizclaw-e2e/go/gameplay", "client-gameplay")
	h.StartServerFromFixture("server_config.yaml")
	h.InstallFixedAdminContext("admin-a").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "client-gameplay-peer-a-sn").MustSucceed(t)
	applyGameplayCatalog(t, h)
	applyGameplayACL(t, h, "peer-a")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })
	return &isolatedGameplayHarness{ctx: ctx, h: h, peer: peer}
}

func applyGameplayCatalog(t *testing.T, h *clitest.Harness) {
	t.Helper()

	for _, fixture := range []string{
		filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "resources", "04-workflows", "23-flowcraft-pet-care.yaml"),
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
	createGameplayViewBinding(t, ctx, api, "gameplay-default-ruleset-"+h.ContextPublicKey(contextName), apitypes.ACLResourceKindGameruleset, "default-gameplay")
}

func createGameplayViewBinding(t *testing.T, ctx context.Context, api *adminservice.ClientWithResponses, id string, kind apitypes.ACLResourceKind, resourceID string) {
	t.Helper()

	bindingResp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
		Id: &id,
		Policy: apitypes.ACLPolicy{
			Subject:  apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindView, Id: "default-client"},
			Resource: apitypes.ACLResource{Kind: kind, Id: resourceID},
			Role:     "default-client",
		},
	})
	if err != nil {
		t.Fatalf("create gameplay ACL binding %s: %v", id, err)
	}
	if bindingResp.JSON200 == nil && bindingResp.JSON409 == nil {
		t.Fatalf("create gameplay ACL binding %s status %d: %s", id, bindingResp.StatusCode(), strings.TrimSpace(string(bindingResp.Body)))
	}
}

type setupGameplayHarness struct {
	ctx  context.Context
	h    *clitest.Harness
	peer *gizcli.Client
}

func newSetupGameplayHarness(t *testing.T, clientName string) *setupGameplayHarness {
	t.Helper()

	h := clitest.NewSetupHarness(t, clientName)
	identitiesHome := getenvDefault("GIZCLAW_E2E_IDENTITIES_HOME", filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities"))
	contextName := getenvDefault("GIZCLAW_E2E_PEER_IDENTITY", "peer")
	h.SetContextDirAlias("gear1", filepath.Join(identitiesHome, contextName))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("gear1")
	t.Cleanup(func() { peer.Close() })
	return &setupGameplayHarness{ctx: ctx, h: h, peer: peer}
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
