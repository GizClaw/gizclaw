package kv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"sort"
	"strconv"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// Redis is a Store backed by a single Redis server. The physical storage
// registry owns the client; Close therefore does not close it.
type Redis struct {
	client *redis.Client
	opts   *Options
}

// NewRedisWithClient creates a Store that borrows an already-open Redis
// client. Pass nil opts for the default key separator.
func NewRedisWithClient(client *redis.Client, opts *Options) (*Redis, error) {
	if client == nil {
		return nil, errors.New("kv: redis client is nil")
	}
	return &Redis{client: client, opts: opts}, nil
}

func (r *Redis) Get(ctx context.Context, key Key) ([]byte, error) {
	value, err := r.client.Get(ctx, string(r.opts.encode(key))).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, redisOperationError("get", err)
	}
	return value, nil
}

func (r *Redis) Set(ctx context.Context, key Key, value []byte) error {
	if err := r.client.Set(ctx, string(r.opts.encode(key)), value, 0).Err(); err != nil {
		return redisOperationError("set", err)
	}
	return nil
}

func (r *Redis) Delete(ctx context.Context, key Key) error {
	if err := r.client.Del(ctx, string(r.opts.encode(key))).Err(); err != nil {
		return redisOperationError("delete", err)
	}
	return nil
}

func (r *Redis) List(ctx context.Context, prefix Key) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(Entry{}, err)
			return
		}
		pattern := "*"
		if encoded := r.opts.encode(prefix); len(encoded) > 0 {
			pattern = escapeRedisPattern(string(encoded)+string(r.opts.sep())) + "*"
		}
		var keys []string
		iterator := r.client.Scan(ctx, 0, pattern, 0).Iterator()
		for iterator.Next(ctx) {
			keys = append(keys, iterator.Val())
		}
		if err := iterator.Err(); err != nil {
			yield(Entry{}, redisOperationError("list", err))
			return
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := ctx.Err(); err != nil {
				yield(Entry{}, err)
				return
			}
			value, err := r.client.Get(ctx, key).Bytes()
			if errors.Is(err, redis.Nil) {
				continue
			}
			if err != nil {
				yield(Entry{}, redisOperationError("list", err))
				return
			}
			if !yield(Entry{Key: r.opts.decode([]byte(key)), Value: value}, nil) {
				return
			}
		}
	}
}

// ListAfter returns a lexicographically ordered page below prefix.
func (r *Redis) ListAfter(ctx context.Context, prefix, after Key, limit int) ([]Entry, error) {
	if limit <= 0 {
		return nil, nil
	}
	afterBytes := r.opts.encode(after)
	entries := make([]Entry, 0, limit)
	for entry, err := range r.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		if len(afterBytes) > 0 && bytes.Compare(r.opts.encode(entry.Key), afterBytes) <= 0 {
			continue
		}
		entries = append(entries, entry)
		if len(entries) == limit {
			break
		}
	}
	return entries, nil
}

func (r *Redis) BatchSet(ctx context.Context, entries []Entry) error {
	return r.BatchMutate(ctx, entries, nil)
}

func (r *Redis) BatchDelete(ctx context.Context, keys []Key) error {
	return r.BatchMutate(ctx, nil, keys)
}

func (r *Redis) BatchMutate(ctx context.Context, entries []Entry, keys []Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	redisKeys, args, err := r.mutationArguments(entries, keys)
	if err != nil {
		return err
	}
	if len(redisKeys) == 0 {
		return nil
	}
	result, err := redisMutationScript.Run(ctx, r.client, redisKeys, args...).Int64()
	if err != nil {
		return redisOperationError("batch mutate", err)
	}
	if result == 0 {
		return ErrInvalidDeadline
	}
	return nil
}

func (r *Redis) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	_, existing, created, err := r.CreateIfAllAbsent(ctx, []Entry{guard}, entries)
	return existing, created, err
}

func (r *Redis) CreateIfAllAbsent(ctx context.Context, guards []Entry, entries []Entry) (Key, []byte, bool, error) {
	if len(guards) == 0 {
		return nil, nil, false, errors.New("kv: create-if-all-absent requires at least one guard")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, false, err
	}
	var keys []string
	args := []any{len(guards), len(entries)}
	for _, guard := range guards {
		keys = append(keys, string(r.opts.encode(guard.Key)))
	}
	for _, entry := range entries {
		keys = append(keys, string(r.opts.encode(entry.Key)))
		args = append(args, entry.Value, deadlineMilliseconds(entry.Deadline))
	}
	for _, guard := range guards {
		args = append(args, guard.Value, deadlineMilliseconds(guard.Deadline))
	}
	result, err := redisCreateScript.Run(ctx, r.client, keys, args...).Slice()
	if err != nil {
		return nil, nil, false, redisOperationError("create if absent", err)
	}
	status, err := redisResultInt(result, 0)
	if err != nil {
		return nil, nil, false, err
	}
	switch status {
	case 1:
		return nil, nil, true, nil
	case 2:
		return nil, nil, false, ErrInvalidDeadline
	case 0:
		index, parseErr := redisResultInt(result, 1)
		if parseErr != nil || index < 1 || index > int64(len(guards)) || len(result) < 3 {
			return nil, nil, false, errors.New("kv: redis create if absent returned invalid conflict")
		}
		return cloneKey(guards[index-1].Key), redisResultBytes(result[2]), false, nil
	default:
		return nil, nil, false, errors.New("kv: redis create if absent returned invalid status")
	}
}

func (r *Redis) CompareAndMutate(
	ctx context.Context,
	guard Key,
	expected []byte,
	entries []Entry,
	keys []Key,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var redisKeys []string
	redisKeys = append(redisKeys, string(r.opts.encode(guard)))
	args := []any{expected, len(entries)}
	for _, entry := range entries {
		redisKeys = append(redisKeys, string(r.opts.encode(entry.Key)))
		args = append(args, entry.Value, deadlineMilliseconds(entry.Deadline))
	}
	for _, key := range keys {
		redisKeys = append(redisKeys, string(r.opts.encode(key)))
	}
	result, err := redisCompareScript.Run(ctx, r.client, redisKeys, args...).Int64()
	if err != nil {
		return false, redisOperationError("compare and mutate", err)
	}
	switch result {
	case 0:
		return false, nil
	case 1:
		return true, nil
	case 2:
		return false, ErrInvalidDeadline
	default:
		return false, errors.New("kv: redis compare and mutate returned invalid status")
	}
}

func (r *Redis) Close() error { return nil }

func (r *Redis) mutationArguments(entries []Entry, deletes []Key) ([]string, []any, error) {
	var redisKeys []string
	args := []any{len(entries)}
	now := time.Now()
	for _, entry := range entries {
		if !entry.Deadline.IsZero() && !entry.Deadline.After(now) {
			return nil, nil, ErrInvalidDeadline
		}
		redisKeys = append(redisKeys, string(r.opts.encode(entry.Key)))
		args = append(args, entry.Value, deadlineMilliseconds(entry.Deadline))
	}
	for _, key := range deletes {
		redisKeys = append(redisKeys, string(r.opts.encode(key)))
	}
	return redisKeys, args, nil
}

func deadlineMilliseconds(deadline time.Time) int64 {
	if deadline.IsZero() {
		return 0
	}
	return deadline.UnixMilli()
}

func escapeRedisPattern(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char == '\\' || char == '*' || char == '?' || char == '[' || char == ']' {
			builder.WriteByte('\\')
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

type redisStoreOperationError struct {
	operation string
	err       error
}

func (e *redisStoreOperationError) Error() string { return "kv: redis " + e.operation + " failed" }
func (e *redisStoreOperationError) Unwrap() error { return e.err }

func redisOperationError(operation string, err error) error {
	return &redisStoreOperationError{operation: operation, err: err}
}

func redisResultInt(result []any, index int) (int64, error) {
	if index >= len(result) {
		return 0, errors.New("kv: redis script returned incomplete result")
	}
	switch value := result[index].(type) {
	case int64:
		return value, nil
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("kv: redis script returned invalid integer: %w", err)
		}
		return parsed, nil
	default:
		return 0, errors.New("kv: redis script returned invalid integer")
	}
}

func redisResultBytes(value any) []byte {
	switch value := value.(type) {
	case string:
		return []byte(value)
	case []byte:
		return append([]byte(nil), value...)
	default:
		return nil
	}
}

var redisMutationScript = redis.NewScript(`
local count = tonumber(ARGV[1])
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
for i = 1, count do
  local deadline = tonumber(ARGV[2 + (i - 1) * 2 + 1])
  if deadline ~= 0 and deadline <= now_ms then
    return 0
  end
end
for i = 1, count do
  local value = ARGV[2 + (i - 1) * 2]
  local deadline = tonumber(ARGV[2 + (i - 1) * 2 + 1])
  redis.call('SET', KEYS[i], value)
  if deadline ~= 0 then redis.call('PEXPIREAT', KEYS[i], deadline) end
end
for i = count + 1, #KEYS do redis.call('DEL', KEYS[i]) end
return 1
`)

var redisCreateScript = redis.NewScript(`
local guards = tonumber(ARGV[1])
local entries = tonumber(ARGV[2])
for i = 1, guards do
  local current = redis.call('GET', KEYS[i])
  if current then return {0, i, current} end
end
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
for i = 1, entries + guards do
  local deadline = tonumber(ARGV[3 + (i - 1) * 2 + 1])
  if deadline ~= 0 and deadline <= now_ms then return {2} end
end
for i = 1, entries do
  local value = ARGV[3 + (i - 1) * 2]
  local deadline = tonumber(ARGV[3 + (i - 1) * 2 + 1])
  redis.call('SET', KEYS[guards + i], value)
  if deadline ~= 0 then redis.call('PEXPIREAT', KEYS[guards + i], deadline) end
end
for i = 1, guards do
  local offset = entries + i
  local value = ARGV[3 + (offset - 1) * 2]
  local deadline = tonumber(ARGV[3 + (offset - 1) * 2 + 1])
  redis.call('SET', KEYS[i], value)
  if deadline ~= 0 then redis.call('PEXPIREAT', KEYS[i], deadline) end
end
return {1}
`)

var redisCompareScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current or current ~= ARGV[1] then return 0 end
local entries = tonumber(ARGV[2])
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
for i = 1, entries do
  local deadline = tonumber(ARGV[3 + (i - 1) * 2 + 1])
  if deadline ~= 0 and deadline <= now_ms then return 2 end
end
for i = 1, entries do
  local value = ARGV[3 + (i - 1) * 2]
  local deadline = tonumber(ARGV[3 + (i - 1) * 2 + 1])
  redis.call('SET', KEYS[1 + i], value)
  if deadline ~= 0 then redis.call('PEXPIREAT', KEYS[1 + i], deadline) end
end
for i = entries + 2, #KEYS do redis.call('DEL', KEYS[i]) end
return 1
`)
