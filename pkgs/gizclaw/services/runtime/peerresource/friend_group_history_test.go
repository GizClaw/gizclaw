package peerresource

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestFriendGroupMessagesProjectSingleWorkspaceHistoryPage(t *testing.T) {
	ctx := t.Context()
	server, history, groupID := newFriendGroupHistoryServer(t)
	order := rpcapi.WorkspaceHistoryListRequestOrderDesc
	limit := 1
	request := friendGroupHistoryRPCRequest(t, "list", rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListRequest{
		FriendGroupName: groupID,
		Limit:           &limit,
		Order:           &order,
	}, (*rpcapi.RPCPayload).FromFriendGroupMessageListRequest)

	response, handled, err := server.Dispatch(ctx, request)
	if err != nil || !handled || response.Error != nil {
		t.Fatalf("Dispatch(list) = response=%#v handled=%v error=%v", response, handled, err)
	}
	result, err := response.Result.AsFriendGroupMessageListResponse()
	if err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if history.listPageCalls != 1 || history.getCalls != 0 || history.workspaceName != "group-workspace" {
		t.Fatalf("history calls = listPage:%d get:%d workspace:%q", history.listPageCalls, history.getCalls, history.workspaceName)
	}
	if history.listRequest.Limit == nil || *history.listRequest.Limit != limit || history.listRequest.Order == nil || *history.listRequest.Order != apitypes.PeerRunHistoryListRequestOrderDesc {
		t.Fatalf("history list request = %#v", history.listRequest)
	}
	if len(result.Items) != 1 || !result.HasNext || result.NextCursor == nil || *result.NextCursor != "next-history" {
		t.Fatalf("list response = %#v", result)
	}
	item := result.Items[0]
	if item.FriendGroupName != groupID || item.HistoryId != "history-1" || item.Name != "gear" || item.Text != "hello" || item.Type != rpcapi.PeerRunHistoryEntryTypeGear || !item.AudioAvailable || item.SenderPeerPublicKey == nil || *item.SenderPeerPublicKey != "peer-sender" {
		t.Fatalf("projected item = %#v", item)
	}
}

func TestFriendGroupMessagesListPreservesCursorOrderAndStableHistoryIDs(t *testing.T) {
	server, history, groupID := newFriendGroupHistoryServer(t)
	createdAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	history.page = workspace.HistoryEntryPage{
		Entries: []workspace.HistoryEntry{
			{ID: "history-2", Type: "gear", GearID: "peer-sender", Origin: "conversation-stream-1", Name: "gear", CreatedAt: createdAt},
			{ID: "history-3", Type: "gear", GearID: "peer-sender", Origin: "conversation-stream-1", Name: "gear", CreatedAt: createdAt.Add(time.Second)},
		},
		HasNext: true, NextCursor: "history-3",
	}
	cursor := "history-1"
	limit := 2
	order := rpcapi.WorkspaceHistoryListRequestOrderAsc
	request := friendGroupHistoryRPCRequest(t, "page", rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListRequest{
		FriendGroupName: groupID,
		Cursor:          &cursor,
		Limit:           &limit,
		Order:           &order,
	}, (*rpcapi.RPCPayload).FromFriendGroupMessageListRequest)

	response, handled, err := server.Dispatch(t.Context(), request)
	if err != nil || !handled || response.Error != nil {
		t.Fatalf("Dispatch(page) = response=%#v handled=%v error=%v", response, handled, err)
	}
	result, err := response.Result.AsFriendGroupMessageListResponse()
	if err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if history.listPageCalls != 1 || history.getCalls != 0 || history.listRequest.Cursor == nil || *history.listRequest.Cursor != cursor || history.listRequest.Limit == nil || *history.listRequest.Limit != limit || history.listRequest.Order == nil || *history.listRequest.Order != apitypes.PeerRunHistoryListRequestOrderAsc {
		t.Fatalf("history page request/calls = request:%#v list:%d get:%d", history.listRequest, history.listPageCalls, history.getCalls)
	}
	if len(result.Items) != 2 || result.Items[0].HistoryId != "history-2" || result.Items[1].HistoryId != "history-3" || !result.HasNext || result.NextCursor == nil || *result.NextCursor != "history-3" {
		t.Fatalf("list response = %#v", result)
	}
}

func TestFriendGroupMessagesGetAndAudioUseResolvedWorkspace(t *testing.T) {
	ctx := t.Context()
	server, history, groupID := newFriendGroupHistoryServer(t)
	request := friendGroupHistoryRPCRequest(t, "get", rpcapi.RPCMethodServerFriendGroupMessagesGet, rpcapi.FriendGroupMessageGetRequest{
		FriendGroupName: groupID,
		HistoryId:       "history-1",
	}, (*rpcapi.RPCPayload).FromFriendGroupMessageGetRequest)
	response, handled, err := server.Dispatch(ctx, request)
	if err != nil || !handled || response.Error != nil {
		t.Fatalf("Dispatch(get) = response=%#v handled=%v error=%v", response, handled, err)
	}
	item, err := response.Result.AsFriendGroupMessageGetResponse()
	if err != nil || item.HistoryId != "history-1" || history.getCalls != 1 || history.workspaceName != "group-workspace" {
		t.Fatalf("get = item=%#v decode=%v calls=%d workspace=%q", item, err, history.getCalls, history.workspaceName)
	}

	metadata, reader, rpcErr, err := server.PrepareFriendGroupMessageAudioGet(ctx, rpcapi.FriendGroupMessageAudioGetRequest{FriendGroupName: groupID, HistoryId: "history-1"})
	if err != nil || rpcErr != nil {
		t.Fatalf("PrepareFriendGroupMessageAudioGet() = metadata=%#v rpcErr=%#v error=%v", metadata, rpcErr, err)
	}
	defer reader.Close()
	audio, err := io.ReadAll(reader)
	if err != nil || string(audio) != "opus" || metadata.FriendGroupName != groupID || metadata.HistoryId != "history-1" || metadata.MimeType != "audio/opus" || metadata.SizeBytes != 4 {
		t.Fatalf("audio = metadata=%#v bytes=%q error=%v", metadata, audio, err)
	}
}

func TestFriendGroupMessageAudioRejectsMissingHistoryAudio(t *testing.T) {
	server, history, groupID := newFriendGroupHistoryServer(t)
	history.entry.Assets = nil

	metadata, reader, rpcErr, err := server.PrepareFriendGroupMessageAudioGet(t.Context(), rpcapi.FriendGroupMessageAudioGetRequest{FriendGroupName: groupID, HistoryId: "history-1"})
	if err != nil || reader != nil || rpcErr == nil || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || rpcErr.Message != "not found" {
		t.Fatalf("PrepareFriendGroupMessageAudioGet(missing audio) = metadata=%#v reader=%v rpcErr=%#v error=%v", metadata, reader, rpcErr, err)
	}
}

func TestFriendGroupMessageProjectionDoesNotInferSenderOrAudio(t *testing.T) {
	item := friendGroupMessageProjection(" group-a ", workspace.HistoryEntry{
		ID: "history-agent", Type: "agent", GearID: "must-not-leak", Name: "assistant", Text: "text only",
		CreatedAt: time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC), ReplayAvailable: true,
	})
	if item.FriendGroupName != "group-a" || item.HistoryId != "history-agent" || item.SenderPeerPublicKey != nil || item.AudioAvailable {
		t.Fatalf("agent text projection = %#v", item)
	}
}

func TestFriendGroupMessagesRejectNonMemberBeforeHistoryRead(t *testing.T) {
	server, history, groupID := newFriendGroupHistoryServer(t)
	other, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(other): %v", err)
	}
	server.Caller = other.Public
	request := friendGroupHistoryRPCRequest(t, "denied", rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListRequest{FriendGroupName: groupID}, (*rpcapi.RPCPayload).FromFriendGroupMessageListRequest)
	response, handled, err := server.Dispatch(t.Context(), request)
	if err != nil || !handled || response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("Dispatch(non-member) = response=%#v handled=%v error=%v", response, handled, err)
	}
	if history.listPageCalls != 0 || history.getCalls != 0 {
		t.Fatalf("history read before authorization = list:%d get:%d", history.listPageCalls, history.getCalls)
	}
}

func TestFriendGroupMessagesRejectRemovedMemberAcrossReadMethods(t *testing.T) {
	server, history, groupID := newFriendGroupHistoryServer(t)
	caller := server.Caller.String()
	if err := server.FriendGroups.Members.Delete(t.Context(), socialutil.GroupMemberKey(groupID, caller)); err != nil {
		t.Fatalf("remove current member: %v", err)
	}

	listRequest := friendGroupHistoryRPCRequest(t, "removed-list", rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListRequest{FriendGroupName: groupID}, (*rpcapi.RPCPayload).FromFriendGroupMessageListRequest)
	listResponse, handled, err := server.Dispatch(t.Context(), listRequest)
	if err != nil || !handled || listResponse.Error == nil || listResponse.Error.Code != rpcapi.RPCErrorCodeNotFound || listResponse.Error.Message != "not found" {
		t.Fatalf("Dispatch(removed list) = response=%#v handled=%v error=%v", listResponse, handled, err)
	}

	getRequest := friendGroupHistoryRPCRequest(t, "removed-get", rpcapi.RPCMethodServerFriendGroupMessagesGet, rpcapi.FriendGroupMessageGetRequest{FriendGroupName: groupID, HistoryId: "history-1"}, (*rpcapi.RPCPayload).FromFriendGroupMessageGetRequest)
	getResponse, handled, err := server.Dispatch(t.Context(), getRequest)
	if err != nil || !handled || getResponse.Error == nil || getResponse.Error.Code != rpcapi.RPCErrorCodeNotFound || getResponse.Error.Message != "not found" {
		t.Fatalf("Dispatch(removed get) = response=%#v handled=%v error=%v", getResponse, handled, err)
	}

	metadata, reader, rpcErr, err := server.PrepareFriendGroupMessageAudioGet(t.Context(), rpcapi.FriendGroupMessageAudioGetRequest{FriendGroupName: groupID, HistoryId: "history-1"})
	if err != nil || reader != nil || rpcErr == nil || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || rpcErr.Message != "not found" {
		t.Fatalf("PrepareFriendGroupMessageAudioGet(removed) = metadata=%#v reader=%v rpcErr=%#v error=%v", metadata, reader, rpcErr, err)
	}
	if history.listPageCalls != 0 || history.getCalls != 0 {
		t.Fatalf("history read for removed member = list:%d get:%d", history.listPageCalls, history.getCalls)
	}
}

func TestFriendGroupMessagesRejectDeletedGroupBeforeHistoryRead(t *testing.T) {
	server, history, groupID := newFriendGroupHistoryServer(t)
	if err := server.FriendGroups.Groups.Delete(t.Context(), socialutil.GroupKey(groupID)); err != nil {
		t.Fatalf("delete group record: %v", err)
	}

	request := friendGroupHistoryRPCRequest(t, "deleted", rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListRequest{FriendGroupName: groupID}, (*rpcapi.RPCPayload).FromFriendGroupMessageListRequest)
	response, handled, err := server.Dispatch(t.Context(), request)
	if err != nil || !handled || response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound || response.Error.Message != "not found" {
		t.Fatalf("Dispatch(deleted) = response=%#v handled=%v error=%v", response, handled, err)
	}
	if history.listPageCalls != 0 || history.getCalls != 0 {
		t.Fatalf("history read for deleted group = list:%d get:%d", history.listPageCalls, history.getCalls)
	}
}

func TestFriendGroupMessagesRejectPendingDeletionBeforeHistoryRead(t *testing.T) {
	server, history, groupID := newFriendGroupHistoryServer(t)
	pending, err := pendingdeletion.New(
		pendingdeletion.KindFriendGroup,
		groupID,
		nil,
		pendingdeletion.ReasonFriendGroupDelete,
		map[string]string{"friend_group_id": groupID},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("pendingdeletion.New(): %v", err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(t.Context(), server.FriendGroups.RelationshipStore, pending); err != nil {
		t.Fatalf("pendingdeletion.CreateOrGet(): %v", err)
	}

	request := friendGroupHistoryRPCRequest(t, "retiring", rpcapi.RPCMethodServerFriendGroupMessagesList, rpcapi.FriendGroupMessageListRequest{FriendGroupName: groupID}, (*rpcapi.RPCPayload).FromFriendGroupMessageListRequest)
	response, handled, err := server.Dispatch(t.Context(), request)
	if err != nil || !handled || response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound || response.Error.Message != "not found" {
		t.Fatalf("Dispatch(retiring) = response=%#v handled=%v error=%v", response, handled, err)
	}
	if history.listPageCalls != 0 || history.getCalls != 0 {
		t.Fatalf("history read for retiring group = list:%d get:%d", history.listPageCalls, history.getCalls)
	}
}

func newFriendGroupHistoryServer(t *testing.T) (*Server, *fakeFriendGroupHistoryWorkspace, string) {
	t.Helper()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	store := kv.NewMemory(nil)
	groupID := "group-a"
	workspaceName := "group-workspace"
	role := rpcapi.FriendGroupMemberRoleMember
	if err := socialutil.WriteJSON(t.Context(), store, socialutil.GroupKey(groupID), rpcapi.FriendGroupObject{Name: groupID, WorkspaceName: &workspaceName}); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	caller := keyPair.Public.String()
	if err := socialutil.WriteJSON(t.Context(), store, socialutil.GroupMemberKey(groupID, caller), rpcapi.FriendGroupMemberObject{FriendGroupName: &groupID, PeerPublicKey: &caller, Role: &role}); err != nil {
		t.Fatalf("seed group member: %v", err)
	}
	if err := store.Set(t.Context(), socialutil.GroupNameKey(caller, groupID), []byte(groupID)); err != nil {
		t.Fatalf("seed group name index: %v", err)
	}
	createdAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	history := &fakeFriendGroupHistoryWorkspace{page: workspace.HistoryEntryPage{
		Entries: []workspace.HistoryEntry{{
			ID: "history-1", Type: "gear", GearID: "peer-sender", Name: "gear", Text: "hello", CreatedAt: createdAt,
			Assets: []workspace.HistoryAsset{{Name: "audio.opus", MIMEType: "audio/opus", Bytes: 4}},
		}},
		HasNext: true, NextCursor: "next-history",
	}}
	history.entry = history.page.Entries[0]
	return &Server{
		Caller: keyPair.Public,
		FriendGroups: &friendgroup.Server{
			Groups: store, Members: store, Belongs: store, RelationshipStore: store,
		},
		Workspaces: history,
	}, history, groupID
}

func friendGroupHistoryRPCRequest[T any](t *testing.T, id string, method rpcapi.RPCMethod, value T, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCRequest {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := encode(&params, value); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	return &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: id, Method: method, Params: &params}
}

type fakeFriendGroupHistoryWorkspace struct {
	page          workspace.HistoryEntryPage
	entry         workspace.HistoryEntry
	listRequest   apitypes.PeerRunHistoryListRequest
	workspaceName string
	listPageCalls int
	getCalls      int
}

func (*fakeFriendGroupHistoryWorkspace) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	return nil, nil
}
func (*fakeFriendGroupHistoryWorkspace) CreateWorkspace(context.Context, adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	return nil, nil
}
func (*fakeFriendGroupHistoryWorkspace) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return nil, nil
}
func (*fakeFriendGroupHistoryWorkspace) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return nil, nil
}
func (*fakeFriendGroupHistoryWorkspace) PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	return nil, nil
}
func (*fakeFriendGroupHistoryWorkspace) ListWorkspaceHistory(context.Context, string, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return apitypes.PeerRunHistoryListResponse{}, nil
}
func (f *fakeFriendGroupHistoryWorkspace) ListWorkspaceHistoryPage(_ context.Context, name string, request apitypes.PeerRunHistoryListRequest) (workspace.HistoryEntryPage, error) {
	f.listPageCalls++
	f.workspaceName = name
	f.listRequest = request
	return f.page, nil
}
func (f *fakeFriendGroupHistoryWorkspace) GetWorkspaceHistory(_ context.Context, name, historyID string) (workspace.HistoryEntry, error) {
	f.getCalls++
	f.workspaceName = name
	if strings.TrimSpace(historyID) != f.entry.ID {
		return workspace.HistoryEntry{}, kv.ErrNotFound
	}
	return f.entry, nil
}
func (*fakeFriendGroupHistoryWorkspace) ReadWorkspaceHistoryAsset(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("opus")), nil
}
