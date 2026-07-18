package peer

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
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
	if len(entries) != 5 {
		t.Fatalf("entries len = %d", len(entries))
	}
	if len(keys) != 5 {
		t.Fatalf("keys len = %d", len(keys))
	}
	if snKey(sn).String() != "by-sn:sn-index" {
		t.Fatalf("snKey = %s", snKey(sn).String())
	}
}
