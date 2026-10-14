package kv

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	redis "github.com/redis/go-redis/v9"
	"github.com/tikv/client-go/v2/tikv"
)

func migrationValidationClients(t *testing.T) (*Redis, *TiKV) {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DB: 2})
	t.Cleanup(func() { _ = client.Close() })
	return &Redis{client: client}, &TiKV{client: &tikv.KVStore{}}
}

func TestRedisMigrationScopeAndLimits(t *testing.T) {
	source, target := migrationValidationClients(t)
	cases := []struct {
		name    string
		options RedisMigrationOptions
	}{
		{"empty", RedisMigrationOptions{}},
		{"root", RedisMigrationOptions{Prefixes: []Key{{}}}},
		{"empty-segment", RedisMigrationOptions{Prefixes: []Key{{"peers", ""}}}},
		{"separator", RedisMigrationOptions{Prefixes: []Key{{"peers:other"}}}},
		{"overlap", RedisMigrationOptions{Prefixes: []Key{{"peers"}, {"peers", "routes"}}}},
		{"duplicate", RedisMigrationOptions{Prefixes: []Key{{"peers"}, {"peers"}}}},
		{"negative-scan", RedisMigrationOptions{Prefixes: []Key{{"peers"}}, ScanCount: -1}},
		{"large-scan", RedisMigrationOptions{Prefixes: []Key{{"peers"}}, ScanCount: 1025}},
		{"negative-workers", RedisMigrationOptions{Prefixes: []Key{{"peers"}}, Workers: -1}},
		{"large-workers", RedisMigrationOptions{Prefixes: []Key{{"peers"}}, Workers: 33}},
		{"negative-value", RedisMigrationOptions{Prefixes: []Key{{"peers"}}, MaxValueBytes: -1}},
		{"large-value", RedisMigrationOptions{Prefixes: []Key{{"peers"}}, MaxValueBytes: (4 << 20) + 1}},
		{"modes", RedisMigrationOptions{Prefixes: []Key{{"peers"}}, DryRun: true, VerifyOnly: true}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateRedisMigration(source, target, test.options); err == nil {
				t.Fatal("accepted unsafe migration scope/options")
			}
		})
	}
	original := []Key{{"peers"}, {"peers-other"}, {"social", "friends"}}
	normalized, err := validateRedisMigration(source, target, RedisMigrationOptions{Prefixes: original})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Workers != 8 || normalized.ScanCount != 128 || normalized.MaxValueBytes != 1<<20 {
		t.Fatalf("defaults: %+v", normalized)
	}
	original[0][0] = "changed"
	if normalized.Prefixes[0][0] != "peers" {
		t.Fatal("retained caller-owned prefix memory")
	}
	target.opts = &Options{Separator: '/'}
	if _, err := validateRedisMigration(source, target, RedisMigrationOptions{Prefixes: []Key{{"peers"}}}); err == nil {
		t.Fatal("accepted separator mismatch")
	}
}

func TestRedisMigrationCancellationBeforeNetwork(t *testing.T) {
	source, target := migrationValidationClients(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := MigrateRedisToTiKV(ctx, source, target, RedisMigrationOptions{Prefixes: []Key{{"peers"}}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled migration: %v", err)
	}
}

func TestRedisMigrationLiteralGlob(t *testing.T) {
	input := `tenant[one]*?\:peers:`
	want := `tenant\[one\]\*\?\\:peers:`
	if got := escapeRedisMigrationGlob(input); got != want {
		t.Fatalf("glob %q, want %q", got, want)
	}
}

func TestRedisMigrationClaimsDenyNormalAccess(t *testing.T) {
	source, target := migrationValidationClients(t)
	m := redisMigration{source: source, target: target}
	key := Key{"peers", "members"}
	record := redisMigrationRecord{kind: 2, count: 1000}
	raw := m.claim(key, record, 256)
	if _, err := decodeTiKVValue(raw, time.Now()); !errors.Is(err, ErrMigrationInProgress) {
		t.Fatalf("claim was visible as ordinary data: %v", err)
	}
	if count, err := m.claimCount(key, record, raw); err != nil || count != 256 {
		t.Fatalf("resume count=%d error=%v", count, err)
	}
	for _, modified := range [][]byte{nil, raw[:9], append([]byte{}, raw[:len(raw)-1]...)} {
		if _, err := m.claimCount(key, record, modified); !errors.Is(err, ErrMigrationConflict) {
			t.Fatalf("accepted malformed claim: %v", err)
		}
	}
	if _, err := m.claimCount(Key{"peers", "other"}, record, raw); !errors.Is(err, ErrMigrationConflict) {
		t.Fatalf("accepted foreign key: %v", err)
	}
	other := record
	other.count++
	if _, err := m.claimCount(key, other, raw); !errors.Is(err, ErrMigrationConflict) {
		t.Fatalf("accepted changed source cardinality: %v", err)
	}
	final := (tiKVValue{kind: 2, payload: binary.BigEndian.AppendUint64(nil, record.count)}).encode()
	value, err := decodeTiKVValue(final, time.Now())
	if err != nil || value.kind != 2 || !reflect.DeepEqual(value.payload, binary.BigEndian.AppendUint64(nil, 1000)) {
		t.Fatalf("final counter: %+v %v", value, err)
	}
}

// This gate inspects the absolute deadline after a real Redis-to-TiKV import;
// it does not infer TTL preservation from elapsed wall time or sleep thresholds.
func TestRedisMigrationDeadlineRealCluster(t *testing.T) {
	address, redisURL := os.Getenv("GIZCLAW_TEST_TIKV_PD"), os.Getenv("GIZCLAW_TEST_MIGRATION_REDIS")
	if address == "" || redisURL == "" {
		t.Skip("real Redis/TiKV deadline is exercised by the migration Docker gate")
	}
	physical, err := storage.New(map[string]storage.Config{"source": storage.RedisConfig{URL: redisURL}, "target": storage.TiKVConfig{PDAddresses: []string{address}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	primary, _ := physical.Redis("source")
	source, _ := NewRedisWithClient(primary, nil)
	client, _, _ := physical.TiKV("target")
	target, _ := NewTiKVWithClient(client, TiKVReadOptions{}, nil)
	prefix := Key{"deadline-migration-" + rand.Text()}
	key := append(cloneKey(prefix), "value")
	deadline := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	t.Cleanup(func() {
		_ = target.Delete(context.Background(), key)
		_ = primary.Del(context.Background(), string(source.opts.encode(key))).Err()
	})
	if err := source.BatchSet(t.Context(), []Entry{{Key: key, Value: []byte("expires"), Deadline: deadline}}); err != nil {
		t.Fatal(err)
	}
	expires, err := primary.PExpireTime(t.Context(), string(source.opts.encode(key))).Result()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateRedisToTiKV(t.Context(), source, target, RedisMigrationOptions{Prefixes: []Key{prefix}}); err != nil {
		t.Fatal(err)
	}
	raw, exists, err := (&redisMigration{target: target}).rawTarget(t.Context(), key)
	if err != nil || !exists {
		t.Fatalf("target missing: %v", err)
	}
	value, err := decodeTiKVValue(raw, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if value.deadline != deadline.UnixNano() || value.deadline != expires.Nanoseconds() {
		t.Fatalf("absolute expiry changed: target=%d source=%d requested=%d", value.deadline, expires.Nanoseconds(), deadline.UnixNano())
	}
	if _, err := decodeTiKVValue(raw, deadline.Add(time.Nanosecond)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("migrated deadline is not enforced: %v", err)
	}
}

func TestRedisMigrationRejectsClusterOrUnverifiableMode(t *testing.T) {
	for _, info := range []string{"cluster_enabled:1\r\n", "# Cluster\r\n", "unverified", "cluster_enabled:0-suffix\n"} {
		if redisMigrationStandalone(info) {
			t.Fatalf("accepted unverifiable/cluster mode %q", info)
		}
	}
	if !redisMigrationStandalone("# Cluster\r\ncluster_enabled:0\r\n") {
		t.Fatal("rejected standalone Redis INFO response")
	}
}
