package server

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestNewMigratorSkipsUnrelatedMemoryStores(t *testing.T) {
	cfg := validLayeredConfig(t.TempDir())
	cfg.Stores["agent-memory"] = stores.Config{
		Kind: stores.KindMemoryStore,
		Flowcraft: &memorystore.FlowcraftConfig{
			RuntimeID: "app", UserID: "user", ExtractionModel: "requires-loader",
		},
	}
	migrator, err := NewMigrator(cfg)
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
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
