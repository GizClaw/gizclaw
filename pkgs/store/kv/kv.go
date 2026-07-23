// Package kv provides a key-value store interface with hierarchical path-based
// keys. Keys are represented as string slices (e.g., ["user", "profile", "123"])
// and encoded internally using a configurable separator (default ':').
//
// The package uses BadgerDB for both persistent storage and in-memory testing.
package kv

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"strings"
	"time"
)

// Sentinel errors.
var (
	// ErrNotFound is returned when a key does not exist in the store.
	ErrNotFound = errors.New("kv: not found")
	// ErrInvalidDeadline is returned when an entry deadline is already expired.
	ErrInvalidDeadline = errors.New("kv: invalid deadline")
	// ErrCreateIfAbsentUnsupported is returned when a Store does not implement
	// the optional atomic conditional-create capability.
	ErrCreateIfAbsentUnsupported = errors.New("kv: atomic create-if-absent unsupported")
)

// Key is a hierarchical path represented as a slice of string segments.
// For example, Key{"user", "g", "e", "Alice"} encodes to "user:g:e:Alice"
// using the default separator ':'.
//
// Segments must not contain the configured separator character.
type Key []string

// String returns the key as a human-readable string using ':' as separator.
// This is for display/debug only; use Options.encode for storage encoding.
func (k Key) String() string {
	return strings.Join(k, ":")
}

// Entry is a key-value pair returned by List and used by BatchSet.
type Entry struct {
	Key   Key
	Value []byte
	// Deadline sets an absolute expiration time for writes. A zero Deadline means the entry does not expire.
	Deadline time.Time
}

// Store is the interface for a key-value store with path-based keys.
type Store interface {
	// Get retrieves the value for a key. Returns ErrNotFound if not present.
	Get(ctx context.Context, key Key) ([]byte, error)

	// Set stores a key-value pair. Overwrites any existing value.
	Set(ctx context.Context, key Key, value []byte) error

	// Delete removes a key. No error if the key does not exist.
	Delete(ctx context.Context, key Key) error

	// List iterates over entries under the given prefix subtree, i.e.
	// keys that start with "prefix + separator". The key exactly equal to
	// prefix is not included. The iteration order is lexicographic by
	// encoded key.
	List(ctx context.Context, prefix Key) iter.Seq2[Entry, error]

	// BatchSet atomically stores multiple key-value pairs.
	BatchSet(ctx context.Context, entries []Entry) error

	// BatchDelete atomically removes multiple keys.
	BatchDelete(ctx context.Context, keys []Key) error

	// BatchMutate atomically stores entries and removes keys in one transaction.
	// If a key appears in both collections, the delete is applied last.
	BatchMutate(ctx context.Context, entries []Entry, keys []Key) error

	// Close releases any resources held by the store.
	Close() error
}

type createIfAbsentStore interface {
	CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) (existing []byte, created bool, err error)
}

// SupportsCreateIfAbsent reports whether store provides the atomic
// conditional-create capability. Prefixed stores inherit support from their
// underlying store.
func SupportsCreateIfAbsent(store Store) bool {
	if prefixed, ok := store.(*prefixedStore); ok {
		return SupportsCreateIfAbsent(prefixed.base)
	}
	_, ok := store.(createIfAbsentStore)
	return ok
}

// CreateIfAbsent atomically stores guard and entries through the Store's
// optional conditional-create capability. When guard.Key already exists, it
// returns its stored value and leaves every key unchanged. The boolean reports
// whether this call created the guard and entries. If entries contains
// guard.Key, guard wins.
func CreateIfAbsent(ctx context.Context, store Store, guard Entry, entries []Entry) (existing []byte, created bool, err error) {
	conditional, ok := store.(createIfAbsentStore)
	if !ok {
		return nil, false, ErrCreateIfAbsentUnsupported
	}
	return conditional.CreateIfAbsent(ctx, guard, entries)
}

type listAfterStore interface {
	ListAfter(ctx context.Context, prefix, after Key, limit int) ([]Entry, error)
}

// DefaultSeparator is the default separator byte used to encode key segments.
const DefaultSeparator byte = ':'

// Options configures store behavior.
type Options struct {
	// Separator is the byte used to join key segments when encoding to storage.
	// Default is ':' if zero.
	Separator byte
}

// sep returns the effective separator.
func (o *Options) sep() byte {
	if o != nil && o.Separator != 0 {
		return o.Separator
	}
	return DefaultSeparator
}

// encode converts a Key to its byte representation using the separator.
// It panics if any segment contains the separator character, which indicates
// a programming error (keys must be constructed with clean segments).
func (o *Options) encode(k Key) []byte {
	s := o.sep()
	// Validate segments do not contain the separator.
	for _, seg := range k {
		if strings.IndexByte(seg, s) >= 0 {
			panic("kv: key segment " + seg + " contains separator '" + string(s) + "'")
		}
	}
	// Calculate total length to avoid allocations.
	n := 0
	for i, seg := range k {
		if i > 0 {
			n++ // separator
		}
		n += len(seg)
	}
	buf := make([]byte, n)
	pos := 0
	for i, seg := range k {
		if i > 0 {
			buf[pos] = s
			pos++
		}
		pos += copy(buf[pos:], seg)
	}
	return buf
}

// decode converts a byte representation back to a Key using the separator.
func (o *Options) decode(b []byte) Key {
	s := o.sep()
	parts := bytes.Split(b, []byte{s})
	k := make(Key, len(parts))
	for i, p := range parts {
		k[i] = string(p)
	}
	return k
}

// ListAfter returns up to limit entries under the prefix subtree, strictly
// after the provided key. Pass nil for after to start from the beginning of
// the prefix. Stores with a native paging implementation are used directly;
// older Store implementations fall back to in-process filtering over List.
func ListAfter(ctx context.Context, store Store, prefix, after Key, limit int) ([]Entry, error) {
	if limit <= 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if pager, ok := store.(listAfterStore); ok {
		return pager.ListAfter(ctx, prefix, after, limit)
	}

	entries := make([]Entry, 0, limit)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		if len(after) > 0 && strings.Compare(entry.Key.String(), after.String()) <= 0 {
			continue
		}
		entries = append(entries, entry)
		if len(entries) >= limit {
			break
		}
	}
	return entries, nil
}
