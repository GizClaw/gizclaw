package kv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

// ErrMigrationConflict means the destination contains different live data.
var ErrMigrationConflict = errors.New("kv: migration destination conflict")

// ErrMigrationUnsupported means Redis contains data outside the KV contract.
var ErrMigrationUnsupported = errors.New("kv: unsupported migration source")

// ErrMigrationInProgress prevents normal reads and mutations from observing a
// collection until all its members have been imported and verified.
var ErrMigrationInProgress = errors.New("kv: collection migration in progress")

// RedisMigrationOptions selects an offline, prefix-scoped Redis-to-TiKV import.
// Stop all source and destination writers, including deletion/expiry workers,
// before starting. DryRun validates without writing; VerifyOnly requires an
// exact destination. Neither mode deletes source or destination data.
// Failed imports can be rerun: identical records are verified and incomplete
// collections resume idempotently. Each rerun scans the selected prefixes anew.
type RedisMigrationOptions struct {
	Prefixes   []Key
	DryRun     bool
	VerifyOnly bool
	// ScanCount is a Redis COUNT hint, defaulting to 128 (maximum 1024).
	ScanCount int
	// Workers bounds concurrent keys per page, defaulting to 8 (maximum 32).
	Workers int
	// MaxValueBytes defaults to 1 MiB and cannot exceed 4 MiB.
	MaxValueBytes int
}

// RedisMigrationResult counts scan visits (Redis SCAN may repeat keys), newly
// published records, verified existing records, and expired scan candidates.
// MemberVisits includes collection validation and can include SSCAN repeats.
type RedisMigrationResult struct {
	Visited             uint64 `json:"scan_visits"`
	Imported            uint64 `json:"imported"`
	WouldImport         uint64 `json:"would_import"`
	Existing            uint64 `json:"existing_verified_visits"`
	Expired             uint64 `json:"expired_visits"`
	Strings             uint64 `json:"string_visits"`
	Sets                uint64 `json:"set_visits"`
	OrderedSets         uint64 `json:"ordered_set_visits"`
	MemberVisits        uint64 `json:"member_visits"`
	DestinationVerified uint64 `json:"destination_verified"`
	DryRun              bool   `json:"dry_run"`
	VerifyOnly          bool   `json:"verify_only"`
}

type redisMigration struct {
	source  *Redis
	target  *TiKV
	options RedisMigrationOptions
}
type redisMigrationRecord struct {
	kind     byte
	deadline int64
	payload  []byte
	count    uint64
}

// MigrateRedisToTiKV imports selected namespaces from the Redis primary using
// bounded SCAN/SSCAN/ZSCAN pages and fresh TiKV transactions. String deadlines
// are preserved absolutely; persistent sets and zero-score ordered sets are
// supported. Other types, expiring collections and oversized values fail.
// Native Redis Cluster is rejected; source credentials must permit ROLE and INFO.
// Existing live data is never overwritten. Final verification checks both
// directions and detects extra target records before callers switch backends.
func MigrateRedisToTiKV(ctx context.Context, source *Redis, target *TiKV, options RedisMigrationOptions) (RedisMigrationResult, error) {
	result := RedisMigrationResult{DryRun: options.DryRun, VerifyOnly: options.VerifyOnly}
	options, err := validateRedisMigration(source, target, options)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	role, err := source.client.Do(ctx, "ROLE").Slice()
	if err != nil {
		return result, redisOperationError("migration primary check", err)
	}
	if len(role) == 0 || role[0] != "master" {
		return result, errors.New("kv: migration source must be the Redis primary")
	}
	info, err := source.client.Info(ctx, "cluster").Result()
	if err != nil {
		return result, redisOperationError("migration cluster check", err)
	}
	if !redisMigrationStandalone(info) {
		return result, fmt.Errorf("%w: source must be a standalone Redis replication group", ErrMigrationUnsupported)
	}
	m := redisMigration{source: source, target: target, options: options}
	page := func(keys []string) error {
		group, bounded := errgroup.WithContext(ctx)
		group.SetLimit(options.Workers)
		results := make(chan RedisMigrationResult, len(keys))
		seen := make(map[string]bool, len(keys))
		for _, key := range keys {
			if seen[key] {
				continue
			}
			seen[key] = true
			group.Go(func() error { delta, err := m.migrateKey(bounded, key); results <- delta; return err })
		}
		err := group.Wait()
		close(results)
		for delta := range results {
			result.Visited += delta.Visited
			result.Imported += delta.Imported
			result.WouldImport += delta.WouldImport
			result.Existing += delta.Existing
			result.Expired += delta.Expired
			result.Strings += delta.Strings
			result.Sets += delta.Sets
			result.OrderedSets += delta.OrderedSets
			result.MemberVisits += delta.MemberVisits
		}
		return err
	}
	for _, prefix := range options.Prefixes {
		encoded := string(source.opts.encode(prefix))
		if err := page([]string{encoded}); err != nil {
			return result, err
		}
		var cursor uint64
		for {
			keys, next, err := source.client.Scan(ctx, cursor, escapeRedisMigrationGlob(encoded+string(source.opts.sep()))+"*", int64(options.ScanCount)).Result()
			if err != nil {
				return result, redisOperationError("migration scan", err)
			}
			if err := page(keys); err != nil {
				return result, err
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	for _, prefix := range options.Prefixes {
		err := m.eachTargetRecord(ctx, prefix, func(key Key, raw []byte) error {
			if _, err := decodeTiKVValue(raw, time.Now()); errors.Is(err, ErrNotFound) {
				return nil
			} else if err != nil {
				return err
			}
			record, exists, err := m.readSource(ctx, string(source.opts.encode(key)))
			if err != nil {
				return err
			}
			if !exists {
				return migrationKeyError("extra destination", key, ErrMigrationConflict)
			}
			if err := m.verifyRecord(ctx, key, record, raw); err != nil {
				return migrationKeyError("verify destination", key, err)
			}
			result.DestinationVerified++
			return nil
		})
		if err != nil {
			return result, tiKVError("migration verification", err)
		}
	}
	return result, nil
}

func redisMigrationStandalone(info string) bool {
	for line := range strings.SplitSeq(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && key == "cluster_enabled" {
			return value == "0"
		}
	}
	return false
}

func validateRedisMigration(source *Redis, target *TiKV, options RedisMigrationOptions) (RedisMigrationOptions, error) {
	if source == nil || target == nil || source.client == nil || target.client == nil {
		return options, errors.New("kv: Redis and TiKV migration clients are required")
	}
	if source.opts.sep() != target.opts.sep() {
		return options, errors.New("kv: migration key separators must match")
	}
	if options.DryRun && options.VerifyOnly {
		return options, errors.New("kv: dry-run and verify-only are mutually exclusive")
	}
	if options.ScanCount == 0 {
		options.ScanCount = 128
	}
	if options.Workers == 0 {
		options.Workers = 8
	}
	if options.MaxValueBytes == 0 {
		options.MaxValueBytes = 1 << 20
	}
	if options.ScanCount < 1 || options.ScanCount > 1024 || options.Workers < 1 || options.Workers > 32 || options.MaxValueBytes < 1 || options.MaxValueBytes > 4<<20 {
		return options, errors.New("kv: migration requires scan count 1..1024, workers 1..32 and max value bytes 1..4194304")
	}
	if len(options.Prefixes) == 0 {
		return options, errors.New("kv: migration requires explicit non-empty prefixes")
	}
	options.Prefixes = slices.Clone(options.Prefixes)
	for i, prefix := range options.Prefixes {
		if len(prefix) == 0 {
			return options, errors.New("kv: migration prefix must not be empty")
		}
		options.Prefixes[i] = slices.Clone(prefix)
		for _, part := range prefix {
			if part == "" || strings.ContainsRune(part, rune(source.opts.sep())) {
				return options, errors.New("kv: migration prefix has an empty segment or separator")
			}
		}
		if len(target.recordKey(prefix)) > 4096 {
			return options, errors.New("kv: migration prefix exceeds the encoded key limit")
		}
		for _, previous := range options.Prefixes[:i] {
			a, b := string(source.opts.encode(prefix)), string(source.opts.encode(previous))
			sep := string(source.opts.sep())
			if a == b || strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep) {
				return options, errors.New("kv: migration prefixes must not overlap")
			}
		}
	}
	return options, nil
}

func escapeRedisMigrationGlob(value string) string {
	var result strings.Builder
	for i := range len(value) {
		if strings.ContainsRune(`\*?[]`, rune(value[i])) {
			result.WriteByte('\\')
		}
		result.WriteByte(value[i])
	}
	return result.String()
}

var redisMigrationSnapshot = redis.NewScript(`
local kind = redis.call('TYPE', KEYS[1]).ok
if kind == 'none' then return {'none'} end
local deadline = redis.call('PEXPIRETIME', KEYS[1])
if kind == 'string' then
 local length = redis.call('STRLEN', KEYS[1])
 if length > tonumber(ARGV[1]) then return {'oversized', deadline, length} end
 return {kind, deadline, redis.call('GET', KEYS[1])}
end
if kind == 'set' then return {kind, deadline, redis.call('SCARD', KEYS[1])} end
if kind == 'zset' then return {kind, deadline, redis.call('ZCARD', KEYS[1])} end
return {kind, deadline, 0}
`)

func (m *redisMigration) readSource(ctx context.Context, key string) (redisMigrationRecord, bool, error) {
	var record redisMigrationRecord
	raw, err := redisMigrationSnapshot.Run(ctx, m.source.client, []string{key}, m.options.MaxValueBytes).Slice()
	if err != nil {
		return record, false, redisOperationError("migration source snapshot", err)
	}
	if len(raw) == 1 && raw[0] == "none" {
		return record, false, nil
	}
	if len(raw) != 3 {
		return record, false, errors.New("kv: invalid Redis migration snapshot")
	}
	deadline, ok := raw[1].(int64)
	if !ok {
		return record, false, errors.New("kv: invalid Redis migration deadline")
	}
	switch raw[0] {
	case "string":
		value, ok := raw[2].(string)
		if !ok {
			return record, false, errors.New("kv: invalid Redis migration string")
		}
		record.payload = []byte(value)
		if deadline != -1 {
			if deadline <= 0 || deadline > math.MaxInt64/int64(time.Millisecond) {
				return record, false, ErrMigrationUnsupported
			}
			record.deadline = deadline * int64(time.Millisecond)
		}
	case "set", "zset":
		if deadline != -1 {
			return record, false, fmt.Errorf("%w: expiring collection", ErrMigrationUnsupported)
		}
		count, ok := raw[2].(int64)
		if !ok || count <= 0 {
			return record, false, errors.New("kv: invalid Redis migration cardinality")
		}
		record.count = uint64(count)
		record.kind = 1
		if raw[0] == "zset" {
			record.kind = 2
		}
	case "oversized":
		return record, false, fmt.Errorf("%w: string exceeds the %d-byte limit", ErrMigrationUnsupported, m.options.MaxValueBytes)
	case "hash", "list", "stream":
		return record, false, fmt.Errorf("%w: Redis %s", ErrMigrationUnsupported, raw[0])
	default:
		return record, false, ErrMigrationUnsupported
	}
	return record, true, nil
}

func (m *redisMigration) migrateKey(ctx context.Context, encoded string) (RedisMigrationResult, error) {
	var result RedisMigrationResult
	key := m.source.opts.decode([]byte(encoded))
	record, exists, err := m.readSource(ctx, encoded)
	if err != nil {
		return result, migrationKeyError("read source", key, err)
	}
	if !exists {
		return result, nil
	}
	result.Visited = 1
	if len(m.target.recordKey(key)) > 4096 {
		return result, migrationKeyError("key length", key, ErrMigrationUnsupported)
	}
	if record.deadline != 0 && record.deadline <= time.Now().UnixNano() {
		result.Expired = 1
		return result, nil
	}
	if record.kind == 0 {
		result.Strings = 1
	} else if record.kind == 1 {
		result.Sets = 1
	} else {
		result.OrderedSets = 1
	}
	if record.kind != 0 {
		err := m.eachSourceMember(ctx, key, record, func(members []string) error { result.MemberVisits += uint64(len(members)); return nil })
		if err != nil {
			return result, migrationKeyError("validate collection", key, err)
		}
	}
	imported, err := m.importRecord(ctx, key, record)
	if errors.Is(err, ErrInvalidDeadline) && record.deadline != 0 && record.deadline <= time.Now().UnixNano() {
		result.Expired = 1
		return result, nil
	}
	if err != nil {
		return result, migrationKeyError("import", key, err)
	}
	if imported {
		if m.options.DryRun {
			result.WouldImport = 1
		} else {
			result.Imported = 1
		}
	} else {
		result.Existing = 1
	}
	return result, nil
}

func (m *redisMigration) eachSourceMember(ctx context.Context, key Key, record redisMigrationRecord, fn func([]string) error) error {
	var cursor uint64
	for {
		var values []string
		var next uint64
		var err error
		encoded := string(m.source.opts.encode(key))
		if record.kind == 1 {
			values, next, err = m.source.client.SScan(ctx, encoded, cursor, "", int64(m.options.ScanCount)).Result()
		} else {
			var raw []string
			raw, next, err = m.source.client.ZScan(ctx, encoded, cursor, "", int64(m.options.ScanCount)).Result()
			if err == nil {
				if len(raw)%2 != 0 {
					return errors.New("kv: invalid Redis ordered migration page")
				}
				for i := 0; i < len(raw); i += 2 {
					if raw[i+1] != "0" {
						return fmt.Errorf("%w: ordered collection has a nonzero score", ErrMigrationUnsupported)
					}
					values = append(values, raw[i])
				}
			}
		}
		if err != nil {
			return redisOperationError("migration member scan", err)
		}
		for _, member := range values {
			if len(m.target.memberPrefix(key))+len(member) > 4096 {
				return fmt.Errorf("%w: encoded member key exceeds 4096 bytes", ErrMigrationUnsupported)
			}
		}
		// Redis COUNT is a hint; split compact collection responses into bounded writes.
		for len(values) > 0 {
			n := min(len(values), 128)
			if err := fn(values[:n]); err != nil {
				return err
			}
			values = values[n:]
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	current, exists, err := m.readSource(ctx, string(m.source.opts.encode(key)))
	if err != nil {
		return err
	}
	if !exists || current.kind != record.kind || current.count != record.count {
		return errors.New("kv: Redis collection changed during offline migration")
	}
	return nil
}

func migrationKeyError(operation string, key Key, err error) error {
	digest := sha256.Sum256([]byte(key.String()))
	return fmt.Errorf("kv: migration %s key sha256:%x: %w", operation, digest[:8], err)
}

// Claims are ordinary versioned TiKV headers with reserved kind 3. The source
// identity and intended cardinality bind a resumable import to one collection.
func (m *redisMigration) claim(key Key, record redisMigrationRecord, count uint64) []byte {
	id := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", m.source.client.Options().Addr, m.source.client.Options().DB, m.source.opts.encode(key))))
	payload := append([]byte{record.kind}, id[:]...)
	payload = binary.BigEndian.AppendUint64(payload, record.count)
	payload = binary.BigEndian.AppendUint64(payload, count)
	return (tiKVValue{kind: 3, payload: payload}).encode()
}
func (m *redisMigration) claimCount(key Key, record redisMigrationRecord, raw []byte) (uint64, error) {
	expected := m.claim(key, record, 0)
	if len(raw) != len(expected) || !bytes.Equal(raw[:len(raw)-8], expected[:len(expected)-8]) {
		return 0, ErrMigrationConflict
	}
	return binary.BigEndian.Uint64(raw[len(raw)-8:]), nil
}
