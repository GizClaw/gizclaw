//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func TestServerFriendRPC(t *testing.T) {
	env := newSocialRPCHarness(t)

	friendAB := createRPCFriendByInviteToken(t, env, env.a, env.b, env.peer["peer-b"])
	friendAC := createRPCFriendByInviteToken(t, env, env.a, env.c, env.peer["peer-c"])
	if friendAB.WorkspaceName == nil || *friendAB.WorkspaceName == "" || friendAC.WorkspaceName == nil || *friendAC.WorkspaceName == "" {
		t.Fatalf("accepted friend workspaces are empty: ab=%#v ac=%#v", friendAB, friendAC)
	}

	limit := 1
	first, err := env.a.ListFriends(env.ctx, "friend.list.page1", rpcapi.FriendListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("friend.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("friend.list page1 = %#v", first)
	}
	second, err := env.a.ListFriends(env.ctx, "friend.list.page2", rpcapi.FriendListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("friend.list page2: %v", err)
	}
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("friend.list page2 = %#v", second)
	}
	if friendAC.Id == nil {
		t.Fatalf("friend ac id is nil: %#v", friendAC)
	}
	deleted, err := env.a.DeleteFriend(env.ctx, "friend.delete", rpcapi.FriendDeleteRequest{Id: *friendAC.Id})
	if err != nil {
		t.Fatalf("friend.delete: %v", err)
	}
	if deleted.Id == nil || *deleted.Id != *friendAC.Id {
		t.Fatalf("friend.delete id = %#v, want %q", deleted.Id, *friendAC.Id)
	}
}
