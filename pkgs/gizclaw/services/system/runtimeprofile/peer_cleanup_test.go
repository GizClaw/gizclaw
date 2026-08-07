package runtimeprofile

import (
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestDeleteOwnerProfileBindingPreservesGlobalAndForeignState(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	retiring, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	entries := []kv.Entry{
		{Key: ownerProfileKey(retiring.Public.String()), Value: []byte("profile-a")},
		{Key: ownerProfileKey(foreign.Public.String()), Value: []byte("profile-a")},
		{Key: profileKey("profile-a"), Value: []byte(`{"id":"profile-a"}`)},
		{Key: tokenKey("token-a"), Value: []byte(`{"id":"token-a"}`)},
		{Key: tokenHashKey("hash-a"), Value: []byte("token-a")},
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if err := server.DeleteOwnerProfileBinding(ctx, retiring.Public.String()); err != nil {
		t.Fatal(err)
	}
	if err := server.DeleteOwnerProfileBinding(ctx, retiring.Public.String()); err != nil {
		t.Fatalf("replay delete: %v", err)
	}
	if _, err := store.Get(ctx, ownerProfileKey(retiring.Public.String())); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retiring binding error = %v", err)
	}
	for _, entry := range entries[1:] {
		if _, err := store.Get(ctx, entry.Key); err != nil {
			t.Fatalf("preserved key %v: %v", entry.Key, err)
		}
	}
}

func TestDeleteOwnerProfileBindingRejectsNonCanonicalKeyWithoutMutation(t *testing.T) {
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	if err := server.DeleteOwnerProfileBinding(t.Context(), " peer "); err == nil {
		t.Fatal("DeleteOwnerProfileBinding() error = nil")
	}
}
