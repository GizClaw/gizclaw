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

var errRegistryEntryClosed = errors.New("memory store: registry entry closed")

// Registry shares physical backends and transports by RuntimeProfile binding.
// Every Resolve call constructs a Workspace-scoped logical Store and returns a
// reference-counted lease. The final lease closes the physical dependencies.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*registryEntry
	open    func(context.Context, Request) (sharedBackend, error)
}

type registryEntry struct {
	backend sharedBackend
	ready   chan struct{}
	err     error
	refs    int
	active  int
	closing bool
	idle    chan struct{}
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*registryEntry), open: openSharedBackend}
}

func (registry *Registry) Resolve(ctx context.Context, request Request) (Result, error) {
	if registry == nil {
		return Build(ctx, request)
	}
	if err := validateLayoutBinding(request); err != nil {
		return Result{}, err
	}
	key, err := registryKey(request)
	if err != nil {
		return Result{}, err
	}

	entry, owner, opener := registry.reserve(key)
	if owner {
		backend, openErr := opener(ctx, request)
		registry.publish(key, entry, backend, openErr)
	}
	select {
	case <-entry.ready:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}

	registry.mu.Lock()
	if entry.err != nil {
		err := entry.err
		registry.mu.Unlock()
		return Result{}, err
	}
	if entry.closing || registry.entries[key] != entry {
		registry.mu.Unlock()
		return Result{}, errRegistryEntryClosed
	}
	entry.active++
	backend := entry.backend
	registry.mu.Unlock()

	result, logicalCloser, resolveErr := backend.NewStore(ctx, request)
	if resolveErr == nil {
		var bound memory.Store
		bound, resolveErr = memory.BindApp(result.Store, request.WorkspaceID)
		if resolveErr != nil {
			resolveErr = fmt.Errorf("memory store: bind Workspace scope: %w", resolveErr)
		} else {
			result.Store = bound
		}
	}
	if resolveErr != nil && logicalCloser != nil {
		resolveErr = errors.Join(resolveErr, logicalCloser.Close())
		logicalCloser = nil
	}

	if resolveErr == nil && !registry.acceptResolve(key, entry) {
		var closeErr error
		if logicalCloser != nil {
			closeErr = logicalCloser.Close()
		}
		backendToClose := registry.finishFailedResolve(key, entry)
		if backendToClose != nil {
			closeErr = errors.Join(closeErr, backendToClose.Close())
		}
		return Result{}, errors.Join(errRegistryEntryClosed, closeErr)
	}
	var backendToClose sharedBackend
	if resolveErr != nil {
		backendToClose = registry.finishFailedResolve(key, entry)
	}
	if backendToClose != nil {
		resolveErr = errors.Join(resolveErr, backendToClose.Close())
	}
	if resolveErr != nil {
		return Result{}, resolveErr
	}
	result.Closer = &registryLease{
		registry: registry,
		key:      key,
		entry:    entry,
		logical:  logicalCloser,
	}
	return result, nil
}

func (registry *Registry) reserve(key string) (*registryEntry, bool, func(context.Context, Request) (sharedBackend, error)) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.entries == nil {
		registry.entries = make(map[string]*registryEntry)
	}
	entry := registry.entries[key]
	if entry != nil {
		return entry, false, nil
	}
	entry = &registryEntry{ready: make(chan struct{}), idle: make(chan struct{})}
	registry.entries[key] = entry
	opener := registry.open
	if opener == nil {
		opener = openSharedBackend
	}
	return entry, true, opener
}

func (registry *Registry) publish(key string, entry *registryEntry, backend sharedBackend, err error) {
	registry.mu.Lock()
	entry.backend = backend
	entry.err = err
	if err != nil && registry.entries[key] == entry {
		delete(registry.entries, key)
	}
	close(entry.ready)
	registry.mu.Unlock()
}

func (registry *Registry) acceptResolve(key string, entry *registryEntry) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if entry.closing || registry.entries[key] != entry {
		return false
	}
	entry.active--
	entry.refs++
	return true
}

func (registry *Registry) finishFailedResolve(key string, entry *registryEntry) sharedBackend {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry.active--
	if entry.closing && entry.active == 0 {
		close(entry.idle)
	}
	if !entry.closing && entry.active == 0 && entry.refs == 0 && registry.entries[key] == entry {
		delete(registry.entries, key)
		entry.closing = true
		close(entry.idle)
		return entry.backend
	}
	return nil
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
		request.ProfileID,
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
		if lease.registry.entries[lease.key] == lease.entry && !lease.entry.closing {
			lease.entry.refs--
			if lease.entry.refs == 0 && lease.entry.active == 0 {
				delete(lease.registry.entries, lease.key)
				lease.entry.closing = true
				close(lease.entry.idle)
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
	for _, entry := range entries {
		if entry == nil || entry.closing {
			continue
		}
		entry.closing = true
		if entry.active == 0 {
			close(entry.idle)
		}
	}
	registry.mu.Unlock()
	var result error
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		<-entry.ready
		<-entry.idle
		if entry.backend != nil {
			result = errors.Join(result, entry.backend.Close())
		}
	}
	return result
}
