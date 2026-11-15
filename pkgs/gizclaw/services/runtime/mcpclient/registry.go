// Package mcpclient owns Server-wide MCP connections and resource caches.
package mcpclient

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/mcp"
	mcpproto "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/singleflight"
)

// Registry shares connections by Profile, source alias, and connection settings.
// Its constructor performs no I/O. Close cancels opens, workers, and requests.
// Consumers hold leases; the last lease releases that source's dependencies.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*entry
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
}

type entry struct {
	ready  chan struct{}
	cancel context.CancelFunc
	source *Source
	err    error
	refs   int
}

// Source is one shared connection, local resource cache, and tool catalog.
// It belongs to the Registry; callers release leases rather than closing it.
type Source struct {
	Client    *mcp.Client
	Cache     *mcp.ResourceCache
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	tools     atomic.Pointer[json.RawMessage]
	resources atomic.Bool
	toolLoads singleflight.Group
	closeOnce sync.Once
}

// NewRegistry creates an empty Server-owned connection registry.
func NewRegistry() *Registry {
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{entries: make(map[string]*entry), ctx: ctx, cancel: cancel}
}

// Acquire returns a shared source and a lease. Network initialization occurs
// outside the registry lock. A canceled waiter does not cancel other consumers.
func (r *Registry) Acquire(ctx context.Context, profileID, alias string, config apitypes.RuntimeProfileMCPServer) (*Source, io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, nil, err
	}
	// The opener runs asynchronously, so own all nested header values before
	// returning control to a caller that may reuse its Profile snapshot.
	var owned apitypes.RuntimeProfileMCPServer
	if err := json.Unmarshal(encoded, &owned); err != nil {
		return nil, nil, err
	}
	config = owned
	key := fmt.Sprintf("%s/%s/%x", profileID, alias, sha256.Sum256(encoded))
	var opening context.Context
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, nil, errors.New("MCP registry is closed")
	}
	e := r.entries[key]
	if e == nil {
		if len(r.entries) >= 128 {
			r.mu.Unlock()
			return nil, nil, errors.New("MCP source limit exceeded")
		}
		life, cancel := context.WithCancel(r.ctx)
		opening = life
		e = &entry{ready: make(chan struct{}), cancel: cancel}
		r.entries[key] = e
	}
	e.refs++
	r.mu.Unlock()
	if opening != nil {
		go func() {
			e.source, e.err = open(opening, config)
			close(e.ready)
		}()
	}
	lease := &sourceLease{release: func() error { return r.release(key, e) }}
	select {
	case <-ctx.Done():
		_ = lease.Close()
		return nil, nil, ctx.Err()
	case <-e.ready:
		if e.err != nil {
			_ = lease.Close()
			return nil, nil, errors.New("MCP connection initialization failed")
		}
		if e.source.ctx.Err() != nil {
			_ = lease.Close()
			return nil, nil, errors.New("MCP source is closed")
		}
		return e.source, lease, nil
	}
}

func (r *Registry) release(key string, e *entry) error {
	r.mu.Lock()
	e.refs--
	last := e.refs == 0 && r.entries[key] == e
	if last {
		delete(r.entries, key)
	}
	r.mu.Unlock()
	if !last {
		return nil
	}
	e.cancel()
	<-e.ready
	if e.source != nil {
		return e.source.close()
	}
	return nil
}

// Close stops all sources even if a consumer still holds a lease.
func (r *Registry) Close() error {
	r.mu.Lock()
	r.closed = true
	entries := r.entries
	r.entries = make(map[string]*entry)
	r.mu.Unlock()
	r.cancel()
	var result error
	for _, e := range entries {
		e.cancel()
		<-e.ready
		if e.source != nil {
			result = errors.Join(result, e.source.close())
		}
	}
	return result
}

type sourceLease struct {
	once    sync.Once
	release func() error
	err     error
}

func (l *sourceLease) Close() error { l.once.Do(func() { l.err = l.release() }); return l.err }

func open(ctx context.Context, config apitypes.RuntimeProfileMCPServer) (*Source, error) {
	headers := make(http.Header)
	if config.Headers != nil {
		for key, value := range *config.Headers {
			headers.Set(key, value)
		}
	}
	interval := 5 * time.Minute
	if config.RefreshInterval != nil {
		var err error
		interval, err = time.ParseDuration(*config.RefreshInterval)
		if err != nil || interval < time.Second || interval > 24*time.Hour {
			return nil, errors.New("invalid MCP refresh interval")
		}
	}
	client, err := mcp.Dial(ctx, mcp.Config{Endpoint: config.Endpoint, Headers: headers})
	if err != nil {
		return nil, err
	}
	cache, err := mcp.NewResourceCache(client, mcp.CacheOptions{})
	if err != nil {
		return nil, errors.Join(err, client.Close())
	}
	life, cancel := context.WithCancel(ctx)
	source := &Source{Client: client, Cache: cache, ctx: life, cancel: cancel, done: make(chan struct{})}
	go source.run(interval)
	return source, nil
}

// Prepare initializes only the selected capabilities. Existing cached resources
// remain usable if the remote server later becomes unavailable.
func (s *Source) Prepare(ctx context.Context, resources, tools bool) error {
	if resources {
		if s.Cache.Snapshot().Version() == 0 {
			if err := s.Cache.Refresh(ctx); err != nil {
				return errors.New("MCP resource initialization failed")
			}
		}
		s.resources.Store(true)
	}
	if tools && s.tools.Load() == nil {
		return s.refreshTools(ctx)
	}
	return nil
}

// Tools returns an independent copy of the last successfully loaded catalog.
func (s *Source) Tools() ([]*mcpproto.Tool, error) {
	data := s.tools.Load()
	if data == nil {
		return nil, errors.New("MCP tool catalog is unavailable")
	}
	var tools []*mcpproto.Tool
	if err := json.Unmarshal(*data, &tools); err != nil {
		return nil, err
	}
	return tools, nil
}

func (s *Source) refreshTools(ctx context.Context) error {
	result := s.toolLoads.DoChan("tools", func() (any, error) {
		tools, err := s.Client.ListTools(s.ctx)
		if err != nil {
			return nil, errors.New("MCP tool discovery failed")
		}
		data, err := json.Marshal(tools)
		if err == nil {
			value := json.RawMessage(data)
			s.tools.Store(&value)
		}
		return nil, err
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case value := <-result:
		return value.Err
	}
}

func (s *Source) run(interval time.Duration) {
	defer close(s.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
		if s.resources.Load() && s.Cache.Refresh(s.ctx) != nil && s.ctx.Err() == nil {
			slog.WarnContext(s.ctx, "MCP resource refresh failed; retaining cached version")
		}
		if s.tools.Load() != nil && s.refreshTools(s.ctx) != nil && s.ctx.Err() == nil {
			slog.WarnContext(s.ctx, "MCP tool discovery failed; retaining catalog")
		}
	}
}

func (s *Source) close() error {
	var err error
	s.closeOnce.Do(func() { s.cancel(); err = s.Client.Close(); <-s.done })
	return err
}
