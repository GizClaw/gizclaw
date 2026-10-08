package socialutil

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (index RecoveryIndex) recordKey(id string) kv.Key {
	return append(append(kv.Key{}, index.Root...), EscapeStoreSegment(id))
}

func canonicalRecoveryBucket(bucket string) bool {
	decoded, err := hex.DecodeString(bucket)
	return err == nil && len(decoded) == 1 && hex.EncodeToString(decoded) == bucket
}

// Only background maintenance repairs metadata. IDs remains a read-only view.
// Fixed bucket keys let discovery recover from directory loss without adding
// database-wide key enumeration to the KV contract.
func (index RecoveryIndex) repairIDs(ctx context.Context, store kv.Store) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		buckets, err := recoveryMembers(ctx, store, index.directory())
		if errors.Is(err, kv.ErrWrongType) {
			err = index.repairDirectoryType(ctx, store)
			if err != nil && !yield("", err) {
				return
			}
			// An unreadable directory must not hide accessible canonical
			// buckets. Its original value remains for operator inspection.
		} else if err != nil {
			yield("", err)
			return
		}
		known := make(map[string]bool, 256)
		invalid := 0
		for _, bucket := range buckets {
			if canonicalRecoveryBucket(bucket) {
				known[bucket] = true
				continue
			}
			// Bound extra discovery caused by corrupt directory entries. Each
			// successful removal leaves room for the next pass to make progress.
			if invalid == 256 {
				if !yield("", errors.New("social: excessive invalid recovery buckets")) {
					return
				}
				break
			}
			invalid++
			err := index.repairInvalidBucket(ctx, store, bucket)
			if err != nil && !yield("", err) {
				return
			}
		}
		for number := range 256 {
			bucket := fmt.Sprintf("%02x", number)
			if !index.repairBucket(ctx, store, bucket, true, known, yield) {
				return
			}
		}
	}
}

// An invalid directory member is untrusted input, including key separators
// rejected by a KV encoder. Isolate that one metadata entry; never let it panic
// through the worker or drop its directory reference when it cannot be read.
func (index RecoveryIndex) repairInvalidBucket(ctx context.Context, store kv.Store, bucket string) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("social: invalid recovery bucket key")
		}
	}()
	var firstErr error
	keepGoing := index.repairBucket(ctx, store, bucket, false, nil, func(_ string, err error) bool {
		if firstErr == nil {
			firstErr = err
		}
		return ctx.Err() == nil
	})
	if firstErr != nil {
		return firstErr
	}
	if !keepGoing {
		return ctx.Err()
	}
	// Never detach a bucket that still contains work, including a member
	// added concurrently after our read.
	_, err = recoveryMutation(ctx, store, kv.Mutation{
		Conditions:    []kv.Condition{{Key: index.bucket(bucket)}},
		RemoveMembers: []kv.SetMembers{{Key: index.directory(), Members: []string{bucket}}},
	})
	return err
}

func (index RecoveryIndex) repairDirectoryType(ctx context.Context, store kv.Store) error {
	readCtx, cancel := context.WithTimeout(ctx, recoveryAttemptTimeout)
	defer cancel()
	value, err := store.Get(readCtx, index.directory())
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, kv.ErrWrongType) {
		// Another worker may already have restored a Set. Other unreadable
		// types remain intact rather than being deleted without a comparison.
		_, err = store.ListMembers(readCtx, index.directory())
		return err
	}
	if err != nil {
		return err
	}
	if value == nil {
		value = []byte{}
	}
	matched, err := store.ApplyMutation(readCtx, kv.Mutation{
		Conditions: []kv.Condition{{Key: index.directory(), Expected: value}},
		DeleteKeys: []kv.Key{index.directory()},
	})
	if !matched && (err == nil || errors.Is(err, kv.ErrWrongType)) {
		_, err = store.ListMembers(readCtx, index.directory())
	}
	return err
}

func (index RecoveryIndex) repairBucket(ctx context.Context, store kv.Store, bucket string, canonical bool, known map[string]bool, yield func(string, error) bool) bool {
	if err := ctx.Err(); err != nil {
		yield("", err)
		return false
	}
	ids, err := recoveryMembers(ctx, store, index.bucket(bucket))
	if err != nil {
		return yield("", err)
	}
	if canonical && len(ids) != 0 && !known[bucket] {
		_, err := recoveryMutation(ctx, store, kv.Mutation{
			AddMembers: []kv.SetMembers{{Key: index.directory(), Members: []string{bucket}}},
		})
		if err != nil && !yield("", err) {
			return false
		}
		known[bucket] = err == nil
	}
	// Preserve deterministic attempt ordering within each exact collection.
	// A moved member is attempted only in its canonical bucket, this pass or
	// the next, so duplicate or misplaced memberships cannot repeat an attempt.
	slices.Sort(ids)
	for _, member := range ids {
		ready, err := index.repairMember(ctx, store, bucket, member)
		if err != nil {
			if !yield("", err) {
				return false
			}
			continue
		}
		if ready && !yield(member, nil) {
			return false
		}
	}
	return true
}

func (index RecoveryIndex) repairMember(ctx context.Context, store kv.Store, bucket, member string) (ready bool, err error) {
	// Member IDs are also untrusted. Their escaped record key may be
	// unaddressable under a custom KV separator; preserve that member and
	// isolate the encoder rejection instead of panicking through the worker.
	defer func() {
		if recover() != nil {
			ready = false
			err = errors.New("social: invalid recovery member key")
		}
	}()
	attemptCtx, cancel := context.WithTimeout(ctx, recoveryAttemptTimeout)
	defer cancel()
	removal := kv.SetMembers{Key: index.bucket(bucket), Members: []string{member}}
	id := strings.TrimSpace(member)
	if id == "" {
		// Empty identities cannot name a work record. Only their derived
		// membership is removed; no business record is read or changed.
		_, err := store.ApplyMutation(attemptCtx, kv.Mutation{RemoveMembers: []kv.SetMembers{removal}})
		return false, err
	}
	key := index.recordKey(id)
	value, err := store.Get(attemptCtx, key)
	if errors.Is(err, kv.ErrNotFound) {
		// The absence guard prevents a stale reader from unindexing a newly
		// published incarnation of the same work identity.
		_, err = store.ApplyMutation(attemptCtx, kv.Mutation{
			Conditions:    []kv.Condition{{Key: key}},
			RemoveMembers: []kv.SetMembers{removal},
		})
		return false, err
	}
	if err != nil {
		return false, err
	}
	if value == nil {
		value = []byte{}
	}
	if member == id && recoveryBucket(id) == bucket {
		return true, nil
	}
	// Work records are authoritative, including records that their domain
	// later rejects as malformed. Repairs preserve their original bytes and
	// only move the membership while that exact record is still current.
	_, err = store.ApplyMutation(attemptCtx, kv.Mutation{
		Conditions:    []kv.Condition{{Key: key, Expected: value}},
		AddMembers:    index.Add(id),
		RemoveMembers: []kv.SetMembers{removal},
	})
	return false, err
}

func recoveryMutation(ctx context.Context, store kv.Store, mutation kv.Mutation) (bool, error) {
	mutationCtx, cancel := context.WithTimeout(ctx, recoveryAttemptTimeout)
	defer cancel()
	return store.ApplyMutation(mutationCtx, mutation)
}
