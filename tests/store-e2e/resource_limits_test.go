//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func assertSharedResourceLimits(t *testing.T, first, second kv.Store) {
	keyStores := []kv.Store{kv.Prefixed(first, kv.Key{"limits", "api-keys"}), kv.Prefixed(second, kv.Key{"limits", "api-keys"})}
	keyServers := []*apikey.Server{apikey.NewServer(keyStores[0]), apikey.NewServer(keyStores[1])}
	owner := giznet.PublicKey{1}.String()
	t.Cleanup(func() {
		_ = keyServers[0].CleanupPeer(context.Background(), owner)
		_ = keyStores[0].Delete(context.Background(), kv.Key{"retired", owner})
	})
	for range apikey.PeerAPIKeyLimit - 1 {
		if _, err := keyServers[0].Create(t.Context(), owner, "existing", false); err != nil {
			t.Fatal(err)
		}
	}
	assertOneRedisWinner(t, runRedisRace(t, 2, func(i int) (bool, error) {
		_, err := keyServers[i].Create(t.Context(), owner, "concurrent", false)
		if errors.Is(err, apikey.ErrPeerAPIKeyLimit) {
			return false, nil
		}
		return err == nil, err
	}))
	page, err := keyServers[1].ListOwner(t.Context(), owner, "", 100)
	if err != nil || len(page.Items) != apikey.PeerAPIKeyLimit {
		t.Fatalf("shared API key count=%d err=%v", len(page.Items), err)
	}

	peerStores := []kv.Store{kv.Prefixed(first, kv.Key{"limits", "peers"}), kv.Prefixed(second, kv.Key{"limits", "peers"})}
	peerServers := []*peer.Server{{Store: peerStores[0]}, {Store: peerStores[1]}}
	sn := "shared-sn"
	imeis := []apitypes.PeerIMEI{{Tac: "12345678", Serial: "shared"}}
	cleanup := []kv.Key{{"by-sn", sn}, {"by-imei", imeis[0].Tac, imeis[0].Serial}}
	for i := 1; i <= peer.IdentifierIndexPeerLimit+1; i++ {
		cleanup = append(cleanup, kv.Key{"by-pubkey", giznet.PublicKey{byte(i)}.String()})
	}
	t.Cleanup(func() { _ = peerStores[0].BatchDelete(context.Background(), cleanup) })
	registration := func(i int) apitypes.Peer {
		return apitypes.Peer{PublicKey: giznet.PublicKey{byte(i)}.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive,
			Device: apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn, Imeis: &imeis}}}
	}
	for i := 1; i < peer.IdentifierIndexPeerLimit; i++ {
		if _, err := peerServers[0].SavePeer(t.Context(), registration(i)); err != nil {
			t.Fatal(err)
		}
	}
	assertOneRedisWinner(t, runRedisRace(t, 2, func(i int) (bool, error) {
		_, err := peerServers[i].SavePeer(t.Context(), registration(peer.IdentifierIndexPeerLimit+i))
		if errors.Is(err, peer.ErrIdentifierIndexPeerLimit) {
			return false, nil
		}
		return err == nil, err
	}))
	for _, index := range cleanup[:2] {
		members, err := peerStores[1].ListMembers(t.Context(), index)
		if err != nil || len(members) != peer.IdentifierIndexPeerLimit {
			t.Fatalf("shared identifier count=%d err=%v", len(members), err)
		}
	}
}
