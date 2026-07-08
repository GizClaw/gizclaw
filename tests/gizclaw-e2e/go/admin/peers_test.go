//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
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
	if get.JSON200 == nil || get.JSON200.PublicKey != env.peerKey {
		t.Fatalf("get peer = %#v", get.JSON200)
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
