package kv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
)

const badgerCollectionMeta byte = 1
const badgerOrderedCollectionMeta byte = 2

// Record and collection-member namespaces are physically disjoint.
func (b *Badger) recordKey(key Key) []byte {
	return append([]byte{1}, b.opts.encode(key)...)
}
func (b *Badger) memberPrefix(key Key) []byte {
	encoded := b.opts.encode(key)
	prefix := make([]byte, 9, 9+len(encoded))
	binary.BigEndian.PutUint64(prefix[1:], uint64(len(encoded)))
	return append(prefix, encoded...)
}

func (b *Badger) deleteCollection(tx *badger.Txn, key []byte) error {
	item, err := tx.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if item.UserMeta() != badgerCollectionMeta && item.UserMeta() != badgerOrderedCollectionMeta {
		return nil
	}
	prefix := b.memberPrefix(b.opts.decode(key[1:]))
	options := badger.DefaultIteratorOptions
	options.Prefix = prefix
	options.PrefetchValues = false
	it := tx.NewIterator(options)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		if err := tx.Delete(it.Item().KeyCopy(nil)); err != nil {
			return err
		}
	}
	return nil
}
func (b *Badger) deleteRecord(tx *badger.Txn, key []byte) error {
	if err := b.deleteCollection(tx, key); err != nil {
		return err
	}
	return tx.Delete(key)
}
func (b *Badger) putRecord(tx *badger.Txn, entry *badger.Entry) error {
	if err := b.deleteCollection(tx, entry.Key); err != nil {
		return err
	}
	return tx.SetEntry(entry)
}

// AddMembers adds members idempotently to one collection.
func (b *Badger) AddMembers(ctx context.Context, key Key, members ...string) error {
	_, err := b.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// RemoveMembers removes members idempotently from one collection.
func (b *Badger) RemoveMembers(ctx context.Context, key Key, members ...string) error {
	_, err := b.ApplyMutation(ctx, Mutation{RemoveMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// HasMember performs an exact member-key lookup.
func (b *Badger) HasMember(ctx context.Context, key Key, member string) (bool, error) {
	release, err := b.acquire()
	if err != nil {
		return false, err
	}
	defer release()
	found := false
	err = b.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := tx.Get(b.recordKey(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if item.UserMeta() != badgerCollectionMeta {
			return ErrWrongType
		}
		_, err = tx.Get(append(b.memberPrefix(key), member...))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		found = err == nil
		return err
	})
	return found, err
}

// ListMembers enumerates only the member-key range of one exact collection.
func (b *Badger) ListMembers(ctx context.Context, key Key) ([]string, error) {
	release, err := b.acquire()
	if err != nil {
		return nil, err
	}
	defer release()
	var result []string
	err = b.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := tx.Get(b.recordKey(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if item.UserMeta() != badgerCollectionMeta {
			return ErrWrongType
		}
		prefix := b.memberPrefix(key)
		options := badger.DefaultIteratorOptions
		options.Prefix = prefix
		options.PrefetchValues = false
		it := tx.NewIterator(options)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			result = append(result, string(it.Item().Key()[len(prefix):]))
		}
		return nil
	})
	return result, err
}

// ApplyMutation updates record and collection keys in one optimistic transaction.
func (b *Badger) ApplyMutation(ctx context.Context, mutation Mutation) (bool, error) {
	release, err := b.acquire()
	if err != nil {
		return false, err
	}
	defer release()
	if err := mutation.validate(ctx, b.opts); err != nil {
		return false, err
	}
	for {
		matched := false
		err := b.db.Update(func(tx *badger.Txn) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, c := range mutation.Conditions {
				item, err := tx.Get(b.recordKey(c.Key))
				if errors.Is(err, badger.ErrKeyNotFound) {
					if c.Expected != nil {
						return nil
					}
					continue
				}
				if err != nil {
					return err
				}
				if c.Expected == nil {
					return nil
				}
				if item.UserMeta() != 0 {
					return ErrWrongType
				}
				value, err := item.ValueCopy(nil)
				if err != nil {
					return err
				}
				if !bytes.Equal(value, c.Expected) {
					return nil
				}
			}
			for _, entry := range mutation.Entries {
				item := badger.NewEntry(b.recordKey(entry.Key), entry.Value)
				if !entry.Deadline.IsZero() {
					ttl := time.Until(entry.Deadline)
					if ttl <= 0 {
						return ErrInvalidDeadline
					}
					item = item.WithTTL(ttl)
				}
				if err := b.putRecord(tx, item); err != nil {
					return err
				}
			}
			for _, key := range mutation.DeleteKeys {
				if err := b.deleteRecord(tx, b.recordKey(key)); err != nil {
					return err
				}
			}
			for phase, groups := range [][]SetMembers{mutation.AddMembers, mutation.RemoveMembers, mutation.AddOrderedMembers, mutation.RemoveOrderedMembers} {
				expectedMeta := byte(1 + phase/2)
				for _, group := range groups {
					if len(group.Members) == 0 {
						continue
					}
					key := b.recordKey(group.Key)
					item, err := tx.Get(key)
					count := uint64(0)
					if err == nil {
						if item.UserMeta() != expectedMeta {
							return ErrWrongType
						}
						raw, err := item.ValueCopy(nil)
						if err != nil {
							return err
						}
						if len(raw) != 8 {
							return errors.New("kv: invalid collection counter")
						}
						count = binary.BigEndian.Uint64(raw)
					} else if !errors.Is(err, badger.ErrKeyNotFound) {
						return err
					}
					prefix := b.memberPrefix(group.Key)
					for _, member := range group.Members {
						memberKey := append(bytes.Clone(prefix), member...)
						_, err := tx.Get(memberKey)
						exists := err == nil
						if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
							return err
						}
						if phase%2 == 0 && !exists {
							if err := tx.Set(memberKey, nil); err != nil {
								return err
							}
							count++
						}
						if phase%2 == 1 && exists {
							if err := tx.Delete(memberKey); err != nil {
								return err
							}
							count--
						}
					}
					if count == 0 {
						if err := tx.Delete(key); err != nil {
							return err
						}
					} else {
						raw := binary.BigEndian.AppendUint64(nil, count)
						if err := tx.SetEntry(badger.NewEntry(key, raw).WithMeta(expectedMeta)); err != nil {
							return err
						}
					}
				}
			}
			matched = true
			return ctx.Err()
		})
		if errors.Is(err, badger.ErrConflict) && ctx.Err() == nil {
			continue
		}
		return matched && err == nil, err
	}
}
