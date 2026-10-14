//go:build store_e2e

package store_e2e_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	redis "github.com/redis/go-redis/v9"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
)

func TestTiKVRedisMigration(t *testing.T) {
	address := requiredEnvironment(t, "GIZCLAW_TEST_TIKV_PD")
	redisURL := requiredEnvironment(t, "GIZCLAW_TEST_MIGRATION_REDIS")
	configureTiKVLearner(t, address)
	physical, err := storage.New(map[string]storage.Config{"source": storage.RedisConfig{URL: redisURL}, "target": storage.TiKVConfig{PDAddresses: []string{address}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	primary, err := physical.Redis("source")
	if err != nil {
		t.Fatal(err)
	}
	unavailableReader := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DB: primary.Options().DB, DialTimeout: 20 * time.Millisecond})
	t.Cleanup(func() { _ = unavailableReader.Close() })
	source, err := kv.NewRedisReadWriteWithClients(unavailableReader, primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := physical.TiKV("target")
	if err != nil {
		t.Fatal(err)
	}
	target, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{StaleReadDelay: 30 * time.Second, Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	strong := kv.WithStrongReads(target)
	newPrefix := func() kv.Key { return kv.Key{"migration-" + rand.Text(), "literal*?[scope]"} }
	t.Run("native-cluster-source-is-rejected", func(t *testing.T) {
		url := requiredEnvironment(t, "GIZCLAW_TEST_MIGRATION_CLUSTER_REDIS")
		options, err := redis.ParseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		client := redis.NewClient(options)
		defer client.Close()
		clusterSource, err := kv.NewRedisWithClient(client, nil)
		if err != nil {
			t.Fatal(err)
		}
		prefix := newPrefix()
		result, err := kv.MigrateRedisToTiKV(t.Context(), clusterSource, target, kv.RedisMigrationOptions{Prefixes: []kv.Key{prefix}, DryRun: true})
		if !errors.Is(err, kv.ErrMigrationUnsupported) || result.Imported != 0 {
			t.Fatalf("cluster source could yield incomplete success: %+v %v", result, err)
		}
	})

	t.Run("binary-types-ttl-and-repeat", func(t *testing.T) {
		ctx := t.Context()
		prefix := newPrefix()
		key := func(name string) kv.Key { return append(slices.Clone(prefix), name) }
		binaryValue := []byte{0, 255, ':', 0, 'x'}
		if err := source.Set(ctx, prefix, []byte{}); err != nil {
			t.Fatal(err)
		}
		if err := source.Set(ctx, key("binary"), binaryValue); err != nil {
			t.Fatal(err)
		}
		if err := source.Set(ctx, key("empty"), []byte{}); err != nil {
			t.Fatal(err)
		}
		if err := source.AddMembers(ctx, key("set"), "", "a:\x00b", "你好"); err != nil {
			t.Fatal(err)
		}
		var members []string
		for i := range 700 {
			members = append(members, fmt.Sprintf("member-%04d", i))
		}
		if _, err := source.ApplyMutation(ctx, kv.Mutation{AddOrderedMembers: []kv.SetMembers{{Key: key("ordered"), Members: members}}}); err != nil {
			t.Fatal(err)
		}
		outside := kv.Key{prefix[0] + "-other", "keep"}
		if err := source.Set(ctx, outside, []byte("outside")); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Minute).Truncate(time.Millisecond)
		if err := source.BatchSet(ctx, []kv.Entry{{Key: key("ttl"), Value: []byte("expires"), Deadline: deadline}}); err != nil {
			t.Fatal(err)
		}
		options := kv.RedisMigrationOptions{Prefixes: []kv.Key{prefix}, ScanCount: 31, Workers: 3, DryRun: true}
		result, err := kv.MigrateRedisToTiKV(ctx, source, target, options)
		if err != nil {
			t.Fatal(err)
		}
		if result.Imported != 0 || result.WouldImport != 6 {
			t.Fatalf("dry-run result: %+v", result)
		}
		if _, err := strong.Get(ctx, prefix); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("dry-run wrote target: %v", err)
		}
		options.DryRun = false
		result, err = kv.MigrateRedisToTiKV(ctx, source, target, options)
		if err != nil {
			t.Fatal(err)
		}
		if result.Imported != 6 || result.DestinationVerified != 6 {
			t.Fatalf("import result: %+v", result)
		}
		if value, err := strong.Get(ctx, key("binary")); err != nil || !bytes.Equal(value, binaryValue) {
			t.Fatalf("binary value: %x %v", value, err)
		}
		if value, err := strong.Get(ctx, key("empty")); err != nil || len(value) != 0 {
			t.Fatalf("empty value: %x %v", value, err)
		}
		got, err := strong.ListMembers(ctx, key("set"))
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		want := []string{"", "a:\x00b", "你好"}
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("set: %q", got)
		}
		ordered, err := strong.RangeOrderedMembers(ctx, key("ordered"), kv.OrderedRange{Limit: 1000})
		if err != nil || !slices.Equal(ordered, members) {
			t.Fatalf("ordered count=%d error=%v", len(ordered), err)
		}
		if _, err := strong.Get(ctx, outside); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("escaped selected namespace: %v", err)
		}
		result, err = kv.MigrateRedisToTiKV(ctx, source, target, options)
		if err != nil || result.Imported != 0 {
			t.Fatalf("repeat: %+v %v", result, err)
		}
		options.VerifyOnly = true
		if _, err := kv.MigrateRedisToTiKV(ctx, source, target, options); err != nil {
			t.Fatalf("strong verification used stale Learner state: %v", err)
		}
		if value, err := primary.Get(ctx, strings.Join(key("binary"), ":")).Bytes(); err != nil || !bytes.Equal(value, binaryValue) {
			t.Fatalf("source modified: %x %v", value, err)
		}
	})
	t.Run("conflicting-live-value", func(t *testing.T) {
		prefix := newPrefix()
		key := append(slices.Clone(prefix), "record")
		if err := source.Set(t.Context(), key, []byte("old-redis")); err != nil {
			t.Fatal(err)
		}
		if err := target.Set(t.Context(), key, []byte("new-tikv")); err != nil {
			t.Fatal(err)
		}
		if _, err := kv.MigrateRedisToTiKV(t.Context(), source, target, kv.RedisMigrationOptions{Prefixes: []kv.Key{prefix}}); !errors.Is(err, kv.ErrMigrationConflict) {
			t.Fatalf("conflict: %v", err)
		}
		value, err := strong.Get(t.Context(), key)
		if err != nil || string(value) != "new-tikv" {
			t.Fatalf("overwrote target: %q %v", value, err)
		}
	})
	t.Run("extra-target-key-denies-cutover", func(t *testing.T) {
		prefix := newPrefix()
		key := append(slices.Clone(prefix), "record")
		extra := append(slices.Clone(prefix), "orphan")
		if err := source.Set(t.Context(), key, []byte("same")); err != nil {
			t.Fatal(err)
		}
		if err := target.Set(t.Context(), key, []byte("same")); err != nil {
			t.Fatal(err)
		}
		if err := target.Set(t.Context(), extra, []byte("preserve")); err != nil {
			t.Fatal(err)
		}
		if _, err := kv.MigrateRedisToTiKV(t.Context(), source, target, kv.RedisMigrationOptions{Prefixes: []kv.Key{prefix}, VerifyOnly: true}); !errors.Is(err, kv.ErrMigrationConflict) {
			t.Fatalf("extra key: %v", err)
		}
		if value, err := strong.Get(t.Context(), extra); err != nil || string(value) != "preserve" {
			t.Fatalf("deleted extra target: %q %v", value, err)
		}
	})
	t.Run("reject-unsupported-before-claim", func(t *testing.T) {
		for _, kind := range []string{"hash", "list", "scored-zset", "expiring-set", "oversized-value", "oversized-member"} {
			t.Run(kind, func(t *testing.T) {
				prefix := newPrefix()
				key := append(slices.Clone(prefix), "unsupported")
				encoded := strings.Join(key, ":")
				ctx := t.Context()
				var err error
				switch kind {
				case "hash":
					err = primary.HSet(ctx, encoded, "field", "value").Err()
				case "list":
					err = primary.RPush(ctx, encoded, "value").Err()
				case "scored-zset":
					err = primary.ZAdd(ctx, encoded, redis.Z{Score: 1, Member: "value"}).Err()
				case "expiring-set":
					err = primary.SAdd(ctx, encoded, "value").Err()
					if err == nil {
						err = primary.Expire(ctx, encoded, time.Minute).Err()
					}
				case "oversized-value":
					err = primary.Set(ctx, encoded, strings.Repeat("v", 65), 0).Err()
				case "oversized-member":
					err = primary.SAdd(ctx, encoded, strings.Repeat("v", 5000)).Err()
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := kv.MigrateRedisToTiKV(ctx, source, target, kv.RedisMigrationOptions{Prefixes: []kv.Key{prefix}, MaxValueBytes: 64}); !errors.Is(err, kv.ErrMigrationUnsupported) {
					t.Fatalf("unsupported %s: %v", kind, err)
				}
				if _, err := strong.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("published unsupported data or claim: %v", err)
				}
			})
		}
	})

	t.Run("cli-primary-selection-and-cutover", func(t *testing.T) {
		binary := requiredEnvironment(t, "GIZCLAW_TEST_MIGRATION_CLI")
		prefix := newPrefix()
		key := append(slices.Clone(prefix), "record")
		excluded := kv.Key{"excluded-" + rand.Text(), "record"}
		if err := source.Set(t.Context(), key, []byte("cli-data")); err != nil {
			t.Fatal(err)
		}
		if err := source.Set(t.Context(), excluded, []byte("excluded")); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		config := fmt.Sprintf("storage:\n  old:\n    kind: redis.readwrite\n    read_url: redis://127.0.0.1:1/0\n    write_url: %s\n  new:\n    kind: tikv\n    pd_addresses: [%s]\n  unused:\n    kind: postgresql\n    dsn: postgres://127.0.0.1:1/unused\nstores:\n  selected:\n    kind: keyvalue\n    storage: old\n    prefix: %s\n  excluded:\n    kind: keyvalue\n    storage: old\n    prefix: %s\n", redisURL, address, strings.Join(prefix, "/"), excluded[0])
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		run := func(flags ...string) kv.RedisMigrationResult {
			t.Helper()
			args := []string{"kv-migrate", dir, "--source", "old", "--target", "new", "--stores", "selected"}
			args = append(args, flags...)
			command := exec.CommandContext(t.Context(), binary, args...)
			var stderr bytes.Buffer
			command.Stderr = &stderr
			output, err := command.Output()
			if err != nil {
				t.Fatalf("migration CLI: %v %s", err, stderr.String())
			}
			var result kv.RedisMigrationResult
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("CLI result: %v %s", err, output)
			}
			return result
		}
		dry := run("--dry-run")
		if dry.Imported != 0 || dry.WouldImport != 1 {
			t.Fatalf("CLI dry-run: %+v", dry)
		}
		if result := run(); result.Imported != 1 {
			t.Fatalf("CLI import: %+v", result)
		}
		config = strings.Replace(config, "storage: old\n    prefix: "+strings.Join(prefix, "/"), "storage: new\n    prefix: "+strings.Join(prefix, "/"), 1)
		if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		if result := run("--verify-only"); result.DestinationVerified != 1 {
			t.Fatalf("post-cutover verification: %+v", result)
		}
		if _, err := strong.Get(t.Context(), excluded); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("CLI migrated excluded store: %v", err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		failed := exec.CommandContext(t.Context(), binary, "kv-migrate", dir, "--source", "old", "--target", "new", "--dry-run")
		var diagnostics bytes.Buffer
		failed.Stderr = &diagnostics
		output, err := failed.Output()
		if err == nil {
			t.Fatal("CLI accepted missing configuration")
		}
		var partial kv.RedisMigrationResult
		if err := json.Unmarshal(output, &partial); err != nil || !partial.DryRun || partial.Imported != 0 {
			t.Fatalf("CLI error polluted JSON report: %s %v", output, err)
		}
	})

	t.Run("resume-interrupted-large-collection", func(t *testing.T) {
		prefix := newPrefix()
		key := append(slices.Clone(prefix), "members")
		var members []string
		for i := range 700 {
			members = append(members, fmt.Sprintf("resume-%04d", i))
		}
		if err := source.AddMembers(t.Context(), key, members...); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		original := client.GetTiKVClient()
		barrier := &migrationCancelClient{Client: original, cancel: cancel}
		client.SetTiKVClient(barrier)
		_, err := kv.MigrateRedisToTiKV(ctx, source, target, kv.RedisMigrationOptions{Prefixes: []kv.Key{prefix}, ScanCount: 31, Workers: 1})
		client.SetTiKVClient(original)
		if !errors.Is(err, context.Canceled) || barrier.prewrites.Load() < 3 {
			t.Fatalf("fault did not interrupt member import: %v count=%d", err, barrier.prewrites.Load())
		}
		if _, err := strong.ListMembers(t.Context(), key); !errors.Is(err, kv.ErrMigrationInProgress) {
			t.Fatalf("partial collection was exposed: %v", err)
		}
		if err := target.Set(t.Context(), key, []byte("overwrite")); !errors.Is(err, kv.ErrMigrationInProgress) {
			t.Fatalf("normal write bypassed migration guard: %v", err)
		}
		if err := target.Delete(t.Context(), key); !errors.Is(err, kv.ErrMigrationInProgress) {
			t.Fatalf("normal delete bypassed migration guard: %v", err)
		}
		if _, err := kv.MigrateRedisToTiKV(t.Context(), source, target, kv.RedisMigrationOptions{Prefixes: []kv.Key{prefix}, ScanCount: 31, Workers: 1}); err != nil {
			t.Fatalf("resume: %v", err)
		}
		got, err := strong.ListMembers(t.Context(), key)
		slices.Sort(got)
		if err != nil || !slices.Equal(got, members) {
			t.Fatalf("resume lost or duplicated members: %d %v", len(got), err)
		}
		if err := target.AddMembers(t.Context(), key, "after-resume"); err != nil {
			t.Fatalf("completed collection not writable: %v", err)
		}
	})
}

type migrationCancelClient struct {
	tikv.Client
	prewrites atomic.Int32
	cancel    context.CancelFunc
}

func (c *migrationCancelClient) SendRequest(ctx context.Context, address string, request *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	// The first transaction claims the key; the second commits one member page.
	// Cancel before the third prewrite to test durable, partial-import recovery.
	if request.Type == tikvrpc.CmdPrewrite && c.prewrites.Add(1) == 3 {
		c.cancel()
		return nil, context.Canceled
	}
	return c.Client.SendRequest(ctx, address, request, timeout)
}
