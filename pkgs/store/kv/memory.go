package kv

import (
	"bytes"
	"context"
	"iter"
	"sort"
	"sync"
	"time"
)

type memoryEntry struct {
	value     []byte
	expiresAt time.Time
}

// Memory is an in-memory Store implementation backed by a sorted map.
// It is safe for concurrent use and intended primarily for testing.
type Memory struct {
	mu   sync.RWMutex
	data map[string]memoryEntry
	opts *Options
}

// NewMemory creates a new in-memory Store.
// Pass nil for default options.
func NewMemory(opts *Options) *Memory {
	return &Memory{
		data: make(map[string]memoryEntry),
		opts: opts,
	}
}

func (m *Memory) Get(ctx context.Context, key Key) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	k := string(m.opts.encode(key))
	m.mu.RLock()
	entry, ok := m.data[k]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if entry.expired(time.Now()) {
		m.mu.Lock()
		if current, ok := m.data[k]; ok && current.expired(time.Now()) {
			delete(m.data, k)
		}
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	cp := make([]byte, len(entry.value))
	copy(cp, entry.value)
	return cp, nil
}

func (m *Memory) Set(ctx context.Context, key Key, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	k := string(m.opts.encode(key))
	cp := make([]byte, len(value))
	copy(cp, value)
	m.mu.Lock()
	m.data[k] = memoryEntry{value: cp}
	m.mu.Unlock()
	return nil
}

func (m *Memory) Delete(ctx context.Context, key Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	k := string(m.opts.encode(key))
	m.mu.Lock()
	delete(m.data, k)
	m.mu.Unlock()
	return nil
}

func (m *Memory) List(ctx context.Context, prefix Key) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(Entry{}, err)
			return
		}

		p := m.opts.encode(prefix)
		var prefixBytes []byte
		if len(p) > 0 {
			prefixBytes = append(p, m.opts.sep())
		}

		m.mu.RLock()
		type pair struct {
			key string
			val []byte
		}
		var matches []pair
		now := time.Now()
		for k, entry := range m.data {
			if entry.expired(now) {
				continue
			}
			if len(prefixBytes) == 0 || bytes.HasPrefix([]byte(k), prefixBytes) {
				cp := make([]byte, len(entry.value))
				copy(cp, entry.value)
				matches = append(matches, pair{key: k, val: cp})
			}
		}
		m.mu.RUnlock()

		sort.Slice(matches, func(i, j int) bool {
			return matches[i].key < matches[j].key
		})

		for _, match := range matches {
			if err := ctx.Err(); err != nil {
				yield(Entry{}, err)
				return
			}
			entry := Entry{
				Key:   m.opts.decode([]byte(match.key)),
				Value: match.val,
			}
			if !yield(entry, nil) {
				return
			}
		}
	}
}

// ListAfter returns up to limit entries under the prefix subtree, strictly
// after the provided key.
func (m *Memory) ListAfter(ctx context.Context, prefix, after Key, limit int) ([]Entry, error) {
	if limit <= 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, limit)
	for entry, err := range m.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		if len(after) > 0 && entry.Key.String() <= after.String() {
			continue
		}
		entries = append(entries, entry)
		if len(entries) >= limit {
			break
		}
	}
	return entries, nil
}

func (m *Memory) BatchSet(ctx context.Context, entries []Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	type preparedEntry struct {
		key   string
		entry memoryEntry
	}
	prepared := make([]preparedEntry, 0, len(entries))
	for _, e := range entries {
		if !e.Deadline.IsZero() && !e.Deadline.After(now) {
			return ErrInvalidDeadline
		}
		k := string(m.opts.encode(e.Key))
		cp := make([]byte, len(e.Value))
		copy(cp, e.Value)
		entry := memoryEntry{value: cp}
		entry.expiresAt = e.Deadline
		prepared = append(prepared, preparedEntry{key: k, entry: entry})
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range prepared {
		m.data[e.key] = e.entry
	}
	return nil
}

func (m *Memory) BatchDelete(ctx context.Context, keys []Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		k := string(m.opts.encode(key))
		delete(m.data, k)
	}
	return nil
}

func (m *Memory) BatchMutate(ctx context.Context, entries []Entry, keys []Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	type preparedEntry struct {
		key   string
		entry memoryEntry
	}
	prepared := make([]preparedEntry, 0, len(entries))
	for _, item := range entries {
		if !item.Deadline.IsZero() && !item.Deadline.After(now) {
			return ErrInvalidDeadline
		}
		value := append([]byte(nil), item.Value...)
		prepared = append(prepared, preparedEntry{
			key:   string(m.opts.encode(item.Key)),
			entry: memoryEntry{value: value, expiresAt: item.Deadline},
		})
	}
	encodedDeletes := make([]string, len(keys))
	for i, key := range keys {
		encodedDeletes[i] = string(m.opts.encode(key))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range prepared {
		m.data[item.key] = item.entry
	}
	for _, key := range encodedDeletes {
		delete(m.data, key)
	}
	return nil
}

func (m *Memory) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	now := time.Now()
	if !guard.Deadline.IsZero() && !guard.Deadline.After(now) {
		return nil, false, ErrInvalidDeadline
	}
	type preparedEntry struct {
		key   string
		entry memoryEntry
	}
	prepared := make([]preparedEntry, 0, len(entries))
	for _, item := range entries {
		if !item.Deadline.IsZero() && !item.Deadline.After(now) {
			return nil, false, ErrInvalidDeadline
		}
		prepared = append(prepared, preparedEntry{
			key:   string(m.opts.encode(item.Key)),
			entry: memoryEntry{value: append([]byte(nil), item.Value...), expiresAt: item.Deadline},
		})
	}
	guardKey := string(m.opts.encode(guard.Key))
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.data[guardKey]; ok && !current.expired(now) {
		return append([]byte(nil), current.value...), false, nil
	}
	m.data[guardKey] = memoryEntry{value: append([]byte(nil), guard.Value...), expiresAt: guard.Deadline}
	for _, item := range prepared {
		m.data[item.key] = item.entry
	}
	return nil, true, nil
}

func (m *Memory) Close() error {
	return nil
}

func (e memoryEntry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

var _ Store = (*Memory)(nil)
