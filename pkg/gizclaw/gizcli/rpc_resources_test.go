package gizcli

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func TestRPCResourceClientWrappers(t *testing.T) {
	client := &rpcClient{}

	t.Run("workspace", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceList, rpcapi.WorkspaceListResponse{}, (*rpcapi.RPCResponse_Result).FromWorkspaceListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceListResponse, error) {
			return client.ListWorkspaces(ctx, conn, "workspace-list", rpcapi.WorkspaceListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceGet, rpcapi.WorkspaceGetResponse{}, (*rpcapi.RPCResponse_Result).FromWorkspaceGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceGetResponse, error) {
			return client.GetWorkspace(ctx, conn, "workspace-get", rpcapi.WorkspaceGetRequest{Name: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceCreate, rpcapi.WorkspaceCreateResponse{}, (*rpcapi.RPCResponse_Result).FromWorkspaceCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceCreateResponse, error) {
			return client.CreateWorkspace(ctx, conn, "workspace-create", rpcapi.WorkspaceCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspacePut, rpcapi.WorkspacePutResponse{}, (*rpcapi.RPCResponse_Result).FromWorkspacePutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspacePutResponse, error) {
			return client.PutWorkspace(ctx, conn, "workspace-put", rpcapi.WorkspacePutRequest{Name: "main"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkspaceDelete, rpcapi.WorkspaceDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromWorkspaceDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkspaceDeleteResponse, error) {
			return client.DeleteWorkspace(ctx, conn, "workspace-delete", rpcapi.WorkspaceDeleteRequest{Name: "main"})
		})
	})

	t.Run("workflow", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowList, rpcapi.WorkflowListResponse{}, (*rpcapi.RPCResponse_Result).FromWorkflowListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowListResponse, error) {
			return client.ListWorkflows(ctx, conn, "workflow-list", rpcapi.WorkflowListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowGet, rpcapi.WorkflowGetResponse{}, (*rpcapi.RPCResponse_Result).FromWorkflowGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
			return client.GetWorkflow(ctx, conn, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowCreate, rpcapi.WorkflowCreateResponse{}, (*rpcapi.RPCResponse_Result).FromWorkflowCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowCreateResponse, error) {
			return client.CreateWorkflow(ctx, conn, "workflow-create", rpcapi.WorkflowCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowPut, rpcapi.WorkflowPutResponse{}, (*rpcapi.RPCResponse_Result).FromWorkflowPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowPutResponse, error) {
			return client.PutWorkflow(ctx, conn, "workflow-put", rpcapi.WorkflowPutRequest{Name: "flow"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerWorkflowDelete, rpcapi.WorkflowDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromWorkflowDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.WorkflowDeleteResponse, error) {
			return client.DeleteWorkflow(ctx, conn, "workflow-delete", rpcapi.WorkflowDeleteRequest{Name: "flow"})
		})
	})

	t.Run("model", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelList, rpcapi.ModelListResponse{}, (*rpcapi.RPCResponse_Result).FromModelListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelListResponse, error) {
			return client.ListModels(ctx, conn, "model-list", rpcapi.ModelListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelGet, rpcapi.ModelGetResponse{}, (*rpcapi.RPCResponse_Result).FromModelGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelGetResponse, error) {
			return client.GetModel(ctx, conn, "model-get", rpcapi.ModelGetRequest{Id: "llm"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelCreate, rpcapi.ModelCreateResponse{}, (*rpcapi.RPCResponse_Result).FromModelCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelCreateResponse, error) {
			return client.CreateModel(ctx, conn, "model-create", rpcapi.ModelCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelPut, rpcapi.ModelPutResponse{}, (*rpcapi.RPCResponse_Result).FromModelPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelPutResponse, error) {
			return client.PutModel(ctx, conn, "model-put", rpcapi.ModelPutRequest{Id: "llm"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerModelDelete, rpcapi.ModelDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromModelDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ModelDeleteResponse, error) {
			return client.DeleteModel(ctx, conn, "model-delete", rpcapi.ModelDeleteRequest{Id: "llm"})
		})
	})

	t.Run("credential", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialList, rpcapi.CredentialListResponse{}, (*rpcapi.RPCResponse_Result).FromCredentialListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialListResponse, error) {
			return client.ListCredentials(ctx, conn, "credential-list", rpcapi.CredentialListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialGet, rpcapi.CredentialGetResponse{}, (*rpcapi.RPCResponse_Result).FromCredentialGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialGetResponse, error) {
			return client.GetCredential(ctx, conn, "credential-get", rpcapi.CredentialGetRequest{Name: "openai"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialCreate, rpcapi.CredentialCreateResponse{}, (*rpcapi.RPCResponse_Result).FromCredentialCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialCreateResponse, error) {
			return client.CreateCredential(ctx, conn, "credential-create", rpcapi.CredentialCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialPut, rpcapi.CredentialPutResponse{}, (*rpcapi.RPCResponse_Result).FromCredentialPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialPutResponse, error) {
			return client.PutCredential(ctx, conn, "credential-put", rpcapi.CredentialPutRequest{Name: "openai"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerCredentialDelete, rpcapi.CredentialDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromCredentialDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.CredentialDeleteResponse, error) {
			return client.DeleteCredential(ctx, conn, "credential-delete", rpcapi.CredentialDeleteRequest{Name: "openai"})
		})
	})

	t.Run("social", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactList, rpcapi.ContactListResponse{}, (*rpcapi.RPCResponse_Result).FromContactListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactListResponse, error) {
			return client.ListContacts(ctx, conn, "contact-list", rpcapi.ContactListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactGet, rpcapi.ContactGetResponse{}, (*rpcapi.RPCResponse_Result).FromContactGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactGetResponse, error) {
			return client.GetContact(ctx, conn, "contact-get", rpcapi.ContactGetRequest{Id: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactCreate, rpcapi.ContactCreateResponse{}, (*rpcapi.RPCResponse_Result).FromContactCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactCreateResponse, error) {
			return client.CreateContact(ctx, conn, "contact-create", rpcapi.ContactCreateRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactPut, rpcapi.ContactPutResponse{}, (*rpcapi.RPCResponse_Result).FromContactPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactPutResponse, error) {
			return client.PutContact(ctx, conn, "contact-put", rpcapi.ContactPutRequest{Id: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerContactDelete, rpcapi.ContactDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromContactDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ContactDeleteResponse, error) {
			return client.DeleteContact(ctx, conn, "contact-delete", rpcapi.ContactDeleteRequest{Id: "contact-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendRequestsList, rpcapi.FriendRequestListResponse{}, (*rpcapi.RPCResponse_Result).FromFriendRequestListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendRequestListResponse, error) {
			return client.ListFriendRequests(ctx, conn, "friend-requests-list", rpcapi.FriendRequestListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendRequestsCreate, rpcapi.FriendRequestCreateResponse{}, (*rpcapi.RPCResponse_Result).FromFriendRequestCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendRequestCreateResponse, error) {
			return client.CreateFriendRequest(ctx, conn, "friend-requests-create", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendRequestsAccept, rpcapi.FriendRequestAcceptResponse{}, (*rpcapi.RPCResponse_Result).FromFriendRequestAcceptResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendRequestAcceptResponse, error) {
			return client.AcceptFriendRequest(ctx, conn, "friend-requests-accept", rpcapi.FriendRequestAcceptRequest{Id: "request-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendRequestsReject, rpcapi.FriendRequestRejectResponse{}, (*rpcapi.RPCResponse_Result).FromFriendRequestRejectResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendRequestRejectResponse, error) {
			return client.RejectFriendRequest(ctx, conn, "friend-requests-reject", rpcapi.FriendRequestRejectRequest{Id: "request-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendList, rpcapi.FriendListResponse{}, (*rpcapi.RPCResponse_Result).FromFriendListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendListResponse, error) {
			return client.ListFriends(ctx, conn, "friend-list", rpcapi.FriendListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendDelete, rpcapi.FriendDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromFriendDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendDeleteResponse, error) {
			return client.DeleteFriend(ctx, conn, "friend-delete", rpcapi.FriendDeleteRequest{Id: "friend-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupList, rpcapi.FriendGroupListResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupListResponse, error) {
			return client.ListFriendGroups(ctx, conn, "friend-group-list", rpcapi.FriendGroupListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupGet, rpcapi.FriendGroupGetResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupGetResponse, error) {
			return client.GetFriendGroup(ctx, conn, "friend-group-get", rpcapi.FriendGroupGetRequest{Id: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupCreate, rpcapi.FriendGroupCreateResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupCreateResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupCreateResponse, error) {
			return client.CreateFriendGroup(ctx, conn, "friend-group-create", rpcapi.FriendGroupCreateRequest{Name: "family"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupPut, rpcapi.FriendGroupPutResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupPutResponse, error) {
			return client.PutFriendGroup(ctx, conn, "friend-group-put", rpcapi.FriendGroupPutRequest{Id: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupDelete, rpcapi.FriendGroupDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupDeleteResponse, error) {
			return client.DeleteFriendGroup(ctx, conn, "friend-group-delete", rpcapi.FriendGroupDeleteRequest{Id: "group-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersList, rpcapi.FriendGroupMemberListResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberListResponse, error) {
			return client.ListFriendGroupMembers(ctx, conn, "friend-group-members-list", rpcapi.FriendGroupMemberListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersAdd, rpcapi.FriendGroupMemberAddResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberAddResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberAddResponse, error) {
			return client.AddFriendGroupMember(ctx, conn, "friend-group-members-add", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: "group-a", PeerId: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersPut, rpcapi.FriendGroupMemberPutResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberPutResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberPutResponse, error) {
			return client.PutFriendGroupMember(ctx, conn, "friend-group-members-put", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: "group-a", Id: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMembersDelete, rpcapi.FriendGroupMemberDeleteResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberDeleteResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMemberDeleteResponse, error) {
			return client.DeleteFriendGroupMember(ctx, conn, "friend-group-members-delete", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: "group-a", Id: "peer-b"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageListResponse, error) {
			return client.ListFriendGroupMessages(ctx, conn, "friend-group-messages-list", rpcapi.FriendGroupMessageListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesGet, rpcapi.FriendGroupMessageGetResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageGetResponse, error) {
			return client.GetFriendGroupMessage(ctx, conn, "friend-group-messages-get", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: "group-a", Id: "message-a"})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFriendGroupMessagesSend, rpcapi.FriendGroupMessageSendResponse{}, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageSendResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FriendGroupMessageSendResponse, error) {
			return client.SendFriendGroupMessage(ctx, conn, "friend-group-messages-send", rpcapi.FriendGroupMessageSendRequest{FriendGroupId: "group-a", AudioContentType: "audio/opus"})
		})
	})

	t.Run("firmware", func(t *testing.T) {
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFirmwareList, rpcapi.FirmwareListResponse{}, (*rpcapi.RPCResponse_Result).FromFirmwareListResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FirmwareListResponse, error) {
			return client.ListFirmwares(ctx, conn, "firmware-list", rpcapi.FirmwareListRequest{})
		})
		runRPCResultWrapperTest(t, rpcapi.RPCMethodServerFirmwareGet, rpcapi.FirmwareGetResponse{}, (*rpcapi.RPCResponse_Result).FromFirmwareGetResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.FirmwareGetResponse, error) {
			return client.GetFirmware(ctx, conn, "firmware-get", rpcapi.FirmwareGetRequest{FirmwareId: "devkit"})
		})
		runFirmwareDownloadWrapperTest(t, client)
	})
}

func runRPCResultWrapperTest[Resp any](
	t *testing.T,
	wantMethod rpcapi.RPCMethod,
	response Resp,
	encode func(*rpcapi.RPCResponse_Result, Resp) error,
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
		serverErrCh <- writeRPCResponseWithEOS(serverSide, resourceResponse(req.Id, response, encode))
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

func resourceResponse[T any](id string, value T, encode func(*rpcapi.RPCResponse_Result, T) error) *rpcapi.RPCResponse {
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
		if req.Method != rpcapi.RPCMethodServerFirmwareDownload {
			serverErrCh <- &unexpectedRPCMethodError{got: req.Method, want: rpcapi.RPCMethodServerFirmwareDownload}
			return
		}
		resp := resourceResponse(req.Id, rpcapi.FirmwareDownloadResponse{
			FirmwareId: "devkit",
			Channel:    rpcapi.FirmwareChannelNameStable,
			Artifact: rpcapi.FirmwareBinMetadata{
				Name: "main",
				Kind: rpcapi.FirmwareArtifactKindApp,
			},
		}, (*rpcapi.RPCResponse_Result).FromFirmwareDownloadResponse)
		if err := rpcapi.WriteResponse(serverSide, resp); err != nil {
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
	result, err := client.DownloadFirmware(context.Background(), clientSide, "firmware-download", rpcapi.FirmwareDownloadRequest{
		FirmwareId:   "devkit",
		Channel:      rpcapi.FirmwareChannelNameStable,
		ArtifactName: "main",
	}, &out)
	if err != nil {
		t.Fatalf("firmware download call error = %v", err)
	}
	if result.Metadata.Artifact.Name != "main" || result.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("firmware download result = %#v payload %q", result, out.String())
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("firmware download server error = %v", err)
	}
}

func resourceWorkspace(name string) rpcapi.Workspace {
	return rpcapi.Workspace{Name: name, WorkflowName: "flow-a"}
}

func resourceWorkflowDoc(name string) rpcapi.WorkflowDocument {
	return rpcapi.WorkflowDocument{
		ApiVersion: rpcapi.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
		Kind:       rpcapi.FlowcraftWorkflowKindFlowcraftWorkflow,
		Metadata:   rpcapi.WorkflowMetadata{Name: name},
		Spec:       rpcapi.FlowcraftWorkflowSpec{"entry_agent": ""},
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
