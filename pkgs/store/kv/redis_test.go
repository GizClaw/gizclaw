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
