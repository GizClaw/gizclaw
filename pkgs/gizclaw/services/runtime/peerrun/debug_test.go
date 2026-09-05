package peerrun

import (
	"testing"

	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestDebugModePersistsInRuntimeStore(t *testing.T) {
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
	key := giznet.PublicKey{1}
	if mode, err := s.GetDebugMode(t.Context(), key); err != nil || mode != "off" {
		t.Fatalf("default=%q %v", mode, err)
	}
	for _, mode := range []string{"readonly", "fullcontrol", "off"} {
		if err := s.SetDebugMode(t.Context(), key, mode); err != nil {
			t.Fatal(err)
		}
		reopened := &Server{Store: store}
		if got, err := reopened.GetDebugMode(t.Context(), key); err != nil || got != mode {
			t.Fatalf("stored=%q %v", got, err)
		}
	}
	if err := s.SetDebugMode(t.Context(), key, "invalid"); !errors.Is(err, ErrInvalidDebugMode) {
		t.Fatal(err)
	}
	if err := store.Set(t.Context(), kv.Key{"by-peer", key.String(), "debug-mode"}, []byte("invalid")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDebugMode(t.Context(), key); !errors.Is(err, ErrInvalidDebugMode) {
		t.Fatal("corrupt mode accepted")
	}
}
