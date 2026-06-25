//go:build gizclaw_e2e

package admin_test

import (
	"sort"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func TestAdminAPIPeerFriendsListGetCreateDelete(t *testing.T) {
	env := newAdminAPIHarness(t)
	relationID := adminAPIRelationID(env.adminKey, env.peerKey)

	_, _ = env.api.DeletePeerFriendWithResponse(env.ctx, env.adminKey, relationID)
	_, _ = env.api.DeletePeerFriendWithResponse(env.ctx, env.adminKey, env.peerKey)
	created, err := env.api.CreatePeerFriendWithResponse(env.ctx, env.adminKey, adminservice.AdminFriendCreateRequest{
		PeerPublicKey: env.peerKey,
	})
	if err != nil {
		t.Fatalf("create peer friend: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Id == nil || *created.JSON200.Id != env.peerKey || created.JSON200.PeerPublicKey == nil || *created.JSON200.PeerPublicKey != env.peerKey {
		t.Fatalf("created friend = %#v", created.JSON200)
	}

	get, err := env.api.GetPeerFriendWithResponse(env.ctx, env.adminKey, relationID)
	if err != nil {
		t.Fatalf("get peer friend: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.WorkspaceName == nil || *get.JSON200.WorkspaceName == "" {
		t.Fatalf("get friend = %#v", get.JSON200)
	}

	ownerRows := collectAdminPagesInt(t, 1, func(cursor *string, limit int) ([]rpcapi.FriendObject, bool, *string) {
		resp, err := env.api.ListPeerFriendsWithResponse(env.ctx, env.adminKey, &adminservice.ListPeerFriendsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list owner friends: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list owner friends missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, ownerRows, env.peerKey, func(item rpcapi.FriendObject) string {
		if item.Id == nil {
			return ""
		}
		return *item.Id
	})

	peerRows, err := env.api.ListPeerFriendsWithResponse(env.ctx, env.peerKey, &adminservice.ListPeerFriendsParams{Limit: ptr(10)})
	if err != nil {
		t.Fatalf("list peer friends: %v", err)
	}
	requireStatusOK(t, peerRows, peerRows.Body)
	if peerRows.JSON200 == nil || len(peerRows.JSON200.Items) != 1 || peerRows.JSON200.Items[0].PeerPublicKey == nil || *peerRows.JSON200.Items[0].PeerPublicKey != env.adminKey {
		t.Fatalf("peer friend rows = %#v", peerRows.JSON200)
	}

	deleted, err := env.api.DeletePeerFriendWithResponse(env.ctx, env.adminKey, env.peerKey)
	if err != nil {
		t.Fatalf("delete peer friend: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
	missing, err := env.api.GetPeerFriendWithResponse(env.ctx, env.adminKey, relationID)
	if err != nil {
		t.Fatalf("get deleted peer friend: %v", err)
	}
	if missing.StatusCode() != 404 {
		t.Fatalf("get deleted peer friend status = %d body=%s", missing.StatusCode(), string(missing.Body))
	}
}

func adminAPIRelationID(a, b string) string {
	parts := []string{a, b}
	sort.Strings(parts)
	return parts[0] + ":" + parts[1]
}
