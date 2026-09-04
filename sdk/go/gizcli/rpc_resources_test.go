package gizcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCResourceClientWrappers(t *testing.T) {
	client := &rpcClient{}

	t.Run("API key", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerAPIKeyCreate, rpcapi.APIKeyCreateResponse{}, (*rpcapi.RPCPayload).FromAPIKeyCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.APIKeyCreateResponse, error) {
			return client.CreateAPIKey(ctx, conn, "api-key-create", rpcapi.APIKeyCreateRequest{DisplayName: "phone"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerAPIKeyList, rpcapi.APIKeyListResponse{}, (*rpcapi.RPCPayload).FromAPIKeyListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.APIKeyListResponse, error) {
			return client.ListAPIKeys(ctx, conn, "api-key-list", rpcapi.APIKeyListRequest{Limit: 25})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerAPIKeyRevoke, rpcapi.APIKeyRevokeResponse{}, (*rpcapi.RPCPayload).FromAPIKeyRevokeResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.APIKeyRevokeResponse, error) {
			return client.RevokeAPIKey(ctx, conn, "api-key-revoke", rpcapi.APIKeyRevokeRequest{Name: "key-a"})
		})
	})

	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerPeerDelete, rpcapi.ServerPeerDeleteResponse{}, (*rpcapi.RPCPayload).FromServerPeerDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerPeerDeleteResponse, error) {
		return client.DeletePeer(ctx, conn, "peer-delete", rpcapi.ServerPeerDeleteRequest{})
	})

	t.Run("workspace", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceList, rpcapi.WorkspaceListResponse{}, (*rpcapi.RPCPayload).FromWorkspaceListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceListResponse, error) {
			return client.ListWorkspaces(ctx, conn, "workspace-list", rpcapi.WorkspaceListRequest{Collection: "assistants"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceGet, rpcapi.WorkspaceGetResponse{}, (*rpcapi.RPCPayload).FromWorkspaceGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceGetResponse, error) {
			return client.GetWorkspace(ctx, conn, "workspace-get", rpcapi.WorkspaceGetRequest{Name: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceCreate, rpcapi.WorkspaceCreateResponse{}, (*rpcapi.RPCPayload).FromWorkspaceCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceCreateResponse, error) {
			return client.CreateWorkspace(ctx, conn, "workspace-create", rpcapi.WorkspaceCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspacePut, rpcapi.WorkspacePutResponse{}, (*rpcapi.RPCPayload).FromWorkspacePutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspacePutResponse, error) {
			return client.PutWorkspace(ctx, conn, "workspace-put", rpcapi.WorkspacePutRequest{Name: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceInputPut, rpcapi.WorkspaceInputPutResponse{}, (*rpcapi.RPCPayload).FromWorkspaceInputPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceInputPutResponse, error) {
			return client.PutWorkspaceInput(ctx, conn, "workspace-input-put", rpcapi.WorkspaceInputPutRequest{Name: "main", Input: rpcapi.WorkspaceInputModeRealtime})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceDelete, rpcapi.WorkspaceDeleteResponse{}, (*rpcapi.RPCPayload).FromWorkspaceDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceDeleteResponse, error) {
			return client.DeleteWorkspace(ctx, conn, "workspace-delete", rpcapi.WorkspaceDeleteRequest{Name: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceHistoryList, rpcapi.WorkspaceHistoryListResponse{}, (*rpcapi.RPCPayload).FromWorkspaceHistoryListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceHistoryListResponse, error) {
			return client.ListWorkspaceHistory(ctx, conn, "workspace-history-list", rpcapi.WorkspaceHistoryListRequest{WorkspaceName: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceHistoryGet, rpcapi.WorkspaceHistoryGetResponse{}, (*rpcapi.RPCPayload).FromWorkspaceHistoryGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceHistoryGetResponse, error) {
			return client.GetWorkspaceHistory(ctx, conn, "workspace-history-get", rpcapi.WorkspaceHistoryGetRequest{WorkspaceName: "main", HistoryName: "h1"})
		})
		runWorkspaceHistoryAudioDownloadWrapperTest(t, client)
	})

	t.Run("workflow", func(t *testing.T) {
		runWorkflowGetWrapperTest(t, client)
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowList, rpcapi.WorkflowListResponse{}, (*rpcapi.RPCPayload).FromWorkflowListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowListResponse, error) {
			return client.ListWorkflows(ctx, conn, "workflow-list", rpcapi.WorkflowListRequest{Collection: "assistants"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowGet, rpcapi.WorkflowGetResponse{}, (*rpcapi.RPCPayload).FromWorkflowGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
			return client.GetWorkflow(ctx, conn, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow"})
		})
	})

	t.Run("model", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelList, rpcapi.ModelListResponse{}, (*rpcapi.RPCPayload).FromModelListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelListResponse, error) {
			return client.ListModels(ctx, conn, "model-list", rpcapi.ModelListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelGet, rpcapi.ModelGetResponse{Value: resourceModel("llm")}, (*rpcapi.RPCPayload).FromModelGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelGetResponse, error) {
			return client.GetModel(ctx, conn, "model-get", rpcapi.ModelGetRequest{Name: "llm"})
		})
	})

	t.Run("social", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactList, rpcapi.ContactListResponse{}, (*rpcapi.RPCPayload).FromContactListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactListResponse, error) {
			return client.ListContacts(ctx, conn, "contact-list", rpcapi.ContactListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactGet, rpcapi.ContactGetResponse{}, (*rpcapi.RPCPayload).FromContactGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactGetResponse, error) {
			return client.GetContact(ctx, conn, "contact-get", rpcapi.ContactGetRequest{Name: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactCreate, rpcapi.ContactCreateResponse{}, (*rpcapi.RPCPayload).FromContactCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactCreateResponse, error) {
			return client.CreateContact(ctx, conn, "contact-create", rpcapi.ContactCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactPut, rpcapi.ContactPutResponse{}, (*rpcapi.RPCPayload).FromContactPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactPutResponse, error) {
			return client.PutContact(ctx, conn, "contact-put", rpcapi.ContactPutRequest{Name: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactDelete, rpcapi.ContactDeleteResponse{}, (*rpcapi.RPCPayload).FromContactDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactDeleteResponse, error) {
			return client.DeleteContact(ctx, conn, "contact-delete", rpcapi.ContactDeleteRequest{Name: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendInviteTokenGet, rpcapi.FriendInviteTokenGetResponse{}, (*rpcapi.RPCPayload).FromFriendInviteTokenGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendInviteTokenGetResponse, error) {
			return client.GetFriendInviteToken(ctx, conn, "friend-invite-token-get", rpcapi.FriendInviteTokenGetRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendInviteTokenCreate, rpcapi.FriendInviteTokenCreateResponse{}, (*rpcapi.RPCPayload).FromFriendInviteTokenCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendInviteTokenCreateResponse, error) {
			return client.CreateFriendInviteToken(ctx, conn, "friend-invite-token-create", rpcapi.FriendInviteTokenCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendInviteTokenClear, rpcapi.FriendInviteTokenClearResponse{}, (*rpcapi.RPCPayload).FromFriendInviteTokenClearResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendInviteTokenClearResponse, error) {
			return client.ClearFriendInviteToken(ctx, conn, "friend-invite-token-clear", rpcapi.FriendInviteTokenClearRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendAdd, rpcapi.FriendAddResponse{}, (*rpcapi.RPCPayload).FromFriendAddResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendAddResponse, error) {
			return client.AddFriend(ctx, conn, "friend-add", rpcapi.FriendAddRequest{InviteToken: "token-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendList, rpcapi.FriendListResponse{}, (*rpcapi.RPCPayload).FromFriendListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendListResponse, error) {
			return client.ListFriends(ctx, conn, "friend-list", rpcapi.FriendListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendDelete, rpcapi.FriendDeleteResponse{}, (*rpcapi.RPCPayload).FromFriendDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendDeleteResponse, error) {
			return client.DeleteFriend(ctx, conn, "friend-delete", rpcapi.FriendDeleteRequest{Name: "friend-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupList, rpcapi.FriendGroupListResponse{}, (*rpcapi.RPCPayload).FromFriendGroupListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupListResponse, error) {
			return client.ListFriendGroups(ctx, conn, "friend-group-list", rpcapi.FriendGroupListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupGet, rpcapi.FriendGroupGetResponse{}, (*rpcapi.RPCPayload).FromFriendGroupGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupGetResponse, error) {
			return client.GetFriendGroup(ctx, conn, "friend-group-get", rpcapi.FriendGroupGetRequest{Name: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupCreate, rpcapi.FriendGroupCreateResponse{}, (*rpcapi.RPCPayload).FromFriendGroupCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupCreateResponse, error) {
			return client.CreateFriendGroup(ctx, conn, "friend-group-create", rpcapi.FriendGroupCreateRequest{Name: "family"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupPut, rpcapi.FriendGroupPutResponse{}, (*rpcapi.RPCPayload).FromFriendGroupPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupPutResponse, error) {
			return client.PutFriendGroup(ctx, conn, "friend-group-put", rpcapi.FriendGroupPutRequest{Name: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupDelete, rpcapi.FriendGroupDeleteResponse{}, (*rpcapi.RPCPayload).FromFriendGroupDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupDeleteResponse, error) {
			return client.DeleteFriendGroup(ctx, conn, "friend-group-delete", rpcapi.FriendGroupDeleteRequest{Name: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupInviteTokenGet, rpcapi.FriendGroupInviteTokenGetResponse{}, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupInviteTokenGetResponse, error) {
			return client.GetFriendGroupInviteToken(ctx, conn, "friend-group-invite-token-get", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupName: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupInviteTokenCreate, rpcapi.FriendGroupInviteTokenCreateResponse{}, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupInviteTokenCreateResponse, error) {
			return client.CreateFriendGroupInviteToken(ctx, conn, "friend-group-invite-token-create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupInviteTokenClear, rpcapi.FriendGroupInviteTokenClearResponse{}, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenClearResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupInviteTokenClearResponse, error) {
			return client.ClearFriendGroupInviteToken(ctx, conn, "friend-group-invite-token-clear", rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupName: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupJoin, rpcapi.FriendGroupJoinResponse{}, (*rpcapi.RPCPayload).FromFriendGroupJoinResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupJoinResponse, error) {
			return client.JoinFriendGroup(ctx, conn, "friend-group-join", rpcapi.FriendGroupJoinRequest{InviteToken: "token-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersList, rpcapi.FriendGroupMemberListResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberListResponse, error) {
			return client.ListFriendGroupMembers(ctx, conn, "friend-group-members-list", rpcapi.FriendGroupMemberListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersAdd, rpcapi.FriendGroupMemberAddResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberAddResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberAddResponse, error) {
			return client.AddFriendGroupMember(ctx, conn, "friend-group-members-add", rpcapi.FriendGroupMemberAddRequest{FriendGroupName: "group-a", PeerPublicKey: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersPut, rpcapi.FriendGroupMemberPutResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberPutResponse, error) {
			return client.PutFriendGroupMember(ctx, conn, "friend-group-members-put", rpcapi.FriendGroupMemberPutRequest{FriendGroupName: "group-a", Name: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersDelete, rpcapi.FriendGroupMemberDeleteResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
			return client.DeleteFriendGroupMember(ctx, conn, "friend-group-members-delete", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupName: "group-a", Name: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMessageListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageListResponse, error) {
			return client.ListFriendGroupMessages(ctx, conn, "friend-group-messages-list", rpcapi.FriendGroupMessageListRequest{FriendGroupName: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesGet, rpcapi.FriendGroupMessageGetResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMessageGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageGetResponse, error) {
			return client.GetFriendGroupMessage(ctx, conn, "friend-group-messages-get", rpcapi.FriendGroupMessageGetRequest{FriendGroupName: "group-a", HistoryName: "history-a"})
		})
		runFriendGroupMessageAudioDownloadWrapperTest(t, client)
	})

	t.Run("firmware", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFirmwareGet, rpcapi.FirmwareGetResponse{}, (*rpcapi.RPCPayload).FromFirmwareGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FirmwareGetResponse, error) {
			return client.GetFirmware(ctx, conn, "firmware-get", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNameStable})
		})
	})

	t.Run("gameplay pixa", func(t *testing.T) {
		runBadgeDefPixaDownloadWrapperTest(t, client)
	})
}

func TestClientDeletePeerUsesRPCConnection(t *testing.T) {
	client, serverConn, cleanup := connectedFirmwareTestClient(t)
	defer cleanup()

	listener := serverConn.ListenService(ServicePeerRPC)
	defer listener.Close()
	serverErrCh := make(chan error, 1)
	go func() {
		stream, err := listener.Accept()
		if err != nil {
			serverErrCh <- err
			return
		}
		req, err := readRPCRequestWithEOS(stream)
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != rpcapi.RPCMethodServerPeerDelete {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerPeerDelete}
			return
		}
		serverErrCh <- writeRPCResponseWithEOS(stream, req.Method, resourceResponse(req.Id, rpcapi.ServerPeerDeleteResponse{}, (*rpcapi.RPCPayload).FromServerPeerDeleteResponse))
	}()

	response, err := client.DeletePeer(context.Background(), "peer-delete", rpcapi.ServerPeerDeleteRequest{})
	if err != nil {
		t.Fatalf("DeletePeer() error = %v", err)
	}
	if response == nil {
		t.Fatal("DeletePeer() response = nil")
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("peer delete server error = %v", err)
	}
}

func TestRPCRegisterReturnsRuntimeProfileName(t *testing.T) {
	client := &rpcClient{}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverErrCh <- writeRPCResponseWithEOS(serverSide, req.Method, resourceResponse(
			req.Id,
			rpcapi.ServerRegisterResponse{RuntimeProfileName: "profile-a"},
			(*rpcapi.RPCPayload).FromServerRegisterResponse,
		))
	}()

	response, err := client.Register(context.Background(), clientSide, "register", "token")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if response.RuntimeProfileName != "profile-a" {
		t.Fatalf("Register() response = %#v", response)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("register server error = %v", err)
	}
}

func runWorkflowGetWrapperTest(t *testing.T, client *rpcClient) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverErrCh <- writeRPCResponseWithEOS(serverSide, req.Method, resourceResponse(
			req.Id,
			resourceWorkflowDoc("localized-flow"),
			(*rpcapi.RPCPayload).FromWorkflowGetResponse,
		))
	}()

	got, err := client.GetWorkflow(context.Background(), clientSide, "workflow-get", rpcapi.WorkflowGetRequest{Name: "localized-flow"})
	if err != nil {
		t.Fatalf("workflow get call error = %v", err)
	}
	if got.Value.Name != "localized-flow" {
		t.Fatalf("workflow get = %#v", got)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("workflow get server error = %v", err)
	}
}

func runRPCResultWrapperTest[Resp any](
	t *testing.T,
	wantMethod rpcapi.RPCMethod,
	response Resp,
	encode func(*rpcapi.RPCPayload, Resp) error,
	call func(context.Context, net.Conn) (*Resp, error),
) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != wantMethod {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: wantMethod}
			return
		}
		serverErrCh <- writeRPCResponseWithEOS(serverSide, req.Method, resourceResponse(req.Id, response, encode))
	}()

	resp, err := call(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("%s call error = %v", wantMethod, err)
	}
	if resp == nil {
		t.Fatalf("%s response = nil", wantMethod)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("%s server error = %v", wantMethod, err)
	}
}

type unexpectedRPCMethodError struct {
	got  rpcapi.RPCMethod
	want rpcapi.RPCMethod
}

func (e *unexpectedRPCMethodError) Error() string {
	return "unexpected RPC method: got " + string(e.got) + ", want " + string(e.want)
}

func resourceResponse[T any](id string, value T, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCResponse {
	resp, err := newRPCResultResponse(id, value, encode)
	if err != nil {
		panic(err)
	}
	return resp
}

func runBadgeDefPixaDownloadWrapperTest(t *testing.T, client *rpcClient) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("badgedef-pixa")
	pixaPath := "badge-defs/badge-a/pixa"
	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != rpcapi.RPCMethodServerBadgeDefPixaDownload {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerBadgeDefPixaDownload}
			return
		}
		resp := resourceResponse(req.Id, rpcapi.BadgeDefPixaDownloadResponse{Name: "badge-a", PixaPath: &pixaPath, SizeBytes: int64(len(payload))}, (*rpcapi.RPCPayload).FromBadgeDefPixaDownloadResponse)
		if err := rpcapi.WriteResponseForMethod(serverSide, req.Method, resp); err != nil {
			serverErrCh <- err
			return
		}
		if err := rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload}); err != nil {
			serverErrCh <- err
			return
		}
		serverErrCh <- rpcapi.WriteEOS(serverSide)
	}()

	var out bytes.Buffer
	result, err := client.DownloadBadgeDefPixa(context.Background(), clientSide, "badgedef-pixa-download", rpcapi.BadgeDefPixaDownloadRequest{Name: "badge-a"}, &out)
	if err != nil {
		t.Fatalf("badgedef pixa download call error = %v", err)
	}
	if result.Metadata.Name != "badge-a" || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("badgedef pixa download result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("badgedef pixa download server error = %v", err)
	}
}

func runWorkspaceHistoryAudioDownloadWrapperTest(t *testing.T, client *rpcClient) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("opus-payload")
	mimeType := "audio/" + strings.Repeat("x", 70_000)
	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != rpcapi.RPCMethodServerWorkspaceHistoryAudioDownload {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerWorkspaceHistoryAudioDownload}
			return
		}
		resp := resourceResponse(req.Id, rpcapi.WorkspaceHistoryAudioDownloadResponse{
			WorkspaceName: "main",
			HistoryName:   "h1",
			MimeType:      mimeType,
			SizeBytes:     int64(len(payload)),
		}, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioDownloadResponse)
		serverStream, err := newRPCStream(context.Background(), serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		metadataEOS, err := serverStream.WriteResponseEnvelopeForMethod(req.Method, resp)
		if err != nil {
			serverErrCh <- err
			return
		}
		if metadataEOS {
			if err := serverStream.WriteEOS(); err != nil {
				serverErrCh <- err
				return
			}
		}
		if err := rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload}); err != nil {
			serverErrCh <- err
			return
		}
		serverErrCh <- rpcapi.WriteEOS(serverSide)
	}()

	var out bytes.Buffer
	result, err := client.DownloadWorkspaceHistoryAudio(context.Background(), clientSide, "workspace-history-audio-download", rpcapi.WorkspaceHistoryAudioDownloadRequest{
		WorkspaceName: "main",
		HistoryName:   "h1",
	}, &out)
	if err != nil {
		t.Fatalf("workspace history audio download call error = %v", err)
	}
	if result.Metadata.MimeType != mimeType || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("workspace history audio download result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("workspace history audio download server error = %v", err)
	}
}

func runFriendGroupMessageAudioDownloadWrapperTest(t *testing.T, client *rpcClient) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("opus-payload")
	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != rpcapi.RPCMethodServerFriendGroupMessagesAudioDownload {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerFriendGroupMessagesAudioDownload}
			return
		}
		resp := resourceResponse(req.Id, rpcapi.FriendGroupMessageAudioDownloadResponse{
			FriendGroupName: "group-a",
			HistoryName:     "history-a",
			MimeType:        "audio/opus",
			SizeBytes:       int64(len(payload)),
		}, (*rpcapi.RPCPayload).FromFriendGroupMessageAudioDownloadResponse)
		serverStream, err := newRPCStream(context.Background(), serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		if _, err := serverStream.WriteResponseEnvelopeForMethod(req.Method, resp); err != nil {
			serverErrCh <- err
			return
		}
		if err := rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload}); err != nil {
			serverErrCh <- err
			return
		}
		serverErrCh <- rpcapi.WriteEOS(serverSide)
	}()

	var out bytes.Buffer
	result, err := client.DownloadFriendGroupMessageAudio(context.Background(), clientSide, "friend-group-message-audio-download", rpcapi.FriendGroupMessageAudioDownloadRequest{
		FriendGroupName: "group-a",
		HistoryName:     "history-a",
	}, &out)
	if err != nil {
		t.Fatalf("friend group message audio download call error = %v", err)
	}
	if result.Metadata.MimeType != "audio/opus" || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("friend group message audio download result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("friend group message audio download server error = %v", err)
	}
}

func TestValidateHistoryAudioMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mimeType string
		size     int64
		wantErr  bool
	}{
		{name: "opus", mimeType: "audio/opus", size: 0},
		{name: "opus parameters", mimeType: "audio/ogg; codecs=opus", size: 1},
		{name: "non audio", mimeType: "application/octet-stream", size: 1, wantErr: true},
		{name: "invalid MIME", mimeType: "not a MIME", size: 1, wantErr: true},
		{name: "negative size", mimeType: "audio/opus", size: -1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHistoryAudioMetadata(tc.mimeType, tc.size)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateHistoryAudioMetadata(%q, %d) error = %v, wantErr=%v", tc.mimeType, tc.size, err, tc.wantErr)
			}
		})
	}
}

func TestDownloadWorkspaceHistoryAudioRejectsMalformedResponse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata rpcapi.WorkspaceHistoryAudioDownloadResponse
		payload  []byte
		want     string
	}{
		{name: "workspace identity", metadata: rpcapi.WorkspaceHistoryAudioDownloadResponse{WorkspaceName: "other", HistoryName: "h1", MimeType: "audio/opus"}, want: "identity mismatch"},
		{name: "history identity", metadata: rpcapi.WorkspaceHistoryAudioDownloadResponse{WorkspaceName: "main", HistoryName: "other", MimeType: "audio/opus"}, want: "identity mismatch"},
		{name: "MIME", metadata: rpcapi.WorkspaceHistoryAudioDownloadResponse{WorkspaceName: "main", HistoryName: "h1", MimeType: "application/octet-stream"}, want: "MIME type"},
		{name: "negative size", metadata: rpcapi.WorkspaceHistoryAudioDownloadResponse{WorkspaceName: "main", HistoryName: "h1", MimeType: "audio/opus", SizeBytes: -1}, want: "invalid history audio size"},
		{name: "truncated", metadata: rpcapi.WorkspaceHistoryAudioDownloadResponse{WorkspaceName: "main", HistoryName: "h1", MimeType: "audio/opus", SizeBytes: 5}, payload: []byte("opus"), want: "size mismatch"},
		{name: "extra", metadata: rpcapi.WorkspaceHistoryAudioDownloadResponse{WorkspaceName: "main", HistoryName: "h1", MimeType: "audio/opus", SizeBytes: 3}, payload: []byte("opus"), want: "size mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverSide, clientSide := net.Pipe()
			serverErrCh := make(chan error, 1)
			go func() {
				defer serverSide.Close()
				req, err := readRPCRequestWithEOS(serverSide)
				if err != nil {
					serverErrCh <- err
					return
				}
				stream, err := newRPCStream(context.Background(), serverSide)
				if err != nil {
					serverErrCh <- err
					return
				}
				resp := resourceResponse(req.Id, tc.metadata, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioDownloadResponse)
				if _, err := stream.WriteResponseEnvelopeForMethod(req.Method, resp); err != nil {
					serverErrCh <- err
					return
				}
				if tc.payload != nil {
					if err := rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: tc.payload}); err != nil {
						serverErrCh <- err
						return
					}
					if err := rpcapi.WriteEOS(serverSide); err != nil {
						serverErrCh <- err
						return
					}
				}
				serverErrCh <- nil
			}()

			var out bytes.Buffer
			_, err := (&rpcClient{}).DownloadWorkspaceHistoryAudio(context.Background(), clientSide, "malformed", rpcapi.WorkspaceHistoryAudioDownloadRequest{WorkspaceName: "main", HistoryName: "h1"}, &out)
			_ = clientSide.Close()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("DownloadWorkspaceHistoryAudio() error = %v, want %q", err, tc.want)
			}
			if serverErr := <-serverErrCh; serverErr != nil {
				t.Fatalf("server error = %v", serverErr)
			}
		})
	}
}

type friendGroupAudioErrorWriter struct {
	err error
}

func (w friendGroupAudioErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type friendGroupAudioShortWriter struct{}

func (friendGroupAudioShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestDownloadFriendGroupMessageAudioRejectsMalformedResponse(t *testing.T) {
	writerErr := errors.New("friend group audio writer failed")
	request := rpcapi.FriendGroupMessageAudioDownloadRequest{FriendGroupName: "group-a", HistoryName: "history-a"}
	validMetadata := rpcapi.FriendGroupMessageAudioDownloadResponse{
		FriendGroupName: request.FriendGroupName,
		HistoryName:     request.HistoryName,
		MimeType:        "audio/opus",
		SizeBytes:       4,
	}

	for _, tc := range []struct {
		name         string
		metadata     rpcapi.FriendGroupMessageAudioDownloadResponse
		frames       []rpcapi.Frame
		writeEOS     bool
		writer       io.Writer
		wantContains string
		wantIs       error
	}{
		{
			name:         "friend group identity",
			metadata:     rpcapi.FriendGroupMessageAudioDownloadResponse{FriendGroupName: "other", HistoryName: request.HistoryName, MimeType: "audio/opus"},
			wantContains: "identity mismatch",
		},
		{
			name:         "history identity",
			metadata:     rpcapi.FriendGroupMessageAudioDownloadResponse{FriendGroupName: request.FriendGroupName, HistoryName: "other", MimeType: "audio/opus"},
			wantContains: "identity mismatch",
		},
		{
			name:         "non audio MIME",
			metadata:     rpcapi.FriendGroupMessageAudioDownloadResponse{FriendGroupName: request.FriendGroupName, HistoryName: request.HistoryName, MimeType: "application/octet-stream"},
			wantContains: "MIME type",
		},
		{
			name:         "malformed MIME",
			metadata:     rpcapi.FriendGroupMessageAudioDownloadResponse{FriendGroupName: request.FriendGroupName, HistoryName: request.HistoryName, MimeType: "not a MIME"},
			wantContains: "MIME type",
		},
		{
			name:         "negative size",
			metadata:     rpcapi.FriendGroupMessageAudioDownloadResponse{FriendGroupName: request.FriendGroupName, HistoryName: request.HistoryName, MimeType: "audio/opus", SizeBytes: -1},
			wantContains: "invalid history audio size",
		},
		{
			name:         "unexpected frame",
			metadata:     validMetadata,
			frames:       []rpcapi.Frame{{Type: rpcapi.FrameTypeText, Payload: []byte(`{}`)}},
			wantContains: "expected binary frame",
		},
		{
			name:         "truncated",
			metadata:     validMetadata,
			frames:       []rpcapi.Frame{{Type: rpcapi.FrameTypeBinary, Payload: []byte("opu")}},
			writeEOS:     true,
			wantContains: "size mismatch",
		},
		{
			name:         "extra",
			metadata:     validMetadata,
			frames:       []rpcapi.Frame{{Type: rpcapi.FrameTypeBinary, Payload: []byte("opus!")}},
			writeEOS:     true,
			wantContains: "size mismatch",
		},
		{
			name:     "writer error",
			metadata: validMetadata,
			frames:   []rpcapi.Frame{{Type: rpcapi.FrameTypeBinary, Payload: []byte("opus")}},
			writer:   friendGroupAudioErrorWriter{err: writerErr},
			wantIs:   writerErr,
		},
		{
			name:     "short write",
			metadata: validMetadata,
			frames:   []rpcapi.Frame{{Type: rpcapi.FrameTypeBinary, Payload: []byte("opus")}},
			writer:   friendGroupAudioShortWriter{},
			wantIs:   io.ErrShortWrite,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverSide, clientSide := net.Pipe()
			serverErrCh := make(chan error, 1)
			go func() {
				defer serverSide.Close()
				req, err := readFriendGroupAudioTestRequest(serverSide, request)
				if err != nil {
					serverErrCh <- err
					return
				}
				resp := resourceResponse(req.Id, tc.metadata, (*rpcapi.RPCPayload).FromFriendGroupMessageAudioDownloadResponse)
				if err := rpcapi.WriteResponseForMethod(serverSide, req.Method, resp); err != nil {
					serverErrCh <- err
					return
				}
				for _, frame := range tc.frames {
					if err := rpcapi.WriteFrame(serverSide, frame); err != nil {
						serverErrCh <- err
						return
					}
				}
				if tc.writeEOS {
					if err := rpcapi.WriteEOS(serverSide); err != nil {
						serverErrCh <- err
						return
					}
				}
				serverErrCh <- nil
			}()

			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			writer := tc.writer
			if writer == nil {
				writer = &bytes.Buffer{}
			}
			result, err := (&rpcClient{}).DownloadFriendGroupMessageAudio(ctx, clientSide, "friend-group-audio-failure", request, writer)
			_ = clientSide.Close()
			if err == nil {
				t.Fatal("DownloadFriendGroupMessageAudio() error = nil")
			}
			if tc.wantContains != "" && !strings.Contains(err.Error(), tc.wantContains) {
				t.Fatalf("DownloadFriendGroupMessageAudio() error = %v, want %q", err, tc.wantContains)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Fatalf("DownloadFriendGroupMessageAudio() error = %v, want errors.Is(%v)", err, tc.wantIs)
			}
			if result != (FriendGroupMessageAudioDownloadResult{}) {
				t.Fatalf("DownloadFriendGroupMessageAudio() result = %#v, want zero result", result)
			}
			waitFriendGroupAudioServer(t, serverErrCh)
		})
	}
}

func TestDownloadFriendGroupMessageAudioReturnsTypedMissingAudioError(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	request := rpcapi.FriendGroupMessageAudioDownloadRequest{FriendGroupName: "group-a", HistoryName: "missing-history"}
	serverErrCh := make(chan error, 1)
	go func() {
		defer serverSide.Close()
		req, err := readFriendGroupAudioTestRequest(serverSide, request)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverErrCh <- writeRPCResponseWithEOS(serverSide, req.Method, rpcapi.Error{
			RequestID: req.Id,
			Code:      rpcapi.StatusCodeNotFound,
			Message:   "not found",
		}.RPCResponse())
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var out bytes.Buffer
	result, err := (&rpcClient{}).DownloadFriendGroupMessageAudio(ctx, clientSide, "friend-group-audio-missing", request, &out)
	if err == nil {
		t.Fatal("DownloadFriendGroupMessageAudio() error = nil")
	}
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("DownloadFriendGroupMessageAudio() error = %T, want rpcapi.Error", err)
	}
	if rpcErr.RequestID != "friend-group-audio-missing" || rpcErr.Code != rpcapi.StatusCodeNotFound || rpcErr.Message != "not found" {
		t.Fatalf("DownloadFriendGroupMessageAudio() rpc error = %+v", rpcErr)
	}
	if result != (FriendGroupMessageAudioDownloadResult{}) {
		t.Fatalf("DownloadFriendGroupMessageAudio() result = %#v, want zero result", result)
	}
	if out.Len() != 0 {
		t.Fatalf("DownloadFriendGroupMessageAudio() output = %q, want empty", out.String())
	}
	waitFriendGroupAudioServer(t, serverErrCh)
}

func TestDownloadFriendGroupMessageAudioStopsOnContextAndTransportFailure(t *testing.T) {
	request := rpcapi.FriendGroupMessageAudioDownloadRequest{FriendGroupName: "group-a", HistoryName: "history-a"}
	for _, tc := range []struct {
		name               string
		deadline           time.Duration
		cancelAfterRequest bool
		closeAfterRequest  bool
		want               error
	}{
		{name: "caller cancellation", cancelAfterRequest: true, want: context.Canceled},
		{name: "caller deadline", deadline: 50 * time.Millisecond, want: context.DeadlineExceeded},
		{name: "remote close", closeAfterRequest: true, want: io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deadline := tc.deadline
			if deadline == 0 {
				deadline = 2 * time.Second
			}
			ctx, cancel := context.WithTimeout(t.Context(), deadline)
			defer cancel()
			cancelRequest := context.CancelFunc(func() {})
			if tc.cancelAfterRequest {
				ctx, cancelRequest = context.WithCancel(ctx)
				defer cancelRequest()
			}

			serverSide, clientSide := net.Pipe()
			serverErrCh := make(chan error, 1)
			go func() {
				defer serverSide.Close()
				if _, err := readFriendGroupAudioTestRequest(serverSide, request); err != nil {
					serverErrCh <- err
					return
				}
				switch {
				case tc.cancelAfterRequest:
					cancelRequest()
					<-ctx.Done()
				case tc.closeAfterRequest:
				default:
					<-ctx.Done()
				}
				serverErrCh <- nil
			}()

			var out bytes.Buffer
			result, err := (&rpcClient{}).DownloadFriendGroupMessageAudio(ctx, clientSide, "friend-group-audio-lifecycle", request, &out)
			_ = clientSide.Close()
			if !errors.Is(err, tc.want) {
				t.Fatalf("DownloadFriendGroupMessageAudio() error = %v, want errors.Is(%v)", err, tc.want)
			}
			if result != (FriendGroupMessageAudioDownloadResult{}) {
				t.Fatalf("DownloadFriendGroupMessageAudio() result = %#v, want zero result", result)
			}
			waitFriendGroupAudioServer(t, serverErrCh)
		})
	}
}

func readFriendGroupAudioTestRequest(conn net.Conn, want rpcapi.FriendGroupMessageAudioDownloadRequest) (*rpcapi.RPCRequest, error) {
	req, err := readRPCRequestWithEOS(conn)
	if err != nil {
		return nil, err
	}
	if req.Method != rpcapi.RPCMethodServerFriendGroupMessagesAudioDownload {
		return nil, &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerFriendGroupMessagesAudioDownload}
	}
	if req.Params == nil {
		return nil, errors.New("friend group message audio request params are missing")
	}
	got, err := req.Params.AsFriendGroupMessageAudioDownloadRequest()
	if err != nil {
		return nil, err
	}
	if got != want {
		return nil, fmt.Errorf("friend group message audio request = %#v, want %#v", got, want)
	}
	return req, nil
}

func waitFriendGroupAudioServer(t *testing.T, serverErrCh <-chan error) {
	t.Helper()
	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("friend group audio server error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("friend group audio server goroutine did not exit")
	}
}

func resourceWorkspace(name string) rpcapi.Workspace {
	return rpcapi.Workspace{Name: name, WorkflowName: "flow-a"}
}

func resourceWorkflowDoc(alias string) rpcapi.WorkflowGetResponse {
	return rpcapi.WorkflowGetResponse{
		Value: rpcapi.Workflow{Name: alias, Collection: "assistants", Driver: rpcapi.WorkflowDriverFlowcraft,
			I18n: map[string]rpcapi.ResourceI18nText{"en": {DisplayName: alias}, "zh-CN": {DisplayName: alias}}},
		RuntimeProfileName: "default", RuntimeProfileRevision: "revision",
	}
}

func resourceModel(alias string) rpcapi.Model {
	return rpcapi.Model{
		Name: alias, Kind: rpcapi.ModelKindLlm,
		I18n:         map[string]rpcapi.ResourceI18nText{"en": {DisplayName: alias}, "zh-CN": {DisplayName: alias}},
		ProviderKind: rpcapi.ModelProviderKindOpenaiTenant,
		OpenAITenant: &rpcapi.OpenAITenantModelProviderData{},
	}
}

func TestPeerHTTPClientDeviceAndContactOperations(t *testing.T) {
	type call struct{ method, path, body string }
	var calls []call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, call{method: r.Method, path: r.URL.RequestURI(), body: string(body)})
		if r.Header.Get("Authorization") != "Bearer gizclaw_sk_v1_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gizclaw/v1/device/status":
			_, _ = w.Write([]byte(`{"volume":35,"muted":true,"battery_percent":80}`))
		case r.Method == http.MethodPut && r.URL.Path == "/gizclaw/v1/device/volume":
			_, _ = w.Write([]byte(`{"status":{"volume":35,"muted":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/gizclaw/v1/device/actions/play-sound":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/gizclaw/v1/device/wifi/saved":
			_, _ = w.Write([]byte(`{"networks":[{"ssid":"home"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/gizclaw/v1/device/wifi/saved/home":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/gizclaw/v1/contacts":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"mom","display_name":"Mom"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/gizclaw/v1/contacts":
			_, _ = w.Write([]byte(`{"items":[{"name":"mom"}],"has_next":false}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/gizclaw/v1/contacts/mom":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	authorize := func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer gizclaw_sk_v1_test")
		return nil
	}
	client, err := peerhttp.NewClientWithResponses(server.URL, peerhttp.WithRequestEditorFn(authorize))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	status, err := client.GetDeviceStatusWithResponse(ctx)
	if err != nil || status.JSON200 == nil || status.JSON200.Volume == nil || *status.JSON200.Volume != 35 || !*status.JSON200.Muted {
		t.Fatalf("GetDeviceStatus = %+v, %v", status, err)
	}
	volume, err := client.SetDeviceVolumeWithResponse(ctx, peerhttp.SetDeviceVolumeJSONRequestBody{Level: 35, Muted: true})
	if err != nil || volume.JSON200 == nil || *volume.JSON200.Status.Volume != 35 {
		t.Fatalf("SetDeviceVolume = %+v, %v", volume, err)
	}
	sound, err := client.PlayDeviceSoundWithResponse(ctx, peerhttp.PlayDeviceSoundJSONRequestBody{Sound: "chime"})
	if err != nil || sound.StatusCode() != http.StatusNoContent {
		t.Fatalf("PlayDeviceSound = %+v, %v", sound, err)
	}
	saved, err := client.ListDeviceSavedWifiWithResponse(ctx)
	if err != nil || saved.JSON200 == nil || len(saved.JSON200.Networks) != 1 || saved.JSON200.Networks[0].Ssid != "home" {
		t.Fatalf("ListDeviceSavedWifi = %+v, %v", saved, err)
	}
	forget, err := client.ForgetDeviceSavedWifiWithResponse(ctx, "home")
	if err != nil || forget.StatusCode() != http.StatusNoContent {
		t.Fatalf("ForgetDeviceSavedWifi = %+v, %v", forget, err)
	}
	created, err := client.CreateContactWithResponse(ctx, peerhttp.CreateContactJSONRequestBody{Name: "mom", DisplayName: new("Mom")})
	if err != nil || created.JSON201 == nil || created.JSON201.Name != "mom" {
		t.Fatalf("CreateContact = %+v, %v", created, err)
	}
	limit := int32(10)
	listed, err := client.ListContactsWithResponse(ctx, &peerhttp.ListContactsParams{Limit: &limit})
	if err != nil || listed.JSON200 == nil || len(listed.JSON200.Items) != 1 {
		t.Fatalf("ListContacts = %+v, %v", listed, err)
	}
	deleted, err := client.DeleteContactWithResponse(ctx, "mom")
	if err != nil || deleted.StatusCode() != http.StatusNoContent {
		t.Fatalf("DeleteContact = %+v, %v", deleted, err)
	}

	want := []call{
		{http.MethodGet, "/gizclaw/v1/device/status", ""},
		{http.MethodPut, "/gizclaw/v1/device/volume", `{"level":35,"muted":true}`},
		{http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"chime"}`},
		{http.MethodGet, "/gizclaw/v1/device/wifi/saved", ""},
		{http.MethodDelete, "/gizclaw/v1/device/wifi/saved/home", ""},
		{http.MethodPost, "/gizclaw/v1/contacts", `{"display_name":"Mom","name":"mom"}`},
		{http.MethodGet, "/gizclaw/v1/contacts?limit=10", ""},
		{http.MethodDelete, "/gizclaw/v1/contacts/mom", ""},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v", calls)
	}
	for i := range want {
		got := calls[i]
		got.body = strings.TrimSpace(got.body)
		if got != want[i] {
			t.Fatalf("call %d = %+v, want %+v", i, got, want[i])
		}
	}
}
