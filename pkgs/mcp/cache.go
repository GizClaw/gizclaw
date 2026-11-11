package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

var (
	// ErrNotCached reports a resource or catalog absent from a local snapshot.
	ErrNotCached = errors.New("mcp: resource not cached")
	// ErrCacheFull reports that publishing a resource would exceed cache limits.
	ErrCacheFull = errors.New("mcp: resource cache limit exceeded")
)

// CacheOptions bounds retained serialized resource data. Zero values select
// 1024 resource bodies and 64 MiB, including catalog metadata. Cache hits do not
// expire on the read path; the owner calls Refresh or runs Run to update them.
type CacheOptions struct {
	MaxEntries int
	MaxBytes   int64
}

// ResourceCache holds local versions for one Client's authorization context.
// Share one cache among workflows using that context. NewResourceCache performs
// no I/O and starts no goroutines. The Client remains caller-owned.
//
// Refresh preloads listed resources. Concrete template URIs are loaded on their
// first ReadResource, then included in subsequent refreshes. Concurrent misses
// for the same URI are combined; canceling one waiter does not cancel the others.
type ResourceCache struct {
	client    *Client
	options   CacheOptions
	state     atomic.Pointer[cacheState]
	loads     singleflight.Group
	reads     singleflight.Group
	refreshes singleflight.Group
}

type cacheState struct {
	version     uint64
	refreshedAt time.Time
	resources   json.RawMessage
	templates   json.RawMessage
	listed      map[string]bool
	contents    map[string]json.RawMessage
}

// Snapshot is an immutable local resource version. Its methods perform no I/O
// and return independent SDK values, so callers cannot mutate shared data.
// Retain one Snapshot throughout a workflow turn for consistent local reads.
// Long-lived snapshots retain their own data even after the cache is refreshed.
type Snapshot struct{ state *cacheState }

// NewResourceCache creates an empty, bounded cache for a connected Client.
func NewResourceCache(client *Client, options CacheOptions) (*ResourceCache, error) {
	if client == nil {
		return nil, errors.New("mcp: resource cache requires a client")
	}
	if options.MaxEntries < 0 || options.MaxBytes < 0 {
		return nil, errors.New("mcp: cache limits must not be negative")
	}
	if options.MaxEntries == 0 {
		options.MaxEntries = 1024
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = 64 << 20
	}
	cache := &ResourceCache{client: client, options: options}
	cache.state.Store(&cacheState{contents: make(map[string]json.RawMessage)})
	return cache, nil
}

// Snapshot returns the current local version without contacting the server.
// An empty cache has version zero and no catalogs or bodies.
func (c *ResourceCache) Snapshot() Snapshot { return Snapshot{state: c.state.Load()} }

// Version is a process-local publication counter, not a remote server revision.
func (s Snapshot) Version() uint64 {
	if s.state == nil {
		return 0
	}
	return s.state.version
}

// RefreshedAt reports the last successful full refresh, or zero before one.
// A lazy resource load advances Version without changing RefreshedAt.
func (s Snapshot) RefreshedAt() time.Time {
	if s.state == nil {
		return time.Time{}
	}
	return s.state.refreshedAt
}

// ListResources returns the local catalog, or ErrNotCached before a full refresh.
func (s Snapshot) ListResources() ([]*mcp.Resource, error) {
	if s.state == nil || s.state.resources == nil {
		return nil, ErrNotCached
	}
	return decode[[]*mcp.Resource](s.state.resources)
}

// ListResourceTemplates returns the local template catalog.
func (s Snapshot) ListResourceTemplates() ([]*mcp.ResourceTemplate, error) {
	if s.state == nil || s.state.templates == nil {
		return nil, ErrNotCached
	}
	return decode[[]*mcp.ResourceTemplate](s.state.templates)
}

// ReadResource returns a cached text or binary resource without network I/O.
func (s Snapshot) ReadResource(uri string) (*mcp.ReadResourceResult, error) {
	if s.state == nil {
		return nil, ErrNotCached
	}
	data, ok := s.state.contents[uri]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotCached, uri)
	}
	return decode[*mcp.ReadResourceResult](data)
}

func decode[T any](data json.RawMessage) (T, error) {
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("mcp: decode cached data: %w", err)
	}
	return value, nil
}

// ListResources initializes the cache on first use, then reads the local catalog.
func (c *ResourceCache) ListResources(ctx context.Context) ([]*mcp.Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.state.Load().resources == nil {
		if err := c.Refresh(ctx); err != nil {
			return nil, err
		}
	}
	return c.Snapshot().ListResources()
}

// ListResourceTemplates initializes the cache on first use, then reads locally.
func (c *ResourceCache) ListResourceTemplates(ctx context.Context) ([]*mcp.ResourceTemplate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.state.Load().templates == nil {
		if err := c.Refresh(ctx); err != nil {
			return nil, err
		}
	}
	return c.Snapshot().ListResourceTemplates()
}

// ReadResource serves cached bodies locally, including when the server is
// unavailable. A cache miss loads and publishes a body before returning it.
// Failed reads are not cached. Cancellation is honored even on a cache hit.
func (c *ResourceCache) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result, err := c.Snapshot().ReadResource(uri); !errors.Is(err, ErrNotCached) {
		return result, err
	}
	result := c.reads.DoChan(uri, func() (any, error) {
		requestCtx, cancel := c.client.operationContext(context.WithoutCancel(ctx))
		defer cancel()
		if data, ok := c.state.Load().contents[uri]; ok {
			return data, nil
		}
		data, err := c.load(requestCtx, uri)
		if err != nil {
			return nil, err
		}
		for {
			current := c.state.Load()
			if existing, ok := current.contents[uri]; ok {
				return existing, nil
			}
			next := *current
			next.version++
			next.contents = maps.Clone(current.contents)
			next.contents[uri] = data
			if err := c.checkSize(&next); err != nil {
				return nil, err
			}
			if err := requestCtx.Err(); err != nil {
				return nil, err
			}
			if c.state.CompareAndSwap(current, &next) {
				return data, nil
			}
		}
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		return decode[*mcp.ReadResourceResult](value.Val.(json.RawMessage))
	}
}

// load owns the shared network request independently of any individual waiter.
// It is bounded by the Client timeout and canceled when the Client is closed.
func (c *ResourceCache) load(ctx context.Context, uri string) (json.RawMessage, error) {
	result := c.loads.DoChan(uri, func() (any, error) {
		requestCtx, cancel := c.client.operationContext(context.WithoutCancel(ctx))
		defer cancel()
		resource, err := c.client.ReadResource(requestCtx, uri)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(resource)
		if err != nil {
			return nil, fmt.Errorf("mcp: encode resource: %w", err)
		}
		if int64(len(data)) > c.options.MaxBytes {
			return nil, ErrCacheFull
		}
		return json.RawMessage(data), nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		return value.Val.(json.RawMessage), nil
	}
}

// Refresh loads catalogs, all listed resources, and previously read template
// URIs, then publishes the complete local version atomically. Removed catalog
// resources are dropped. A failure leaves the previous snapshot unchanged.
// Remote data is fetched through the SDK, which honors server cache hints; MCP
// does not provide a transaction spanning different remote resources.
func (c *ResourceCache) Refresh(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result := c.refreshes.DoChan("refresh", func() (any, error) {
		requestCtx, cancel := c.client.operationContext(context.WithoutCancel(ctx))
		defer cancel()
		return nil, c.refresh(requestCtx)
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case value := <-result:
		return value.Err
	}
}

func (c *ResourceCache) refresh(ctx context.Context) error {
	base := c.state.Load()
	resources, err := c.client.ListResources(ctx)
	if err != nil {
		return err
	}
	templates, err := c.client.ListResourceTemplates(ctx)
	if err != nil {
		return err
	}
	listed := make(map[string]bool, len(resources))
	for _, resource := range resources {
		if resource.URI == "" || listed[resource.URI] {
			return errors.New("mcp: empty or duplicate resource URI")
		}
		listed[resource.URI] = true
	}
	wanted := maps.Clone(listed)
	for uri := range base.contents {
		if !base.listed[uri] {
			wanted[uri] = true
		}
	}
	if len(wanted) > c.options.MaxEntries {
		return ErrCacheFull
	}
	resourceJSON, err := json.Marshal(resources)
	if err != nil {
		return fmt.Errorf("mcp: encode resource catalog: %w", err)
	}
	templateJSON, err := json.Marshal(templates)
	if err != nil {
		return fmt.Errorf("mcp: encode template catalog: %w", err)
	}
	var size atomic.Int64
	size.Store(int64(len(resourceJSON)) + int64(len(templateJSON)))
	if size.Load() > c.options.MaxBytes {
		return ErrCacheFull
	}
	uris := slices.Sorted(maps.Keys(wanted))
	bodies := make([]json.RawMessage, len(uris))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(8)
	for index, uri := range uris {
		group.Go(func() error {
			data, err := c.load(groupCtx, uri)
			if err != nil {
				return err
			}
			if size.Add(int64(len(data))) > c.options.MaxBytes {
				return ErrCacheFull
			}
			bodies[index] = data
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	refreshed := make(map[string]json.RawMessage, len(uris))
	for index, uri := range uris {
		refreshed[uri] = bodies[index]
	}
	for {
		current := c.state.Load()
		contents := maps.Clone(refreshed)
		// Preserve concrete URIs first read while this refresh was running.
		for uri, data := range current.contents {
			if _, existed := base.contents[uri]; !existed {
				if _, fetched := contents[uri]; !fetched {
					contents[uri] = data
				}
			}
		}
		next := &cacheState{version: current.version + 1, refreshedAt: time.Now(), resources: resourceJSON, templates: templateJSON, listed: listed, contents: contents}
		if err := c.checkSize(next); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.state.CompareAndSwap(current, next) {
			return nil
		}
	}
}

func (c *ResourceCache) checkSize(state *cacheState) error {
	if len(state.contents) > c.options.MaxEntries {
		return ErrCacheFull
	}
	size := int64(len(state.resources)) + int64(len(state.templates))
	for _, data := range state.contents {
		if int64(len(data)) > c.options.MaxBytes-size {
			return ErrCacheFull
		}
		size += int64(len(data))
	}
	if size > c.options.MaxBytes {
		return ErrCacheFull
	}
	return nil
}

// Run refreshes immediately and periodically until ctx is canceled or the
// Client is closed. The caller starts and joins this blocking worker explicitly.
// If onError is non-nil, refresh failures are reported there and retried on the
// next tick while the last good snapshot remains available. Otherwise Run
// returns the first refresh error. The callback runs synchronously without locks.
func (c *ResourceCache) Run(ctx context.Context, interval time.Duration, onError func(error)) error {
	if interval <= 0 {
		return errors.New("mcp: refresh interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := c.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if c.client.ctx.Err() != nil {
				return ErrClosed
			}
			if onError == nil {
				return err
			}
			onError(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.client.ctx.Done():
			return ErrClosed
		case <-ticker.C:
		}
	}
}
