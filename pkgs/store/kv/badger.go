package kv

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Badger is a persistent Store backed by BadgerDB.
type Badger struct {
	db     *badger.DB
	opts   *Options
	ownsDB bool
}

// NewBadger opens (or creates) a BadgerDB store at dir.
// Pass nil opts for defaults.
func NewBadger(dir string, opts *Options) (*Badger, error) {
	dbOpts := badger.DefaultOptions(dir).
		WithLogger(nil)
	db, err := badger.Open(dbOpts)
	if err != nil {
		return nil, err
	}
	return &Badger{db: db, opts: opts, ownsDB: true}, nil
}

// NewBadgerWithDB creates a Store that borrows an already-open Badger DB.
// Closing the returned Store does not close db.
func NewBadgerWithDB(db *badger.DB, opts *Options) (*Badger, error) {
	if db == nil {
		return nil, errors.New("kv: badger db is nil")
	}
	return &Badger{db: db, opts: opts}, nil
}

func (b *Badger) Get(ctx context.Context, key Key) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var val []byte
	err := b.db.View(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := txn.Get(b.opts.encode(key))
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		val, err = item.ValueCopy(nil)
		return err
	})
	if err == badger.ErrKeyNotFound {
		return nil, ErrNotFound
	}
	return val, err
}

func (b *Badger) Set(ctx context.Context, key Key, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return txn.Set(b.opts.encode(key), value)
	})
}

func (b *Badger) Delete(ctx context.Context, key Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := b.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return txn.Delete(b.opts.encode(key))
	})
	if err == badger.ErrKeyNotFound {
		return nil
	}
	return err
}

func (b *Badger) List(ctx context.Context, prefix Key) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(Entry{}, err)
			return
		}
		err := b.db.View(func(txn *badger.Txn) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			iterOpts := badger.DefaultIteratorOptions
			iterOpts.Prefix = b.listPrefix(prefix)
			it := txn.NewIterator(iterOpts)
			defer it.Close()

			for it.Seek(iterOpts.Prefix); it.Valid(); it.Next() {
				if err := ctx.Err(); err != nil {
					return err
				}
				item := it.Item()
				val, err := item.ValueCopy(nil)
				if err != nil {
					if !yield(Entry{}, err) {
						return nil
					}
					continue
				}
				entry := Entry{
					Key:   b.opts.decode(item.KeyCopy(nil)),
					Value: val,
				}
				if !yield(entry, nil) {
					return nil
				}
			}
			return nil
		})
		if err != nil {
			yield(Entry{}, err)
		}
	}
}

func (b *Badger) ListAfter(ctx context.Context, prefix, after Key, limit int) ([]Entry, error) {
	if limit <= 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, limit)
	prefixBytes := b.listPrefix(prefix)

	var afterBytes []byte
	if len(after) > 0 {
		afterBytes = b.opts.encode(after)
	}

	err := b.db.View(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		iterOpts := badger.DefaultIteratorOptions
		iterOpts.Prefix = prefixBytes
		it := txn.NewIterator(iterOpts)
		defer it.Close()

		switch {
		case len(afterBytes) > 0 && len(prefixBytes) > 0 && bytes.Compare(afterBytes, prefixBytes) < 0:
			it.Seek(prefixBytes)
		case len(afterBytes) > 0:
			it.Seek(afterBytes)
		case len(prefixBytes) > 0:
			it.Seek(prefixBytes)
		default:
			it.Rewind()
		}

		for ; it.Valid(); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(prefixBytes) > 0 && !it.ValidForPrefix(prefixBytes) {
				break
			}

			item := it.Item()
			key := item.KeyCopy(nil)
			if len(afterBytes) > 0 && bytes.Equal(key, afterBytes) {
				continue
			}

			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			entries = append(entries, Entry{
				Key:   b.opts.decode(key),
				Value: val,
			})
			if len(entries) >= limit {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (b *Badger) BatchSet(ctx context.Context, entries []Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.db.Update(func(txn *badger.Txn) error {
		now := time.Now()
		for _, e := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			entry := badger.NewEntry(b.opts.encode(e.Key), e.Value)
			if !e.Deadline.IsZero() {
				ttl := e.Deadline.Sub(now)
				if ttl <= 0 {
					return ErrInvalidDeadline
				}
				entry = entry.WithTTL(ttl)
			}
			if err := txn.SetEntry(entry); err != nil {
				return err
			}
		}
		return nil
	})
}

func (b *Badger) BatchDelete(ctx context.Context, keys []Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.db.Update(func(txn *badger.Txn) error {
		for _, k := range keys {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := txn.Delete(b.opts.encode(k)); err != nil && err != badger.ErrKeyNotFound {
				return err
			}
		}
		return nil
	})
}

func (b *Badger) BatchMutate(ctx context.Context, entries []Entry, keys []Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.db.Update(func(txn *badger.Txn) error {
		now := time.Now()
		for _, item := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			entry := badger.NewEntry(b.opts.encode(item.Key), item.Value)
			if !item.Deadline.IsZero() {
				ttl := item.Deadline.Sub(now)
				if ttl <= 0 {
					return ErrInvalidDeadline
				}
				entry = entry.WithTTL(ttl)
			}
			if err := txn.SetEntry(entry); err != nil {
				return err
			}
		}
		for _, key := range keys {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := txn.Delete(b.opts.encode(key)); err != nil && err != badger.ErrKeyNotFound {
				return err
			}
		}
		return nil
	})
}

func (b *Badger) CreateIfAbsent(ctx context.Context, guard Entry, entries []Entry) ([]byte, bool, error) {
	_, existing, created, err := b.CreateIfAllAbsent(ctx, []Entry{guard}, entries)
	return existing, created, err
}

func (b *Badger) CreateIfAllAbsent(ctx context.Context, guards []Entry, entries []Entry) (Key, []byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, false, err
	}
	if len(guards) == 0 {
		return nil, nil, false, errors.New("kv: create-if-all-absent requires at least one guard")
	}
	for {
		var conflict Key
		var existing []byte
		created := false
		err := b.db.Update(func(txn *badger.Txn) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, guard := range guards {
				item, err := txn.Get(b.opts.encode(guard.Key))
				if err == nil {
					conflict = cloneKey(guard.Key)
					existing, err = item.ValueCopy(nil)
					return err
				}
				if !errors.Is(err, badger.ErrKeyNotFound) {
					return err
				}
			}
			now := time.Now()
			all := make([]Entry, 0, len(entries)+len(guards))
			all = append(all, entries...)
			all = append(all, guards...)
			for _, entry := range all {
				item := badger.NewEntry(b.opts.encode(entry.Key), entry.Value)
				if !entry.Deadline.IsZero() {
					ttl := entry.Deadline.Sub(now)
					if ttl <= 0 {
						return ErrInvalidDeadline
					}
					item = item.WithTTL(ttl)
				}
				if err := txn.SetEntry(item); err != nil {
					return err
				}
			}
			created = true
			return nil
		})
		if errors.Is(err, badger.ErrConflict) && ctx.Err() == nil {
			continue
		}
		return conflict, existing, created, err
	}
}

func (b *Badger) CompareAndMutate(
	ctx context.Context,
	guard Key,
	expected []byte,
	entries []Entry,
	keys []Key,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	guardKey := b.opts.encode(guard)
	for {
		matched := false
		err := b.db.Update(func(txn *badger.Txn) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			item, err := txn.Get(guardKey)
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			current, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			if !bytes.Equal(current, expected) {
				return nil
			}
			now := time.Now()
			for _, entry := range entries {
				item := badger.NewEntry(b.opts.encode(entry.Key), entry.Value)
				if !entry.Deadline.IsZero() {
					ttl := entry.Deadline.Sub(now)
					if ttl <= 0 {
						return ErrInvalidDeadline
					}
					item = item.WithTTL(ttl)
				}
				if err := txn.SetEntry(item); err != nil {
					return err
				}
			}
			for _, key := range keys {
				if err := txn.Delete(b.opts.encode(key)); err != nil &&
					!errors.Is(err, badger.ErrKeyNotFound) {
					return err
				}
			}
			matched = true
			return nil
		})
		if errors.Is(err, badger.ErrConflict) && ctx.Err() == nil {
			continue
		}
		return matched, err
	}
}

func (b *Badger) Close() error {
	if b == nil || !b.ownsDB || b.db == nil {
		return nil
	}
	b.ownsDB = false
	return b.db.Close()
}

// listPrefix returns the byte prefix for List iteration.
// For a non-empty prefix key it appends the separator so "a:b" doesn't match "a:bc".
func (b *Badger) listPrefix(prefix Key) []byte {
	p := b.opts.encode(prefix)
	if len(p) == 0 {
		return nil
	}
	return append(p, b.opts.sep())
}

// compile-time interface check
var _ Store = (*Badger)(nil)

// OpenBadger is an alias for NewBadger kept for discoverability.
// Deprecated: use NewBadger.
var OpenBadger = NewBadger

// NewBadgerWithOptions opens a BadgerDB store with custom badger.Options.
// This is useful for advanced tuning (e.g. in-memory mode, compression settings).
func NewBadgerWithOptions(dbOpts badger.Options, opts *Options) (*Badger, error) {
	dbOpts = dbOpts.WithLogger(nil)
	db, err := badger.Open(dbOpts)
	if err != nil {
		return nil, err
	}
	return &Badger{db: db, opts: opts, ownsDB: true}, nil
}

// NewBadgerInMemory creates an in-memory BadgerDB store (no disk persistence).
// Useful for integration tests that need a real Badger instance without temp dirs.
func NewBadgerInMemory(opts *Options) (*Badger, error) {
	dbOpts := badger.DefaultOptions("").
		WithInMemory(true).
		WithLogger(nil)
	return NewBadgerWithOptions(dbOpts, opts)
}

// RunGC triggers BadgerDB's value log garbage collection.
// discardRatio is the minimum fraction of entries that must be discarded
// for a rewrite to happen (typically 0.5).
// Returns nil if GC ran, badger.ErrNoRewrite if nothing to collect.
func (b *Badger) RunGC(discardRatio float64) error {
	return b.db.RunValueLogGC(discardRatio)
}

// Size returns the on-disk size in bytes as reported by BadgerDB.
func (b *Badger) Size() (lsm, vlog int64) {
	return b.db.Size()
}
