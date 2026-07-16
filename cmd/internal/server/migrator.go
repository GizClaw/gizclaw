package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

type CmdMigrator struct {
	*gizclaw.Migrator
	stores *stores.Stores
}

func (m *CmdMigrator) Close() error {
	if m == nil {
		return nil
	}
	if m.stores == nil {
		return nil
	}
	err := m.stores.Close()
	m.stores = nil
	return err
}

func NewMigrator(cfg Config) (migrator *CmdMigrator, err error) {
	ss, err := newStoreRegistry(cfg)
	if err != nil {
		return nil, fmt.Errorf("server: stores: %w", err)
	}
	openedStores := ss
	defer func() {
		if err != nil {
			err = errors.Join(err, openedStores.Close())
		}
	}()

	return newMigratorWithStores(cfg, ss, true)
}

func newMigratorWithStores(cfg Config, ss *stores.Stores, owns bool) (*CmdMigrator, error) {
	aclDB, err := ss.SQL(defaultACLStore)
	if err != nil {
		return nil, fmt.Errorf("server: acl store: %w", err)
	}
	var peers *peer.Server
	if storeExists(cfg, defaultPeersStore) {
		peerKV, err := ss.KV(defaultPeersStore)
		if err != nil {
			return nil, fmt.Errorf("server: peers store: %w", err)
		}
		peers = &peer.Server{Store: peerKV}
	}
	var credentials *credential.Server
	if storeExists(cfg, defaultCredentialsStore) {
		credentialKV, err := ss.KV(defaultCredentialsStore)
		if err != nil {
			return nil, fmt.Errorf("server: credentials store: %w", err)
		}
		credentials = &credential.Server{Store: credentialKV}
	}
	return &CmdMigrator{
		Migrator: &gizclaw.Migrator{
			ACL:         &acl.Server{DB: aclDB},
			Credentials: credentials,
			Peers:       peers,
		},
		stores: func() *stores.Stores {
			if owns {
				return ss
			}
			return nil
		}(),
	}, nil
}

func MigrateWorkspace(ctx context.Context, workspace string) error {
	cfg, err := prepareWorkspaceMigrationConfig(workspace)
	if err != nil {
		return err
	}
	migrator, err := NewMigrator(cfg)
	if err != nil {
		return err
	}
	defer migrator.Close()
	return migrator.Migrate(ctx)
}
