package kv

import (
	"context"
	"errors"
	"slices"
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
func TestRedisEscapesScanPatterns(t *testing.T) {
	if got := escapeRedisPattern(`a*[b]?\\c`); got != `a\*\[b\]\?\\\\c` {
		t.Fatalf("escapeRedisPattern() = %q", got)
	}
}

func TestSortUniqueRedisKeys(t *testing.T) {
	keys := []string{"items/c", "items/a", "items/b", "items/a", "items/c"}
	if got, want := sortUniqueRedisKeys(keys), []string{"items/a", "items/b", "items/c"}; !slices.Equal(got, want) {
		t.Fatalf("sortUniqueRedisKeys() = %q, want %q", got, want)
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
