//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
	flowcraftredis8 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft/redis8"
)

const flowcraftRedis8URLEnv = "GIZCLAW_LOCOMO_E2E_FLOWCRAFT_REDIS8_URL"

func TestLoCoMoFlowcraftRedis8BackendSmoke(t *testing.T) {
	client := newFlowcraftRedis8Client(t)
	prefix := fmt.Sprintf("gizclaw:test:locomo:backend-smoke:%d", time.Now().UnixNano())
	backend, err := flowcraftredis8.OpenBackend(context.Background(), client, prefix)
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	if err := (closeGroup{backend, redis8Cleanup{client: client, prefix: prefix}, client}).Close(); err != nil {
		t.Fatal(err)
	}
}

func newFlowcraftRedis8Store(t *testing.T, profile string, config memoryflowcraft.Config) (memorystore.Store, io.Closer) {
	t.Helper()
	client := newFlowcraftRedis8Client(t)
	prefix := fmt.Sprintf("gizclaw:test:locomo:%s:%d", profile, time.Now().UnixNano())
	store, err := flowcraftredis8.New(context.Background(), flowcraftredis8.Config{
		Client: client, Prefix: prefix, Flowcraft: config,
	})
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	return store, closeGroup{store, redis8Cleanup{client: client, prefix: prefix}, client}
}

func newFlowcraftRedis8Client(t *testing.T) *redis.Client {
	t.Helper()
	rawURL := strings.TrimSpace(os.Getenv(flowcraftRedis8URLEnv))
	if err := validateRequired(map[string]string{flowcraftRedis8URLEnv: rawURL}, flowcraftRedis8URLEnv); err != nil {
		t.Fatal(err)
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", flowcraftRedis8URLEnv, err)
	}
	return redis.NewClient(options)
}

type redis8Cleanup struct {
	client *redis.Client
	prefix string
}

func (cleanup redis8Cleanup) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sum := sha256.Sum256([]byte(cleanup.prefix))
	indexName := "giz_fc8_" + hex.EncodeToString(sum[:8])
	_, dropErr := cleanup.client.Do(ctx, "FT.DROPINDEX", indexName).Result()
	if dropErr != nil && strings.Contains(strings.ToLower(dropErr.Error()), "unknown index") {
		dropErr = nil
	}
	var cursor uint64
	var deleteErr error
	for {
		keys, next, err := cleanup.client.Scan(ctx, cursor, cleanup.prefix+"*", 100).Result()
		if err != nil {
			deleteErr = err
			break
		}
		if len(keys) > 0 {
			if err := cleanup.client.Del(ctx, keys...).Err(); err != nil {
				deleteErr = err
				break
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return errors.Join(dropErr, deleteErr)
}

type closeGroup []io.Closer

func (group closeGroup) Close() error {
	var err error
	for _, closer := range group {
		if closer != nil {
			err = errors.Join(err, closer.Close())
		}
	}
	return err
}
