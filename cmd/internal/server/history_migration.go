package server

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

// MigrateWorkspaceHistory opens only the SQL connector selected by the final
// Workspace History configuration. It does not start a Server or initialize any
// old LogStore, and never generates identity keys or edits the config file.
func MigrateWorkspaceHistory(ctx context.Context, directory, workspaceTable, einoTable string, dryRun bool) (workspace.HistoryMigrationResult, error) {
	var result workspace.HistoryMigrationResult
	root, err := filepath.Abs(directory)
	if err != nil {
		return result, err
	}
	file, err := LoadConfig(filepath.Join(root, workspaceConfigFile))
	if err != nil {
		return result, err
	}
	if file.Services == nil || file.Services.Workspace == nil {
		return result, fmt.Errorf("history migration: services.workspace is required")
	}
	binding := file.Services.Workspace
	history, ok := file.Stores[binding.HistoryStore]
	if !ok || history.Kind != stores.KindSQL {
		return result, fmt.Errorf("history migration: workspace.history_store must reference a sql Store")
	}
	for name, logical := range file.Stores {
		if (logical.Kind == stores.KindLogMutable || logical.Kind == stores.KindLogImmutable) && file.Storage[logical.Storage].Kind == storage.KindPostgreSQL {
			return result, fmt.Errorf("history migration: stores.%s still uses the retired PostgreSQL LogStore backend", name)
		}
		if logical.Storage == history.Storage && (logical.Table == workspaceTable || logical.Table == einoTable) {
			return result, fmt.Errorf("history migration: stores.%s still references a source table; update the final configuration first", name)
		}
	}
	assets, ok := file.Stores[binding.HistoryAssetsStore]
	if !ok || assets.Kind != stores.KindObjectStore {
		return result, fmt.Errorf("history migration: workspace history assets Store is required")
	}
	assetConfig, err := assets.runtimeConfig()
	if err != nil || assetConfig.TTL <= 0 {
		return result, fmt.Errorf("history migration: history assets require positive retention")
	}
	connector, ok := file.Storage[history.Storage]
	if !ok {
		return result, fmt.Errorf("history migration: SQL storage reference does not exist")
	}
	physicalConfig, err := connector.runtimeConfig()
	if err != nil {
		return result, err
	}
	if connector.Kind != storage.KindPostgreSQL && connector.Kind != storage.KindSQLite {
		return result, fmt.Errorf("history migration: only PostgreSQL and SQLite are supported")
	}
	configs := resolveWorkspaceStorageConfigs(root, map[string]storage.Config{history.Storage: physicalConfig})
	physical, err := storage.New(configs)
	if err != nil {
		return result, err
	}
	defer physical.Close()
	db, err := physical.SQL(history.Storage)
	if err != nil {
		return result, err
	}
	return workspace.MigrateHistory(ctx, db, workspace.HistoryMigration{
		WorkspaceTable: workspaceTable, EinoTable: einoTable,
		Retention: assetConfig.TTL, DryRun: dryRun,
	})
}
