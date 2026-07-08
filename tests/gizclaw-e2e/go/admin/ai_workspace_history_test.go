//go:build gizclaw_e2e

package admin_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestAdminAPIWorkspaceHistoryListAndGetFromSocialConversation(t *testing.T) {
	env := newAdminAPIHarness(t)
	workspaceName, texts := createAdminSocialConversationHistory(t, env)
	order := adminhttp.Asc
	limit := 2

	history, err := env.api.ListWorkspaceHistoryWithResponse(env.ctx, workspaceName, &adminhttp.ListWorkspaceHistoryParams{
		Limit: &limit,
		Order: &order,
	})
	if err != nil {
		t.Fatalf("list workspace history: %v", err)
	}
	requireStatusOK(t, history, history.Body)
	if history.JSON200 == nil || len(history.JSON200.Items) != 2 || !history.JSON200.HasNext || history.JSON200.NextCursor == nil {
		t.Fatalf("workspace history list = %#v", history.JSON200)
	}
	first := history.JSON200.Items[0]
	if first.Id == "" || first.Text == "" || !first.ReplayAvailable || first.GearId == nil {
		t.Fatalf("first workspace history = %#v", first)
	}
	if first.Text != texts[0] {
		t.Fatalf("first workspace history text = %q", first.Text)
	}

	next, err := env.api.ListWorkspaceHistoryWithResponse(env.ctx, workspaceName, &adminhttp.ListWorkspaceHistoryParams{
		Limit:  &limit,
		Cursor: history.JSON200.NextCursor,
		Order:  &order,
	})
	if err != nil {
		t.Fatalf("list workspace history next page: %v", err)
	}
	requireStatusOK(t, next, next.Body)
	if next.JSON200 == nil || len(next.JSON200.Items) != 1 || next.JSON200.Items[0].Text != texts[2] {
		t.Fatalf("workspace history next page = %#v", next.JSON200)
	}

	get, err := env.api.GetWorkspaceHistoryWithResponse(env.ctx, workspaceName, first.Id)
	if err != nil {
		t.Fatalf("get workspace history: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != first.Id || get.JSON200.Text != first.Text {
		t.Fatalf("workspace history get = %#v, want %#v", get.JSON200, first)
	}
}

func TestAdminAPISocialWorkspaceHistoryStartsEmptyUntilConversation(t *testing.T) {
	env := newAdminAPIHarness(t)

	friend, err := env.api.GetPeerFriendWithResponse(env.ctx, e2eSocialAdminPublicKey, e2eSocialRelationID)
	if err != nil {
		t.Fatalf("get shared social friend: %v", err)
	}
	requireStatusOK(t, friend, friend.Body)
	if friend.JSON200 == nil || friend.JSON200.WorkspaceName == nil || *friend.JSON200.WorkspaceName == "" {
		t.Fatalf("shared social friend = %#v", friend.JSON200)
	}
	requireAdminSocialWorkspaceHistoryEmpty(t, env, "friend", *friend.JSON200.WorkspaceName)

	group, err := env.api.GetFriendGroupWithResponse(env.ctx, e2eSocialGroupID)
	if err != nil {
		t.Fatalf("get shared social friend group: %v", err)
	}
	requireStatusOK(t, group, group.Body)
	if group.JSON200 == nil || group.JSON200.WorkspaceName == nil || *group.JSON200.WorkspaceName == "" {
		t.Fatalf("shared social friend group = %#v", group.JSON200)
	}
	requireAdminSocialWorkspaceHistoryEmpty(t, env, "friend_group", *group.JSON200.WorkspaceName)
}

func requireAdminSocialWorkspaceHistoryEmpty(t *testing.T, env *adminAPIHarness, label, workspaceName string) {
	t.Helper()
	order := adminhttp.Asc
	limit := 10
	history, err := env.api.ListWorkspaceHistoryWithResponse(env.ctx, workspaceName, &adminhttp.ListWorkspaceHistoryParams{
		Limit: &limit,
		Order: &order,
	})
	if err != nil {
		t.Fatalf("%s list workspace history: %v", label, err)
	}
	requireStatusOK(t, history, history.Body)
	if history.JSON200 == nil || len(history.JSON200.Items) != 0 || history.JSON200.HasNext {
		t.Fatalf("%s workspace history = %#v, want empty setup history", label, history.JSON200)
	}
}

func createAdminSocialConversationHistory(t *testing.T, env *adminAPIHarness) (string, []string) {
	t.Helper()

	const (
		writerContext = "admin-history-writer"
		readerContext = "admin-history-reader"
	)
	env.h.CreateContext(writerContext).MustSucceed(t)
	env.h.RegisterContext(writerContext, "--sn", "admin-history-writer-sn").MustSucceed(t)
	env.h.CreateContext(readerContext).MustSucceed(t)
	env.h.RegisterContext(readerContext, "--sn", "admin-history-reader-sn").MustSucceed(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	reader := env.h.ConnectClientFromContext(readerContext)
	defer reader.Close()
	token, err := reader.CreateFriendInviteToken(ctx, "admin.history.friend.invite_token.create", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("create friend invite token: %v", err)
	}

	writer := env.h.ConnectClientFromContext(writerContext)
	defer writer.Close()
	friend, err := writer.AddFriend(ctx, "admin.history.friend.add", rpcapi.FriendAddRequest{InviteToken: token.InviteToken})
	if err != nil {
		t.Fatalf("add friend by invite token: %v", err)
	}
	workspaceName := adminStringValue(friend.WorkspaceName)
	if strings.TrimSpace(workspaceName) == "" {
		t.Fatalf("friend workspace name is empty: %#v", friend)
	}

	if _, err := writer.SetServerRunWorkspace(ctx, "admin.history.workspace.set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName}); err != nil {
		t.Fatalf("set run workspace %q: %v", workspaceName, err)
	}
	state, err := writer.ReloadServerRunWorkspace(ctx, "admin.history.workspace.reload")
	if err != nil {
		t.Fatalf("reload run workspace %q: %v", workspaceName, err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning {
		t.Fatalf("workspace runtime state = %#v, want running", state)
	}

	texts := []string{
		"admin history social round one",
		"admin history social round two",
		"admin history social round three",
	}
	for _, text := range texts {
		out := sendAdminChatText(t, ctx, writer, text)
		waitForAdminWorkspaceHistoryText(t, env, workspaceName, text)
		_ = out.Close()
	}
	return workspaceName, texts
}

func sendAdminChatText(t *testing.T, ctx context.Context, client *gizcli.Client, text string) genx.Stream {
	t.Helper()
	out, err := client.Transform(ctx, "admin.history.chatroom", adminChatTextStream(text))
	if err != nil {
		t.Fatalf("transform chat text %q: %v", text, err)
	}
	return out
}

func waitForAdminWorkspaceHistoryText(t *testing.T, env *adminAPIHarness, workspaceName, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	limit := 20
	for {
		history, err := env.api.ListWorkspaceHistoryWithResponse(env.ctx, workspaceName, &adminhttp.ListWorkspaceHistoryParams{Limit: &limit})
		if err == nil && history.JSON200 != nil {
			for _, item := range history.JSON200.Items {
				if item.Text == text {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("list workspace history while waiting for %q: %v", text, err)
			}
			t.Fatalf("workspace history text %q not found in %q", text, workspaceName)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func adminChatTextStream(text string) genx.Stream {
	return &adminSliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "admin-chat-text", Label: "transcript"}},
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "admin-chat-text", Label: "transcript", EndOfStream: true}},
	}}
}

type adminSliceStream struct {
	chunks []*genx.MessageChunk
}

func (s *adminSliceStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	next := s.chunks[0]
	s.chunks = s.chunks[1:]
	return next, nil
}

func (s *adminSliceStream) Close() error {
	s.chunks = nil
	return nil
}

func (s *adminSliceStream) CloseWithError(error) error {
	s.chunks = nil
	return nil
}

func adminStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
