package kv

import (
	"context"
	"errors"
	"strings"

	redis "github.com/redis/go-redis/v9"
)

// AddMembers adds members idempotently to one collection.
func (r *Redis) AddMembers(ctx context.Context, key Key, members ...string) error {
	_, err := r.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// RemoveMembers removes members idempotently from one collection.
func (r *Redis) RemoveMembers(ctx context.Context, key Key, members ...string) error {
	_, err := r.ApplyMutation(ctx, Mutation{RemoveMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// HasMember tests membership without enumerating the collection.
func (r *Redis) HasMember(ctx context.Context, key Key, member string) (bool, error) {
	result, err := r.client.SIsMember(ctx, string(r.opts.encode(key)), member).Result()
	return result, setRedisError(err)
}

// ListMembers reads one exact collection. Missing collections return no members.
func (r *Redis) ListMembers(ctx context.Context, key Key) ([]string, error) {
	result, err := r.client.SMembers(ctx, string(r.opts.encode(key))).Result()
	return result, setRedisError(err)
}

func setRedisError(err error) error {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "WRONGTYPE") {
		return ErrWrongType
	}
	return redisOperationError("collection", err)
}

// ApplyMutation updates records and their collection indexes in one Redis
// script. False means a condition failed and no key was changed.
func (r *Redis) ApplyMutation(ctx context.Context, mutation Mutation) (bool, error) {
	if err := mutation.validate(ctx, r.opts); err != nil {
		return false, err
	}
	var keys []string
	args := []any{}
	appendOp := func(op string, key Key, values ...any) {
		keys = append(keys, string(r.opts.encode(key)))
		args = append(args, op, len(values))
		args = append(args, values...)
	}
	for _, c := range mutation.Conditions {
		if c.Expected == nil {
			appendOp("absent", c.Key)
		} else {
			appendOp("equal", c.Key, c.Expected)
		}
	}
	for _, entry := range mutation.Entries {
		deadline := int64(0)
		if !entry.Deadline.IsZero() {
			deadline = entry.Deadline.UnixMilli()
		}
		appendOp("put", entry.Key, entry.Value, deadline)
	}
	for _, key := range mutation.DeleteKeys {
		appendOp("delete", key)
	}
	for _, groups := range []struct {
		op     string
		values []SetMembers
	}{{"add", mutation.AddMembers}, {"remove", mutation.RemoveMembers}, {"zadd", mutation.AddOrderedMembers}, {"zremove", mutation.RemoveOrderedMembers}} {
		for _, group := range groups.values {
			values := make([]any, len(group.Members))
			for i, member := range group.Members {
				values[i] = member
			}
			if len(values) > 0 {
				appendOp(groups.op, group.Key, values...)
			}
		}
	}
	if len(keys) == 0 {
		return true, ctx.Err()
	}
	result, err := redisSetMutationScript.Run(ctx, r.client, keys, args...).Int64()
	if err != nil {
		return false, setRedisError(err)
	}
	switch result {
	case -1:
		return false, ErrInvalidDeadline
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, errors.New("kv: invalid collection mutation result")
	}
}

// Validate every command before writing: Redis script errors do not roll back
// earlier writes. The second pass contains only validated operations.
var redisSetMutationScript = redis.NewScript(`
local operations = {}
local offset = 1
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
for i, key in ipairs(KEYS) do
 local op = ARGV[offset]
 local count = tonumber(ARGV[offset+1])
 local values = {}
 for j=1,count do values[j] = ARGV[offset+1+j] end
 offset = offset + 2 + count
 local kind = redis.call('TYPE', key).ok
 if op == 'equal' then
  if kind ~= 'none' and kind ~= 'string' then return redis.error_reply('WRONGTYPE Operation against a key holding the wrong kind of value') end
  if redis.call('GET', key) ~= values[1] then return 0 end
 elseif op == 'absent' then
  if kind ~= 'none' then return 0 end
 elseif op == 'put' then
  if tonumber(values[2]) ~= 0 and tonumber(values[2]) <= now then return -1 end
 elseif op == 'zadd' or op == 'zremove' then
  if kind ~= 'none' and kind ~= 'zset' then return redis.error_reply('WRONGTYPE Operation against a key holding the wrong kind of value') end
 elseif op == 'add' or op == 'remove' then
  if kind ~= 'none' and kind ~= 'set' then return redis.error_reply('WRONGTYPE Operation against a key holding the wrong kind of value') end
 end
 operations[i] = {op=op, key=key, values=values}
end
for _, command in ipairs(operations) do
 local op, key, values = command.op, command.key, command.values
 if op == 'put' then
  if tonumber(values[2]) == 0 then redis.call('SET', key, values[1])
  else redis.call('SET', key, values[1], 'PXAT', values[2]) end
 elseif op == 'delete' then redis.call('DEL', key)
 elseif op == 'zadd' then
  for _, member in ipairs(values) do redis.call('ZADD', key, 0, member) end
 elseif op == 'zremove' then
  for _, member in ipairs(values) do redis.call('ZREM', key, member) end
 elseif op == 'add' then
  for _, member in ipairs(values) do redis.call('SADD', key, member) end
 elseif op == 'remove' then
  for _, member in ipairs(values) do redis.call('SREM', key, member) end
 end
end
return 1
`)
