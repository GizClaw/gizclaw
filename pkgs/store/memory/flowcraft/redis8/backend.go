// Package redis8 implements a complete Flowcraft Memory backend on Redis 8.
package redis8

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/memory/retrieval"
	"github.com/redis/go-redis/v9"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

const defaultPrefix = "gizclaw:flowcraft:redis8"

// Backend owns the Redis representations of all Flowcraft persistence ports.
// It clones the injected client's options and uses RESP2 for the stable
// go-redis Search response contract without mutating the caller's client.
type Backend struct {
	client *redis.Client
	prefix string
	state  *stateStore
	index  *Index
}

// Config configures a Redis 8 backend and one logical Flowcraft Store.
type Config struct {
	Client    *redis.Client
	Prefix    string
	Flowcraft memoryflowcraft.Config
}

// OpenBackend validates Redis 8 and Redis Search before exposing any port.
func OpenBackend(ctx context.Context, client *redis.Client, prefix string) (*Backend, error) {
	if client == nil {
		return nil, errors.New("flowcraft redis8: client is required")
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = defaultPrefix
	}
	if strings.ContainsAny(prefix, " \t\r\n{}[]") {
		return nil, errors.New("flowcraft redis8: prefix contains unsupported characters")
	}
	options := *client.Options()
	options.Protocol = 2
	options.UnstableResp3 = false
	client = redis.NewClient(&options)
	info, err := client.Info(ctx, "server").Result()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("flowcraft redis8: inspect server: %w", err)
	}
	major, err := redisMajorVersion(info)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	if major < 8 {
		_ = client.Close()
		return nil, fmt.Errorf("flowcraft redis8: Redis 8 or newer is required, server reports major version %d", major)
	}
	index := newIndex(client, prefix)
	if err := index.ensure(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("flowcraft redis8: Redis Search is required: %w", err)
	}
	return &Backend{client: client, prefix: prefix, state: newStateStore(client, prefix), index: index}, nil
}

var redisVersionPattern = regexp.MustCompile(`(?m)^redis_version:([0-9]+)(?:\.[0-9]+)*\r?$`)

func redisMajorVersion(info string) (int, error) {
	match := redisVersionPattern.FindStringSubmatch(info)
	if len(match) != 2 {
		return 0, errors.New("flowcraft redis8: redis_version missing from INFO server")
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("flowcraft redis8: parse redis_version: %w", err)
	}
	return major, nil
}

func (backend *Backend) TemporalStore() flowrecall.TemporalStore {
	return &temporalStore{state: backend.state}
}
func (backend *Backend) EvidenceStore() flowrecall.EvidenceStore {
	return &evidenceStore{state: backend.state}
}
func (backend *Backend) AsyncSemanticQueue() flowrecall.AsyncSemanticQueue {
	return &asyncQueue{state: backend.state}
}
func (backend *Backend) SideEffectOutbox() flowrecall.SideEffectOutbox {
	return &sideEffectOutbox{state: backend.state}
}
func (backend *Backend) RetrievalIndex() retrieval.Index { return backend.index }
func (backend *Backend) Close() error {
	if backend == nil || backend.client == nil {
		return nil
	}
	return backend.client.Close()
}

// Store is the GizClaw memory.Store implemented by the Redis 8 provider.
type Store struct {
	*memoryflowcraft.Store
	backend *Backend
}

// New constructs a complete logical Memory Store on an already-open Redis client.
func New(ctx context.Context, config Config) (*Store, error) {
	if config.Flowcraft.GraphEnabled {
		return nil, errors.New("flowcraft redis8: graph_enabled requires a Flowcraft graph store injection port")
	}
	backend, err := OpenBackend(ctx, config.Client, config.Prefix)
	if err != nil {
		return nil, err
	}
	flowConfig := config.Flowcraft
	flowConfig.TemporalStore = backend.TemporalStore()
	flowConfig.EvidenceStore = backend.EvidenceStore()
	flowConfig.AsyncQueue = backend.AsyncSemanticQueue()
	flowConfig.SideEffectOutbox = backend.SideEffectOutbox()
	flowConfig.RetrievalIndex = backend.RetrievalIndex()
	store, err := memoryflowcraft.New(ctx, flowConfig)
	if err != nil {
		return nil, errors.Join(err, backend.Close())
	}
	return &Store{Store: store, backend: backend}, nil
}

// Close releases the logical Flowcraft Store and the provider-owned Redis client.
func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	return errors.Join(store.Store.Close(), store.backend.Close())
}

var _ memory.Store = (*Store)(nil)
