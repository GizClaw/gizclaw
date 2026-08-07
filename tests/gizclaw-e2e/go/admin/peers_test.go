//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminAPIPeersListGetAndLookup(t *testing.T) {
	env := newAdminAPIHarness(t)

	limit := int32(10)
	list, err := env.api.ListPeersWithResponse(env.ctx, &adminhttp.ListPeersParams{Limit: &limit})
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	requireStatusOK(t, list, list.Body)
	if list.JSON200 == nil || len(list.JSON200.Items) == 0 {
		t.Fatalf("list peers = %#v", list.JSON200)
	}

	get, err := env.api.GetPeerWithResponse(env.ctx, env.peerKey)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil {
		t.Fatalf("get peer = %#v", get.JSON200)
	}
	registration, err := get.JSON200.AsExternalRef0Registration()
	if err != nil || registration.PublicKey != env.peerKey {
		t.Fatalf("get peer registration = %#v, %v", registration, err)
	}

	found, err := env.api.FindPubKeyBySNWithResponse(env.ctx, env.peerSN)
	if err != nil {
		t.Fatalf("find peer by SN: %v", err)
	}
	requireStatusOK(t, found, found.Body)
	if found.JSON200 == nil || found.JSON200.PublicKey != env.peerKey {
		t.Fatalf("find peer by SN = %#v", found.JSON200)
	}
}

func TestPeerDeletionFinalizesPermanentTombstone(t *testing.T) {
	env := newAdminAPIHarness(t)
	testCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	env.ctx = testCtx
	peerClient := env.h.ConnectClientFromContext("admin-api-peer")
	defer peerClient.Close()
	profile, found, err := clitest.RuntimeProfileByID(env.ctx, env.api, "default-gameplay")
	if err != nil || !found {
		t.Fatalf("resolve deletion RuntimeProfile: found=%v error=%v", found, err)
	}
	if profile.Spec.Workflows.Collections == nil {
		profile.Spec.Workflows.Collections = apitypes.RuntimeProfileWorkflowCollections{}
	}
	profile.Spec.Workflows.Collections["deletion"] = map[string]apitypes.RuntimeProfileBinding{
		"workspace": {
			ResourceId: "flowcraft-scenario-000",
			I18n: map[string]apitypes.RuntimeProfileI18nText{
				"en":    {DisplayName: "Deletion Workspace"},
				"zh-CN": {DisplayName: "删除工作区"},
			},
		},
	}
	if _, err := clitest.UpsertRuntimeProfile(env.ctx, env.api, adminhttp.RuntimeProfileUpsert{Id: profile.Id, Spec: profile.Spec}); err != nil {
		t.Fatalf("update deletion RuntimeProfile: %v", err)
	}
	tokenID := mutationName("peer-delete-token")
	if err := clitest.DeleteRegistrationTokenByID(env.ctx, env.api, tokenID); err != nil {
		t.Fatalf("retire old deletion RegistrationToken: %v", err)
	}
	token, err := env.api.CreateRegistrationTokenWithResponse(env.ctx, adminhttp.RegistrationTokenUpsert{
		Id: tokenID, Token: tokenID, RuntimeProfileId: profile.Id,
	})
	if err != nil || token.JSON200 == nil {
		t.Fatalf("create deletion RegistrationToken: status=%d body=%s error=%v", token.StatusCode(), token.Body, err)
	}
	if _, err := env.admin.Register(env.ctx, "peer.delete.register.admin", token.JSON200.Token); err != nil {
		t.Fatalf("register foreign Peer RuntimeProfile: %v", err)
	}
	if _, err := peerClient.Register(env.ctx, "peer.delete.register", token.JSON200.Token); err != nil {
		t.Fatalf("register deletion RuntimeProfile: %v", err)
	}

	input := rpcapi.WorkspaceInputModePushToTalk
	var workspaceParameters rpcapi.WorkspaceParameters
	if err := workspaceParameters.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{
		AgentType: rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Input:     &input,
	}); err != nil {
		t.Fatal(err)
	}
	userWorkspaceName := mutationName("peer-delete-workspace")
	userWorkspace, err := peerClient.CreateWorkspace(env.ctx, "peer.delete.workspace", rpcapi.WorkspaceCreateRequest{
		Name: userWorkspaceName, Collection: "deletion", WorkflowName: "workspace", Parameters: &workspaceParameters,
	})
	if err != nil {
		t.Fatalf("seed owned Workspace: %v", err)
	}
	storedUserWorkspace, found, err := clitest.WorkspaceByName(env.ctx, env.api, userWorkspace.Name)
	if err != nil || !found {
		t.Fatalf("resolve owned Workspace: found=%v error=%v", found, err)
	}
	petName := mutationName("peer-delete-pet")
	adopted, err := peerClient.AdoptPet(env.ctx, "peer.delete.pet", rpcapi.RuntimeAdoptRequest{Name: petName, DisplayName: "Delete Me"})
	if err != nil {
		t.Fatalf("seed owned Pet: %v", err)
	}
	storedPet := requirePeerDeletionPetByName(t, env, adopted.Pet.Name)

	ownedContactID := mutationName("peer-delete-owned")
	foreignContactID := mutationName("peer-delete-foreign")
	for _, body := range []adminhttp.AdminContactCreateRequest{
		{Id: ownedContactID, OwnerPublicKey: env.peerKey, Name: ownedContactID, DisplayName: ptr("Owned")},
		{Id: foreignContactID, OwnerPublicKey: env.adminKey, Name: foreignContactID, DisplayName: ptr("Foreign")},
	} {
		response, err := env.api.CreateContactWithResponse(env.ctx, body)
		if err != nil || response.StatusCode() != http.StatusOK {
			t.Fatalf("seed Contact %q: status=%d body=%s error=%v", body.Id, response.StatusCode(), response.Body, err)
		}
	}
	t.Cleanup(func() { _, _ = env.api.DeleteContactWithResponse(context.Background(), env.adminKey, foreignContactID) })

	relationID := adminAPIRelationID(env.adminKey, env.peerKey)
	_, _ = env.api.DeletePeerFriendWithResponse(env.ctx, env.adminKey, relationID)
	directFriend, err := env.api.CreatePeerFriendWithResponse(env.ctx, env.adminKey, adminhttp.AdminFriendCreateRequest{PeerPublicKey: env.peerKey})
	if err != nil || directFriend.JSON200 == nil {
		t.Fatalf("seed direct Friend: status=%d body=%s error=%v", directFriend.StatusCode(), directFriend.Body, err)
	}
	ownedGroupID := mutationName("peer-delete-owned-group")
	ownedGroup, err := env.api.CreateFriendGroupWithResponse(env.ctx, adminhttp.AdminFriendGroupCreateRequest{
		Id: ownedGroupID, Name: ownedGroupID, OwnerPublicKey: env.peerKey,
	})
	if err != nil || ownedGroup.JSON200 == nil {
		t.Fatalf("seed owned Friend Group: status=%d body=%s error=%v", ownedGroup.StatusCode(), ownedGroup.Body, err)
	}
	foreignGroupID := mutationName("peer-delete-foreign-group")
	foreignGroup, err := env.api.CreateFriendGroupWithResponse(env.ctx, adminhttp.AdminFriendGroupCreateRequest{
		Id: foreignGroupID, Name: foreignGroupID, OwnerPublicKey: env.adminKey,
	})
	if err != nil || foreignGroup.JSON200 == nil {
		t.Fatalf("seed foreign Friend Group: status=%d body=%s error=%v", foreignGroup.StatusCode(), foreignGroup.Body, err)
	}
	foreignMember, err := env.api.CreateFriendGroupMemberWithResponse(env.ctx, foreignGroupID, adminhttp.AdminFriendGroupMemberCreateRequest{
		Id: customid.MembershipName(foreignGroupID, env.peerKey), Name: foreignGroupID,
		PeerPublicKey: env.peerKey, Role: rpcapi.FriendGroupMemberRoleMember,
	})
	if err != nil || foreignMember.JSON200 == nil {
		t.Fatalf("seed foreign Friend Group membership: status=%d body=%s error=%v", foreignMember.StatusCode(), foreignMember.Body, err)
	}

	deleted, err := env.api.DeletePeerWithResponse(env.ctx, env.peerKey)
	if err != nil || deleted.StatusCode() != http.StatusOK || deleted.JSON200 == nil {
		t.Fatalf("delete Peer: status=%d body=%s error=%v", deleted.StatusCode(), deleted.Body, err)
	}
	if diagnostic, err := env.api.GetPeerWithResponse(env.ctx, env.peerKey); err != nil || diagnostic.StatusCode() != http.StatusOK {
		t.Fatalf("Admin Peer get must remain available during retirement: status=%d body=%s error=%v", diagnostic.StatusCode(), diagnostic.Body, err)
	}
	if blockedPet, err := env.api.GetPeerPetWithResponse(env.ctx, env.peerKey, storedPet.Id); err != nil || blockedPet.StatusCode() != http.StatusConflict {
		t.Fatalf("post-marker Pet read: status=%d body=%s error=%v", blockedPet.StatusCode(), blockedPet.Body, err)
	}
	requirePeerLoginConflict(t, env, env.peerKey, "admin-api-peer")

	blockedContact, err := env.api.CreateContactWithResponse(env.ctx, adminhttp.AdminContactCreateRequest{
		Id: mutationName("peer-delete-blocked"), OwnerPublicKey: env.peerKey,
		Name: mutationName("peer-delete-blocked"), DisplayName: ptr("Blocked"),
	})
	if err != nil || blockedContact.StatusCode() != http.StatusConflict || blockedContact.JSON409 == nil {
		t.Fatalf("post-marker Contact: status=%d body=%s error=%v", blockedContact.StatusCode(), blockedContact.Body, err)
	}
	if blockedContact.JSON409.Error.Code != "PEER_PENDING_DELETION" && blockedContact.JSON409.Error.Code != "PEER_DELETED" {
		t.Fatalf("post-marker Contact code = %q", blockedContact.JSON409.Error.Code)
	}
	if blockedFriend, err := env.api.DeletePeerFriendWithResponse(env.ctx, env.adminKey, relationID); err != nil || blockedFriend.StatusCode() != http.StatusConflict || blockedFriend.JSON409 == nil {
		t.Fatalf("post-marker Friend mutation: status=%d body=%s error=%v", blockedFriend.StatusCode(), blockedFriend.Body, err)
	}
	if blockedGroup, err := env.api.DeleteFriendGroupWithResponse(env.ctx, ownedGroupID); err != nil || blockedGroup.StatusCode() != http.StatusConflict || blockedGroup.JSON409 == nil {
		t.Fatalf("post-marker Friend Group mutation: status=%d body=%s error=%v", blockedGroup.StatusCode(), blockedGroup.Body, err)
	}
	if _, err := peerClient.GetWorkspace(env.ctx, "peer.delete.fenced", rpcapi.WorkspaceGetRequest{Name: userWorkspaceName}); err == nil {
		t.Fatal("post-marker RPC work reached the retiring Peer")
	}

	deadline := time.Now().Add(2 * time.Minute)
	var tombstone apitypes.RegistrationTombstone
	for {
		get, getErr := env.api.GetPeerWithResponse(env.ctx, env.peerKey)
		if getErr != nil {
			t.Fatalf("get retiring Peer: %v", getErr)
		}
		if get.StatusCode() == http.StatusOK && get.JSON200 != nil {
			var projection map[string]any
			if err := json.Unmarshal(get.Body, &projection); err != nil {
				t.Fatalf("decode Admin Peer projection %s: %v", get.Body, err)
			}
			if projection["status"] == string(apitypes.RegistrationTombstoneStatusDeleted) {
				candidate, decodeErr := get.JSON200.AsExternalRef0RegistrationTombstone()
				if decodeErr != nil {
					t.Fatalf("decode Admin tombstone projection %s: %v", get.Body, decodeErr)
				}
				tombstone = candidate
				if len(projection) != 2 {
					t.Fatalf("Admin tombstone projection = %s", get.Body)
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("Peer deletion did not complete: last status=%d body=%s", get.StatusCode(), get.Body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if tombstone.PublicKey != env.peerKey || tombstone.Status != apitypes.RegistrationTombstoneStatusDeleted {
		t.Fatalf("tombstone = %#v", tombstone)
	}

	owned, err := env.api.GetContactWithResponse(env.ctx, env.peerKey, ownedContactID)
	if err != nil || owned.StatusCode() != http.StatusNotFound {
		t.Fatalf("owned Contact after retirement: status=%d body=%s error=%v", owned.StatusCode(), owned.Body, err)
	}
	foreign, err := env.api.GetContactWithResponse(env.ctx, env.adminKey, foreignContactID)
	if err != nil || foreign.StatusCode() != http.StatusOK {
		t.Fatalf("foreign Contact after retirement: status=%d body=%s error=%v", foreign.StatusCode(), foreign.Body, err)
	}
	if got, err := env.api.GetWorkspaceWithResponse(env.ctx, storedUserWorkspace.Id); err != nil || got.StatusCode() != http.StatusNotFound {
		t.Fatalf("owned Workspace after retirement: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	if got, err := env.api.GetWorkspaceWithResponse(env.ctx, storedPet.WorkspaceId); err != nil || got.StatusCode() != http.StatusNotFound {
		t.Fatalf("Pet system Workspace after retirement: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	if got, err := env.api.GetPeerPetWithResponse(env.ctx, env.peerKey, storedPet.Id); err != nil || got.StatusCode() != http.StatusConflict {
		t.Fatalf("retired Peer Pet endpoint: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	requirePeerGameplayPurged(t, env.peerKey, storedPet.Id)
	if got, err := env.api.GetPeerFriendWithResponse(env.ctx, env.adminKey, relationID); err != nil || got.StatusCode() != http.StatusNotFound {
		t.Fatalf("direct Friend after retirement: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	if got, err := env.api.GetFriendGroupWithResponse(env.ctx, ownedGroupID); err != nil || got.StatusCode() != http.StatusNotFound {
		t.Fatalf("owned Friend Group after retirement: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	if got, err := env.api.GetWorkspaceWithResponse(env.ctx, directFriend.JSON200.WorkspaceId); err != nil || got.StatusCode() != http.StatusNotFound {
		t.Fatalf("Direct Chatroom Workspace after retirement: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	if ownedGroup.JSON200.WorkspaceId == nil {
		t.Fatal("owned Friend Group has no Workspace")
	}
	if got, err := env.api.GetWorkspaceWithResponse(env.ctx, *ownedGroup.JSON200.WorkspaceId); err != nil || got.StatusCode() != http.StatusNotFound {
		t.Fatalf("Group Chatroom Workspace after retirement: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	if got, err := env.api.GetFriendGroupWithResponse(env.ctx, foreignGroupID); err != nil || got.StatusCode() != http.StatusOK {
		t.Fatalf("foreign Friend Group after retirement: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	members, err := env.api.ListFriendGroupMembersWithResponse(env.ctx, foreignGroupID, nil)
	if err != nil || members.JSON200 == nil {
		t.Fatalf("foreign members after retirement: status=%d body=%s error=%v", members.StatusCode(), members.Body, err)
	}
	hasRetiredPeer, hasForeignOwner := false, false
	for _, member := range members.JSON200.Items {
		hasRetiredPeer = hasRetiredPeer || member.PeerPublicKey == env.peerKey
		hasForeignOwner = hasForeignOwner || member.PeerPublicKey == env.adminKey
	}
	if hasRetiredPeer || !hasForeignOwner {
		t.Fatalf("foreign memberships after retirement = %#v", members.JSON200.Items)
	}
	if got, err := env.api.GetRegistrationTokenWithResponse(env.ctx, tokenID); err != nil || got.StatusCode() != http.StatusOK {
		t.Fatalf("global RegistrationToken was deleted: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}
	if got, err := env.api.GetWorkflowWithResponse(env.ctx, "flowcraft-scenario-000"); err != nil || got.StatusCode() != http.StatusOK {
		t.Fatalf("global Workflow was deleted: status=%d body=%s error=%v", got.StatusCode(), got.Body, err)
	}

	source := "peer"
	kind := apitypes.PendingDeletionKindPeer
	tasks, err := env.api.ListPendingDeletionsWithResponse(env.ctx, &adminhttp.ListPendingDeletionsParams{Source: &source, Kind: &kind})
	if err != nil || tasks.StatusCode() != http.StatusOK || tasks.JSON200 == nil {
		t.Fatalf("list Peer tasks: status=%d body=%s error=%v", tasks.StatusCode(), tasks.Body, err)
	}
	for _, task := range tasks.JSON200.Items {
		if task.ResourceId == env.peerKey {
			t.Fatalf("completed Peer retained mutable task: %#v", task)
		}
	}

	repeated, err := env.api.DeletePeerWithResponse(env.ctx, env.peerKey)
	if err != nil || repeated.StatusCode() != http.StatusOK || repeated.JSON200 == nil {
		t.Fatalf("repeat Peer delete: status=%d body=%s error=%v", repeated.StatusCode(), repeated.Body, err)
	}
	repeatedTombstone, err := repeated.JSON200.AsExternalRef0RegistrationTombstone()
	if err != nil || repeatedTombstone != tombstone {
		t.Fatalf("repeat delete tombstone = %#v, %v", repeatedTombstone, err)
	}
	requirePeerLoginConflict(t, env, env.peerKey, "admin-api-peer")
	if result := env.h.RegisterContext("admin-api-peer", "--sn", env.peerSN); result.Err == nil {
		t.Fatalf("deleted public key registered again: stdout=%s stderr=%s", result.Stdout, result.Stderr)
	}
}

func requirePeerGameplayPurged(t *testing.T, publicKey, petID string) {
	t.Helper()
	project := os.Getenv("GIZCLAW_E2E_DOCKER_PROJECT")
	if project == "" {
		t.Fatal("GIZCLAW_E2E_DOCKER_PROJECT is required for direct Gameplay verification")
	}
	lookup := exec.CommandContext(t.Context(), "docker", "ps", "-q",
		"--filter", "label=com.docker.compose.project="+project,
		"--filter", "label=com.docker.compose.service=server")
	container, err := lookup.Output()
	if err != nil || strings.TrimSpace(string(container)) == "" {
		t.Fatalf("resolve E2E Server container: output=%q error=%v", container, err)
	}
	command := exec.CommandContext(t.Context(), "docker", "exec", "-w", "/src", strings.TrimSpace(string(container)),
		"go", "run", "./tests/gizclaw-e2e/cmd/assert-peer-gameplay-deleted",
		"--db", "/src/tests/gizclaw-e2e/testdata/server-workspace/data/gameplay.sqlite",
		"--owner", publicKey, "--pet", petID)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("verify retired Peer Gameplay storage: %s: %v", output, err)
	}
}

func requirePeerDeletionPetByName(t *testing.T, env *adminAPIHarness, name string) apitypes.Pet {
	t.Helper()
	response, err := env.api.ListPeerPetsWithResponse(env.ctx, env.peerKey, nil)
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list Peer Pets: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	for _, pet := range response.JSON200.Items {
		if pet.Name == name {
			return pet
		}
	}
	t.Fatalf("Peer Pet %q not found in %#v", name, response.JSON200.Items)
	return apitypes.Pet{}
}

func TestPeerDeletionSurvivesServerRestart(t *testing.T) {
	if strings.TrimSpace(os.Getenv("GIZCLAW_E2E_VERIFY_PEER_DELETION_RESTART")) != "1" {
		t.Skip("restart verification phase is run by run_pending_deletion_tests.sh")
	}
	h := newAdminSetupHarness(t)
	h.InstallFixedAdminContext(adminAPIAdminContext).MustSucceed(t)
	h.RequireAdminContextEndpoint(adminAPIAdminContext)
	h.RequireClientContextEndpoint("admin-api-peer")
	publicKey := h.ContextPublicKey("admin-api-peer")
	admin := h.ConnectClientFromContextEventually(adminAPIAdminContext, 30*time.Second)
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	get, err := api.GetPeerWithResponse(ctx, publicKey)
	if err != nil || get.JSON200 == nil {
		t.Fatalf("get tombstone after restart: status=%d body=%s error=%v", get.StatusCode(), get.Body, err)
	}
	tombstone, err := get.JSON200.AsExternalRef0RegistrationTombstone()
	if err != nil || tombstone.PublicKey != publicKey || tombstone.Status != apitypes.RegistrationTombstoneStatusDeleted {
		t.Fatalf("restart tombstone = %#v, %v", tombstone, err)
	}
	requirePeerLoginConflict(t, &adminAPIHarness{ctx: ctx, h: h}, publicKey, "admin-api-peer")
	if result := h.RegisterContext("admin-api-peer", "--sn", "deleted-after-restart"); result.Err == nil {
		t.Fatalf("deleted public key registered after restart: stdout=%s stderr=%s", result.Stdout, result.Stderr)
	}
}

func requirePeerLoginConflict(t *testing.T, env *adminAPIHarness, publicKey, contextName string) {
	t.Helper()
	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(env.h.ServerPublicKey)); err != nil {
		t.Fatal(err)
	}
	assertion, err := publiclogin.NewLoginAssertion(env.h.ContextKeyPair(contextName), serverPublicKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(env.ctx, http.MethodPost, env.h.PublicHTTPURL()+"/login", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Public-Key", publicKey)
	request.Header.Set("Authorization", "Bearer "+assertion)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("deleted Peer login status=%d body=%s", response.StatusCode, body)
	}
	var failure apitypes.ErrorResponse
	if err := json.Unmarshal(body, &failure); err != nil || (failure.Error.Code != "PEER_PENDING_DELETION" && failure.Error.Code != "PEER_DELETED") {
		t.Fatalf("deleted Peer login response=%s error=%v", body, err)
	}
}
