//go:build gizclaw_e2e

package delete_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestPeerSelfDeletionStopsActiveConnectionAndRuntime(t *testing.T) {
	env := newDeletionHarness(t)
	peer := env.newPeer(t, "delete-peer-self")
	foreign := env.newPeer(t, "delete-peer-foreign")
	workspaceName := "delete-peer-active-workspace"
	_, storedWorkspace := env.createWorkspace(t, peer, workspaceName)
	_, foreignWorkspace := env.createWorkspace(t, foreign, "delete-peer-foreign-kept")
	env.startWorkspace(t, peer, workspaceName)
	requireRunningWorkspace(t, env, peer, workspaceName)
	ownedContactDisplayName := "Delete Peer Owned Contact"
	ownedContact, err := peer.client.CreateContact(env.ctx, "delete.peer.contact.owned", rpcapi.ContactCreateRequest{
		Name: "delete-peer-owned-contact", DisplayName: &ownedContactDisplayName,
	})
	if err != nil {
		t.Fatalf("create Peer-owned Contact: %v", err)
	}
	foreignContactDisplayName := "Delete Peer Foreign Contact"
	foreignContact, err := foreign.client.CreateContact(env.ctx, "delete.peer.contact.foreign", rpcapi.ContactCreateRequest{
		Name: "delete-peer-foreign-contact", DisplayName: &foreignContactDisplayName,
	})
	if err != nil {
		t.Fatalf("create foreign Contact: %v", err)
	}
	storedOwnedContact := findAdminContact(t, env, peer.publicKey, ownedContact.Name)
	storedForeignContact := findAdminContact(t, env, foreign.publicKey, foreignContact.Name)

	friendToken, err := foreign.client.CreateFriendInviteToken(env.ctx, "delete.peer.friend.invite", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("create direct Friend invite: %v", err)
	}
	directFriend, err := peer.client.AddFriend(env.ctx, "delete.peer.friend.add", rpcapi.FriendAddRequest{InviteToken: friendToken.InviteToken})
	if err != nil || directFriend.PeerPublicKey == nil || *directFriend.PeerPublicKey != foreign.publicKey || directFriend.WorkspaceName == nil {
		t.Fatalf("create direct Friend through Peer RPC: friend=%#v error=%v", directFriend, err)
	}
	directChatWorkspace, found, err := clitest.WorkspaceByName(env.ctx, env.api, *directFriend.WorkspaceName)
	if err != nil || !found {
		t.Fatalf("resolve direct Friend Workspace: found=%v error=%v", found, err)
	}

	adopted, err := peer.client.AdoptPet(env.ctx, "delete.peer.pet", rpcapi.RuntimeAdoptRequest{Name: "delete-peer-pet", DisplayName: "Delete Peer Pet"})
	if err != nil {
		t.Fatalf("adopt Peer-owned Pet: %v", err)
	}
	storedPet := findPeerPet(t, env, peer.publicKey, adopted.Pet.Name)
	group, err := peer.client.CreateFriendGroup(env.ctx, "delete.peer.group", rpcapi.FriendGroupCreateRequest{Name: "delete-peer-owned-group"})
	if err != nil {
		t.Fatalf("create Peer-owned Friend Group: %v", err)
	}
	storedGroup := env.findFriendGroup(t, peer.publicKey, group.Name)
	foreignGroup, err := foreign.client.CreateFriendGroup(env.ctx, "delete.peer.group.foreign", rpcapi.FriendGroupCreateRequest{Name: "delete-peer-foreign-group"})
	if err != nil {
		t.Fatalf("create foreign Friend Group: %v", err)
	}
	foreignGroupInvite, err := foreign.client.CreateFriendGroupInviteToken(env.ctx, "delete.peer.group.foreign.invite", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: foreignGroup.Name})
	if err != nil {
		t.Fatalf("create foreign Friend Group invite: %v", err)
	}
	if _, err := peer.client.JoinFriendGroup(env.ctx, "delete.peer.group.foreign.join", rpcapi.FriendGroupJoinRequest{
		Name: foreignGroup.Name, InviteToken: foreignGroupInvite.InviteToken,
	}); err != nil {
		t.Fatalf("join foreign Friend Group: %v", err)
	}
	storedForeignGroup := env.findFriendGroup(t, foreign.publicKey, foreignGroup.Name)

	peerConn := peer.client.PeerConn()
	if peerConn == nil {
		t.Fatal("connected Peer has no transport")
	}
	eventStream, err := peer.client.DialPeerEventStream()
	if err != nil {
		t.Fatalf("open active Event stream: %v", err)
	}
	defer eventStream.Close()
	rpcStream := dialDeletionService(t, peerConn.Dial, gizcli.ServicePeerRPC)
	defer rpcStream.Close()
	httpStream := dialDeletionService(t, peerConn.Dial, gizcli.ServicePeerHTTP)
	defer httpStream.Close()
	writeDeletionPing(t, rpcStream, "delete-peer-active-ping")
	requireDeletionPing(t, rpcStream, "delete-peer-active-ping")
	requireDeletionHTTP(t, httpStream)

	deleted, err := peer.client.DeletePeer(env.ctx, "delete.peer.self", rpcapi.ServerPeerDeleteRequest{})
	if err != nil {
		t.Fatalf("self-delete active Peer: %v", err)
	}
	if deleted == nil {
		t.Fatal("self-delete returned nil response")
	}

	requireStreamTerminated(t, eventStream, "Event")
	requireStreamTerminated(t, rpcStream, "RPC")
	requireStreamTerminated(t, httpStream, "HTTP")
	if _, err := peer.client.GetServerInfo(env.ctx, "delete.peer.post-response"); err == nil {
		t.Fatal("existing Peer connection accepted RPC after self-delete response")
	}
	tombstone := waitPeerTombstone(t, env, peer.publicKey)
	if tombstone.PublicKey != peer.publicKey || tombstone.Status != apitypes.RegistrationTombstoneStatusDeleted {
		t.Fatalf("Peer tombstone = %#v", tombstone)
	}
	env.waitWorkspaceAbsent(t, storedWorkspace.Id)
	if response, err := env.api.GetFriendGroupWithResponse(env.ctx, storedGroup.Id); err != nil || response.StatusCode() != http.StatusNotFound {
		t.Fatalf("owned Friend Group survived Peer deletion: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	if storedGroup.WorkspaceId == nil {
		t.Fatalf("owned Friend Group has no Workspace: %#v", storedGroup)
	}
	env.waitWorkspaceAbsent(t, *storedGroup.WorkspaceId)
	env.waitWorkspaceAbsent(t, directChatWorkspace.Id)
	if response, err := env.api.GetPeerPetWithResponse(env.ctx, peer.publicKey, storedPet.Id); err != nil || response.StatusCode() != http.StatusConflict {
		t.Fatalf("deleted Peer Pet endpoint: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	env.waitWorkspaceAbsent(t, storedPet.WorkspaceId)
	requirePeerGameplayPurged(t, peer.publicKey, storedPet.Id)
	if response, err := env.api.GetContactWithResponse(env.ctx, peer.publicKey, storedOwnedContact.Id); err != nil || response.StatusCode() != http.StatusNotFound {
		t.Fatalf("Peer-owned Contact survived deletion: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	if response, err := env.api.GetContactWithResponse(env.ctx, foreign.publicKey, storedForeignContact.Id); err != nil || response.StatusCode() != http.StatusOK {
		t.Fatalf("foreign Contact was affected: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	assertFriendAbsent(t, env, foreign.publicKey, peer.publicKey)
	if response, err := env.api.GetFriendGroupWithResponse(env.ctx, storedForeignGroup.Id); err != nil || response.StatusCode() != http.StatusOK {
		t.Fatalf("foreign Friend Group was affected: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	assertGroupMemberships(t, env, storedForeignGroup.Id, foreign.publicKey, peer.publicKey)
	if response, err := env.api.GetWorkspaceWithResponse(env.ctx, foreignWorkspace.Id); err != nil || response.StatusCode() != http.StatusOK {
		t.Fatalf("foreign Workspace was affected: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	tokenID := "e2e-delete-token-" + peer.contextName
	if response, err := env.api.GetRegistrationTokenWithResponse(env.ctx, tokenID); err != nil || response.StatusCode() != http.StatusOK {
		t.Fatalf("global RegistrationToken was affected: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	if response, err := env.api.GetWorkflowWithResponse(env.ctx, deleteWorkflowID); err != nil || response.StatusCode() != http.StatusOK {
		t.Fatalf("global Workflow was affected: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	requirePeerLoginRejected(t, env, peer)
	if result := env.h.RegisterContext(peer.contextName, "--sn", peer.serial); result.Err == nil {
		t.Fatalf("deleted public key registered again: stdout=%s stderr=%s", result.Stdout, result.Stderr)
	}
	assertNoPendingDeletion(t, env, "peer", "peer", peer.publicKey)
	repeated, err := env.api.DeletePeerWithResponse(env.ctx, peer.publicKey)
	if err != nil || repeated.JSON200 == nil {
		t.Fatalf("repeat Admin Peer deletion: status=%d body=%s error=%v", repeated.StatusCode(), repeated.Body, err)
	}
	repeatedTombstone, err := repeated.JSON200.AsExternalRef0RegistrationTombstone()
	if err != nil || repeatedTombstone != tombstone {
		t.Fatalf("repeat Peer deletion tombstone=%#v error=%v, want %#v", repeatedTombstone, err, tombstone)
	}
}

func TestAdminPeerDeletionStopsActiveSession(t *testing.T) {
	env := newDeletionHarness(t)
	peer := env.newPeer(t, "delete-peer-admin")
	workspaceName := "delete-peer-admin-active"
	_, storedWorkspace := env.createWorkspace(t, peer, workspaceName)
	env.startWorkspace(t, peer, workspaceName)
	rpcStream := dialDeletionService(t, peer.client.PeerConn().Dial, gizcli.ServicePeerRPC)
	defer rpcStream.Close()
	writeDeletionPing(t, rpcStream, "delete-peer-admin-active-ping")
	requireDeletionPing(t, rpcStream, "delete-peer-admin-active-ping")

	response, err := env.api.DeletePeerWithResponse(env.ctx, peer.publicKey)
	if err != nil || response.JSON200 == nil {
		t.Fatalf("Admin delete active Peer: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	requireStreamTerminated(t, rpcStream, "Admin-deleted RPC")
	_ = waitPeerTombstone(t, env, peer.publicKey)
	env.waitWorkspaceAbsent(t, storedWorkspace.Id)
	requirePeerLoginRejected(t, env, peer)
}

func TestPeerDeletionSurvivesServerRestart(t *testing.T) {
	if strings.TrimSpace(os.Getenv("GIZCLAW_E2E_VERIFY_PEER_DELETION_RESTART")) != "1" {
		t.Skip("restart verification phase is run by run_pending_deletion_tests.sh")
	}
	env := newDeletionHarness(t)
	contextName := "delete-peer-self"
	env.h.RequireClientContextEndpoint(contextName)
	peer := deletionPeer{
		contextName: contextName,
		publicKey:   env.h.ContextPublicKey(contextName),
		serial:      "client-" + contextName + "-" + env.h.ContextPublicKey(contextName),
	}
	tombstone := waitPeerTombstone(t, env, peer.publicKey)
	if tombstone.Status != apitypes.RegistrationTombstoneStatusDeleted {
		t.Fatalf("restart tombstone = %#v", tombstone)
	}
	requirePeerLoginRejected(t, env, peer)
	if result := env.h.RegisterContext(contextName, "--sn", peer.serial); result.Err == nil {
		t.Fatalf("deleted public key registered after restart: stdout=%s stderr=%s", result.Stdout, result.Stderr)
	}
}

func waitPeerTombstone(t *testing.T, env *deletionHarness, publicKey string) apitypes.RegistrationTombstone {
	t.Helper()
	var tombstone apitypes.RegistrationTombstone
	waitUntil(t, env.ctx, "Peer tombstone", func() (bool, string) {
		response, err := env.api.GetPeerWithResponse(env.ctx, publicKey)
		if err != nil {
			return false, err.Error()
		}
		if response.JSON200 == nil {
			return false, fmt.Sprintf("status=%d body=%s", response.StatusCode(), response.Body)
		}
		var projection map[string]any
		if err := json.Unmarshal(response.Body, &projection); err != nil || projection["status"] != string(apitypes.RegistrationTombstoneStatusDeleted) {
			return false, fmt.Sprintf("body=%s decode=%v", response.Body, err)
		}
		if len(projection) != 2 {
			return false, fmt.Sprintf("non-minimal tombstone body=%s", response.Body)
		}
		candidate, err := response.JSON200.AsExternalRef0RegistrationTombstone()
		if err != nil {
			return false, err.Error()
		}
		tombstone = candidate
		return true, ""
	})
	return tombstone
}

func requirePeerLoginRejected(t *testing.T, env *deletionHarness, peer deletionPeer) {
	t.Helper()
	request, err := http.NewRequestWithContext(env.ctx, http.MethodGet, env.h.PublicHTTPURL()+"/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+peer.apiKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("deleted Peer API key status=%d body=%s", response.StatusCode, body)
	}
}

func dialDeletionService(t *testing.T, dial func(uint64) (net.Conn, error), service uint64) net.Conn {
	t.Helper()
	stream, err := dial(service)
	if err != nil {
		t.Fatalf("dial service %d: %v", service, err)
	}
	if err := stream.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatalf("set service deadline: %v", err)
	}
	return stream
}

func writeDeletionPing(t *testing.T, stream io.Writer, id string) {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()}); err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	if err := rpcapi.WriteRequest(stream, &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: id, Method: rpcapi.RPCMethodAllPing, Params: &params}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		t.Fatalf("write ping EOS: %v", err)
	}
}

func requireDeletionPing(t *testing.T, stream io.Reader, id string) {
	t.Helper()
	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodAllPing)
	if err != nil || response.Id != id || response.Error != nil {
		t.Fatalf("read ping %q: response=%#v error=%v", id, response, err)
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		t.Fatalf("read ping EOS: %v", err)
	}
}

func requireDeletionHTTP(t *testing.T, stream net.Conn) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://gizclaw/server-info", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := request.Write(stream); err != nil {
		t.Fatalf("write Peer HTTP request: %v", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(stream), request)
	if err != nil {
		t.Fatalf("read Peer HTTP response: %v", err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("Peer HTTP response status=%d read=%v close=%v", response.StatusCode, readErr, closeErr)
	}
}

func requireStreamTerminated(t *testing.T, stream net.Conn, label string) {
	t.Helper()
	if err := stream.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatalf("set %s termination deadline: %v", label, err)
	}
	buffer := make([]byte, 1)
	if _, err := stream.Read(buffer); err == nil {
		t.Fatalf("%s stream remained readable after Peer delete response", label)
	}
}

func findAdminContact(t *testing.T, env *deletionHarness, ownerPublicKey, name string) adminhttp.AdminContactObject {
	t.Helper()
	limit := 200
	response, err := env.api.ListContactsWithResponse(env.ctx, &adminhttp.ListContactsParams{OwnerPublicKey: &ownerPublicKey, Limit: &limit})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list Contacts for %q: status=%d body=%s error=%v", ownerPublicKey, response.StatusCode(), response.Body, err)
	}
	for _, contact := range response.JSON200.Items {
		if contact.Name == name {
			return contact
		}
	}
	t.Fatalf("Contact owner=%q name=%q not found", ownerPublicKey, name)
	return adminhttp.AdminContactObject{}
}

func assertFriendAbsent(t *testing.T, env *deletionHarness, ownerPublicKey, deletedPublicKey string) {
	t.Helper()
	limit := 200
	response, err := env.api.ListPeerFriendsWithResponse(env.ctx, ownerPublicKey, &adminhttp.ListPeerFriendsParams{Limit: &limit})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list surviving Peer Friends: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	for _, friend := range response.JSON200.Items {
		if friend.PeerPublicKey == deletedPublicKey {
			t.Fatalf("direct Friend relation to deleted Peer survived: %#v", friend)
		}
	}
}

func assertGroupMemberships(t *testing.T, env *deletionHarness, groupID, ownerPublicKey, deletedPublicKey string) {
	t.Helper()
	limit := 200
	response, err := env.api.ListFriendGroupMembersWithResponse(env.ctx, groupID, &adminhttp.ListFriendGroupMembersParams{Limit: &limit})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list surviving Friend Group members: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	hasOwner := false
	for _, member := range response.JSON200.Items {
		hasOwner = hasOwner || member.PeerPublicKey == ownerPublicKey
		if member.PeerPublicKey == deletedPublicKey {
			t.Fatalf("foreign Friend Group retained deleted Peer membership: %#v", member)
		}
	}
	if !hasOwner {
		t.Fatalf("foreign Friend Group lost its owner: %#v", response.JSON200.Items)
	}
}

func requirePeerGameplayPurged(t *testing.T, publicKey, petID string) {
	t.Helper()
	project := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_DOCKER_PROJECT"))
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
