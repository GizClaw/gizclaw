package peer

import (
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestIndexDedupeHelpers(t *testing.T) {
	imeis := dedupeIMEIs([]apitypes.PeerIMEI{
		{Tac: "2", Serial: "b"},
		{Tac: "1", Serial: "a"},
		{Tac: "1", Serial: "a"},
		{Tac: "", Serial: "skip"},
	})
	if len(imeis) != 2 || imeis[0].Tac != "1" || imeis[1].Tac != "2" {
		t.Fatalf("dedupeIMEIs = %+v", imeis)
	}

	labels := dedupeLabels([]apitypes.PeerLabel{
		{Key: "b", Value: "2"},
		{Key: "a", Value: "1"},
		{Key: "a", Value: "1"},
		{Key: "", Value: "skip"},
	})
	if len(labels) != 2 || labels[0].Key != "a" || labels[1].Key != "b" {
		t.Fatalf("dedupeLabels = %+v", labels)
	}
}

func TestSharedPeerIndexesRejectStaleServerWrites(t *testing.T) {
	store := mustBadgerInMemory(t, nil)
	first, second := &Server{Store: store}, &Server{Store: store}
	key := giznet.PublicKey{37}
	makeRecord := func(sn string) apitypes.Peer {
		return apitypes.Peer{PublicKey: key.String(), Device: apitypes.DeviceInfo{
			Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn},
		}}
	}
	initial := makeRecord("initial")
	if err := first.writePeerLocked(t.Context(), initial, nil); err != nil {
		t.Fatal(err)
	}
	winner := makeRecord("winner")
	if err := first.writePeerLocked(t.Context(), winner, &initial); err != nil {
		t.Fatal(err)
	}
	if err := second.writePeerLocked(t.Context(), makeRecord("stale"), &initial); !errors.Is(err, ErrPeerConcurrentUpdate) {
		t.Fatalf("stale update: %v", err)
	}
	if err := second.writePeerLocked(t.Context(), makeRecord("duplicate"), nil); !errors.Is(err, ErrPeerAlreadyExists) {
		t.Fatalf("duplicate create: %v", err)
	}
	marker, err := pendingdeletion.New(pendingdeletion.KindPeer, key.String(), &winner.PublicKey, pendingdeletion.ReasonPeerDelete, struct{}{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(t.Context(), store, marker); err != nil {
		t.Fatal(err)
	}
	if err := second.writePeerLocked(t.Context(), makeRecord("deleting"), &winner); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("update after deletion marker: %v", err)
	}
	for _, sn := range []string{"initial", "winner", "stale", "duplicate", "deleting"} {
		found, err := store.HasMember(t.Context(), kv.Key{"by-sn", sn}, key.String())
		if err != nil || found != (sn == "winner") {
			t.Fatalf("SN %s membership=%v error=%v", sn, found, err)
		}
	}
	stored, err := first.get(t.Context(), key)
	if err != nil || stored.Device.Identifiers == nil || stored.Device.Identifiers.Sn == nil || *stored.Device.Identifiers.Sn != "winner" {
		t.Fatalf("winning record changed: %+v %v", stored, err)
	}
}

func TestIndexEntriesAndKeys(t *testing.T) {
	sn := "sn-index"
	publicKey := giznet.PublicKey{1}
	peer := apitypes.Peer{
		PublicKey: publicKey.String(),
		Role:      apitypes.PeerRoleServer,
		Status:    apitypes.PeerRegistrationStatusActive,
		CreatedAt: time.Unix(1, 0),
		UpdatedAt: time.Unix(2, 0),
		Device: apitypes.DeviceInfo{
			Identifiers: &apitypes.DeviceIdentifiers{
				Sn:     &sn,
				Imeis:  &[]apitypes.PeerIMEI{{Tac: "123", Serial: "456"}},
				Labels: &[]apitypes.PeerLabel{{Key: "site", Value: "lab"}},
			},
		},
	}

	entries := indexEntries(peer)
	keys := indexKeys(peer)
	if len(entries) != 3 {
		t.Fatalf("entries len = %d", len(entries))
	}
	if len(keys) != 3 {
		t.Fatalf("keys len = %d", len(keys))
	}
	sets := identifierSets(peer)
	if len(sets) != 2 || sets[0].Key.String() != "by-sn:sn-index" || len(sets[0].Members) != 1 || sets[0].Members[0] != peer.PublicKey {
		t.Fatalf("identifier sets = %+v", sets)
	}
}
