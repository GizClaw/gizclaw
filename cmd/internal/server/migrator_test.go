package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestNewMigratorRunsACLMigration(t *testing.T) {
	migrator, err := NewMigrator(validLayeredConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	defer migrator.Close()

	for range 2 {
		if err := migrator.Migrate(context.Background()); err != nil {
			t.Fatalf("Migrate() error = %v", err)
		}
	}
	if _, err := migrator.ACL.DB.ExecContext(context.Background(), `INSERT INTO acl_views (name, created_at, updated_at) VALUES ('default', 'now', 'now')`); err != nil {
		t.Fatalf("insert acl view after migration: %v", err)
	}
}

func TestCmdMigratorCloseHandlesNilState(t *testing.T) {
	var nilMigrator *CmdMigrator
	if err := nilMigrator.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
	if err := (&CmdMigrator{}).Close(); err != nil {
		t.Fatalf("empty Close() error = %v", err)
	}
}

func TestMigrateWorkspaceRunsACLMigrationFromWorkspaceConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, workspaceConfigFile), []byte(`
endpoint: "127.0.0.1:0"
storage:
  acl-db:
    kind: sql
    sqlite:
      dir: data/acl.sqlite
stores:
  acl:
    kind: sql
    storage: acl-db
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := MigrateWorkspace(context.Background(), root); err != nil {
		t.Fatalf("MigrateWorkspace() error = %v", err)
	}

	migrator, err := NewMigrator(Config{
		Endpoint: "127.0.0.1:0",
		Storage: map[string]storage.Config{
			"acl-db": {Kind: storage.KindSQL, SQLite: &storage.SQLConfig{Dir: filepath.Join(root, "data", "acl.sqlite")}},
		},
		Stores: map[string]stores.Config{
			"acl": {Kind: stores.KindSQL, Storage: "acl-db"},
		},
	})
	if err != nil {
		t.Fatalf("NewMigrator() after workspace migration error = %v", err)
	}
	defer migrator.Close()
	if _, err := migrator.ACL.DB.ExecContext(context.Background(), `INSERT INTO acl_views (name, created_at, updated_at) VALUES ('workspace', 'now', 'now')`); err != nil {
		t.Fatalf("insert acl view after workspace migration: %v", err)
	}
}

func TestMigrateWorkspaceMigratesLegacyPeerRole(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, workspaceConfigFile), []byte(`
endpoint: "127.0.0.1:0"
storage:
  peer-db:
    kind: keyvalue
    badger:
      dir: data/peer.badger
  acl-db:
    kind: sql
    sqlite:
      dir: data/acl.sqlite
stores:
  peers:
    kind: keyvalue
    storage: peer-db
  acl:
    kind: sql
    storage: acl-db
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := prepareWorkspaceMigrationConfig(root)
	if err != nil {
		t.Fatalf("prepareWorkspaceMigrationConfig() error = %v", err)
	}
	ss, err := newStoreRegistry(cfg)
	if err != nil {
		t.Fatalf("newStoreRegistry() error = %v", err)
	}
	peerStore, err := ss.KV(defaultPeersStore)
	if err != nil {
		t.Fatalf("KV(peers) error = %v", err)
	}
	publicKey := giznet.PublicKey{1}
	legacy := apitypes.Peer{
		PublicKey:     publicKey.String(),
		Role:          apitypes.PeerRole("gear"),
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
		CreatedAt:     time.Unix(1, 0).UTC(),
		UpdatedAt:     time.Unix(1, 0).UTC(),
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy peer: %v", err)
	}
	if err := peerStore.BatchSet(context.Background(), []kv.Entry{
		{Key: kv.Key{"by-pubkey", legacy.PublicKey}, Value: data},
		{Key: kv.Key{"by-role", "gear", legacy.PublicKey}, Value: []byte{1}},
	}); err != nil {
		t.Fatalf("seed legacy peer: %v", err)
	}
	if err := ss.Close(); err != nil {
		t.Fatalf("close seed stores: %v", err)
	}

	if err := MigrateWorkspace(context.Background(), root); err != nil {
		t.Fatalf("MigrateWorkspace() error = %v", err)
	}

	migrator, err := NewMigrator(cfg)
	if err != nil {
		t.Fatalf("NewMigrator() after migration error = %v", err)
	}
	defer migrator.Close()
	migrated, err := migrator.Peers.LoadPeer(context.Background(), publicKey)
	if err != nil {
		t.Fatalf("LoadPeer() error = %v", err)
	}
	if migrated.Role != apitypes.PeerRoleClient {
		t.Fatalf("Role = %q, want %q", migrated.Role, apitypes.PeerRoleClient)
	}
	if _, err := migrator.Peers.Store.Get(context.Background(), kv.Key{"by-role", "gear", legacy.PublicKey}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("legacy role index err = %v, want %v", err, kv.ErrNotFound)
	}
	if _, err := migrator.Peers.Store.Get(context.Background(), kv.Key{"by-role", "client", legacy.PublicKey}); err != nil {
		t.Fatalf("client role index missing: %v", err)
	}
}

func TestNewMigratorRequiresACLLogicalStore(t *testing.T) {
	if _, err := NewMigrator(Config{}); err == nil {
		t.Fatal("NewMigrator() error = nil")
	}
}
