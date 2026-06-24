package peer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestMigrationUpdatesLegacyPeerRole(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	publicKey := giznet.PublicKey{1}
	now := time.Unix(1, 0).UTC()
	legacy := apitypes.Peer{
		PublicKey:     publicKey.String(),
		Role:          legacyClientPeerRole,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy peer: %v", err)
	}
	if err := store.BatchSet(ctx, []kv.Entry{
		{Key: peerKey(legacy.PublicKey), Value: data},
		{Key: roleKey(legacyClientPeerRole, legacy.PublicKey), Value: []byte{1}},
	}); err != nil {
		t.Fatalf("seed legacy peer: %v", err)
	}

	for range 2 {
		if err := server.Migration(ctx); err != nil {
			t.Fatalf("Migration() error = %v", err)
		}
	}

	migrated, err := server.LoadPeer(ctx, publicKey)
	if err != nil {
		t.Fatalf("LoadPeer() error = %v", err)
	}
	if migrated.Role != apitypes.PeerRoleClient {
		t.Fatalf("Role = %q, want %q", migrated.Role, apitypes.PeerRoleClient)
	}
	if _, err := store.Get(ctx, roleKey(legacyClientPeerRole, legacy.PublicKey)); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("legacy role index err = %v, want %v", err, kv.ErrNotFound)
	}
	if _, err := store.Get(ctx, roleKey(apitypes.PeerRoleClient, legacy.PublicKey)); err != nil {
		t.Fatalf("client role index missing: %v", err)
	}
}
