package vecstore

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// HNSWConfig configures a new [HNSW] index.
type HNSWConfig struct {
	// Dim is the vector dimension. Required; must be positive.
	// All inserted vectors must have exactly this many elements.
	Dim int

	// M is the maximum number of connections per node per layer (except
	// layer 0, which allows 2*M). Higher values improve recall but
	// increase memory usage and insertion time. Default: 16.
	M int

	// EfConstruction is the size of the dynamic candidate list during
	// index building. Higher values produce a higher-quality graph at
	// the cost of slower insertion. Default: 200.
	EfConstruction int

	// EfSearch is the default size of the dynamic candidate list during
	// search queries. Higher values improve recall at the cost of higher
	// latency. Can be adjusted at runtime via [HNSW.SetEfSearch].
	// Default: 50.
	EfSearch int
}

func (c *HNSWConfig) setDefaults() {
	if c.M < 2 {
		c.M = 16
	}
	if c.EfConstruction <= 0 {
		c.EfConstruction = 200
	}
	if c.EfSearch <= 0 {
		c.EfSearch = 50
	}
}

// maxConns returns the maximum number of connections at the given layer.
// Layer 0 allows 2*M; higher layers allow M.
func (c *HNSWConfig) maxConns(layer int) int {
	if layer == 0 {
		return c.M * 2
	}
	return c.M
}

// ---------------------------------------------------------------------------
// Internal priority-queue types for beam search
// ---------------------------------------------------------------------------

// distItem pairs a node's internal ID with its distance to a query vector.
type distItem struct {
	id   uint32
	dist float32
}

// minDistHeap is a min-heap ordered by distance (closest first).
type minDistHeap []distItem

func (h minDistHeap) Len() int           { return len(h) }
func (h minDistHeap) Less(i, j int) bool { return h[i].dist < h[j].dist }
func (h minDistHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minDistHeap) Push(x any)        { *h = append(*h, x.(distItem)) }
func (h *minDistHeap) Pop() any          { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

// maxDistHeap is a max-heap ordered by distance (farthest first).
type maxDistHeap []distItem

func (h maxDistHeap) Len() int           { return len(h) }
func (h maxDistHeap) Less(i, j int) bool { return h[i].dist > h[j].dist }
func (h maxDistHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *maxDistHeap) Push(x any)        { *h = append(*h, x.(distItem)) }
func (h *maxDistHeap) Pop() any          { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

// ---------------------------------------------------------------------------
// Node
// ---------------------------------------------------------------------------

// hnswNode is a single vector in the HNSW graph.
type hnswNode struct {
	id      string     // external string ID
	vector  []float32  // the vector data (len == Dim)
	level   int        // highest layer this node appears on (0-based)
	friends [][]uint32 // friends[layer] = neighbor internal IDs at that layer
}

// ---------------------------------------------------------------------------
// HNSW
// ---------------------------------------------------------------------------

// HNSW is a Hierarchical Navigable Small World index implementing [Index].
//
// It provides approximate nearest-neighbor search with O(log n) query time
// by organizing vectors into a multi-layer navigable graph. Higher layers
// contain exponentially fewer nodes and act as "express lanes" for fast
// traversal; layer 0 contains all nodes for precise local search.
//
// All methods are safe for concurrent use.
type HNSW struct {
	mu       sync.RWMutex
	cfg      HNSWConfig
	nodes    []*hnswNode       // internal ID → node; nil = free slot
	idMap    map[string]uint32 // external ID → internal ID
	entryID  int32             // entry point internal ID; -1 if empty
	maxLevel int               // highest occupied layer in the graph
	count    int               // number of active (non-nil) nodes
	free     []uint32          // recycled internal IDs for reuse
	levelMul float64           // 1/ln(M), for random level generation

	// Persistence. When objects is non-nil, Flush and Close write the index
	// through the objectstore.ObjectStore interface. fileMu serialises file I/O
	// so that concurrent flushes do not race. dirty is an atomic flag
	// set by mutating operations (Insert/Delete) without holding fileMu,
	// avoiding a lock-order inversion between mu and fileMu.
	objects objectstore.ObjectStore
	name    string // object name within objects
	fileMu  sync.Mutex
	dirty   atomic.Bool
}

// Compile-time interface check.
var _ Index = (*HNSW)(nil)

// NewHNSW creates an empty HNSW index with the given configuration.
// Panics if cfg.Dim is not positive.
func NewHNSW(cfg HNSWConfig) *HNSW {
	if cfg.Dim <= 0 {
		panic("vecstore: HNSWConfig.Dim must be positive")
	}
	cfg.setDefaults()
	return &HNSW{
		cfg:      cfg,
		idMap:    make(map[string]uint32),
		entryID:  -1,
		levelMul: 1.0 / math.Log(float64(cfg.M)),
	}
}

// OpenHNSW opens or creates a persistent HNSW index.
// The index is stored in objects under the given name. If the object exists, it
// is loaded and the dimension is validated against cfg. If the object does
// not exist, a new empty index is created.
//
// Subsequent mutations mark the index as dirty, and [HNSW.Flush] /
// [HNSW.Close] write the index through objects.
func OpenHNSW(objects objectstore.ObjectStore, name string, cfg HNSWConfig) (*HNSW, error) {
	if objects == nil {
		return nil, fmt.Errorf("vecstore: hnsw object store is nil")
	}
	r, err := objects.Get(name)
	if err == nil {
		defer r.Close()
		idx, loadErr := LoadHNSW(r)
		if loadErr != nil {
			return nil, fmt.Errorf("vecstore: hnsw load %q: %w", name, loadErr)
		}
		if got := idx.Config().Dim; got != cfg.Dim {
			return nil, fmt.Errorf("vecstore: hnsw %q: dimension mismatch: file=%d config=%d", name, got, cfg.Dim)
		}
		idx.objects = objects
		idx.name = name
		return idx, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("vecstore: hnsw open %q: %w", name, err)
	}

	idx := NewHNSW(cfg)
	idx.objects = objects
	idx.name = name
	return idx, nil
}

// Name returns the object name within the [objectstore.ObjectStore] for a persistent index,
// or "" for in-memory indexes.
func (h *HNSW) Name() string { return h.name }

// Remove removes the underlying object from the [objectstore.ObjectStore].
// No-op for in-memory indexes. After Remove the index must not be used.
func (h *HNSW) Remove() error {
	if h.objects == nil {
		return nil
	}
	h.fileMu.Lock()
	defer h.fileMu.Unlock()
	return h.objects.Delete(h.name)
}

// SetEfSearch adjusts the search-time candidate list size.
// Larger values improve recall at the cost of higher latency.
func (h *HNSW) SetEfSearch(ef int) error {
	if ef <= 0 {
		return fmt.Errorf("vecstore: EfSearch must be positive, got %d", ef)
	}
	h.mu.Lock()
	h.cfg.EfSearch = ef
	h.mu.Unlock()
	return nil
}

// Config returns a copy of the index configuration.
func (h *HNSW) Config() HNSWConfig {
	h.mu.RLock()
	cfg := h.cfg
	h.mu.RUnlock()
	return cfg
}

// Len returns the number of vectors in the index.
func (h *HNSW) Len() int {
	h.mu.RLock()
	n := h.count
	h.mu.RUnlock()
	return n
}

// Flush persists the index through the [objectstore.ObjectStore] if it has pending changes.
// For in-memory indexes (created via [NewHNSW]) this is a no-op.
func (h *HNSW) Flush() error {
	if h.objects == nil {
		return nil
	}
	h.fileMu.Lock()
	defer h.fileMu.Unlock()
	return h.flushLocked()
}

// Close persists any pending changes and releases resources.
// The index should not be used after Close.
func (h *HNSW) Close() error {
	if h.objects == nil {
		return nil
	}
	h.fileMu.Lock()
	defer h.fileMu.Unlock()
	return h.flushLocked()
}

func (h *HNSW) markDirty() {
	if h.objects != nil {
		h.dirty.Store(true)
	}
}

func (h *HNSW) flushLocked() error {
	if !h.dirty.Swap(false) {
		return nil
	}
	pr, pw := io.Pipe()
	putErr := make(chan error, 1)
	go func() {
		putErr <- h.objects.Put(h.name, pr)
		_ = pr.Close()
	}()
	if err := h.Save(pw); err != nil {
		_ = pw.CloseWithError(err)
		err = errors.Join(err, <-putErr)
		h.dirty.Store(true)
		return fmt.Errorf("vecstore: hnsw save %q: %w", h.name, err)
	}
	if err := pw.Close(); err != nil {
		err = errors.Join(err, <-putErr)
		h.dirty.Store(true)
		return fmt.Errorf("vecstore: hnsw flush %q: %w", h.name, err)
	}
	if err := <-putErr; err != nil {
		h.dirty.Store(true)
		return fmt.Errorf("vecstore: hnsw put %q: %w", h.name, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Insert
// ---------------------------------------------------------------------------

// Insert adds or replaces a vector with the given ID.
// Returns an error if the vector dimension does not match the configured Dim.
func (h *HNSW) Insert(id string, vector []float32) error {
	if len(vector) != h.cfg.Dim {
		return fmt.Errorf("vecstore: dimension mismatch: got %d, want %d", len(vector), h.cfg.Dim)
	}

	// Copy to avoid caller mutation.
	vec := make([]float32, len(vector))
	copy(vec, vector)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Replace existing entry if present.
	if oldIdx, ok := h.idMap[id]; ok {
		h.removeLocked(oldIdx)
	}

	// Allocate an internal ID (reuse a free slot or append).
	var idx uint32
	if n := len(h.free); n > 0 {
		idx = h.free[n-1]
		h.free = h.free[:n-1]
	} else {
		idx = uint32(len(h.nodes))
		h.nodes = append(h.nodes, nil)
	}

	level := h.randomLevel()
	nd := &hnswNode{
		id:      id,
		vector:  vec,
		level:   level,
		friends: make([][]uint32, level+1),
	}
	h.nodes[idx] = nd
	h.idMap[id] = idx
	h.count++

	// First node — set as entry point and return.
	if h.entryID < 0 {
		h.entryID = int32(idx)
		h.maxLevel = level
		h.markDirty()
		return nil
	}

	// Phase 1: Greedy descent from the top layer down to level+1.
	// At each layer above the new node's level we only track the single
	// closest node (ef=1 greedy walk).
	cur := uint32(h.entryID)
	curDist := CosineDistance(vec, h.nodes[cur].vector)

	for lev := h.maxLevel; lev > level; lev-- {
		changed := true
		for changed {
			changed = false
			curNode := h.nodes[cur]
			if curNode == nil || lev >= len(curNode.friends) {
				break
			}
			for _, fID := range curNode.friends[lev] {
				if h.nodes[fID] == nil {
					continue
				}
				d := CosineDistance(vec, h.nodes[fID].vector)
				if d < curDist {
					cur = fID
					curDist = d
					changed = true
				}
			}
		}
	}

	// Phase 2: At each layer from min(level, maxLevel) down to 0,
	// do a beam search, select neighbors, and connect bidirectionally.
	topInsert := min(level, h.maxLevel)

	ep := []uint32{cur}
	for lev := topInsert; lev >= 0; lev-- {
		candidates := h.searchLayer(vec, ep, h.cfg.EfConstruction, lev)

		maxC := h.cfg.maxConns(lev)
		neighbors := h.selectClosest(vec, candidates, maxC)
		nd.friends[lev] = neighbors

		// Add bidirectional connections and prune if necessary.
		for _, nID := range neighbors {
			nn := h.nodes[nID]
			if nn == nil || lev >= len(nn.friends) {
				continue
			}
			nn.friends[lev] = append(nn.friends[lev], idx)
			if len(nn.friends[lev]) > maxC {
				nn.friends[lev] = h.selectClosest(nn.vector, nn.friends[lev], maxC)
			}
		}

		// Use candidates as entry points for the next (lower) layer.
		ep = candidates
	}

	// Update the global entry point if the new node is higher.
	if level > h.maxLevel {
		h.entryID = int32(idx)
		h.maxLevel = level
	}

	h.markDirty()
	return nil
}

// BatchInsert adds or replaces multiple vectors.
// ids and vectors must have the same length.
func (h *HNSW) BatchInsert(ids []string, vectors [][]float32) error {
	if len(ids) != len(vectors) {
		return fmt.Errorf("vecstore: BatchInsert length mismatch: %d ids, %d vectors", len(ids), len(vectors))
	}
	for i, id := range ids {
		if err := h.Insert(id, vectors[i]); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

// Search returns the top-k nearest vectors to the query, ordered by
// ascending distance (closest first).
func (h *HNSW) Search(query []float32, topK int) ([]Match, error) {
	if len(query) != h.cfg.Dim {
		return nil, fmt.Errorf("vecstore: dimension mismatch: got %d, want %d", len(query), h.cfg.Dim)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 || topK <= 0 {
		return nil, nil
	}

	// ef must be at least topK to guarantee enough candidates.
	ef := max(h.cfg.EfSearch, topK)

	// Phase 1: Greedy descent from top layer to layer 1.
	cur := uint32(h.entryID)
	entry := h.nodes[cur]
	if entry == nil {
		return nil, nil
	}
	curDist := CosineDistance(query, entry.vector)

	for lev := h.maxLevel; lev > 0; lev-- {
		changed := true
		for changed {
			changed = false
			nd := h.nodes[cur]
			if nd == nil || lev >= len(nd.friends) {
				break
			}
			for _, fID := range nd.friends[lev] {
				fn := h.nodes[fID]
				if fn == nil {
					continue
				}
				d := CosineDistance(query, fn.vector)
				if d < curDist {
					cur = fID
					curDist = d
					changed = true
				}
			}
		}
	}

	// Phase 2: Beam search at layer 0 with ef candidates.
	candidateIDs := h.searchLayer(query, []uint32{cur}, ef, 0)

	// Score, sort, and trim to topK.
	type scored struct {
		id   string
		dist float32
	}
	results := make([]scored, 0, len(candidateIDs))
	for _, cID := range candidateIDs {
		nd := h.nodes[cID]
		if nd == nil {
			continue
		}
		results = append(results, scored{id: nd.id, dist: CosineDistance(query, nd.vector)})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].dist < results[j].dist
	})
	if len(results) > topK {
		results = results[:topK]
	}

	matches := make([]Match, len(results))
	for i, r := range results {
		matches[i] = Match{ID: r.id, Distance: r.dist}
	}
	return matches, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// Delete removes a vector by ID. No error if the ID does not exist.
func (h *HNSW) Delete(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	idx, ok := h.idMap[id]
	if !ok {
		return nil
	}
	h.removeLocked(idx)
	h.markDirty()
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// randomLevel generates a random layer for a new node using an exponential
// distribution: P(level >= l) = exp(-l * ln(M)). Most nodes end up at
// layer 0; higher layers are exponentially rarer.
func (h *HNSW) randomLevel() int {
	// Use 1-rand to get (0,1] and avoid log(0).
	r := max(rand.Float64(), math.SmallestNonzeroFloat64)
	// Cap the level to prevent pathological cases.
	level := min(int(-math.Log(r)*h.levelMul), 31)
	return level
}

// searchLayer performs a beam search on a single layer, starting from the
// given entry points. It returns up to ef internal node IDs closest to
// the query vector.
func (h *HNSW) searchLayer(query []float32, entryPoints []uint32, ef int, layer int) []uint32 {
	visited := make(map[uint32]struct{}, ef*2)

	var candidates minDistHeap
	var results maxDistHeap

	for _, ep := range entryPoints {
		nd := h.nodes[ep]
		if nd == nil {
			continue
		}
		visited[ep] = struct{}{}
		d := CosineDistance(query, nd.vector)
		heap.Push(&candidates, distItem{id: ep, dist: d})
		heap.Push(&results, distItem{id: ep, dist: d})
	}

	for candidates.Len() > 0 {
		closest := heap.Pop(&candidates).(distItem)

		// If the closest unvisited candidate is farther than the farthest
		// result and we already have ef results, stop expanding.
		if results.Len() >= ef && closest.dist > results[0].dist {
			break
		}

		nd := h.nodes[closest.id]
		if nd == nil || layer >= len(nd.friends) {
			continue
		}

		for _, fID := range nd.friends[layer] {
			if _, seen := visited[fID]; seen {
				continue
			}
			visited[fID] = struct{}{}

			fn := h.nodes[fID]
			if fn == nil {
				continue
			}

			d := CosineDistance(query, fn.vector)
			if results.Len() < ef || d < results[0].dist {
				heap.Push(&candidates, distItem{id: fID, dist: d})
				heap.Push(&results, distItem{id: fID, dist: d})
				if results.Len() > ef {
					heap.Pop(&results)
				}
			}
		}
	}

	out := make([]uint32, results.Len())
	for i := range out {
		out[i] = results[i].id
	}
	return out
}

// selectClosest returns up to maxN internal IDs from candidates that are
// closest to the query vector.
func (h *HNSW) selectClosest(query []float32, candidates []uint32, maxN int) []uint32 {
	if len(candidates) <= maxN {
		out := make([]uint32, len(candidates))
		copy(out, candidates)
		return out
	}

	type scored struct {
		id   uint32
		dist float32
	}
	items := make([]scored, 0, len(candidates))
	for _, cID := range candidates {
		if h.nodes[cID] == nil {
			continue
		}
		items = append(items, scored{id: cID, dist: CosineDistance(query, h.nodes[cID].vector)})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].dist < items[j].dist
	})
	if len(items) > maxN {
		items = items[:maxN]
	}

	out := make([]uint32, len(items))
	for i := range items {
		out[i] = items[i].id
	}
	return out
}

// removeLocked removes a node by internal ID. Caller must hold h.mu for writing.
func (h *HNSW) removeLocked(idx uint32) {
	nd := h.nodes[idx]
	if nd == nil {
		return
	}

	// Edges are not guaranteed to stay perfectly symmetric after neighbor
	// pruning, so remove every incoming reference to this slot before reusing it.
	for _, other := range h.nodes {
		if other == nil {
			continue
		}
		for lev := range other.friends {
			other.friends[lev] = removeAllFrom(other.friends[lev], idx)
		}
	}

	delete(h.idMap, nd.id)
	h.nodes[idx] = nil
	h.free = append(h.free, idx)
	h.count--

	// If we just removed the entry point, find a replacement.
	if h.entryID == int32(idx) {
		h.findNewEntry()
	}
}

// findNewEntry scans all nodes to elect a new entry point (the node with
// the highest level). Called after the current entry point is deleted.
func (h *HNSW) findNewEntry() {
	if h.count == 0 {
		h.entryID = -1
		h.maxLevel = 0
		return
	}
	best := int32(-1)
	bestLevel := -1
	for i, nd := range h.nodes {
		if nd != nil && nd.level > bestLevel {
			best = int32(i)
			bestLevel = nd.level
		}
	}
	if best < 0 {
		// No active nodes found — treat as empty regardless of count.
		h.entryID = -1
		h.maxLevel = 0
		h.count = 0
		return
	}
	h.entryID = best
	h.maxLevel = bestLevel
}

// removeAllFrom removes every occurrence of val from s.
func removeAllFrom(s []uint32, val uint32) []uint32 {
	out := s[:0]
	for _, v := range s {
		if v != val {
			out = append(out, v)
		}
	}
	return out
}
