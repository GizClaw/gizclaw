//go:build gizclaw_e2e

package social_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func newSocialSimulatorHarness(t *testing.T) *clitest.Harness {
	t.Helper()

	h := clitest.NewSetupHarness(t, "client-social")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "client-social-admin-sn").MustSucceed(t)
	chatroomWorkflow := filepath.Join(h.RepoRoot, "test", "gizclaw-e2e", "testdata", "resources", "040-workflow-chatroom.json")
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	applySocialResourceFile(t, api, chatroomWorkflow)
	for _, peer := range []string{"peer-a", "peer-b", "peer-c", "peer-d"} {
		h.CreateContext(peer).MustSucceed(t)
		h.RegisterContext(peer, "--sn", "client-social-"+peer+"-sn").MustSucceed(t)
	}
	return h
}

func applySocialResourceFile(t *testing.T, api *adminservice.ClientWithResponses, resourcePath string) {
	t.Helper()

	data, err := os.ReadFile(resourcePath)
	if err != nil {
		t.Fatalf("read social resource %s: %v", resourcePath, err)
	}
	var resource apitypes.Resource
	if err := json.Unmarshal(data, &resource); err != nil {
		t.Fatalf("decode social resource %s: %v", resourcePath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := api.ApplyResourceWithResponse(ctx, resource)
	if err != nil {
		t.Fatalf("apply social resource %s: %v", resourcePath, err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("apply social resource %s status %d: %s", resourcePath, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
}

func setSocialChatWorkspaceInputMode(t *testing.T, h *clitest.Harness, workspaceName string, input apitypes.WorkspaceInputMode) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create social admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := api.GetWorkspaceWithResponse(ctx, workspaceName)
	if err != nil {
		t.Fatalf("get social workspace %q: %v", workspaceName, err)
	}
	if got.JSON200 == nil {
		t.Fatalf("get social workspace %q status %d: %s", workspaceName, got.StatusCode(), strings.TrimSpace(string(got.Body)))
	}
	if got.JSON200.Parameters == nil {
		t.Fatalf("get social workspace %q has nil parameters", workspaceName)
	}
	typed, err := got.JSON200.Parameters.AsChatRoomWorkspaceParameters()
	if err != nil {
		t.Fatalf("decode social workspace %q parameters: %v", workspaceName, err)
	}
	typed.Input = &input
	var params apitypes.WorkspaceParameters
	if err := params.FromChatRoomWorkspaceParameters(typed); err != nil {
		t.Fatalf("encode social workspace %q parameters: %v", workspaceName, err)
	}
	body := adminservice.WorkspaceUpsert{
		Name:         string(got.JSON200.Name),
		WorkflowName: string(got.JSON200.WorkflowName),
		Parameters:   &params,
	}
	updated, err := api.PutWorkspaceWithResponse(ctx, workspaceName, body)
	if err != nil {
		t.Fatalf("put social workspace %q: %v", workspaceName, err)
	}
	if updated.JSON200 == nil {
		t.Fatalf("put social workspace %q status %d: %s", workspaceName, updated.StatusCode(), strings.TrimSpace(string(updated.Body)))
	}
}

func assertContactRPCs(t *testing.T, h *clitest.Harness) {
	t.Helper()

	alice := mustCreateContact(t, h, "peer-a", "Alice", "+1 555 0100")
	bob := mustCreateContact(t, h, "peer-a", "Bob", "+1 555 0101")
	got := mustGetContact(t, h, "peer-a", stringValue(alice.Id))
	if stringValue(got.DisplayName) != "Alice" {
		t.Fatalf("contact.get display_name = %q, want Alice", stringValue(got.DisplayName))
	}
	updated := mustPutContact(t, h, "peer-a", stringValue(alice.Id), "Alice Zhang", "+1 555 0102")
	if stringValue(updated.DisplayName) != "Alice Zhang" {
		t.Fatalf("contact.put display_name = %q, want Alice Zhang", stringValue(updated.DisplayName))
	}
	if err := getContactError(t, h, "peer-b", stringValue(alice.Id)); err == nil {
		t.Fatal("peer-b unexpectedly read peer-a contact")
	}
	limit := 1
	first := mustListContacts(t, h, "peer-a", rpcapi.ContactListRequest{Limit: &limit})
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("contact first page = %#v, want one item and next cursor", first)
	}
	second := mustListContacts(t, h, "peer-a", rpcapi.ContactListRequest{Limit: &limit, Cursor: first.NextCursor})
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("contact second page = %#v, want final item", second)
	}
	deleted := mustDeleteContact(t, h, "peer-a", stringValue(bob.Id))
	if stringValue(deleted.Id) != stringValue(bob.Id) {
		t.Fatalf("contact.delete id = %q, want %q", stringValue(deleted.Id), stringValue(bob.Id))
	}
}

func createAcceptedFriendRequest(t *testing.T, h *clitest.Harness, fromContext, toContext, toPeerID, code string) rpcapi.FriendObject {
	t.Helper()

	mustReportFriendOTP(t, h, toContext, code)
	if err := createFriendRequestError(t, h, fromContext, toPeerID, "000000", ""); err == nil {
		t.Fatal("friend request with wrong device-reported OTP unexpectedly succeeded")
	}
	mustReportFriendOTP(t, h, toContext, code)
	req := mustCreateFriendRequest(t, h, fromContext, toPeerID, code, "hi")
	if req.State == nil || *req.State != rpcapi.FriendRequestStatePending {
		t.Fatalf("friend request state = %v, want pending", req.State)
	}
	box := rpcapi.FriendRequestBoxIncoming
	state := rpcapi.FriendRequestStatePending
	limit := 1
	incoming := mustListFriendRequests(t, h, toContext, rpcapi.FriendRequestListRequest{Box: &box, State: &state, Limit: &limit})
	if len(incoming.Items) != 1 || stringValue(incoming.Items[0].Id) != stringValue(req.Id) {
		t.Fatalf("incoming friend requests = %#v, want %q", incoming, stringValue(req.Id))
	}
	accepted := mustAcceptFriendRequest(t, h, toContext, stringValue(req.Id))
	if accepted.State == nil || *accepted.State != rpcapi.FriendRequestStateAccepted {
		t.Fatalf("accepted friend request state = %v, want accepted", accepted.State)
	}
	acceptedAgain := mustAcceptFriendRequest(t, h, toContext, stringValue(req.Id))
	if stringValue(acceptedAgain.Id) != stringValue(req.Id) || acceptedAgain.State == nil || *acceptedAgain.State != rpcapi.FriendRequestStateAccepted {
		t.Fatalf("second accept = %#v, want same accepted request", acceptedAgain)
	}
	friends := mustListFriends(t, h, fromContext, rpcapi.FriendListRequest{})
	for _, friend := range friends.Items {
		if stringValue(friend.PeerId) == toPeerID {
			return friend
		}
	}
	t.Fatalf("friend relation with %s not found in %#v", toPeerID, friends)
	return rpcapi.FriendObject{}
}

func assertFriendOTPFailureCases(t *testing.T, h *clitest.Harness, peerB string) {
	t.Helper()

	if err := createFriendRequestError(t, h, "peer-a", peerB, "", ""); err == nil {
		t.Fatal("friend request without code unexpectedly succeeded")
	}
	if err := reportFriendOTPError(t, h, "peer-b", "abc123"); err == nil {
		t.Fatal("malformed device friend OTP unexpectedly reported")
	}
	if err := createFriendRequestError(t, h, "peer-a", peerB, "abc123", ""); err == nil {
		t.Fatal("friend request with malformed code unexpectedly succeeded")
	}
	mustReportFriendOTP(t, h, "peer-b", "456789")
	time.Sleep(3 * time.Second)
	if err := createFriendRequestError(t, h, "peer-a", peerB, "456789", ""); err == nil {
		t.Fatal("friend request with expired code unexpectedly succeeded")
	}

	mustReportFriendOTP(t, h, "peer-b", "567890")
	req := mustCreateFriendRequest(t, h, "peer-c", peerB, "567890", "")
	if err := createFriendRequestError(t, h, "peer-a", peerB, "567890", ""); err == nil {
		t.Fatal("friend request with already-consumed code unexpectedly succeeded")
	}
	rejected := mustRejectFriendRequest(t, h, "peer-b", stringValue(req.Id))
	if rejected.State == nil || *rejected.State != rpcapi.FriendRequestStateRejected {
		t.Fatalf("rejected consumed-code setup request state = %v, want rejected", rejected.State)
	}
}

func assertRejectedFriendRequest(t *testing.T, h *clitest.Harness, peerB string) {
	t.Helper()

	mustReportFriendOTP(t, h, "peer-b", "345678")
	req := mustCreateFriendRequest(t, h, "peer-c", peerB, "345678", "")
	rejected := mustRejectFriendRequest(t, h, "peer-b", stringValue(req.Id))
	if rejected.State == nil || *rejected.State != rpcapi.FriendRequestStateRejected {
		t.Fatalf("rejected friend request state = %v, want rejected", rejected.State)
	}
}

func assertFriendPagination(t *testing.T, h *clitest.Harness, firstFriend, secondFriend rpcapi.FriendObject) {
	t.Helper()

	limit := 1
	first := mustListFriends(t, h, "peer-a", rpcapi.FriendListRequest{Limit: &limit})
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("friend first page = %#v, want one item and next cursor", first)
	}
	second := mustListFriends(t, h, "peer-a", rpcapi.FriendListRequest{Limit: &limit, Cursor: first.NextCursor})
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("friend second page = %#v, want final item", second)
	}
	got := map[string]bool{stringValue(first.Items[0].Id): true, stringValue(second.Items[0].Id): true}
	if !got[stringValue(firstFriend.Id)] || !got[stringValue(secondFriend.Id)] {
		t.Fatalf("friend pagination ids = %#v, want %q and %q", got, stringValue(firstFriend.Id), stringValue(secondFriend.Id))
	}
	box := rpcapi.FriendRequestBoxOutgoing
	requests := mustListFriendRequests(t, h, "peer-a", rpcapi.FriendRequestListRequest{Box: &box, Limit: &limit})
	if len(requests.Items) != 1 || !requests.HasNext || requests.NextCursor == nil {
		t.Fatalf("friend request first page = %#v, want pagination", requests)
	}
	requests = mustListFriendRequests(t, h, "peer-a", rpcapi.FriendRequestListRequest{Box: &box, Limit: &limit, Cursor: requests.NextCursor})
	if len(requests.Items) != 1 || requests.HasNext {
		t.Fatalf("friend request second page = %#v, want final item", requests)
	}
}

func assertFriendGroupPagination(t *testing.T, h *clitest.Harness, wantIDs []string) {
	t.Helper()

	limit := 1
	first := mustListFriendGroups(t, h, "peer-a", rpcapi.FriendGroupListRequest{Limit: &limit})
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("group first page = %#v, want one item and next cursor", first)
	}
	second := mustListFriendGroups(t, h, "peer-a", rpcapi.FriendGroupListRequest{Limit: &limit, Cursor: first.NextCursor})
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("group second page = %#v, want final item", second)
	}
	got := map[string]bool{stringValue(first.Items[0].Id): true, stringValue(second.Items[0].Id): true}
	for _, id := range wantIDs {
		if !got[id] {
			t.Fatalf("group pagination ids = %#v, missing %q", got, id)
		}
	}
}

func assertFriendGroupMemberPagination(t *testing.T, h *clitest.Harness, friendGroupID string) {
	t.Helper()

	limit := 1
	first := mustListFriendGroupMembers(t, h, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: &friendGroupID, Limit: &limit})
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("friend group member first page = %#v, want one item and next cursor", first)
	}
	second := mustListFriendGroupMembers(t, h, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: &friendGroupID, Limit: &limit, Cursor: first.NextCursor})
	if len(second.Items) != 1 {
		t.Fatalf("friend group member second page = %#v, want one item", second)
	}
}

func mustSocialRPC[T any](t *testing.T, h *clitest.Harness, contextName, requestID string, call func(context.Context, *gizcli.Client) (*T, error)) T {
	t.Helper()

	client := h.ConnectClientFromContext(contextName)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := call(ctx, client)
	if err != nil {
		t.Fatalf("%s via %s: %v", requestID, contextName, err)
	}
	if out == nil {
		t.Fatalf("%s via %s returned nil", requestID, contextName)
	}
	return *out
}

func socialRPCError[T any](t *testing.T, h *clitest.Harness, contextName, requestID string, call func(context.Context, *gizcli.Client) (*T, error)) error {
	t.Helper()

	client := h.ConnectClientFromContext(contextName)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := call(ctx, client)
	return err
}

func mustCreateContact(t *testing.T, h *clitest.Harness, contextName, displayName, phoneNumber string) rpcapi.ContactObject {
	return mustSocialRPC(t, h, contextName, "contact.create", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ContactCreateResponse, error) {
		return client.CreateContact(ctx, "contact.create", rpcapi.ContactCreateRequest{
			DisplayName: &displayName,
			PhoneNumber: &phoneNumber,
		})
	})
}

func mustGetContact(t *testing.T, h *clitest.Harness, contextName, id string) rpcapi.ContactObject {
	return mustSocialRPC(t, h, contextName, "contact.get", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ContactGetResponse, error) {
		return client.GetContact(ctx, "contact.get", rpcapi.ContactGetRequest{Id: id})
	})
}

func getContactError(t *testing.T, h *clitest.Harness, contextName, id string) error {
	return socialRPCError(t, h, contextName, "contact.get", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ContactGetResponse, error) {
		return client.GetContact(ctx, "contact.get", rpcapi.ContactGetRequest{Id: id})
	})
}

func mustPutContact(t *testing.T, h *clitest.Harness, contextName, id, displayName, phoneNumber string) rpcapi.ContactObject {
	return mustSocialRPC(t, h, contextName, "contact.put", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ContactPutResponse, error) {
		return client.PutContact(ctx, "contact.put", rpcapi.ContactPutRequest{
			Id:          id,
			DisplayName: &displayName,
			PhoneNumber: &phoneNumber,
		})
	})
}

func mustListContacts(t *testing.T, h *clitest.Harness, contextName string, request rpcapi.ContactListRequest) rpcapi.ContactListResponse {
	return mustSocialRPC(t, h, contextName, "contact.list", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ContactListResponse, error) {
		return client.ListContacts(ctx, "contact.list", request)
	})
}

func mustDeleteContact(t *testing.T, h *clitest.Harness, contextName, id string) rpcapi.ContactObject {
	return mustSocialRPC(t, h, contextName, "contact.delete", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ContactDeleteResponse, error) {
		return client.DeleteContact(ctx, "contact.delete", rpcapi.ContactDeleteRequest{Id: id})
	})
}

func mustReportFriendOTP(t *testing.T, h *clitest.Harness, contextName, code string) rpcapi.ServerGetRunStatusResponse {
	return mustSocialRPC(t, h, contextName, "server.run.status", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ServerGetRunStatusResponse, error) {
		return client.GetServerRunStatus(ctx, "server.run.status", rpcapi.ServerGetRunStatusRequest{FriendOtp: &code})
	})
}

func reportFriendOTPError(t *testing.T, h *clitest.Harness, contextName, code string) error {
	return socialRPCError(t, h, contextName, "server.run.status", func(ctx context.Context, client *gizcli.Client) (*rpcapi.ServerGetRunStatusResponse, error) {
		return client.GetServerRunStatus(ctx, "server.run.status", rpcapi.ServerGetRunStatusRequest{FriendOtp: &code})
	})
}

func mustCreateFriendRequest(t *testing.T, h *clitest.Harness, contextName, toPeerID, code, message string) rpcapi.FriendRequestObject {
	return mustSocialRPC(t, h, contextName, "friend.requests.create", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendRequestCreateResponse, error) {
		request := rpcapi.FriendRequestCreateRequest{ToPeerId: toPeerID, Code: code}
		if message != "" {
			request.Message = &message
		}
		return client.CreateFriendRequest(ctx, "friend.requests.create", request)
	})
}

func createFriendRequestError(t *testing.T, h *clitest.Harness, contextName, toPeerID, code, message string) error {
	return socialRPCError(t, h, contextName, "friend.requests.create", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendRequestCreateResponse, error) {
		request := rpcapi.FriendRequestCreateRequest{ToPeerId: toPeerID, Code: code}
		if message != "" {
			request.Message = &message
		}
		return client.CreateFriendRequest(ctx, "friend.requests.create", request)
	})
}

func mustListFriendRequests(t *testing.T, h *clitest.Harness, contextName string, request rpcapi.FriendRequestListRequest) rpcapi.FriendRequestListResponse {
	return mustSocialRPC(t, h, contextName, "friend.requests.list", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendRequestListResponse, error) {
		return client.ListFriendRequests(ctx, "friend.requests.list", request)
	})
}

func mustAcceptFriendRequest(t *testing.T, h *clitest.Harness, contextName, id string) rpcapi.FriendRequestObject {
	return mustSocialRPC(t, h, contextName, "friend.requests.accept", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendRequestAcceptResponse, error) {
		return client.AcceptFriendRequest(ctx, "friend.requests.accept", rpcapi.FriendRequestAcceptRequest{Id: id})
	})
}

func mustRejectFriendRequest(t *testing.T, h *clitest.Harness, contextName, id string) rpcapi.FriendRequestObject {
	return mustSocialRPC(t, h, contextName, "friend.requests.reject", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendRequestRejectResponse, error) {
		return client.RejectFriendRequest(ctx, "friend.requests.reject", rpcapi.FriendRequestRejectRequest{Id: id})
	})
}

func mustListFriends(t *testing.T, h *clitest.Harness, contextName string, request rpcapi.FriendListRequest) rpcapi.FriendListResponse {
	return mustSocialRPC(t, h, contextName, "friend.list", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendListResponse, error) {
		return client.ListFriends(ctx, "friend.list", request)
	})
}

func mustDeleteFriend(t *testing.T, h *clitest.Harness, contextName, id string) rpcapi.FriendObject {
	return mustSocialRPC(t, h, contextName, "friend.delete", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendDeleteResponse, error) {
		return client.DeleteFriend(ctx, "friend.delete", rpcapi.FriendDeleteRequest{Id: id})
	})
}

func mustCreateFriendGroup(t *testing.T, h *clitest.Harness, contextName, name, description string) rpcapi.FriendGroupObject {
	return mustSocialRPC(t, h, contextName, "friend_group.create", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupCreateResponse, error) {
		request := rpcapi.FriendGroupCreateRequest{Name: name}
		if description != "" {
			request.Description = &description
		}
		return client.CreateFriendGroup(ctx, "friend_group.create", request)
	})
}

func mustGetFriendGroup(t *testing.T, h *clitest.Harness, contextName, id string) rpcapi.FriendGroupObject {
	return mustSocialRPC(t, h, contextName, "friend_group.get", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupGetResponse, error) {
		return client.GetFriendGroup(ctx, "friend_group.get", rpcapi.FriendGroupGetRequest{Id: id})
	})
}

func getFriendGroupError(t *testing.T, h *clitest.Harness, contextName, id string) error {
	return socialRPCError(t, h, contextName, "friend_group.get", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupGetResponse, error) {
		return client.GetFriendGroup(ctx, "friend_group.get", rpcapi.FriendGroupGetRequest{Id: id})
	})
}

func mustPutFriendGroup(t *testing.T, h *clitest.Harness, contextName, id, name string) rpcapi.FriendGroupObject {
	return mustSocialRPC(t, h, contextName, "friend_group.put", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupPutResponse, error) {
		return client.PutFriendGroup(ctx, "friend_group.put", rpcapi.FriendGroupPutRequest{Id: id, Name: &name})
	})
}

func mustDeleteFriendGroup(t *testing.T, h *clitest.Harness, contextName, id string) rpcapi.FriendGroupObject {
	return mustSocialRPC(t, h, contextName, "friend_group.delete", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupDeleteResponse, error) {
		return client.DeleteFriendGroup(ctx, "friend_group.delete", rpcapi.FriendGroupDeleteRequest{Id: id})
	})
}

func mustListFriendGroups(t *testing.T, h *clitest.Harness, contextName string, request rpcapi.FriendGroupListRequest) rpcapi.FriendGroupListResponse {
	return mustSocialRPC(t, h, contextName, "friend_group.list", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupListResponse, error) {
		return client.ListFriendGroups(ctx, "friend_group.list", request)
	})
}

func mustAddFriendGroupMember(t *testing.T, h *clitest.Harness, contextName, groupID, peerID string, role rpcapi.FriendGroupMemberMutableRole) rpcapi.FriendGroupMemberObject {
	return mustSocialRPC(t, h, contextName, "friend_group.members.add", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupMemberAddResponse, error) {
		return client.AddFriendGroupMember(ctx, "friend_group.members.add", rpcapi.FriendGroupMemberAddRequest{
			FriendGroupId: groupID,
			PeerId:        peerID,
			Role:          role,
		})
	})
}

func mustPutFriendGroupMember(t *testing.T, h *clitest.Harness, contextName, groupID, peerID string, role rpcapi.FriendGroupMemberMutableRole) rpcapi.FriendGroupMemberObject {
	return mustSocialRPC(t, h, contextName, "friend_group.members.put", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupMemberPutResponse, error) {
		return client.PutFriendGroupMember(ctx, "friend_group.members.put", rpcapi.FriendGroupMemberPutRequest{
			FriendGroupId: groupID,
			Id:            peerID,
			Role:          role,
		})
	})
}

func mustDeleteFriendGroupMember(t *testing.T, h *clitest.Harness, contextName, groupID, peerID string) rpcapi.FriendGroupMemberObject {
	return mustSocialRPC(t, h, contextName, "friend_group.members.delete", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
		return client.DeleteFriendGroupMember(ctx, "friend_group.members.delete", rpcapi.FriendGroupMemberDeleteRequest{
			FriendGroupId: groupID,
			Id:            peerID,
		})
	})
}

func mustListFriendGroupMembers(t *testing.T, h *clitest.Harness, contextName string, request rpcapi.FriendGroupMemberListRequest) rpcapi.FriendGroupMemberListResponse {
	return mustSocialRPC(t, h, contextName, "friend_group.members.list", func(ctx context.Context, client *gizcli.Client) (*rpcapi.FriendGroupMemberListResponse, error) {
		return client.ListFriendGroupMembers(ctx, "friend_group.members.list", request)
	})
}

func assertChatWorkspaceHistory(t *testing.T, h *clitest.Harness, writerContext, readerContext, workspaceName string, texts []string) {
	t.Helper()
	if len(texts) < 3 {
		t.Fatalf("social chat history test needs at least 3 rounds, got %d", len(texts))
	}

	writer := h.ConnectClientFromContext(writerContext)
	defer writer.Close()
	reader := h.ConnectClientFromContext(readerContext)
	defer reader.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := reader.SetServerRunWorkspace(ctx, "social.chat.reader.workspace.set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName}); err != nil {
		t.Fatalf("%s set run workspace %q: %v", readerContext, workspaceName, err)
	}
	readerState, err := reader.ReloadServerRunWorkspace(ctx, "social.chat.reader.workspace.reload")
	if err != nil {
		t.Fatalf("%s reload run workspace %q: %v", readerContext, workspaceName, err)
	}
	if readerState.RuntimeState != rpcapi.PeerRunStatusStateRunning {
		t.Fatalf("%s reload workspace state = %#v", readerContext, readerState)
	}
	readerInput := newBlockingStream()
	readerOut, err := reader.Transform(ctx, "chatroom-reader", readerInput)
	if err != nil {
		t.Fatalf("%s open chatroom reader stream: %v", readerContext, err)
	}
	defer readerOut.Close()
	defer readerInput.CloseWithError(io.EOF)
	updatedCh := waitForWorkspaceHistoryUpdated(readerOut)

	if _, err := writer.SetServerRunWorkspace(ctx, "social.chat.workspace.set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName}); err != nil {
		t.Fatalf("%s set run workspace %q: %v", writerContext, workspaceName, err)
	}
	state, err := writer.ReloadServerRunWorkspace(ctx, "social.chat.workspace.reload")
	if err != nil {
		t.Fatalf("%s reload run workspace %q: %v", writerContext, workspaceName, err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning {
		t.Fatalf("%s reload workspace state = %#v", writerContext, state)
	}

	entries := make([]rpcapi.PeerRunHistoryEntry, 0, len(texts))
	for i, text := range texts {
		if i > 0 {
			updatedCh = waitForWorkspaceHistoryUpdated(readerOut)
		}
		entries = append(entries, sendChatTextAndWaitForHistory(t, ctx, h, writer, reader, readerOut, updatedCh, writerContext, readerContext, workspaceName, text))
	}
	assertWorkspaceHistoryResumeOrder(t, ctx, reader, workspaceName, entries)
}

func sendChatTextAndWaitForHistory(t *testing.T, ctx context.Context, h *clitest.Harness, writer, reader interface {
	Transform(context.Context, string, genx.Stream) (genx.Stream, error)
	GetWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryGetRequest) (*rpcapi.WorkspaceHistoryGetResponse, error)
	ListWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error)
	PlayServerRunWorkspaceHistory(context.Context, string, rpcapi.ServerPlayRunWorkspaceHistoryRequest) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error)
}, replayStream genx.Stream, updatedCh <-chan error, writerContext, readerContext, workspaceName, text string) rpcapi.PeerRunHistoryEntry {
	t.Helper()

	out, err := writer.Transform(ctx, "chatroom", chatTextStream(text))
	if err != nil {
		t.Fatalf("%s transform chat text: %v", writerContext, err)
	}
	defer out.Close()

	select {
	case err := <-updatedCh:
		if err != nil {
			t.Fatalf("%s did not observe workspace history update: %v", readerContext, err)
		}
	case <-ctx.Done():
		t.Fatalf("%s did not observe workspace history update before timeout: %v", readerContext, ctx.Err())
	}

	entry := waitForWorkspaceHistoryText(t, ctx, reader, workspaceName, text)
	got, err := reader.GetWorkspaceHistory(ctx, "social.chat.history.get", rpcapi.WorkspaceHistoryGetRequest{
		WorkspaceName: workspaceName,
		HistoryId:     entry.Id,
	})
	if err != nil {
		t.Fatalf("%s workspace history get %q: %v", readerContext, entry.Id, err)
	}
	if got.Text != text || got.Type != rpcapi.PeerRunHistoryEntryTypeGear || got.GearId == nil || *got.GearId != h.ContextPublicKey(writerContext) {
		t.Fatalf("workspace history get = %#v, want text %q from %s", got, text, writerContext)
	}
	play, err := reader.PlayServerRunWorkspaceHistory(ctx, "social.chat.history.play", rpcapi.ServerPlayRunWorkspaceHistoryRequest{HistoryId: entry.Id})
	if err != nil {
		t.Fatalf("%s workspace history play %q: %v", readerContext, entry.Id, err)
	}
	if !play.Accepted {
		t.Fatalf("workspace history play = %#v, want accepted", play)
	}
	waitForWorkspaceHistoryReplayText(t, ctx, replayStream, entry.Id, text)
	return entry
}

func assertWorkspaceHistoryResumeOrder(t *testing.T, ctx context.Context, client interface {
	ListWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error)
}, workspaceName string, entries []rpcapi.PeerRunHistoryEntry) {
	t.Helper()
	if len(entries) < 2 {
		t.Fatalf("workspace history resume order needs at least 2 entries, got %d", len(entries))
	}

	limit := 1
	asc := rpcapi.WorkspaceHistoryListRequestOrderAsc
	desc := rpcapi.WorkspaceHistoryListRequestOrderDesc
	for i := 0; i+1 < len(entries); i++ {
		first := entries[i]
		second := entries[i+1]
		next, err := client.ListWorkspaceHistory(ctx, "social.chat.history.list.next", rpcapi.WorkspaceHistoryListRequest{
			WorkspaceName: workspaceName,
			Cursor:        &first.Id,
			Order:         &asc,
			Limit:         &limit,
		})
		if err != nil {
			t.Fatalf("workspace history list next after %q: %v", first.Id, err)
		}
		if len(next.Items) != 1 || next.Items[0].Id != second.Id {
			t.Fatalf("workspace history next page = %+v, want %q after %q", next, second.Id, first.Id)
		}

		prev, err := client.ListWorkspaceHistory(ctx, "social.chat.history.list.prev", rpcapi.WorkspaceHistoryListRequest{
			WorkspaceName: workspaceName,
			Cursor:        &second.Id,
			Order:         &desc,
			Limit:         &limit,
		})
		if err != nil {
			t.Fatalf("workspace history list previous before %q: %v", second.Id, err)
		}
		if len(prev.Items) != 1 || prev.Items[0].Id != first.Id {
			t.Fatalf("workspace history previous page = %+v, want %q before %q", prev, first.Id, second.Id)
		}
	}

	latest, err := client.ListWorkspaceHistory(ctx, "social.chat.history.list.latest", rpcapi.WorkspaceHistoryListRequest{
		WorkspaceName: workspaceName,
		Order:         &desc,
		Limit:         &limit,
	})
	if err != nil {
		t.Fatalf("workspace history list latest desc: %v", err)
	}
	last := entries[len(entries)-1]
	if len(latest.Items) != 1 || latest.Items[0].Id != last.Id {
		t.Fatalf("workspace history latest desc page = %+v, want %q", latest, last.Id)
	}
}

func waitForWorkspaceHistoryReplayText(t *testing.T, ctx context.Context, stream genx.Stream, historyID string, want string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	boundStreamID := ""
	var got strings.Builder
	for {
		chunk, err := nextWorkspaceHistoryReplayChunk(ctx, stream)
		if err != nil {
			t.Fatalf("history replay %q stream read: %v", historyID, err)
		}
		if !socialChatReplayStreamChunk(chunk, &boundStreamID) {
			continue
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
			t.Fatalf("history replay %q stream %q returned error %q", historyID, boundStreamID, chunk.Ctrl.Error)
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			got.WriteString(string(text))
		}
		if chunk.IsEndOfStream() {
			if got.String() != want {
				t.Fatalf("history replay %q text = %q, want %q", historyID, got.String(), want)
			}
			return
		}
	}
}

func nextWorkspaceHistoryReplayChunk(ctx context.Context, stream genx.Stream) (*genx.MessageChunk, error) {
	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		chunk, err := stream.Next()
		ch <- result{chunk: chunk, err: err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			return nil, got.err
		}
		return got.chunk, nil
	case <-ctx.Done():
		_ = stream.CloseWithError(ctx.Err())
		return nil, ctx.Err()
	}
}

func socialChatReplayStreamChunk(chunk *genx.MessageChunk, boundStreamID *string) bool {
	if chunk == nil || chunk.Ctrl == nil {
		return false
	}
	streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
	if boundStreamID != nil && strings.TrimSpace(*boundStreamID) != "" {
		return streamID == *boundStreamID
	}
	if !strings.HasPrefix(streamID, "history-replay-") {
		return false
	}
	if boundStreamID != nil {
		*boundStreamID = streamID
	}
	return true
}

func waitForWorkspaceHistoryUpdated(stream genx.Stream) <-chan error {
	ch := make(chan error, 1)
	go func() {
		for {
			chunk, err := stream.Next()
			if err != nil {
				ch <- err
				return
			}
			if chunk == nil || chunk.Ctrl == nil {
				continue
			}
			if chunk.Ctrl.Label == "workspace.history.updated" && chunk.Ctrl.Timestamp > 0 {
				ch <- nil
				return
			}
		}
	}()
	return ch
}

func assertWorkspaceHistoryDenied(t *testing.T, h *clitest.Harness, contextName, workspaceName string) {
	t.Helper()

	client := h.ConnectClientFromContext(contextName)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.ListWorkspaceHistory(ctx, "social.chat.history.denied", rpcapi.WorkspaceHistoryListRequest{WorkspaceName: workspaceName}); err == nil {
		t.Fatalf("%s unexpectedly listed workspace history for %q", contextName, workspaceName)
	}
}

func waitForWorkspaceHistoryText(t *testing.T, ctx context.Context, client interface {
	ListWorkspaceHistory(context.Context, string, rpcapi.WorkspaceHistoryListRequest) (*rpcapi.WorkspaceHistoryListResponse, error)
}, workspaceName, text string) rpcapi.PeerRunHistoryEntry {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for {
		list, err := client.ListWorkspaceHistory(ctx, "social.chat.history.list", rpcapi.WorkspaceHistoryListRequest{WorkspaceName: workspaceName})
		if err == nil {
			for _, item := range list.Items {
				if item.Text == text {
					return item
				}
			}
			lastErr = nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			t.Fatalf("history text %q not found in workspace %q, last error: %v", text, workspaceName, lastErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func chatTextStream(text string) genx.Stream {
	return &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "chat-text", Label: "transcript"}},
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "chat-text", Label: "transcript", EndOfStream: true}},
	}}
}

type sliceStream struct {
	chunks []*genx.MessageChunk
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, genx.ErrDone
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *sliceStream) Close() error {
	s.chunks = nil
	return nil
}

func (s *sliceStream) CloseWithError(error) error {
	s.chunks = nil
	return nil
}

type blockingStream struct {
	done chan struct{}
	once sync.Once
}

func newBlockingStream() *blockingStream {
	return &blockingStream{done: make(chan struct{})}
}

func (s *blockingStream) Next() (*genx.MessageChunk, error) {
	<-s.done
	return nil, genx.ErrDone
}

func (s *blockingStream) Close() error {
	return s.CloseWithError(io.EOF)
}

func (s *blockingStream) CloseWithError(error) error {
	s.once.Do(func() {
		close(s.done)
	})
	return nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
