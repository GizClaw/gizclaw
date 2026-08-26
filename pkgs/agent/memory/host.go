package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/agent/embed"
	"github.com/GizClaw/gizclaw-go/pkgs/agent/recall"
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/vecstore"
)

// HostConfig configures a [Host].
//
// Store, ObjectStore, and Embedder are all required. NewHost returns an error if
// any of them is nil.
type HostConfig struct {
	// Store is the shared KV store. Required.
	// The store must be created with the same Separator as configured here.
	// Each persona's data is isolated under "mem{sep}{id}{sep}..." prefixes.
	Store kv.Store

	// ObjectStore provides object storage for per-persona HNSW vector indexes.
	// Required.
	//
	// Each persona's object name is stored in KV on first open and loaded
	// through ObjectStore on subsequent opens.
	ObjectStore objectstore.ObjectStore

	// Embedder converts text to vectors. Required.
	//
	// The embedder's Model() and Dimension() are persisted on first use.
	// Subsequent calls to NewHost with a different model or dimension will
	// return an error to prevent mixing incompatible vector spaces.
	Embedder embed.Embedder

	// Compressor is the shared [Compressor] for LLM-based conversation
	// compression. Optional. If set, all personas opened from this host
	// use it as the default compressor in [Memory.Compress].
	//
	// Callers may still pass a per-call compressor to [Memory.Compress],
	// which takes precedence over this default.
	//
	// Use [NewLLMCompressor] to create an LLM-backed compressor that
	// delegates to registered segmentors and profilers.
	Compressor Compressor

	// CompressPolicy controls when auto-compression is triggered during
	// [Conversation.Append].
	//
	// Zero value disables auto-compression. To enable default thresholds,
	// set this field explicitly to [DefaultCompressPolicy()].
	CompressPolicy CompressPolicy

	// Separator is the KV key separator byte. It must match the Store's
	// configured separator. Labels (entity labels, segment labels) must not
	// contain this character.
	//
	// Zero means [kv.DefaultSeparator] (':'), which forbids ':' in labels.
	// For natural labels like "person:小明", use a non-printable separator
	// (e.g., '\x1F') and create the KV store with the same separator.
	Separator byte
}

// embedMeta is persisted in KV to track which embedding model was used.
// On subsequent opens, the host verifies that the current Embedder matches.
type embedMeta struct {
	Model     string `json:"model"`
	Dimension int    `json:"dim"`
}

// Host is the process-level entry point for the memory system.
// It manages Memory instances for many personas, all sharing a single
// KV store and embedder. Each persona gets its own HNSW vector index
// object whose name is tracked in KV and opened through [HostConfig.ObjectStore].
//
// Host is safe for concurrent use. Multiple goroutines can call Open
// simultaneously for different persona IDs.
type Host struct {
	cfg HostConfig

	mu           sync.Mutex
	memories     map[string]*Memory
	closed       bool
	operations   sync.WaitGroup
	personaLocks keyedlock.Locker[string]
}

var errHostClosed = errors.New("memory: host is closed")

// NewHost creates a new Host and validates configuration.
//
// Store, ObjectStore, and Embedder must all be non-nil. NewHost checks the KV store
// for previously persisted embedding model metadata. If found and the model
// name or dimension differs, NewHost returns an error. If not found, it
// persists the current model info.
func NewHost(ctx context.Context, cfg HostConfig) (*Host, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("memory: HostConfig.Store is required")
	}
	if cfg.ObjectStore == nil {
		return nil, fmt.Errorf("memory: HostConfig.ObjectStore is required")
	}
	if cfg.Embedder == nil {
		return nil, fmt.Errorf("memory: HostConfig.Embedder is required")
	}

	if err := checkEmbedMeta(ctx, cfg.Store, cfg.Embedder); err != nil {
		return nil, err
	}

	return &Host{
		cfg:      cfg,
		memories: make(map[string]*Memory),
	}, nil
}

// OpenOption configures per-persona overrides when opening a [Memory].
type OpenOption func(*openConfig)

type openConfig struct {
	compressor     Compressor
	compressPolicy *CompressPolicy
	embedder       embed.Embedder
}

// WithCompressor overrides the host-level [Compressor] for this persona.
func WithCompressor(c Compressor) OpenOption {
	return func(o *openConfig) { o.compressor = c }
}

// WithCompressPolicy overrides the host-level [CompressPolicy] for this persona.
func WithCompressPolicy(p CompressPolicy) OpenOption {
	return func(o *openConfig) { o.compressPolicy = &p }
}

// WithEmbedder overrides the host-level [embed.Embedder] for this persona.
// The embedder's Model and Dimension must match the host's configured embedder
// to ensure vector compatibility; Open returns an error on mismatch.
func WithEmbedder(e embed.Embedder) OpenOption {
	return func(o *openConfig) { o.embedder = e }
}

// Open returns a Memory for a persona. Creates the underlying recall.Index
// if this is the first call for the given ID. Subsequent calls with the
// same ID return the same Memory instance (options are ignored for already
// opened personas).
//
// The id should be a stable, unique identifier for the persona (e.g.,
// "cat_girl", "robot_boy"). It is used as the KV key prefix.
//
// Options override host-level defaults for this persona only. Use
// [WithCompressor], [WithCompressPolicy], or [WithEmbedder] to customize.
func (h *Host) Open(id string, opts ...OpenOption) (*Memory, error) {
	if err := validateKeySegment("id", id, h.cfg.Separator); err != nil {
		return nil, err
	}
	release, err := h.personaLocks.Acquire(context.Background(), id)
	if err != nil {
		return nil, err
	}
	defer release()
	if !h.beginOperation() {
		return nil, errHostClosed
	}
	defer h.operations.Done()

	h.mu.Lock()
	if m, ok := h.memories[id]; ok {
		h.mu.Unlock()
		return m, nil
	}
	h.mu.Unlock()

	// Apply options.
	var oc openConfig
	for _, opt := range opts {
		opt(&oc)
	}

	// Resolve embedder.
	emb := h.cfg.Embedder
	if oc.embedder != nil {
		if oc.embedder.Model() != emb.Model() {
			return nil, fmt.Errorf(
				"memory: Open %q: embedder model mismatch: host=%q, per-persona=%q",
				id, emb.Model(), oc.embedder.Model(),
			)
		}
		if oc.embedder.Dimension() != emb.Dimension() {
			return nil, fmt.Errorf(
				"memory: Open %q: embedder dimension mismatch: host=%d, per-persona=%d",
				id, emb.Dimension(), oc.embedder.Dimension(),
			)
		}
		emb = oc.embedder
	}

	// Resolve compressor.
	compressor := h.cfg.Compressor
	if oc.compressor != nil {
		compressor = oc.compressor
	}

	// Resolve compress policy.
	policy := h.cfg.CompressPolicy
	if oc.compressPolicy != nil {
		policy = *oc.compressPolicy
	}

	// Create per-persona vector index.
	vec, err := h.openVec(id, emb.Dimension())
	if err != nil {
		return nil, fmt.Errorf("memory: Open %q: create vec index: %w", id, err)
	}

	idx := recall.NewIndex(recall.IndexConfig{
		Store:     h.cfg.Store,
		Embedder:  emb,
		Vec:       vec,
		Prefix:    memPrefix(id),
		Separator: h.cfg.Separator,
	})

	m := newMemory(id, h.cfg.Store, idx, compressor, policy, h.cfg.Separator)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		_ = idx.Close()
		return nil, errHostClosed
	}
	h.memories[id] = m
	h.mu.Unlock()
	return m, nil
}

func (h *Host) beginOperation() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	h.operations.Add(1)
	return true
}

// openVec creates or loads a per-persona vector index.
// The file name is read from KV (or generated and stored on first open),
// then loaded through ObjectStore.
func (h *Host) openVec(id string, dim int) (vecstore.Index, error) {
	ctx := context.Background()
	key := vecPathKey(id)
	name, err := h.resolveVecName(ctx, key, id)
	if err != nil {
		return nil, err
	}
	return vecstore.OpenHNSW(h.cfg.ObjectStore, name, vecstore.HNSWConfig{Dim: dim})
}

// resolveVecName reads the HNSW file name from KV. If not found, it
// generates "{id}.hnsw" and stores it.
func (h *Host) resolveVecName(ctx context.Context, key kv.Key, id string) (string, error) {
	data, err := h.cfg.Store.Get(ctx, key)
	if err == nil {
		return string(data), nil
	}
	if !errors.Is(err, kv.ErrNotFound) {
		return "", fmt.Errorf("read vecpath: %w", err)
	}
	name := id + ".hnsw"
	if err := h.cfg.Store.Set(ctx, key, []byte(name)); err != nil {
		return "", fmt.Errorf("write vecpath: %w", err)
	}
	return name, nil
}

// Close releases all resources. After Close, the Host should not be used.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.mu.Unlock()
	h.operations.Wait()

	h.mu.Lock()
	memories := h.memories
	h.memories = nil
	h.mu.Unlock()
	var errs []error
	for _, m := range memories {
		if err := m.index.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Delete removes all data for a persona. Safe to call for non-existent IDs.
// This removes the persona from the host's internal cache, closes and removes
// the per-persona vector index, and deletes all KV entries under its prefix.
func (h *Host) Delete(ctx context.Context, id string) error {
	if err := validateKeySegment("id", id, h.cfg.Separator); err != nil {
		return err
	}
	release, err := h.personaLocks.Acquire(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	if !h.beginOperation() {
		return errHostClosed
	}
	defer h.operations.Done()

	// Read the vec file name BEFORE we delete KV entries, because
	// vecPathKey lives under the same prefix that batch-delete removes.
	var vecName string
	if data, err := h.cfg.Store.Get(ctx, vecPathKey(id)); err == nil {
		vecName = string(data)
	}

	// Build prefix: mem{id} - List will automatically append separator
	// to ensure "a" doesn't match "a:b".
	prefix := memPrefix(id)

	// List and delete all entries with this prefix.
	entries, err := listEntries(ctx, h.cfg.Store, prefix)
	if err != nil {
		return fmt.Errorf("memory: delete %q: list entries: %w", id, err)
	}
	if len(entries) > 0 {
		keys := make([]kv.Key, len(entries))
		for i, e := range entries {
			keys[i] = e.Key
		}
		if err := h.cfg.Store.BatchDelete(ctx, keys); err != nil {
			return fmt.Errorf("memory: delete %q: batch delete: %w", id, err)
		}
	}

	// Close the in-memory vec index and remove its backing file.
	h.mu.Lock()
	if m, ok := h.memories[id]; ok {
		_ = m.index.Close()
		delete(h.memories, id)
	}
	h.mu.Unlock()
	if vecName != "" {
		_ = h.cfg.ObjectStore.Delete(vecName)
	}
	return nil
}

// listEntries returns all entries matching the given prefix.
func listEntries(ctx context.Context, store kv.Store, prefix kv.Key) ([]kv.Entry, error) {
	var results []kv.Entry
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		results = append(results, entry)
	}
	return results, nil
}

// checkEmbedMeta verifies embedding model consistency. On first call it
// persists the current model; on subsequent calls it validates the stored
// model matches.
func checkEmbedMeta(ctx context.Context, store kv.Store, emb embed.Embedder) error {
	key := hostMetaKey("embed")

	current := embedMeta{
		Model:     emb.Model(),
		Dimension: emb.Dimension(),
	}

	data, err := store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			// First time — persist the metadata.
			return writeEmbedMeta(ctx, store, key, current)
		}
		return fmt.Errorf("memory: read embed metadata: %w", err)
	}

	// Existing metadata found — verify it matches.
	var stored embedMeta
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("memory: decode embed metadata: %w", err)
	}

	if stored.Model != current.Model {
		return fmt.Errorf(
			"memory: embed model mismatch: stored %q (dim=%d), current %q (dim=%d); "+
				"vectors from different models are incompatible — "+
				"either use the same model or rebuild the index",
			stored.Model, stored.Dimension, current.Model, current.Dimension,
		)
	}

	if stored.Dimension != current.Dimension {
		return fmt.Errorf(
			"memory: embed dimension mismatch for model %q: stored dim=%d, current dim=%d; "+
				"changing dimension requires rebuilding the index",
			current.Model, stored.Dimension, current.Dimension,
		)
	}

	return nil
}

func writeEmbedMeta(ctx context.Context, store kv.Store, key kv.Key, meta embedMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("memory: encode embed metadata: %w", err)
	}
	if err := store.Set(ctx, key, data); err != nil {
		return fmt.Errorf("memory: write embed metadata: %w", err)
	}
	return nil
}
