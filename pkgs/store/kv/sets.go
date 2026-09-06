package kv

import (
	"context"
	"errors"
	"time"
)

// ErrWrongType reports an operation on a key holding another data type.
var ErrWrongType = errors.New("kv: wrong value type")

// SetMembers addresses members of one exact collection. Members are opaque
// strings, not hierarchical keys. Collection enumeration has no ordering guarantee.
type SetMembers struct {
	Key     Key
	Members []string
}

// Condition compares a record before applying a mutation. A nil Expected
// requires absence; a non-nil Expected requires an exact byte match.
type Condition struct {
	Key      Key
	Expected []byte
}

// Mutation atomically updates records and collection indexes. Conditions are
// checked against the initial state. Records are written, keys deleted, members
// added, then members removed. Record and collection keys must be distinct.
// Collections do not expire; removing their last member removes the key.
type Mutation struct {
	Conditions           []Condition
	Entries              []Entry
	DeleteKeys           []Key
	AddMembers           []SetMembers
	RemoveMembers        []SetMembers
	AddOrderedMembers    []SetMembers
	RemoveOrderedMembers []SetMembers
}

func (m Mutation) validate(ctx context.Context, opts *Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	records := make(map[string]bool)
	for _, condition := range m.Conditions {
		records[string(opts.encode(condition.Key))] = true
	}
	for _, entry := range m.Entries {
		if !entry.Deadline.IsZero() && !entry.Deadline.After(now) {
			return ErrInvalidDeadline
		}
		records[string(opts.encode(entry.Key))] = true
	}
	for _, key := range m.DeleteKeys {
		records[string(opts.encode(key))] = true
	}
	collectionKinds := make(map[string]bool)
	for phase, groups := range [][]SetMembers{m.AddMembers, m.RemoveMembers, m.AddOrderedMembers, m.RemoveOrderedMembers} {
		for _, group := range groups {
			key := string(opts.encode(group.Key))
			ordered := phase >= 2
			if previous, exists := collectionKinds[key]; exists && previous != ordered {
				return ErrWrongType
			}
			collectionKinds[key] = ordered
			if records[string(opts.encode(group.Key))] {
				return errors.New("kv: record and collection keys overlap")
			}
		}
	}
	return nil
}
