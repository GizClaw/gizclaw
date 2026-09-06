package peerrun

import (
	"testing"

	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestDebugModePersistsInRuntimeStore(t *testing.T) {
	s := newTestServer(t)
	key := giznet.PublicKey{1}
	if mode, err := s.GetDebugMode(t.Context(), key); err != nil || mode != "off" {
		t.Fatalf("default=%q %v", mode, err)
	}
	for _, mode := range []string{"readonly", "fullcontrol", "off"} {
		if err := s.SetDebugMode(t.Context(), key, mode); err != nil {
			t.Fatal(err)
		}
		reopened := &Server{DB: s.DB}
		if got, err := reopened.GetDebugMode(t.Context(), key); err != nil || got != mode {
			t.Fatalf("stored=%q %v", got, err)
		}
	}
	if err := s.SetDebugMode(t.Context(), key, "invalid"); !errors.Is(err, ErrInvalidDebugMode) {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(t.Context(), `UPDATE peer_runs SET debug_mode='invalid' WHERE public_key=?`, key.String()); err == nil {
		t.Fatal("invalid mode accepted by schema")
	}
}
