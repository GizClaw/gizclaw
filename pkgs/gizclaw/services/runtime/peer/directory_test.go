package peer

import (
	"context"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestIMEIDuplicateUpdateAndLegacyLookup(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	first, second := giznet.PublicKey{1}, giznet.PublicKey{2}
	info := apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Imeis: &[]apitypes.PeerIMEI{{Tac: "t:ac", Serial: "s%erial"}}}}
	saveTestPeer(t, server, first, info)
	saveTestPeer(t, server, second, info)
	check := func(want ...string) {
		t.Helper()
		got, err := server.ListPublicKeysByIMEI(ctx, "t:ac", "s%erial")
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	check(first.String(), second.String())
	// Legacy one-to-one index must recover all collisions from current records.
	if err := store.BatchDelete(ctx, []kv.Key{imeiKey("t:ac", "s%erial", first.String()), imeiKey("t:ac", "s%erial", second.String())}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, imeiPrefix("t:ac", "s%erial"), []byte(second.String())); err != nil {
		t.Fatal(err)
	}
	check(first.String(), second.String())
	// Updating one peer must leave its sibling discoverable.
	record, err := server.LoadPeer(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	record.Device.Identifiers = &apitypes.DeviceIdentifiers{}
	if _, err := server.SavePeer(ctx, record); err != nil {
		t.Fatal(err)
	}
	check(second.String())
	// A stale index never returns an unrelated peer.
	if err := store.Set(ctx, imeiKey("t:ac", "s%erial", first.String()), []byte{1}); err != nil {
		t.Fatal(err)
	}
	check(second.String())
	if err := store.Set(ctx, peerKey(second.String()), encodedPeerTombstone); err != nil {
		t.Fatal(err)
	}
	check()
}
