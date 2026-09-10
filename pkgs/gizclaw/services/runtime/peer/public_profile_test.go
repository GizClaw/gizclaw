package peer

import (
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestGetPublicProfileProjectsOnlyNameAndEmoji(t *testing.T) {
	store := mustBadgerInMemory(t, nil)
	server := &Server{Store: store}
	named, blank, deleting, unknown := giznet.PublicKey{1}, giznet.PublicKey{2}, giznet.PublicKey{3}, giznet.PublicKey{4}
	name, emoji, sn, empty := "Rocket", "🚀", "sn-secret", " "
	saveTestPeer(t, server, named, apitypes.DeviceInfo{
		Name: &name, Emoji: &emoji,
		Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn},
	})
	saveTestPeer(t, server, blank, apitypes.DeviceInfo{Name: &empty})
	saveTestPeer(t, server, deleting, apitypes.DeviceInfo{Name: &name})
	marker, err := pendingdeletion.New(pendingdeletion.KindPeer, deleting.String(), new(deleting.String()), pendingdeletion.ReasonPeerDelete, struct{}{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(t.Context(), store, marker); err != nil {
		t.Fatal(err)
	}

	profile, err := server.GetPublicProfile(t.Context(), named)
	if err != nil || profile.DisplayName == nil || *profile.DisplayName != name || profile.Emoji == nil || *profile.Emoji != emoji {
		t.Fatalf("named profile = %+v, %v", profile, err)
	}
	if profile, err := server.GetPublicProfile(t.Context(), blank); err != nil || profile.DisplayName != nil || profile.Emoji != nil {
		t.Fatalf("blank profile = %+v, %v", profile, err)
	}
	for _, key := range []giznet.PublicKey{deleting, unknown} {
		if _, err := server.GetPublicProfile(t.Context(), key); !errors.Is(err, ErrPeerNotFound) {
			t.Fatalf("GetPublicProfile(%s) error = %v, want %v", key, err, ErrPeerNotFound)
		}
	}
}
