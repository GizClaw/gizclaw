package server

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

// MigrateRedisKV reads connector and namespace definitions without starting a
// Server or opening unrelated storage. It uses only the Redis writer for a
// read/write connector and always verifies TiKV through fresh leader snapshots.
// Stores may already reference the target connector for post-cutover verification.
func MigrateRedisKV(ctx context.Context, directory, sourceName, targetName string, names []string, options kv.RedisMigrationOptions) (kv.RedisMigrationResult, error) {
	result := kv.RedisMigrationResult{DryRun: options.DryRun, VerifyOnly: options.VerifyOnly}
	root, err := filepath.Abs(directory)
	if err != nil {
		return result, err
	}
	file, err := LoadConfig(filepath.Join(root, workspaceConfigFile))
	if err != nil {
		return result, err
	}
	configs, prefixes, err := redisMigrationConfig(file, sourceName, targetName, names)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	physical, err := storage.New(resolveWorkspaceStorageConfigs(root, configs))
	if err != nil {
		return result, err
	}
	defer physical.Close()
	client, err := physical.Redis(sourceName)
	if err != nil {
		return result, err
	}
	source, err := kv.NewRedisWithClient(client, nil)
	if err != nil {
		return result, err
	}
	targetClient, _, err := physical.TiKV(targetName)
	if err != nil {
		return result, err
	}
	target, err := kv.NewTiKVWithClient(targetClient, kv.TiKVReadOptions{}, nil)
	if err != nil {
		return result, err
	}
	options.Prefixes = prefixes
	return kv.MigrateRedisToTiKV(ctx, source, target, options)
}

func redisMigrationConfig(file ConfigFile, sourceName, targetName string, names []string) (map[string]storage.Config, []kv.Key, error) {
	if sourceName == "" || targetName == "" || sourceName == targetName {
		return nil, nil, errors.New("kv migration: distinct source and target storage names are required")
	}
	source, ok := file.Storage[sourceName]
	if !ok || source.Kind != storage.KindRedis && source.Kind != storage.KindRedisReadWrite {
		return nil, nil, errors.New("kv migration: source must reference redis or redis.readwrite storage")
	}
	target, ok := file.Storage[targetName]
	if !ok || target.Kind != storage.KindTiKV {
		return nil, nil, errors.New("kv migration: target must reference tikv storage")
	}
	sourceConfig, err := source.runtimeConfig()
	if err != nil {
		return nil, nil, err
	}
	if split, ok := sourceConfig.(storage.RedisReadWriteConfig); ok {
		sourceConfig = split.Write
	}
	targetConfig, err := target.runtimeConfig()
	if err != nil {
		return nil, nil, err
	}
	selected := slices.Clone(names)
	if len(selected) == 0 {
		for name, logical := range file.Stores {
			if logical.Kind == stores.KindKeyValue && (logical.Storage == sourceName || logical.Storage == targetName) {
				selected = append(selected, name)
			}
		}
	}
	slices.Sort(selected)
	prefixes := make([]kv.Key, 0, len(selected))
	seen := make(map[string]bool)
	for _, name := range selected {
		logical, ok := file.Stores[name]
		if !ok || logical.Kind != stores.KindKeyValue || logical.Storage != sourceName && logical.Storage != targetName {
			return nil, nil, fmt.Errorf("kv migration: store %q must be keyvalue and reference the source or target", name)
		}
		prefix := logical.Prefix
		if prefix == "" || strings.TrimSpace(prefix) != prefix || path.Clean(prefix) != prefix || strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, ":\\") {
			return nil, nil, fmt.Errorf("kv migration: store %q requires a canonical non-empty KV prefix", name)
		}
		if seen[prefix] {
			continue
		}
		seen[prefix] = true
		for previous := range seen {
			if previous != prefix && (strings.HasPrefix(prefix, previous+"/") || strings.HasPrefix(previous, prefix+"/")) {
				return nil, nil, errors.New("kv migration: selected prefixes overlap")
			}
		}
		prefixes = append(prefixes, kv.Key(strings.Split(prefix, "/")))
	}
	if len(prefixes) == 0 {
		return nil, nil, errors.New("kv migration: no keyvalue Stores reference the source or target")
	}
	return map[string]storage.Config{sourceName: sourceConfig, targetName: targetConfig}, prefixes, nil
}
