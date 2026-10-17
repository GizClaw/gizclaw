package kv

import (
	"context"
	"errors"
	"strings"
	"testing"

	redis "github.com/redis/go-redis/v9"
)

func TestRedisOperationErrorPreservesIdentityWithoutDetails(t *testing.T) {
	underlying := errors.New("secret key and endpoint")
	err := redisOperationError("compare and mutate", underlying)
	if !errors.Is(err, underlying) {
		t.Fatal("Redis operation error lost errors.Is identity")
	}
	if strings.Contains(err.Error(), "secret") || err.Error() != "kv: redis compare and mutate failed" {
		t.Fatalf("Redis operation error = %q", err)
	}
}

func TestRedisCanceledContextRetainsIdentity(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisWithClient(client, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Set(ctx, Key{"key"}, []byte("value")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Set() error = %v, want context.Canceled", err)
	}
}

type mgetReadHook struct {
	sizes  []int
	values []any
}

func (h *mgetReadHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *mgetReadHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, command redis.Cmder) error {
		if command.Name() != "mget" {
			return errors.New("expected native MGET")
		}
		h.sizes = append(h.sizes, len(command.Args())-1)
		command.(*redis.SliceCmd).SetVal(h.values)
		return nil
	}
}
func (h *mgetReadHook) ProcessPipelineHook(redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error {
		return errors.New("MGET must not use a GET pipeline")
	}
}

func TestRedisBatchGetUsesMGET(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "reader.invalid:6379"})
	t.Cleanup(func() { _ = client.Close() })
	hook := &mgetReadHook{values: []any{"value", nil, "", "value", nil, "\x00\xff", "a", "b", "c", "d"}}
	client.AddHook(hook)
	store, err := NewRedisWithClient(client, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := []Key{{"record"}, {"missing"}, {"empty"}, {"record"}, {"set"}, {"binary"}, {"a"}, {"b"}, {"c"}, {"d"}}
	results := Prefixed(store, Key{"tenant"}).BatchGet(t.Context(), keys)
	if len(hook.sizes) != 1 || hook.sizes[0] != len(keys) || len(results) != len(keys) {
		t.Fatalf("batch of ten keys was split: MGET sizes=%v, results=%d", hook.sizes, len(results))
	}
	for i, result := range results {
		if hook.values[i] == nil {
			if !errors.Is(result.Err, ErrNotFound) {
				t.Fatalf("missing/non-string result = %+v", result)
			}
		} else if result.Err != nil || string(result.Value) != hook.values[i].(string) {
			t.Fatalf("batch result = %+v", result)
		}
	}
	results[0].Value[0] = 'X'
	if string(results[3].Value) != "value" {
		t.Fatal("duplicate MGET results share mutable bytes")
	}
	store.BatchGet(t.Context(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, result := range store.BatchGet(ctx, keys) {
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("canceled batch result = %+v", result)
		}
	}
	if len(hook.sizes) != 1 {
		t.Fatalf("empty or canceled batch issued another MGET: %v", hook.sizes)
	}
}

func TestRedisBatchGetRejectsMalformedReply(t *testing.T) {
	for _, values := range [][]any{{}, {int64(1)}} {
		client := redis.NewClient(&redis.Options{Addr: "reader.invalid:6379"})
		t.Cleanup(func() { _ = client.Close() })
		client.AddHook(&mgetReadHook{values: values})
		store, err := NewRedisWithClient(client, nil)
		if err != nil {
			t.Fatal(err)
		}
		if result := store.BatchGet(t.Context(), []Key{{"key"}})[0]; result.Err == nil || errors.Is(result.Err, ErrNotFound) {
			t.Fatalf("malformed MGET reply = %+v", result)
		}
	}
}

func TestRedisReadWriteRequiresCompatibleClients(t *testing.T) {
	read := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	write := redis.NewClient(&redis.Options{Addr: "127.0.0.1:2"})
	otherDB := redis.NewClient(&redis.Options{Addr: "127.0.0.1:2", DB: 1})
	t.Cleanup(func() {
		_ = read.Close()
		_ = write.Close()
		_ = otherDB.Close()
	})
	for _, clients := range [][2]*redis.Client{{nil, write}, {read, nil}, {read, otherDB}} {
		if _, err := NewRedisReadWriteWithClients(clients[0], clients[1], nil); err == nil {
			t.Fatal("NewRedisReadWriteWithClients accepted invalid clients")
		}
	}
}

func TestRedisReadWriteDoesNotFallBackToWriter(t *testing.T) {
	read := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	write := redis.NewClient(&redis.Options{Addr: "127.0.0.1:2"})
	t.Cleanup(func() { _ = write.Close() })
	store, err := NewRedisReadWriteWithClients(read, write, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), Key{"key"}); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("Get() error = %v, want the closed reader error", err)
	}
	if _, err := store.HasMember(t.Context(), Key{"set"}, "peer"); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("HasMember() error = %v, want the closed reader error", err)
	}
	if _, err := store.ListMembers(t.Context(), Key{"set"}); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("ListMembers() error = %v, want the closed reader error", err)
	}
	if _, err := store.RangeOrderedMembers(t.Context(), Key{"ordered"}, OrderedRange{Limit: 1}); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("RangeOrderedMembers() error = %v, want the closed reader error", err)
	}
}

type primaryReadHook struct{ calls int }

func (h *primaryReadHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *primaryReadHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *primaryReadHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		h.calls++
		switch result := cmd.(type) {
		case *redis.StringCmd:
			result.SetVal("value")
		case *redis.BoolCmd:
			result.SetVal(true)
		case *redis.StringSliceCmd:
			result.SetVal([]string{"member"})
		default:
			return errors.New("unexpected read command")
		}
		return nil
	}
}

func TestExplicitPrimaryReadsCrossPrefixedViewsWithoutChangingOrdinaryReads(t *testing.T) {
	reader := redis.NewClient(&redis.Options{Addr: "reader.invalid:6379"})
	writer := redis.NewClient(&redis.Options{Addr: "writer.invalid:6379"})
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	readHook, writeHook := &primaryReadHook{}, &primaryReadHook{}
	reader.AddHook(readHook)
	writer.AddHook(writeHook)
	root, err := NewRedisReadWriteWithClients(reader, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := Prefixed(Prefixed(root, Key{"outer"}), Key{"inner"})
	query := func(ctx context.Context) {
		if _, err := store.Get(ctx, Key{"record"}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.HasMember(ctx, Key{"set"}, "member"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListMembers(ctx, Key{"set"}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RangeOrderedMembers(ctx, Key{"ordered"}, OrderedRange{Limit: 1}); err != nil {
			t.Fatal(err)
		}
	}
	query(t.Context())
	query(WithPrimaryReads(t.Context()))
	query(t.Context())
	if readHook.calls != 8 || writeHook.calls != 4 {
		t.Fatalf("reader=%d writer=%d", readHook.calls, writeHook.calls)
	}
}
