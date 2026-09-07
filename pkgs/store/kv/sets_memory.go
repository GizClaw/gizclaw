package kv

import (
	"bytes"
	"context"
	"maps"
	"time"
)

// AddMembers adds members idempotently to one collection.
func (m *Memory) AddMembers(ctx context.Context, key Key, members ...string) error {
	_, err := m.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// RemoveMembers removes members idempotently from one collection.
func (m *Memory) RemoveMembers(ctx context.Context, key Key, members ...string) error {
	_, err := m.ApplyMutation(ctx, Mutation{RemoveMembers: []SetMembers{{Key: key, Members: members}}})
	return err
}

// HasMember tests membership without enumerating the collection.
func (m *Memory) HasMember(ctx context.Context, key Key, member string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	encoded := string(m.opts.encode(key))
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, exists := m.data[encoded]
	if !exists || entry.expired(time.Now()) {
		return false, nil
	}
	if entry.members == nil || entry.ordered {
		return false, ErrWrongType
	}
	_, found := entry.members[member]
	return found, nil
}

// ListMembers reads one exact collection. Missing collections return no members.
func (m *Memory) ListMembers(ctx context.Context, key Key) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded := string(m.opts.encode(key))
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, exists := m.data[encoded]
	if !exists || entry.expired(time.Now()) {
		return nil, nil
	}
	if entry.members == nil || entry.ordered {
		return nil, ErrWrongType
	}
	members := make([]string, 0, len(entry.members))
	for member := range entry.members {
		members = append(members, member)
	}
	return members, nil
}

// ApplyMutation atomically updates records and collection indexes after all
// conditions, deadlines and collection types have been checked.
func (m *Memory) ApplyMutation(ctx context.Context, mutation Mutation) (bool, error) {
	if err := mutation.validate(ctx, m.opts); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	now := time.Now()
	for _, condition := range mutation.Conditions {
		entry, exists := m.data[string(m.opts.encode(condition.Key))]
		exists = exists && !entry.expired(now)
		if condition.Expected == nil {
			if exists {
				return false, nil
			}
		} else {
			if exists && entry.members != nil {
				return false, ErrWrongType
			}
			if !exists || !bytes.Equal(entry.value, condition.Expected) {
				return false, nil
			}
		}
	}
	// Stage touched keys only; failure leaves the original maps unchanged.
	staged := make(map[string]memoryEntry)
	deletes := make(map[string]bool)
	for _, entry := range mutation.Entries {
		if !entry.Deadline.IsZero() && !entry.Deadline.After(now) {
			return false, ErrInvalidDeadline
		}
		staged[string(m.opts.encode(entry.Key))] = memoryEntry{value: bytes.Clone(entry.Value), expiresAt: entry.Deadline}
	}
	for _, key := range mutation.DeleteKeys {
		deletes[string(m.opts.encode(key))] = true
	}
	for phase, groups := range [][]SetMembers{mutation.AddMembers, mutation.RemoveMembers, mutation.AddOrderedMembers, mutation.RemoveOrderedMembers} {
		for _, group := range groups {
			if len(group.Members) == 0 {
				continue
			}
			key := string(m.opts.encode(group.Key))
			entry, stagedAlready := staged[key]
			if !stagedAlready {
				var exists bool
				entry, exists = m.data[key]
				if exists && !entry.expired(now) {
					if entry.members == nil || entry.ordered != (phase >= 2) {
						return false, ErrWrongType
					}
					entry.members = maps.Clone(entry.members)
				} else {
					entry = memoryEntry{members: make(map[string]struct{}), ordered: phase >= 2}
				}
			}
			for _, member := range group.Members {
				if phase%2 == 0 {
					entry.members[member] = struct{}{}
				} else {
					delete(entry.members, member)
				}
			}
			staged[key] = entry
			deletes[key] = len(entry.members) == 0
		}
	}
	maps.Copy(m.data, staged)
	for key, remove := range deletes {
		if remove {
			delete(m.data, key)
		}
	}
	return true, nil
}
