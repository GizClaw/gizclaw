package memorystore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// Registry shares physical backends and transports by RuntimeProfile binding.
// Every Resolve call constructs a Workspace-scoped logical Store and returns a
// reference-counted lease. The final lease closes the physical dependencies.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*registryEntry
}

type registryEntry struct {
	backend sharedBackend
	refs    int
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*registryEntry)}
}

func (registry *Registry) Resolve(ctx context.Context, request Request) (Result, error) {
	if registry == nil {
		return Build(ctx, request)
	}
	key, err := registryKey(request)
	if err != nil {
		return Result{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.entries == nil {
		registry.entries = make(map[string]*registryEntry)
	}
	entry := registry.entries[key]
	if entry == nil {
		backend, err := openSharedBackend(ctx, request)
		if err != nil {
			return Result{}, err
		}
		entry = &registryEntry{backend: backend}
		registry.entries[key] = entry
	}
	result, logicalCloser, err := entry.backend.NewStore(ctx, request)
	if err != nil {
		if entry.refs == 0 {
			delete(registry.entries, key)
			_ = entry.backend.Close()
		}
		return Result{}, err
	}
	bound, err := memory.BindApp(result.Store, request.WorkspaceName)
	if err != nil {
		if logicalCloser != nil {
			_ = logicalCloser.Close()
		}
		if entry.refs == 0 {
			delete(registry.entries, key)
			_ = entry.backend.Close()
		}
		return Result{}, fmt.Errorf("memory store: bind Workspace scope: %w", err)
	}
	entry.refs++
	result.Store = bound
	result.Closer = &registryLease{
		registry: registry,
		key:      key,
		entry:    entry,
		logical:  logicalCloser,
	}
	return result, nil
}

func registryKey(request Request) (string, error) {
	switch request.Binding.Driver {
	case "flowcraft":
	case "mem0":
	case "volc_mem0":
	default:
		return "", fmt.Errorf("memory store: unsupported registry driver %q", request.Binding.Driver)
	}
	identity, err := json.Marshal(request.Binding.Connection)
	if err != nil {
		return "", fmt.Errorf("memory store: encode binding identity: %w", err)
	}
	digest := sha256.Sum256(identity)
	return fmt.Sprintf(
		"%s\x00%s\x00%s\x00%x",
		request.ProfileName,
		request.BindingName,
		request.Binding.Driver,
		digest[:16],
	), nil
}

type registryLease struct {
	registry *Registry
	key      string
	entry    *registryEntry
	logical  io.Closer
	once     sync.Once
	err      error
}

func (lease *registryLease) Close() error {
	if lease == nil {
		return nil
	}
	lease.once.Do(func() {
		if lease.registry == nil || lease.entry == nil {
			return
		}
		if lease.logical != nil {
			lease.err = lease.logical.Close()
		}
		var backend sharedBackend
		lease.registry.mu.Lock()
		if lease.registry.entries[lease.key] == lease.entry {
			lease.entry.refs--
			if lease.entry.refs == 0 {
				delete(lease.registry.entries, lease.key)
				backend = lease.entry.backend
			}
		}
		lease.registry.mu.Unlock()
		if backend != nil {
			lease.err = errors.Join(lease.err, backend.Close())
		}
	})
	return lease.err
}

func (registry *Registry) Close() error {
	if registry == nil {
		return nil
	}
	registry.mu.Lock()
	entries := registry.entries
	registry.entries = make(map[string]*registryEntry)
	registry.mu.Unlock()
	var result error
	for _, entry := range entries {
		if entry != nil && entry.backend != nil {
			result = errors.Join(result, entry.backend.Close())
		}
	}
	return result
}
