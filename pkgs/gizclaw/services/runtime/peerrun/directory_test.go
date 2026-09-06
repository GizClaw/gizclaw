package peerrun

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestDirectoryPagesByRegistrationTime(t *testing.T) {
	s := newTestServer(t)
	a, b, c := giznet.PublicKey{1}, giznet.PublicKey{2}, giznet.PublicKey{3}
	base := time.Unix(100, 0)
	for i, key := range []giznet.PublicKey{c, a, b} {
		if err := s.RememberPeer(t.Context(), key, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.PutStatus(t.Context(), giznet.PublicKey{4}, apitypes.PeerStatus{}); err != nil {
		t.Fatal(err)
	}
	cursor := ""
	for i, want := range []string{c.String(), a.String(), b.String()} {
		keys, more, err := s.ListPeerPublicKeys(t.Context(), cursor, 1)
		if err != nil || len(keys) != 1 || keys[0] != want || more != (i < 2) {
			t.Fatalf("page %d: %v %v %v", i, keys, more, err)
		}
		cursor = keys[0]
	}
	keys, more, err := s.ListPeerPublicKeys(t.Context(), "unknown", 50)
	if err != nil || len(keys) != 0 || more {
		t.Fatalf("unknown cursor: %v %v %v", keys, more, err)
	}
	if err := s.RememberPeer(t.Context(), a, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	keys, _, err = s.ListPeerPublicKeys(t.Context(), "", 200)
	if err != nil || len(keys) != 3 {
		t.Fatalf("duplicate/runtime-only directory entries: %v %v", keys, err)
	}
}
