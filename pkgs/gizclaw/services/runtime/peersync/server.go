// Package peersync retains owner-scoped state checkpoints for incremental
// synchronization. Domain services still own the state projected by callers.
package peersync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	retention      = 24 * time.Hour
	maxCheckpoints = 64
	// MaxTimestamp keeps synchronization timestamps lossless in JavaScript.
	MaxTimestamp int64 = 1<<53 - 1
)

// ErrInvalidTimestamp reports an invalid synchronization timestamp.
var ErrInvalidTimestamp = errors.New("peersync: invalid timestamp")

// Snapshot contains the complete current Peer-visible state, keyed by canonical
// Peer HTTP resource paths. Values are the same JSON objects as the read APIs.
type Snapshot map[string]json.RawMessage

// Result describes the changes needed to replace the caller's previous state.
// Reset requires the client to clear its staged state before applying Upserts.
// State and Timestamp must be committed together after the complete result;
// clients must discard an incomplete batch instead of retaining partial changes.
type Result struct {
	Reset     bool
	Upserts   Snapshot
	Deletes   []string
	Timestamp int64
}

// Server stores checkpoint hashes in the shared Peer KV store. It owns no
// goroutines, subscriptions, or domain resource data.
type Server struct {
	Store kv.Store
	now   func() time.Time
}

type checkpoint struct {
	Version int               `json:"version"`
	Hashes  map[string]string `json:"hashes"`
}

type head struct {
	Version    int     `json:"version"`
	Timestamps []int64 `json:"timestamps"`
}

func stateKey(owner giznet.PublicKey, part string) kv.Key {
	return kv.Key{"peer-sync", owner.String(), part}
}

func checkpointKey(owner giznet.PublicKey, timestamp int64) kv.Key {
	return kv.Key{"peer-sync", owner.String(), "checkpoints", strconv.FormatInt(timestamp, 10)}
}

func (s *Server) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// Sync compares current state with one retained checkpoint belonging to owner,
// then atomically retains a new checkpoint. Zero, unknown, and expired timestamps
// produce a complete reset. Hashes, rather than resource payloads, are persisted.
// The last 64 checkpoints are retained for at most 24 hours, including across
// Server restarts when the configured Peer KV store is persistent.
func (s *Server) Sync(ctx context.Context, owner giznet.PublicKey, timestamp int64, current Snapshot) (Result, error) {
	if timestamp < 0 || timestamp > MaxTimestamp {
		return Result{}, ErrInvalidTimestamp
	}
	if s == nil || s.Store == nil || owner.IsZero() {
		return Result{}, errors.New("peersync: owner or store is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	previous, found, err := s.loadCheckpoint(ctx, owner, timestamp)
	if err != nil {
		return Result{}, err
	}
	result := Result{Reset: !found, Upserts: make(Snapshot)}
	next := checkpoint{Version: 1, Hashes: make(map[string]string, len(current))}
	for key, value := range current {
		if key == "" || !json.Valid(value) {
			return Result{}, errors.New("peersync: invalid snapshot")
		}
		digest := sha256.Sum256(value)
		next.Hashes[key] = hex.EncodeToString(digest[:])
		if previous.Hashes[key] != next.Hashes[key] {
			result.Upserts[key] = slices.Clone(value)
		}
	}
	for key := range previous.Hashes {
		if _, exists := current[key]; !exists {
			result.Deletes = append(result.Deletes, key)
		}
	}
	slices.Sort(result.Deletes)
	result.Timestamp, err = s.saveCheckpoint(ctx, owner, next)
	return result, err
}

func (s *Server) loadCheckpoint(ctx context.Context, owner giznet.PublicKey, timestamp int64) (checkpoint, bool, error) {
	if timestamp == 0 || timestamp < s.clock().Add(-retention).UnixMilli() {
		return checkpoint{}, false, nil
	}
	data, err := s.Store.Get(ctx, checkpointKey(owner, timestamp))
	if errors.Is(err, kv.ErrNotFound) {
		return checkpoint{}, false, nil
	}
	if err != nil {
		return checkpoint{}, false, fmt.Errorf("peersync: load checkpoint: %w", err)
	}
	var value checkpoint
	if err := json.Unmarshal(data, &value); err != nil || value.Version != 1 || value.Hashes == nil {
		return checkpoint{}, false, errors.New("peersync: invalid stored checkpoint")
	}
	return value, true, nil
}

func (s *Server) saveCheckpoint(ctx context.Context, owner giznet.PublicKey, value checkpoint) (int64, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	key := stateKey(owner, "head")
	for range 32 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		expected, err := s.Store.Get(ctx, key)
		if err != nil && !errors.Is(err, kv.ErrNotFound) {
			return 0, fmt.Errorf("peersync: load head: %w", err)
		}
		latest := head{Version: 1}
		if err == nil {
			if err := json.Unmarshal(expected, &latest); err != nil || latest.Version != 1 || len(latest.Timestamps) == 0 || len(latest.Timestamps) > maxCheckpoints {
				return 0, errors.New("peersync: invalid stored head")
			}
		} else {
			expected = nil
		}
		now := s.clock()
		timestamp := now.UnixMilli()
		var deleted []kv.Key
		for i, previous := range latest.Timestamps {
			if previous <= 0 || previous >= MaxTimestamp || (i > 0 && previous <= latest.Timestamps[i-1]) {
				return 0, errors.New("peersync: invalid stored timestamp")
			}
			if previous >= timestamp {
				timestamp = previous + 1
			}
		}
		if timestamp <= 0 || timestamp > MaxTimestamp {
			return 0, errors.New("peersync: clock is out of range")
		}
		kept := make([]int64, 0, maxCheckpoints)
		for i, previous := range latest.Timestamps {
			if previous < now.Add(-retention).UnixMilli() || i < len(latest.Timestamps)-(maxCheckpoints-1) {
				deleted = append(deleted, checkpointKey(owner, previous))
			} else {
				kept = append(kept, previous)
			}
		}
		latest.Timestamps = append(kept, timestamp)
		headData, err := json.Marshal(latest)
		if err != nil {
			return 0, err
		}
		deadline := now.Add(retention)
		committed, err := s.Store.ApplyMutation(ctx, kv.Mutation{
			Conditions: []kv.Condition{{Key: key, Expected: expected}},
			Entries: []kv.Entry{
				{Key: key, Value: headData, Deadline: deadline},
				{Key: checkpointKey(owner, timestamp), Value: payload, Deadline: deadline},
			},
			DeleteKeys: deleted,
		})
		if err != nil {
			return 0, fmt.Errorf("peersync: save checkpoint: %w", err)
		}
		if committed {
			return timestamp, nil
		}
	}
	return 0, errors.New("peersync: checkpoint contention; retry synchronization")
}
