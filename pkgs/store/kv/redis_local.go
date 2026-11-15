package kv

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	redis "github.com/redis/go-redis/v9"
)

// Pending postimages preserve local visibility until a read proves replication
// progress. Retiring them advances a compact root floor, derived only from local
// writes. Every subsequent ordinary read must prove coverage of that floor; a
// regressing/reconnected reader fails closed rather than resurrecting old data.
// Missing metadata before retirement conservatively retains pending postimages.
type redisLocal struct {
	locks      keyedlock.Locker[string]
	mu         sync.Mutex
	images     map[string]redisPostimage
	floor      redisProgress
	swept      redisProgress
	sweepDirty bool
}

type redisProgress struct {
	id             string
	offset         int64
	previousID     string
	previousOffset int64
}
type redisPostimage struct {
	kind     string
	value    []byte
	members  []string // bytewise sorted for zset postimages
	deadline int64
	progress redisProgress
}

func (r *Redis) localReads(ctx context.Context) bool {
	if r.local == nil || requiresStrongReads(ctx) {
		return false
	}
	r.local.mu.Lock()
	defer r.local.mu.Unlock()
	return len(r.local.images) > 0 || r.local.floor.id != ""
}

func (r *Redis) lockRedisKeys(ctx context.Context, keys []string) (func(), error) {
	if r.local == nil {
		return func() {}, ctx.Err()
	}
	keys = slices.Clone(keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	releases := make([]func(), 0, len(keys))
	release := func() {
		for _, unlock := range slices.Backward(releases) {
			unlock()
		}
	}
	for _, key := range keys {
		unlock, err := r.local.locks.Acquire(ctx, key)
		if err != nil {
			release()
			return nil, err
		}
		releases = append(releases, unlock)
	}
	return release, nil
}

func parseRedisProgress(info string, writer bool) redisProgress {
	fields := make(map[string]string)
	for line := range strings.SplitSeq(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok {
			fields[key] = value
		}
	}
	role := fields["role"]
	offsetKey := "master_repl_offset"
	if role != "master" {
		if writer || role != "slave" || fields["master_sync_in_progress"] != "0" {
			return redisProgress{}
		}
		// slave_repl_offset is applied progress; slave_read_repl_offset may include
		// bytes received but not executed and must never establish visibility.
		offsetKey = "slave_repl_offset"
	}
	id := fields["master_replid"]
	offset, err := strconv.ParseInt(fields[offsetKey], 10, 64)
	if len(id) != 40 || strings.Trim(id, "0") == "" || err != nil || offset < 0 {
		return redisProgress{}
	}
	for _, char := range id {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return redisProgress{}
		}
	}
	previousOffset, _ := strconv.ParseInt(fields["second_repl_offset"], 10, 64)
	return redisProgress{id: id, offset: offset, previousID: fields["master_replid2"], previousOffset: previousOffset}
}

// covers accepts inherited history only below Redis's second_repl_offset:
// that boundary is one past the last byte shared with master_replid2.
func (p redisProgress) covers(required redisProgress) bool {
	if p.id == "" || required.id == "" || p.offset < required.offset {
		return false
	}
	return p.id == required.id || p.previousID == required.id && required.offset < p.previousOffset
}

type redisMutation struct {
	script    *redis.Script
	postimage string
}

// The original script decides success. Only then does its wrapper capture the
// complete typed postimages, atomically with the business mutations.
func newRedisMutation(source, success string) redisMutation {
	return redisMutation{script: redis.NewScript(source), postimage: redisPostimageScript(source, success)}
}

func redisPostimageScript(source, success string) string {
	return "redis.call('PEXPIRETIME', KEYS[1])\nlocal result = (function()\n" + source + "\nend)()\nif type(result) == 'table' and result.err then return result end\nlocal images = {}\nif " + success + ` then
 local seen = {}
 for i,key in ipairs(KEYS) do
  if not seen[key] then
   seen[key] = true
   local kind = redis.call('TYPE',key).ok
   local deadline = redis.call('PEXPIRETIME',key)
   if deadline < 0 then deadline = 0 end
   local value = {}
   if kind == 'string' then value = redis.call('GET',key)
   elseif kind == 'set' then value = redis.call('SMEMBERS',key)
   elseif kind == 'zset' then value = redis.call('ZRANGE',key,0,-1) end
   images[#images+1] = {i,kind,deadline,value}
  end
 end
end
return {result,images}`
}

// runRedisMutation is called with the sorted key locks held through publication.
// INFO failures cannot turn a committed successful mutation into a retryable
// failure. EVAL is the last command in one MULTI/EXEC; go-redis does not retry
// transaction read failures. Optional INFO requests run separately on the pinned
// connection, so their transport failures can never replay an acknowledged EVAL.
func (r *Redis) runRedisMutation(ctx context.Context, mutation redisMutation, keys []string, args []any) (any, error) {
	if r.local == nil {
		return mutation.script.Run(ctx, r.client, keys, args...).Result()
	}
	conn := r.client.Conn()
	defer conn.Close()
	before := conn.Info(ctx, "replication")
	pipe := conn.TxPipeline()
	result := pipe.Eval(ctx, mutation.postimage, keys, args...)
	_, _ = pipe.Exec(ctx)
	raw, err := result.Slice()
	if err != nil {
		return nil, err
	}
	if len(raw) != 2 {
		return nil, errors.New("kv: invalid Redis postimage response")
	}
	images, ok := raw[1].([]any)
	if !ok {
		return nil, errors.New("kv: invalid Redis postimages")
	}
	if len(images) == 0 {
		return raw[0], nil
	}
	after := conn.Info(ctx, "replication")
	var progress redisProgress
	if before.Err() == nil && after.Err() == nil {
		a, b := parseRedisProgress(before.Val(), true), parseRedisProgress(after.Val(), true)
		if a.id != "" && a.id == b.id && a.offset <= b.offset {
			progress = b
		}
	}
	// Parsing is guaranteed by our script, independent of physical business bytes.
	decoded := make(map[string]redisPostimage, len(images))
	for _, item := range images {
		row := item.([]any)
		key := keys[int(row[0].(int64))-1]
		image := redisPostimage{kind: row[1].(string), deadline: row[2].(int64), progress: progress}
		if image.kind == "string" {
			image.value = []byte(row[3].(string))
		} else if members, ok := row[3].([]any); ok {
			for _, member := range members {
				image.members = append(image.members, member.(string))
			}
			if image.kind == "zset" {
				slices.Sort(image.members)
			}
		}
		decoded[key] = image
	}
	r.local.mu.Lock()
	maps.Copy(r.local.images, decoded)
	// A reader may already have swept this position before publication.
	r.local.sweepDirty = true
	r.local.mu.Unlock()
	return raw[0], nil
}

// retire is called with the map mutex held. Only acknowledged local fences,
// never the reader's arbitrary latest offset, advance the required floor.
func (local *redisLocal) retire(progress redisProgress) bool {
	if local.floor.id != "" && !progress.covers(local.floor) {
		return false
	}
	if local.floor.id != "" {
		local.floor.id = progress.id
	}
	// Missing proof cannot cover any image. Repeated proof needs another sweep
	// only after publication; neither fast path bypasses the current floor check.
	if progress.id == "" || !local.sweepDirty && local.swept == progress {
		return true
	}
	for key, image := range local.images {
		if !progress.covers(image.progress) {
			continue
		}
		if local.floor.id == "" || image.progress.offset > local.floor.offset {
			local.floor = redisProgress{id: progress.id, offset: image.progress.offset}
		}
		delete(local.images, key)
	}
	local.swept, local.sweepDirty = progress, false
	return true
}

// readRedisLocal runs INFO and the data query in one MULTI/EXEC attempt.
// Atomic execution excludes topology changes between proof and data query.
// Progress is accepted only when that entire attempt succeeded. Per-key coordination prevents an
// in-flight old response from racing a newer write/publication. The small map
// mutex never spans network IO.
func (r *Redis) readRedisLocal(ctx context.Context, keys []string, queue func(redis.Pipeliner)) (map[string]redisPostimage, error) {
	// Key locks prevent same-key publication. Pin immutable images before IO so
	// another key's retirement sweep cannot strand this in-flight reader.
	pinned := make(map[string]redisPostimage, len(keys))
	r.local.mu.Lock()
	for _, key := range keys {
		if image, ok := r.local.images[key]; ok {
			pinned[key] = image
		}
	}
	r.local.mu.Unlock()
	pipe := r.reader.TxPipeline()
	info := pipe.Info(ctx, "replication")
	queue(pipe)
	_, execErr := pipe.Exec(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	progress := redisProgress{}
	// redis.Nil and WRONGTYPE are valid data responses; INFO still belongs to the
	// same completed attempt. Transport failures must never establish coverage.
	if execErr == nil || errors.Is(execErr, redis.Nil) || strings.HasPrefix(execErr.Error(), "WRONGTYPE") {
		if info.Err() == nil {
			progress = parseRedisProgress(info.Val(), false)
		}
	}
	r.local.mu.Lock()
	defer r.local.mu.Unlock()
	floorCovered := r.local.retire(progress)
	for key, image := range pinned {
		if floorCovered && progress.covers(image.progress) {
			delete(pinned, key)
			continue
		}
		if image.deadline != 0 && image.deadline <= time.Now().UnixMilli() {
			image.kind = "none"
			pinned[key] = image
		}
	}
	if !floorCovered {
		if execErr != nil {
			return pinned, redisOperationError("reader replication coverage", execErr)
		}
		return pinned, errors.New("kv: Redis reader cannot prove coverage of acknowledged local writes")
	}
	return pinned, nil
}

func (r *Redis) getLocal(ctx context.Context, key Key) ([]byte, error) {
	encoded := string(r.opts.encode(key))
	release, err := r.lockRedisKeys(ctx, []string{encoded})
	if err != nil {
		return nil, err
	}
	defer release()
	var cmd *redis.StringCmd
	images, err := r.readRedisLocal(ctx, []string{encoded}, func(p redis.Pipeliner) { cmd = p.Get(ctx, encoded) })
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if image, ok := images[encoded]; ok {
		switch image.kind {
		case "none":
			return nil, ErrNotFound
		case "string":
			return slices.Clone(image.value), nil
		default:
			return nil, ErrWrongType
		}
	}
	if err != nil {
		return nil, err
	}
	value, err := cmd.Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, setRedisError(err)
	}
	return value, nil
}

func (r *Redis) batchGetLocal(ctx context.Context, keys []Key) []GetResult {
	results := make([]GetResult, len(keys))
	if len(keys) == 0 {
		return results
	}
	encoded := make([]string, len(keys))
	for i, key := range keys {
		encoded[i] = string(r.opts.encode(key))
	}
	release, err := r.lockRedisKeys(ctx, encoded)
	if err != nil {
		for i := range results {
			results[i].Err = err
		}
		return results
	}
	defer release()
	var cmd *redis.SliceCmd
	images, err := r.readRedisLocal(ctx, encoded, func(p redis.Pipeliner) { cmd = p.MGet(ctx, encoded...) })
	if err == nil {
		err = cmd.Err()
	}
	if err == nil && len(cmd.Val()) != len(keys) {
		err = errors.New("invalid MGET response length")
	}
	for i, key := range encoded {
		if ctx.Err() != nil {
			results[i].Err = ctx.Err()
			continue
		}
		if image, ok := images[key]; ok {
			if image.kind == "string" {
				results[i].Value = slices.Clone(image.value)
			} else {
				results[i].Err = ErrNotFound
			}
			continue
		}
		if err != nil {
			results[i].Err = redisOperationError("batch get", err)
			continue
		}
		switch value := cmd.Val()[i].(type) {
		case nil:
			results[i].Err = ErrNotFound
		case string:
			results[i].Value = []byte(value)
		default:
			results[i].Err = errors.New("kv: invalid MGET response type")
		}
	}
	return results
}

func (r *Redis) collectionLocal(ctx context.Context, key Key, ordered bool, query OrderedRange) ([]string, error) {
	encoded := string(r.opts.encode(key))
	release, err := r.lockRedisKeys(ctx, []string{encoded})
	if err != nil {
		return nil, err
	}
	defer release()
	var cmd *redis.StringSliceCmd
	images, err := r.readRedisLocal(ctx, []string{encoded}, func(p redis.Pipeliner) {
		if ordered {
			low, high := "-", "+"
			if query.After != nil {
				low = "(" + *query.After
			}
			if query.Before != nil {
				high = "(" + *query.Before
			}
			cmd = p.ZRangeByLex(ctx, encoded, &redis.ZRangeBy{Min: low, Max: high, Count: int64(query.Limit)})
		} else {
			cmd = p.SMembers(ctx, encoded)
		}
	})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	image, ok := images[encoded]
	if !ok {
		if err != nil {
			return nil, err
		}
		return cmd.Val(), setRedisError(cmd.Err())
	}
	if image.kind == "none" {
		return nil, nil
	}
	kind := "set"
	if ordered {
		kind = "zset"
	}
	if image.kind != kind {
		return nil, ErrWrongType
	}
	if !ordered {
		return slices.Clone(image.members), nil
	}
	members := image.members
	if query.After != nil {
		start, found := slices.BinarySearch(members, *query.After)
		if found {
			start++
		}
		members = members[start:]
	}
	if query.Before != nil {
		end, _ := slices.BinarySearch(members, *query.Before)
		members = members[:end]
	}
	return slices.Clone(members[:min(len(members), query.Limit)]), nil
}

func (r *Redis) hasMemberLocal(ctx context.Context, key Key, member string) (bool, error) {
	encoded := string(r.opts.encode(key))
	release, err := r.lockRedisKeys(ctx, []string{encoded})
	if err != nil {
		return false, err
	}
	defer release()
	var cmd *redis.BoolCmd
	images, err := r.readRedisLocal(ctx, []string{encoded}, func(p redis.Pipeliner) { cmd = p.SIsMember(ctx, encoded, member) })
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	image, ok := images[encoded]
	if !ok {
		if err != nil {
			return false, err
		}
		return cmd.Val(), setRedisError(cmd.Err())
	}
	if image.kind == "none" {
		return false, nil
	}
	if image.kind != "set" {
		return false, ErrWrongType
	}
	return slices.Contains(image.members, member), nil
}
