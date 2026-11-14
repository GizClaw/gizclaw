package kv

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLocalRealPostimageDeduplication(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_REDIS_DSN")
	if dsn == "" {
		t.Skip("real Redis DSN is not configured")
	}
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	keys := []string{t.Name() + ":record", t.Name() + ":ordered"}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Del(ctx, keys...).Err()
	})
	deadline := time.Now().Add(time.Minute).UnixMilli()
	if err := client.SetArgs(ctx, keys[0], "final", redis.SetArgs{ExpireAt: time.UnixMilli(deadline)}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.ZAdd(ctx, keys[1], redis.Z{Score: 0, Member: "member"}).Err(); err != nil {
		t.Fatal(err)
	}
	// Capturing postimages must preserve the business script's duplicate KEYS
	// and order while returning the final state of each physical key once.
	ordered := []string{keys[0], keys[0], keys[1], keys[0], keys[1]}
	raw, err := client.Eval(ctx, redisPostimageScript("return KEYS", "true"), ordered).Slice()
	if err != nil {
		t.Fatal(err)
	}
	returned := raw[0].([]any)
	for i, key := range ordered {
		if returned[i] != key {
			t.Fatalf("business key %d = %v, want %s", i, returned[i], key)
		}
	}
	images := raw[1].([]any)
	if len(images) != 2 {
		t.Fatalf("postimages = %d, want two physical keys", len(images))
	}
	record, collection := images[0].([]any), images[1].([]any)
	if record[0] != int64(1) || record[1] != "string" || record[2] != deadline || record[3] != "final" {
		t.Fatalf("record postimage = %v", record)
	}
	if collection[0] != int64(3) || collection[1] != "zset" || collection[2] != int64(0) || !slices.Equal(collection[3].([]any), []any{"member"}) {
		t.Fatalf("ordered postimage = %v", collection)
	}
}
