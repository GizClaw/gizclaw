package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
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
	return &CmdMigrator{
		Migrator: &gizclaw.Migrator{
			ACL:   &acl.Server{DB: aclDB},
			Peers: peers,
		},
		stores: ss,
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
