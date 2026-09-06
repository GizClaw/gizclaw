package kv

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/dgraph-io/badger/v4"
	redis "github.com/redis/go-redis/v9"
)

// OrderedRange selects members strictly after After and strictly before Before,
// in bytewise ascending order. Nil bounds are unbounded. Limit must be positive.
// Ordered collections are distinct from ordinary Set and value keys, never
// expire, and are mutated through AddOrderedMembers and RemoveOrderedMembers.
type OrderedRange struct {
	After  *string
	Before *string
	Limit  int
}

func (query OrderedRange) validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if query.Limit <= 0 {
		return errors.New("kv: ordered range limit must be positive")
	}
	return nil
}

// RangeOrderedMembers selects a bounded range from one exact ordered collection.
func (m *Memory) RangeOrderedMembers(ctx context.Context, key Key, query OrderedRange) ([]string, error) {
	if err := query.validate(ctx); err != nil {
		return nil, err
	}
	encoded := string(m.opts.encode(key))
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, exists := m.data[encoded]
	if !exists || entry.expired(time.Now()) {
		return nil, nil
	}
	if entry.members == nil || !entry.ordered {
		return nil, ErrWrongType
	}
	members := make([]string, 0)
	for member := range entry.members {
		if query.After != nil && member <= *query.After {
			continue
		}
		if query.Before != nil && member >= *query.Before {
			continue
		}
		members = append(members, member)
	}
	slices.Sort(members)
	return members[:min(len(members), query.Limit)], nil
}

// RangeOrderedMembers uses Redis's lexicographic Sorted Set index. All members
// have score zero; range bounds and the limit are evaluated by Redis.
func (r *Redis) RangeOrderedMembers(ctx context.Context, key Key, query OrderedRange) ([]string, error) {
	if err := query.validate(ctx); err != nil {
		return nil, err
	}
	min, max := "-", "+"
	if query.After != nil {
		min = "(" + *query.After
	}
	if query.Before != nil {
		max = "(" + *query.Before
	}
	members, err := r.client.ZRangeByLex(ctx, string(r.opts.encode(key)), &redis.ZRangeBy{Min: min, Max: max, Count: int64(query.Limit)}).Result()
	return members, setRedisError(err)
}

// RangeOrderedMembers seeks directly into the exact collection's member keys.
func (b *Badger) RangeOrderedMembers(ctx context.Context, key Key, query OrderedRange) ([]string, error) {
	if err := query.validate(ctx); err != nil {
		return nil, err
	}
	release, err := b.acquire()
	if err != nil {
		return nil, err
	}
	defer release()
	var members []string
	err = b.db.View(func(tx *badger.Txn) error {
		item, err := tx.Get(b.recordKey(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if item.UserMeta() != badgerOrderedCollectionMeta {
			return ErrWrongType
		}
		prefix := b.memberPrefix(key)
		options := badger.DefaultIteratorOptions
		options.Prefix, options.PrefetchValues = prefix, false
		it := tx.NewIterator(options)
		defer it.Close()
		start := bytes.Clone(prefix)
		if query.After != nil {
			start = append(start, *query.After...)
		}
		for it.Seek(start); it.ValidForPrefix(prefix); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			member := string(it.Item().Key()[len(prefix):])
			if query.After != nil && member <= *query.After {
				continue
			}
			if query.Before != nil && member >= *query.Before {
				break
			}
			members = append(members, member)
			if len(members) == query.Limit {
				break
			}
		}
		return nil
	})
	return members, err
}

// RangeOrderedMembers pushes range predicates and LIMIT into the member index.
func (s *SQL) RangeOrderedMembers(ctx context.Context, key Key, query OrderedRange) (members []string, err error) {
	defer func() { err = sqlCollectionError(err) }()
	if err := query.validate(ctx); err != nil {
		return nil, err
	}
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	statement := "SELECT k.value_kind, m.member IS NOT NULL, m.member FROM " + s.quoted + " k LEFT JOIN " + s.members + " m ON m.encoded_key=k.encoded_key"
	var args []any
	if query.After != nil {
		statement += " AND m.member > ?"
		args = append(args, []byte(*query.After))
	}
	if query.Before != nil {
		statement += " AND m.member < ?"
		args = append(args, []byte(*query.Before))
	}
	statement += " WHERE k.encoded_key=? AND (k.expires_at_unix_nano IS NULL OR k.expires_at_unix_nano>?) ORDER BY m.member LIMIT ?"
	args = append(args, s.opts.encode(key), time.Now().UnixNano(), query.Limit)
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(statement), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind int
		var member []byte
		var present bool
		if err := rows.Scan(&kind, &present, &member); err != nil {
			return nil, err
		}
		if kind != 2 {
			return nil, ErrWrongType
		}
		if present {
			members = append(members, string(member))
		}
	}
	return members, rows.Err()
}

func (s *prefixedStore) RangeOrderedMembers(ctx context.Context, key Key, query OrderedRange) ([]string, error) {
	return s.base.RangeOrderedMembers(ctx, s.prefixedKey(key), query)
}
