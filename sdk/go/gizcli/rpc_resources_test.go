package gizcli

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCResourceClientWrappers(t *testing.T) {
	client := &rpcClient{}

	t.Run("workspace", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceList, rpcapi.WorkspaceListResponse{}, (*rpcapi.RPCPayload).FromWorkspaceListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceListResponse, error) {
			return client.ListWorkspaces(ctx, conn, "workspace-list", rpcapi.WorkspaceListRequest{})
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
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceDelete, rpcapi.WorkspaceDeleteResponse{}, (*rpcapi.RPCPayload).FromWorkspaceDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceDeleteResponse, error) {
			return client.DeleteWorkspace(ctx, conn, "workspace-delete", rpcapi.WorkspaceDeleteRequest{Name: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceHistoryList, rpcapi.WorkspaceHistoryListResponse{}, (*rpcapi.RPCPayload).FromWorkspaceHistoryListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceHistoryListResponse, error) {
			return client.ListWorkspaceHistory(ctx, conn, "workspace-history-list", rpcapi.WorkspaceHistoryListRequest{WorkspaceName: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceHistoryGet, rpcapi.WorkspaceHistoryGetResponse{}, (*rpcapi.RPCPayload).FromWorkspaceHistoryGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceHistoryGetResponse, error) {
			return client.GetWorkspaceHistory(ctx, conn, "workspace-history-get", rpcapi.WorkspaceHistoryGetRequest{WorkspaceName: "main", HistoryId: "h1"})
		})
		runWorkspaceHistoryAudioGetWrapperTest(t, client)
	})

	t.Run("workflow", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowList, rpcapi.WorkflowListResponse{}, (*rpcapi.RPCPayload).FromWorkflowListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowListResponse, error) {
			return client.ListWorkflows(ctx, conn, "workflow-list", rpcapi.WorkflowListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowGet, rpcapi.WorkflowGetResponse{}, (*rpcapi.RPCPayload).FromWorkflowGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
			return client.GetWorkflow(ctx, conn, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowCreate, rpcapi.WorkflowCreateResponse{}, (*rpcapi.RPCPayload).FromWorkflowCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowCreateResponse, error) {
			return client.CreateWorkflow(ctx, conn, "workflow-create", rpcapi.WorkflowCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowPut, rpcapi.WorkflowPutResponse{}, (*rpcapi.RPCPayload).FromWorkflowPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowPutResponse, error) {
			return client.PutWorkflow(ctx, conn, "workflow-put", rpcapi.WorkflowPutRequest{Name: "flow"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowDelete, rpcapi.WorkflowDeleteResponse{}, (*rpcapi.RPCPayload).FromWorkflowDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowDeleteResponse, error) {
			return client.DeleteWorkflow(ctx, conn, "workflow-delete", rpcapi.WorkflowDeleteRequest{Name: "flow"})
		})
	})

	t.Run("model", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelList, rpcapi.ModelListResponse{}, (*rpcapi.RPCPayload).FromModelListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelListResponse, error) {
			return client.ListModels(ctx, conn, "model-list", rpcapi.ModelListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelGet, rpcapi.ModelGetResponse{}, (*rpcapi.RPCPayload).FromModelGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelGetResponse, error) {
			return client.GetModel(ctx, conn, "model-get", rpcapi.ModelGetRequest{Id: "llm"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelCreate, rpcapi.ModelCreateResponse{}, (*rpcapi.RPCPayload).FromModelCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelCreateResponse, error) {
			return client.CreateModel(ctx, conn, "model-create", rpcapi.ModelCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelPut, rpcapi.ModelPutResponse{}, (*rpcapi.RPCPayload).FromModelPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelPutResponse, error) {
			return client.PutModel(ctx, conn, "model-put", rpcapi.ModelPutRequest{Id: "llm"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelDelete, rpcapi.ModelDeleteResponse{}, (*rpcapi.RPCPayload).FromModelDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelDeleteResponse, error) {
			return client.DeleteModel(ctx, conn, "model-delete", rpcapi.ModelDeleteRequest{Id: "llm"})
		})
	})

	t.Run("credential", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialList, rpcapi.CredentialListResponse{}, (*rpcapi.RPCPayload).FromCredentialListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialListResponse, error) {
			return client.ListCredentials(ctx, conn, "credential-list", rpcapi.CredentialListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialGet, rpcapi.CredentialGetResponse{}, (*rpcapi.RPCPayload).FromCredentialGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialGetResponse, error) {
			return client.GetCredential(ctx, conn, "credential-get", rpcapi.CredentialGetRequest{Name: "openai"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialCreate, rpcapi.CredentialCreateResponse{}, (*rpcapi.RPCPayload).FromCredentialCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialCreateResponse, error) {
			return client.CreateCredential(ctx, conn, "credential-create", rpcapi.CredentialCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialPut, rpcapi.CredentialPutResponse{}, (*rpcapi.RPCPayload).FromCredentialPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialPutResponse, error) {
			return client.PutCredential(ctx, conn, "credential-put", rpcapi.CredentialPutRequest{Name: "openai"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialDelete, rpcapi.CredentialDeleteResponse{}, (*rpcapi.RPCPayload).FromCredentialDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialDeleteResponse, error) {
			return client.DeleteCredential(ctx, conn, "credential-delete", rpcapi.CredentialDeleteRequest{Name: "openai"})
		})
	})

	t.Run("social", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactList, rpcapi.ContactListResponse{}, (*rpcapi.RPCPayload).FromContactListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactListResponse, error) {
			return client.ListContacts(ctx, conn, "contact-list", rpcapi.ContactListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactGet, rpcapi.ContactGetResponse{}, (*rpcapi.RPCPayload).FromContactGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactGetResponse, error) {
			return client.GetContact(ctx, conn, "contact-get", rpcapi.ContactGetRequest{Id: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactCreate, rpcapi.ContactCreateResponse{}, (*rpcapi.RPCPayload).FromContactCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactCreateResponse, error) {
			return client.CreateContact(ctx, conn, "contact-create", rpcapi.ContactCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactPut, rpcapi.ContactPutResponse{}, (*rpcapi.RPCPayload).FromContactPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactPutResponse, error) {
			return client.PutContact(ctx, conn, "contact-put", rpcapi.ContactPutRequest{Id: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactDelete, rpcapi.ContactDeleteResponse{}, (*rpcapi.RPCPayload).FromContactDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactDeleteResponse, error) {
			return client.DeleteContact(ctx, conn, "contact-delete", rpcapi.ContactDeleteRequest{Id: "contact-a"})
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
			return client.DeleteFriend(ctx, conn, "friend-delete", rpcapi.FriendDeleteRequest{Id: "friend-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupList, rpcapi.FriendGroupListResponse{}, (*rpcapi.RPCPayload).FromFriendGroupListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupListResponse, error) {
			return client.ListFriendGroups(ctx, conn, "friend-group-list", rpcapi.FriendGroupListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupGet, rpcapi.FriendGroupGetResponse{}, (*rpcapi.RPCPayload).FromFriendGroupGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupGetResponse, error) {
			return client.GetFriendGroup(ctx, conn, "friend-group-get", rpcapi.FriendGroupGetRequest{Id: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupCreate, rpcapi.FriendGroupCreateResponse{}, (*rpcapi.RPCPayload).FromFriendGroupCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupCreateResponse, error) {
			return client.CreateFriendGroup(ctx, conn, "friend-group-create", rpcapi.FriendGroupCreateRequest{Name: "family"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupPut, rpcapi.FriendGroupPutResponse{}, (*rpcapi.RPCPayload).FromFriendGroupPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupPutResponse, error) {
			return client.PutFriendGroup(ctx, conn, "friend-group-put", rpcapi.FriendGroupPutRequest{Id: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupDelete, rpcapi.FriendGroupDeleteResponse{}, (*rpcapi.RPCPayload).FromFriendGroupDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupDeleteResponse, error) {
			return client.DeleteFriendGroup(ctx, conn, "friend-group-delete", rpcapi.FriendGroupDeleteRequest{Id: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupInviteTokenGet, rpcapi.FriendGroupInviteTokenGetResponse{}, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupInviteTokenGetResponse, error) {
			return client.GetFriendGroupInviteToken(ctx, conn, "friend-group-invite-token-get", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupInviteTokenCreate, rpcapi.FriendGroupInviteTokenCreateResponse{}, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupInviteTokenCreateResponse, error) {
			return client.CreateFriendGroupInviteToken(ctx, conn, "friend-group-invite-token-create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupInviteTokenClear, rpcapi.FriendGroupInviteTokenClearResponse{}, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenClearResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupInviteTokenClearResponse, error) {
			return client.ClearFriendGroupInviteToken(ctx, conn, "friend-group-invite-token-clear", rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupId: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupJoin, rpcapi.FriendGroupJoinResponse{}, (*rpcapi.RPCPayload).FromFriendGroupJoinResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupJoinResponse, error) {
			return client.JoinFriendGroup(ctx, conn, "friend-group-join", rpcapi.FriendGroupJoinRequest{InviteToken: "token-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersList, rpcapi.FriendGroupMemberListResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberListResponse, error) {
			return client.ListFriendGroupMembers(ctx, conn, "friend-group-members-list", rpcapi.FriendGroupMemberListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersAdd, rpcapi.FriendGroupMemberAddResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberAddResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberAddResponse, error) {
			return client.AddFriendGroupMember(ctx, conn, "friend-group-members-add", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: "group-a", PeerPublicKey: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersPut, rpcapi.FriendGroupMemberPutResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberPutResponse, error) {
			return client.PutFriendGroupMember(ctx, conn, "friend-group-members-put", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: "group-a", Id: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersDelete, rpcapi.FriendGroupMemberDeleteResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMemberDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
			return client.DeleteFriendGroupMember(ctx, conn, "friend-group-members-delete", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: "group-a", Id: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMessageListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageListResponse, error) {
			return client.ListFriendGroupMessages(ctx, conn, "friend-group-messages-list", rpcapi.FriendGroupMessageListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesGet, rpcapi.FriendGroupMessageGetResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMessageGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageGetResponse, error) {
			return client.GetFriendGroupMessage(ctx, conn, "friend-group-messages-get", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: "group-a", Id: "message-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesSend, rpcapi.FriendGroupMessageSendResponse{}, (*rpcapi.RPCPayload).FromFriendGroupMessageSendResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageSendResponse, error) {
			return client.SendFriendGroupMessage(ctx, conn, "friend-group-messages-send", rpcapi.FriendGroupMessageSendRequest{FriendGroupId: "group-a", AudioContentType: "audio/opus"})
		})
	})

	t.Run("firmware", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFirmwareList, rpcapi.FirmwareListResponse{}, (*rpcapi.RPCPayload).FromFirmwareListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FirmwareListResponse, error) {
			return client.ListFirmwares(ctx, conn, "firmware-list", rpcapi.FirmwareListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFirmwareGet, rpcapi.FirmwareGetResponse{}, (*rpcapi.RPCPayload).FromFirmwareGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FirmwareGetResponse, error) {
			return client.GetFirmware(ctx, conn, "firmware-get", rpcapi.FirmwareGetRequest{FirmwareId: "devkit"})
		})
		runFirmwareDownloadWrapperTest(t, client)
	})

	t.Run("gameplay pixa", func(t *testing.T) {
		runPetDefPixaDownloadWrapperTest(t, client)
		runBadgeDefPixaDownloadWrapperTest(t, client)
	})
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

func runFirmwareDownloadWrapperTest(t *testing.T, client *rpcClient) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("firmware-payload")
	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != rpcapi.RPCMethodServerFirmwareFilesDownload {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerFirmwareFilesDownload}
			return
		}
		resp := resourceResponse(req.Id, rpcapi.FirmwareFilesDownloadResponse{
			FirmwareId: "devkit",
			Channel:    rpcapi.FirmwareChannelNameStable,
			Path:       "firmware.bin",
			Artifact:   rpcapi.FirmwareArtifact{TarPath: "devkit/stable/artifact/artifact.tar", Size: 1024, ContentType: "application/x-tar"},
			File:       rpcapi.FirmwareArtifactEntry{Path: "firmware.bin", Type: rpcapi.FirmwareArtifactEntryTypeFile, Size: int64(len(payload))},
		}, (*rpcapi.RPCPayload).FromFirmwareFilesDownloadResponse)
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
	result, err := client.DownloadFirmware(context.Background(), clientSide, "firmware-download", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware.bin",
	}, &out)
	if err != nil {
		t.Fatalf("firmware download call error = %v", err)
	}
	if result.Metadata.File.Path != "firmware.bin" || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("firmware download result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("firmware download server error = %v", err)
	}
}

func runPetDefPixaDownloadWrapperTest(t *testing.T, client *rpcClient) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("petdef-pixa")
	pixaPath := "pet-defs/petdef-a/pixa"
	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != rpcapi.RPCMethodServerPetDefPixaDownload {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerPetDefPixaDownload}
			return
		}
		resp := resourceResponse(req.Id, rpcapi.PetDefPixaDownloadResponse{Id: "petdef-a", PixaPath: &pixaPath, SizeBytes: int64(len(payload))}, (*rpcapi.RPCPayload).FromPetDefPixaDownloadResponse)
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
	result, err := client.DownloadPetDefPixa(context.Background(), clientSide, "petdef-pixa-download", rpcapi.PetDefPixaDownloadRequest{Id: "petdef-a"}, &out)
	if err != nil {
		t.Fatalf("petdef pixa download call error = %v", err)
	}
	if result.Metadata.Id != "petdef-a" || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("petdef pixa download result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("petdef pixa download server error = %v", err)
	}
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
		resp := resourceResponse(req.Id, rpcapi.BadgeDefPixaDownloadResponse{Id: "badge-a", PixaPath: &pixaPath, SizeBytes: int64(len(payload))}, (*rpcapi.RPCPayload).FromBadgeDefPixaDownloadResponse)
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
	result, err := client.DownloadBadgeDefPixa(context.Background(), clientSide, "badgedef-pixa-download", rpcapi.BadgeDefPixaDownloadRequest{Id: "badge-a"}, &out)
	if err != nil {
		t.Fatalf("badgedef pixa download call error = %v", err)
	}
	if result.Metadata.Id != "badge-a" || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("badgedef pixa download result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("badgedef pixa download server error = %v", err)
	}
}

func runWorkspaceHistoryAudioGetWrapperTest(t *testing.T, client *rpcClient) {
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
		if req.Method != rpcapi.RPCMethodServerWorkspaceHistoryAudioGet {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerWorkspaceHistoryAudioGet}
			return
		}
		workspaceName := "main"
		if len(payload) > 0 {
			workspaceName = strings.Repeat("w", 70000)
		}
		resp := resourceResponse(req.Id, rpcapi.WorkspaceHistoryAudioGetResponse{
			WorkspaceName: workspaceName,
			HistoryId:     "h1",
			MimeType:      "audio/opus",
			SizeBytes:     int64(len(payload)),
		}, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioGetResponse)
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
	result, err := client.GetWorkspaceHistoryAudio(context.Background(), clientSide, "workspace-history-audio-get", rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "main",
		HistoryId:     "h1",
	}, &out)
	if err != nil {
		t.Fatalf("workspace history audio get call error = %v", err)
	}
	if result.Metadata.MimeType != "audio/opus" || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("workspace history audio get result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("workspace history audio get server error = %v", err)
	}
}

func resourceWorkspace(name string) rpcapi.Workspace {
	return rpcapi.Workspace{Name: name, WorkflowName: "flow-a"}
}

func resourceWorkflowDoc(name string) rpcapi.WorkflowDocument {
	spec := rpcapi.FlowcraftWorkflowSpec{"entry_agent": ""}
	return rpcapi.WorkflowDocument{
		Metadata: rpcapi.WorkflowMetadata{Name: name},
		Spec: rpcapi.WorkflowSpec{
			Driver:    rpcapi.WorkflowDriverFlowcraft,
			Flowcraft: &spec,
		},
	}
}

func resourceModel(id string) rpcapi.Model {
	return rpcapi.Model{
		Id:     id,
		Kind:   rpcapi.ModelKindLlm,
		Source: rpcapi.ModelSourceManual,
		Provider: rpcapi.ModelProvider{
			Kind: rpcapi.ModelProviderKindOpenaiTenant,
			Name: "global",
		},
	}
}

func resourceCredential(name string) rpcapi.Credential {
	return rpcapi.Credential{
		Name:     name,
		Provider: "openai",
		Body:     testRPCOpenAICredentialBody("sk-test"),
	}
}
