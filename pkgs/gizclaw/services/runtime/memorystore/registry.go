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
	// draining holds one barrier per binding whose physical backend is being
	// closed but whose exclusive resources are not released yet. A binding
	// backed by a local BBH index owns a badger directory lock, so a Resolve
	// that reopened the same binding before the previous backend finished
	// closing would fail with "Another process is using this Badger database".
	draining map[string]chan struct{}
	open     func(context.Context, Request) (sharedBackend, error)
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

	entry, opener, drained := registry.reserve(key)
	for entry == nil {
		select {
		case <-drained:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
		entry, opener, drained = registry.reserve(key)
	}
	if opener != nil {
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
		backendToClose, drained := registry.finishFailedResolve(key, entry)
		closeErr = errors.Join(closeErr, registry.closeDrained(key, backendToClose, drained))
		return Result{}, errors.Join(errRegistryEntryClosed, closeErr)
	}
	if resolveErr != nil {
		backendToClose, drained := registry.finishFailedResolve(key, entry)
		resolveErr = errors.Join(resolveErr, registry.closeDrained(key, backendToClose, drained))
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

// reserve returns the shared entry for key. When the previous backend for that
// binding is still releasing its physical resources it returns a nil entry and
// the barrier the caller must wait on before reserving again.
func (registry *Registry) reserve(key string) (*registryEntry, func(context.Context, Request) (sharedBackend, error), <-chan struct{}) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.entries == nil {
		registry.entries = make(map[string]*registryEntry)
	}
	if entry := registry.entries[key]; entry != nil {
		return entry, nil, nil
	}
	if drained := registry.draining[key]; drained != nil {
		return nil, nil, drained
	}
	entry := &registryEntry{ready: make(chan struct{}), idle: make(chan struct{})}
	registry.entries[key] = entry
	opener := registry.open
	if opener == nil {
		opener = openSharedBackend
	}
	return entry, opener, nil
}

// beginDrain reserves the binding while its physical backend closes. Callers
// hold registry.mu and must pair every barrier with finishDrain.
func (registry *Registry) beginDrain(key string) chan struct{} {
	if registry.draining == nil {
		registry.draining = make(map[string]chan struct{})
	}
	drained := make(chan struct{})
	registry.draining[key] = drained
	return drained
}

func (registry *Registry) finishDrain(key string, drained chan struct{}) {
	if drained == nil {
		return
	}
	registry.mu.Lock()
	if registry.draining[key] == drained {
		delete(registry.draining, key)
	}
	registry.mu.Unlock()
	close(drained)
}

// closeDrained closes the physical backend and only then releases the binding
// so a waiting Resolve reopens it without contending for its files.
func (registry *Registry) closeDrained(key string, backend sharedBackend, drained chan struct{}) error {
	var err error
	if backend != nil {
		err = backend.Close()
	}
	registry.finishDrain(key, drained)
	return err
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

func (registry *Registry) finishFailedResolve(key string, entry *registryEntry) (sharedBackend, chan struct{}) {
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
		if entry.backend == nil {
			return nil, nil
		}
		return entry.backend, registry.beginDrain(key)
	}
	return nil, nil
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
		var drained chan struct{}
		lease.registry.mu.Lock()
		if lease.registry.entries[lease.key] == lease.entry && !lease.entry.closing {
			lease.entry.refs--
			if lease.entry.refs == 0 && lease.entry.active == 0 {
				delete(lease.registry.entries, lease.key)
				lease.entry.closing = true
				close(lease.entry.idle)
				if backend = lease.entry.backend; backend != nil {
					drained = lease.registry.beginDrain(lease.key)
				}
			}
		}
		lease.registry.mu.Unlock()
		lease.err = errors.Join(lease.err, lease.registry.closeDrained(lease.key, backend, drained))
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
	drains := make(map[string]chan struct{}, len(entries))
	for key, entry := range entries {
		if entry == nil {
			continue
		}
		drains[key] = registry.beginDrain(key)
		if entry.closing {
			continue
		}
		entry.closing = true
		if entry.active == 0 {
			close(entry.idle)
		}
	}
	registry.mu.Unlock()
	var result error
	for key, entry := range entries {
		if entry == nil {
			continue
		}
		<-entry.ready
		<-entry.idle
		result = errors.Join(result, registry.closeDrained(key, entry.backend, drains[key]))
	}
	return result
}
