package vecstore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/store/filesystem"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testDirFS is a minimal [filesystem.FS] backed by a local directory.
// Used only in tests — production implementations live elsewhere.
type testDirFS struct{ root string }

var _ filesystem.FS = (*testDirFS)(nil)

func newTestDirFS(root string) *testDirFS { return &testDirFS{root: root} }

func (d *testDirFS) Open(name string) (fs.File, error) {
	return os.Open(filepath.Join(d.root, name))
}

func (d *testDirFS) Create(name string) (io.WriteCloser, error) {
	path := filepath.Join(d.root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.Create(path)
}

func (d *testDirFS) Remove(name string) error {
	err := os.Remove(filepath.Join(d.root, name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (d *testDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(filepath.Join(d.root, name))
}

func (d *testDirFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(filepath.Join(d.root, name))
}

func (d *testDirFS) Glob(pattern string) ([]string, error) {
	return fs.Glob(d, pattern)
}

func (d *testDirFS) Sub(dir string) (fs.FS, error) {
	return os.DirFS(filepath.Join(d.root, dir)), nil
}

func (d *testDirFS) MkdirAll(name string) error {
	return os.MkdirAll(filepath.Join(d.root, name), 0o755)
}

func (d *testDirFS) Rename(oldName, newName string) error {
	oldPath := filepath.Join(d.root, oldName)
	newPath := filepath.Join(d.root, newName)
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func (d *testDirFS) RemoveAll(name string) error {
	return os.RemoveAll(filepath.Join(d.root, name))
}

// newTestHNSW creates an HNSW index with small parameters for fast tests.
func newTestHNSW(dim int) *HNSW {
	return NewHNSW(HNSWConfig{
		Dim:            dim,
		M:              8,
		EfConstruction: 64,
		EfSearch:       32,
	})
}

// randVec generates a random unit vector of the given dimension using rng.
func randVec(rng *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	var norm float64
	for i := range v {
		x := float32(rng.NormFloat64())
		v[i] = x
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range v {
			v[i] /= float32(norm)
		}
	}
	return v
}

// bruteForceSearch returns the top-k IDs by brute-force cosine distance.
func bruteForceSearch(ids []string, vecs [][]float32, query []float32, topK int) []string {
	type scored struct {
		id   string
		dist float32
	}
	results := make([]scored, len(ids))
	for i, id := range ids {
		results[i] = scored{id: id, dist: CosineDistance(query, vecs[i])}
	}
	// Simple selection sort for small k — good enough for tests.
	for i := 0; i < topK && i < len(results); i++ {
		best := i
		for j := i + 1; j < len(results); j++ {
			if results[j].dist < results[best].dist {
				best = j
			}
		}
		results[i], results[best] = results[best], results[i]
	}
	n := topK
	if n > len(results) {
		n = len(results)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = results[i].id
	}
	return out
}

func containsUint32(s []uint32, want uint32) bool {
	for _, got := range s {
		if got == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

func TestHNSWInsertAndSearch(t *testing.T) {
	h := newTestHNSW(4)

	_ = h.Insert("a", []float32{1, 0, 0, 0})
	_ = h.Insert("b", []float32{0, 1, 0, 0})
	_ = h.Insert("c", []float32{0.9, 0.1, 0, 0})

	matches, err := h.Search([]float32{1, 0, 0, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].ID != "a" {
		t.Errorf("top match = %q, want 'a'", matches[0].ID)
	}
	if matches[1].ID != "c" {
		t.Errorf("second match = %q, want 'c'", matches[1].ID)
	}
}

func TestHNSWBatchInsert(t *testing.T) {
	h := newTestHNSW(3)

	ids := []string{"a", "b", "c"}
	vecs := [][]float32{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
	if err := h.BatchInsert(ids, vecs); err != nil {
		t.Fatal(err)
	}
	if h.Len() != 3 {
		t.Errorf("Len = %d, want 3", h.Len())
	}

	matches, err := h.Search([]float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != "a" {
		t.Errorf("expected match 'a', got %v", matches)
	}
}

func TestHNSWBatchInsertMismatch(t *testing.T) {
	h := newTestHNSW(3)
	err := h.BatchInsert([]string{"a", "b"}, [][]float32{{1, 0, 0}})
	if err == nil {
		t.Fatal("expected error for mismatched lengths")
	}
}

func TestHNSWDimensionMismatch(t *testing.T) {
	h := newTestHNSW(4)

	if err := h.Insert("a", []float32{1, 0, 0}); err == nil {
		t.Error("expected error for wrong dimension on Insert")
	}

	_ = h.Insert("b", []float32{1, 0, 0, 0})
	if _, err := h.Search([]float32{1, 0}, 1); err == nil {
		t.Error("expected error for wrong dimension on Search")
	}
}

func TestHNSWDelete(t *testing.T) {
	h := newTestHNSW(3)

	_ = h.Insert("a", []float32{1, 0, 0})
	_ = h.Insert("b", []float32{0, 1, 0})
	_ = h.Insert("c", []float32{0, 0, 1})

	if h.Len() != 3 {
		t.Fatalf("Len = %d, want 3", h.Len())
	}

	_ = h.Delete("b")
	if h.Len() != 2 {
		t.Errorf("Len after delete = %d, want 2", h.Len())
	}

	// Search should not return the deleted vector.
	matches, err := h.Search([]float32{0, 1, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		if m.ID == "b" {
			t.Error("deleted vector 'b' still returned in search")
		}
	}

	// Delete nonexistent — no error.
	if err := h.Delete("nonexistent"); err != nil {
		t.Fatal(err)
	}
}

func TestHNSWDeleteRemovesDanglingIncomingEdges(t *testing.T) {
	h := newTestHNSW(2)

	// Build a minimally inconsistent graph directly. This can happen after
	// neighbor pruning, where one side drops the backlink while the other side
	// still retains the edge.
	h.mu.Lock()
	h.nodes = []*hnswNode{
		{
			id:      "a",
			vector:  []float32{1, 0},
			level:   0,
			friends: [][]uint32{{1}},
		},
		{
			id:      "b",
			vector:  []float32{0, 1},
			level:   0,
			friends: [][]uint32{{}},
		},
	}
	h.idMap = map[string]uint32{
		"a": 0,
		"b": 1,
	}
	h.entryID = 0
	h.maxLevel = 0
	h.count = 2
	h.mu.Unlock()

	if err := h.Delete("b"); err != nil {
		t.Fatal(err)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	if containsUint32(h.nodes[0].friends[0], 1) {
		t.Fatalf("dangling edge to deleted slot still present: %v", h.nodes[0].friends[0])
	}
}

func TestHNSWDeleteEntryPoint(t *testing.T) {
	h := newTestHNSW(3)

	_ = h.Insert("a", []float32{1, 0, 0})
	_ = h.Insert("b", []float32{0, 1, 0})

	// Delete both and verify the index becomes empty.
	_ = h.Delete("a")
	_ = h.Delete("b")
	if h.Len() != 0 {
		t.Fatalf("Len = %d, want 0", h.Len())
	}

	// Insert again after emptying.
	_ = h.Insert("c", []float32{0, 0, 1})
	matches, err := h.Search([]float32{0, 0, 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != "c" {
		t.Errorf("expected match 'c', got %v", matches)
	}
}

func TestHNSWUpdateExisting(t *testing.T) {
	h := newTestHNSW(3)

	_ = h.Insert("a", []float32{1, 0, 0})
	_ = h.Insert("b", []float32{0, 1, 0})

	// Update "a" to a new vector.
	_ = h.Insert("a", []float32{0, 0, 1})

	if h.Len() != 2 {
		t.Fatalf("Len = %d, want 2 (update should not increase count)", h.Len())
	}

	matches, err := h.Search([]float32{0, 0, 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != "a" {
		t.Errorf("expected updated 'a', got %v", matches)
	}
}

func TestHNSWSearchEmpty(t *testing.T) {
	h := newTestHNSW(3)
	matches, err := h.Search([]float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if matches != nil {
		t.Errorf("expected nil for empty index, got %v", matches)
	}
}

func TestHNSWSearchTopKZero(t *testing.T) {
	h := newTestHNSW(3)
	_ = h.Insert("a", []float32{1, 0, 0})

	matches, err := h.Search([]float32{1, 0, 0}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if matches != nil {
		t.Errorf("expected nil for topK=0, got %v", matches)
	}
}

func TestHNSWSingleNode(t *testing.T) {
	h := newTestHNSW(3)

	_ = h.Insert("only", []float32{0.5, 0.5, 0.5})
	matches, err := h.Search([]float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != "only" {
		t.Errorf("expected single match 'only', got %v", matches)
	}
}

func TestHNSWFlush(t *testing.T) {
	if err := newTestHNSW(3).Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestHNSWClose(t *testing.T) {
	if err := newTestHNSW(3).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHNSWSetEfSearch(t *testing.T) {
	h := newTestHNSW(3)
	if err := h.SetEfSearch(200); err != nil {
		t.Fatal(err)
	}

	h.mu.RLock()
	ef := h.cfg.EfSearch
	h.mu.RUnlock()

	if ef != 200 {
		t.Errorf("EfSearch = %d, want 200", ef)
	}
}

func TestHNSWSetEfSearchRejectsNonPositiveValues(t *testing.T) {
	h := newTestHNSW(2)
	h.mu.RLock()
	original := h.cfg.EfSearch
	h.mu.RUnlock()

	if err := h.SetEfSearch(-1); err == nil {
		t.Fatal("expected SetEfSearch(-1) to return error")
	}
	h.mu.RLock()
	ef := h.cfg.EfSearch
	h.mu.RUnlock()
	if ef != original {
		t.Fatalf("EfSearch changed after SetEfSearch(-1): got %d, want %d", ef, original)
	}

	if err := h.SetEfSearch(0); err == nil {
		t.Fatal("expected SetEfSearch(0) to return error")
	}
	h.mu.RLock()
	ef = h.cfg.EfSearch
	h.mu.RUnlock()
	if ef != original {
		t.Fatalf("EfSearch changed after SetEfSearch(0): got %d, want %d", ef, original)
	}
}

func TestHNSWSaveRejectsInvalidEfSearch(t *testing.T) {
	h := newTestHNSW(2)
	if err := h.Insert("a", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}

	h.mu.Lock()
	h.cfg.EfSearch = 0
	h.mu.Unlock()

	var buf bytes.Buffer
	if err := h.Save(&buf); err == nil {
		t.Fatal("expected Save to reject invalid EfSearch")
	}
}

func TestNewHNSWPanicsOnZeroDim(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for Dim=0")
		}
	}()
	NewHNSW(HNSWConfig{Dim: 0})
}

// ---------------------------------------------------------------------------
// Save / Load
// ---------------------------------------------------------------------------

func TestHNSWSaveLoad(t *testing.T) {
	h := newTestHNSW(4)

	_ = h.Insert("a", []float32{1, 0, 0, 0})
	_ = h.Insert("b", []float32{0, 1, 0, 0})
	_ = h.Insert("c", []float32{0, 0, 1, 0})
	_ = h.Delete("b")

	// Save.
	var buf bytes.Buffer
	if err := h.Save(&buf); err != nil {
		t.Fatal(err)
	}

	// Load.
	h2, err := LoadHNSW(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Verify metadata.
	if h2.Len() != h.Len() {
		t.Errorf("loaded Len = %d, want %d", h2.Len(), h.Len())
	}
	if h2.cfg.Dim != h.cfg.Dim {
		t.Errorf("loaded Dim = %d, want %d", h2.cfg.Dim, h.cfg.Dim)
	}

	// Verify search results match.
	query := []float32{1, 0, 0, 0}
	m1, _ := h.Search(query, 2)
	m2, _ := h2.Search(query, 2)

	if len(m1) != len(m2) {
		t.Fatalf("result count mismatch: original %d, loaded %d", len(m1), len(m2))
	}
	for i := range m1 {
		if m1[i].ID != m2[i].ID {
			t.Errorf("result[%d]: original %q, loaded %q", i, m1[i].ID, m2[i].ID)
		}
	}

	// Verify we can insert into the loaded index.
	if err := h2.Insert("d", []float32{0, 0, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if h2.Len() != 3 {
		t.Errorf("Len after insert into loaded = %d, want 3", h2.Len())
	}
}

func TestHNSWSaveLoadEmpty(t *testing.T) {
	h := newTestHNSW(4)

	var buf bytes.Buffer
	if err := h.Save(&buf); err != nil {
		t.Fatal(err)
	}

	h2, err := LoadHNSW(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if h2.Len() != 0 {
		t.Errorf("loaded empty Len = %d, want 0", h2.Len())
	}

	// Verify it's usable.
	if err := h2.Insert("a", []float32{1, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadHNSWInvalidMagic(t *testing.T) {
	_, err := LoadHNSW(bytes.NewReader([]byte("NOPE")))
	if err == nil {
		t.Error("expected error for invalid magic")
	}
}

func TestLoadHNSWRejectsOversizedFriendList(t *testing.T) {
	var buf bytes.Buffer
	le := binary.LittleEndian
	write := func(v any) {
		if err := binary.Write(&buf, le, v); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if _, err := buf.Write(hnswMagic[:]); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	write(hnswVersion)
	write(uint32(2))  // dim
	write(uint32(8))  // M
	write(uint32(64)) // efConstruction
	write(uint32(32)) // efSearch
	write(uint32(1))  // numSlots
	write(uint32(1))  // activeCount
	write(uint32(0))  // maxLevel
	write(int32(0))   // entryID
	write(uint32(0))  // freeCount

	write(uint8(1))  // active
	write(uint32(1)) // idLen
	if _, err := buf.Write([]byte("a")); err != nil {
		t.Fatalf("write id: %v", err)
	}
	write(uint32(0))  // level
	write(float32(1)) // vector[0]
	write(float32(0)) // vector[1]
	write(uint32(17)) // layer-0 friend count; exceeds 2*M (=16)
	for range 17 {
		write(uint32(0))
	}

	if _, err := LoadHNSW(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected oversized friend list to be rejected")
	}
}

func TestLoadHNSWRejectsZeroEfSearch(t *testing.T) {
	var buf bytes.Buffer
	le := binary.LittleEndian
	write := func(v any) {
		if err := binary.Write(&buf, le, v); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if _, err := buf.Write(hnswMagic[:]); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	write(hnswVersion)
	write(uint32(2)) // dim
	write(uint32(8)) // M
	write(uint32(64))
	write(uint32(0)) // efSearch should be rejected
	write(uint32(0)) // numSlots
	write(uint32(0)) // activeCount
	write(uint32(0)) // maxLevel
	write(int32(-1))
	write(uint32(0)) // freeCount

	if _, err := LoadHNSW(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected zero efSearch to be rejected")
	}
}

func TestLoadHNSWRejectsFreeCountExceedingNumSlots(t *testing.T) {
	var buf bytes.Buffer
	le := binary.LittleEndian
	write := func(v any) {
		if err := binary.Write(&buf, le, v); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if _, err := buf.Write(hnswMagic[:]); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	write(hnswVersion)
	write(uint32(2))  // dim
	write(uint32(8))  // M
	write(uint32(64)) // efConstruction
	write(uint32(32)) // efSearch
	write(uint32(1))  // numSlots
	write(uint32(0))  // activeCount
	write(uint32(0))  // maxLevel
	write(int32(-1))
	write(uint32(2)) // freeCount > numSlots should be rejected

	if _, err := LoadHNSW(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected freeCount greater than numSlots to be rejected")
	}
}

func TestLoadHNSWWithOptionsRejectsNumSlotsExceedingLimit(t *testing.T) {
	var buf bytes.Buffer
	le := binary.LittleEndian
	write := func(v any) {
		if err := binary.Write(&buf, le, v); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if _, err := buf.Write(hnswMagic[:]); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	write(hnswVersion)
	write(uint32(2))  // dim
	write(uint32(8))  // M
	write(uint32(64)) // efConstruction
	write(uint32(32)) // efSearch
	write(uint32(2))  // numSlots exceeds configured MaxSlots=1
	write(uint32(0))  // activeCount
	write(uint32(0))  // maxLevel
	write(int32(-1))
	write(uint32(0)) // freeCount

	if _, err := LoadHNSWWithOptions(bytes.NewReader(buf.Bytes()), HNSWLoadOptions{MaxSlots: 1}); err == nil {
		t.Fatal("expected configured MaxSlots to reject the serialized index")
	}
}

func TestLoadHNSWWithOptionsRejectsIDLenExceedingLimit(t *testing.T) {
	var buf bytes.Buffer
	le := binary.LittleEndian
	write := func(v any) {
		if err := binary.Write(&buf, le, v); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if _, err := buf.Write(hnswMagic[:]); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	write(hnswVersion)
	write(uint32(2))  // dim
	write(uint32(8))  // M
	write(uint32(64)) // efConstruction
	write(uint32(32)) // efSearch
	write(uint32(1))  // numSlots
	write(uint32(1))  // activeCount
	write(uint32(0))  // maxLevel
	write(int32(0))
	write(uint32(0)) // freeCount

	write(uint8(1))  // active
	write(uint32(2)) // idLen exceeds configured MaxIDLen=1
	if _, err := buf.Write([]byte("ab")); err != nil {
		t.Fatalf("write id: %v", err)
	}
	write(uint32(0))
	write(float32(1))
	write(float32(0))
	write(uint32(0)) // no friends

	if _, err := LoadHNSWWithOptions(bytes.NewReader(buf.Bytes()), HNSWLoadOptions{MaxIDLen: 1}); err == nil {
		t.Fatal("expected configured MaxIDLen to reject the serialized index")
	}
}

func TestLoadHNSWRejectsDuplicateNodeIDs(t *testing.T) {
	var buf bytes.Buffer
	le := binary.LittleEndian
	write := func(v any) {
		if err := binary.Write(&buf, le, v); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if _, err := buf.Write(hnswMagic[:]); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	write(hnswVersion)
	write(uint32(2))  // dim
	write(uint32(8))  // M
	write(uint32(64)) // efConstruction
	write(uint32(32)) // efSearch
	write(uint32(2))  // numSlots
	write(uint32(2))  // activeCount
	write(uint32(0))  // maxLevel
	write(int32(0))
	write(uint32(0)) // freeCount

	for range 2 {
		write(uint8(1))  // active
		write(uint32(1)) // idLen
		if _, err := buf.Write([]byte("a")); err != nil {
			t.Fatalf("write id: %v", err)
		}
		write(uint32(0))  // level
		write(float32(1)) // vector[0]
		write(float32(0)) // vector[1]
		write(uint32(0))  // no friends
	}

	if _, err := LoadHNSW(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected duplicate node IDs to be rejected")
	}
}

func TestLoadHNSWRejectsFriendReferencedAboveTargetLevel(t *testing.T) {
	var buf bytes.Buffer
	le := binary.LittleEndian
	write := func(v any) {
		if err := binary.Write(&buf, le, v); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	if _, err := buf.Write(hnswMagic[:]); err != nil {
		t.Fatalf("write magic: %v", err)
	}
	write(hnswVersion)
	write(uint32(2))  // dim
	write(uint32(8))  // M
	write(uint32(64)) // efConstruction
	write(uint32(32)) // efSearch
	write(uint32(2))  // numSlots
	write(uint32(2))  // activeCount
	write(uint32(1))  // maxLevel
	write(int32(0))
	write(uint32(0)) // freeCount

	// Slot 0: level-1 node that illegally references slot 1 at layer 1.
	write(uint8(1))
	write(uint32(1))
	if _, err := buf.Write([]byte("a")); err != nil {
		t.Fatalf("write id: %v", err)
	}
	write(uint32(1))  // level
	write(float32(1)) // vector[0]
	write(float32(0)) // vector[1]
	write(uint32(0))  // layer-0 friends
	write(uint32(1))  // layer-1 friends
	write(uint32(1))  // friend -> slot 1

	// Slot 1: level-0 node, so it must not be referenced from layer 1.
	write(uint8(1))
	write(uint32(1))
	if _, err := buf.Write([]byte("b")); err != nil {
		t.Fatalf("write id: %v", err)
	}
	write(uint32(0))  // level
	write(float32(0)) // vector[0]
	write(float32(1)) // vector[1]
	write(uint32(0))  // layer-0 friends

	if _, err := LoadHNSW(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("expected friend referenced above target level to be rejected")
	}
}

// ---------------------------------------------------------------------------
// Recall quality
// ---------------------------------------------------------------------------

func TestHNSWRecall(t *testing.T) {
	const (
		dim     = 32
		n       = 2000
		queries = 50
		topK    = 10
	)

	rng := rand.New(rand.NewPCG(42, 99))

	// Build index and keep a copy for brute-force.
	h := NewHNSW(HNSWConfig{
		Dim:            dim,
		M:              16,
		EfConstruction: 128,
		EfSearch:       64,
	})

	ids := make([]string, n)
	vecs := make([][]float32, n)
	for i := range n {
		ids[i] = fmt.Sprintf("v-%d", i)
		vecs[i] = randVec(rng, dim)
		if err := h.Insert(ids[i], vecs[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Measure recall over random queries.
	totalRecall := 0.0
	for range queries {
		query := randVec(rng, dim)

		// Brute-force ground truth.
		truth := bruteForceSearch(ids, vecs, query, topK)
		truthSet := make(map[string]struct{}, topK)
		for _, id := range truth {
			truthSet[id] = struct{}{}
		}

		// HNSW result.
		matches, err := h.Search(query, topK)
		if err != nil {
			t.Fatal(err)
		}

		// Count hits.
		hits := 0
		for _, m := range matches {
			if _, ok := truthSet[m.ID]; ok {
				hits++
			}
		}
		totalRecall += float64(hits) / float64(topK)
	}

	avgRecall := totalRecall / float64(queries)
	t.Logf("average recall@%d over %d queries on %d vectors: %.3f", topK, queries, n, avgRecall)

	if avgRecall < 0.80 {
		t.Errorf("recall %.3f is below 0.80 threshold", avgRecall)
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestHNSWConcurrent(t *testing.T) {
	const (
		dim         = 16
		numInserts  = 200
		numSearches = 100
	)

	h := newTestHNSW(dim)
	rng := rand.New(rand.NewPCG(7, 13))

	// Pre-insert some vectors.
	for i := range 50 {
		_ = h.Insert(fmt.Sprintf("pre-%d", i), randVec(rng, dim))
	}

	var wg sync.WaitGroup

	// Concurrent inserts.
	wg.Add(numInserts)
	for i := range numInserts {
		go func(i int) {
			defer wg.Done()
			localRng := rand.New(rand.NewPCG(uint64(i)*17, uint64(i)*31))
			_ = h.Insert(fmt.Sprintf("ins-%d", i), randVec(localRng, dim))
		}(i)
	}

	// Concurrent searches.
	wg.Add(numSearches)
	for i := range numSearches {
		go func(i int) {
			defer wg.Done()
			localRng := rand.New(rand.NewPCG(uint64(i)*41, uint64(i)*53))
			_, _ = h.Search(randVec(localRng, dim), 5)
		}(i)
	}

	// Concurrent deletes.
	wg.Add(20)
	for i := range 20 {
		go func(i int) {
			defer wg.Done()
			_ = h.Delete(fmt.Sprintf("pre-%d", i))
		}(i)
	}

	wg.Wait()

	// Sanity check: the index should still work.
	_, err := h.Search(randVec(rng, dim), 5)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkHNSWInsert(b *testing.B) {
	const dim = 128
	rng := rand.New(rand.NewPCG(1, 2))

	// Pre-build an index with 1000 vectors.
	h := NewHNSW(HNSWConfig{Dim: dim, M: 16, EfConstruction: 100})
	for i := range 1000 {
		_ = h.Insert(fmt.Sprintf("pre-%d", i), randVec(rng, dim))
	}

	// Benchmark inserting additional vectors.
	vecs := make([][]float32, b.N)
	for i := range vecs {
		vecs[i] = randVec(rng, dim)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.Insert(fmt.Sprintf("bench-%d", i), vecs[i])
	}
}

func BenchmarkHNSWSearch(b *testing.B) {
	const dim = 128
	rng := rand.New(rand.NewPCG(3, 4))

	h := NewHNSW(HNSWConfig{Dim: dim, M: 16, EfConstruction: 200, EfSearch: 50})
	for i := range 10000 {
		_ = h.Insert(fmt.Sprintf("v-%d", i), randVec(rng, dim))
	}

	queries := make([][]float32, 1000)
	for i := range queries {
		queries[i] = randVec(rng, dim)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = h.Search(queries[i%len(queries)], 10)
	}
}

func BenchmarkHNSWSearch_VaryEf(b *testing.B) {
	const dim = 128
	rng := rand.New(rand.NewPCG(5, 6))

	h := NewHNSW(HNSWConfig{Dim: dim, M: 16, EfConstruction: 200})
	for i := range 10000 {
		_ = h.Insert(fmt.Sprintf("v-%d", i), randVec(rng, dim))
	}

	queries := make([][]float32, 100)
	for i := range queries {
		queries[i] = randVec(rng, dim)
	}

	for _, ef := range []int{10, 50, 100, 200} {
		b.Run(fmt.Sprintf("ef=%d", ef), func(b *testing.B) {
			if err := h.SetEfSearch(ef); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = h.Search(queries[i%len(queries)], 10)
			}
		})
	}
}

func BenchmarkHNSWSaveLoad(b *testing.B) {
	const dim = 128
	rng := rand.New(rand.NewPCG(7, 8))

	h := NewHNSW(HNSWConfig{Dim: dim, M: 16, EfConstruction: 100})
	for i := range 5000 {
		_ = h.Insert(fmt.Sprintf("v-%d", i), randVec(rng, dim))
	}

	b.Run("Save", func(b *testing.B) {
		var buf bytes.Buffer
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			_ = h.Save(&buf)
		}
		b.SetBytes(int64(buf.Len()))
	})

	var saved bytes.Buffer
	_ = h.Save(&saved)
	data := saved.Bytes()

	b.Run("Load", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = LoadHNSW(bytes.NewReader(data))
		}
	})
}

// ---------------------------------------------------------------------------
// OpenHNSW (persistent file-backed index)
// ---------------------------------------------------------------------------

func TestOpenHNSWCreateNew(t *testing.T) {
	fs := newTestDirFS(t.TempDir())
	idx, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW: %v", err)
	}
	defer idx.Close()

	if idx.Len() != 0 {
		t.Fatalf("Len = %d, want 0", idx.Len())
	}
	if idx.Name() != "test.hnsw" {
		t.Fatalf("Name = %q, want %q", idx.Name(), "test.hnsw")
	}
}

func TestOpenHNSWInsertFlushReopen(t *testing.T) {
	fs := newTestDirFS(t.TempDir())
	idx, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW: %v", err)
	}

	if err := idx.Insert("a", []float32{1, 0, 0}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := idx.Insert("b", []float32{0, 1, 0}); err != nil {
		t.Fatalf("Insert b: %v", err)
	}

	matches, err := idx.Search([]float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 || matches[0].ID != "a" {
		t.Fatalf("Search mismatch: %+v", matches)
	}

	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and verify.
	idx2, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW reopen: %v", err)
	}
	defer idx2.Close()

	if idx2.Len() != 2 {
		t.Fatalf("Len after reopen = %d, want 2", idx2.Len())
	}
	matches2, err := idx2.Search([]float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
	if len(matches2) != 1 || matches2[0].ID != "a" {
		t.Fatalf("Search after reopen mismatch: %+v", matches2)
	}
}

func TestOpenHNSWDeleteMarksDirty(t *testing.T) {
	fs := newTestDirFS(t.TempDir())
	idx, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW: %v", err)
	}

	if err := idx.Insert("a", []float32{1, 0, 0}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := idx.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := idx.Delete("a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	idx2, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW reopen: %v", err)
	}
	defer idx2.Close()

	if idx2.Len() != 0 {
		t.Fatalf("Len after reopen = %d, want 0 (delete should have been persisted)", idx2.Len())
	}
}

func TestOpenHNSWBatchInsert(t *testing.T) {
	fs := newTestDirFS(t.TempDir())
	idx, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW: %v", err)
	}

	if err := idx.BatchInsert(
		[]string{"a", "b"},
		[][]float32{{1, 0, 0}, {0, 1, 0}},
	); err != nil {
		t.Fatalf("BatchInsert: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	idx2, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW reopen: %v", err)
	}
	defer idx2.Close()
	if idx2.Len() != 2 {
		t.Fatalf("Len after reopen = %d, want 2", idx2.Len())
	}
}

func TestOpenHNSWDimMismatch(t *testing.T) {
	fs := newTestDirFS(t.TempDir())
	idx, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW: %v", err)
	}
	if err := idx.Insert("x", []float32{1, 0, 0}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 4}); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

func TestOpenHNSWRemove(t *testing.T) {
	dir := t.TempDir()
	fs := newTestDirFS(dir)
	idx, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW: %v", err)
	}
	if err := idx.Insert("x", []float32{1, 0, 0}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := idx.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	path := filepath.Join(dir, "test.hnsw")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}

	if err := idx.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed, got err=%v", err)
	}
}

func TestOpenHNSWFlushNoopWhenClean(t *testing.T) {
	dir := t.TempDir()
	fs := newTestDirFS(dir)
	idx, err := OpenHNSW(fs, "test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW: %v", err)
	}
	defer idx.Close()

	if err := idx.Flush(); err != nil {
		t.Fatalf("Flush clean: %v", err)
	}
	path := filepath.Join(dir, "test.hnsw")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("no file should be created for clean flush")
	}
}

func TestOpenHNSWNestedDir(t *testing.T) {
	dir := t.TempDir()
	fs := newTestDirFS(dir)
	idx, err := OpenHNSW(fs, "a/b/c/test.hnsw", HNSWConfig{Dim: 3})
	if err != nil {
		t.Fatalf("OpenHNSW nested: %v", err)
	}
	if err := idx.Insert("a", []float32{1, 0, 0}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	path := filepath.Join(dir, "a", "b", "c", "test.hnsw")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}

func TestNewHNSWNameIsEmpty(t *testing.T) {
	h := NewHNSW(HNSWConfig{Dim: 3})
	if h.Name() != "" {
		t.Fatalf("NewHNSW should have empty name, got %q", h.Name())
	}
	if err := h.Flush(); err != nil {
		t.Fatalf("Flush in-memory: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close in-memory: %v", err)
	}
	if err := h.Remove(); err != nil {
		t.Fatalf("Remove in-memory: %v", err)
	}
}
