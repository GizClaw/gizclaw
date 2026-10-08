package socialutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// RecoveryIndex partitions outstanding Social work across 256 exact sets.
// Root identifies one work kind. The directory contains at most 256 bucket
// names and intentionally retains empty buckets to avoid removal/addition races.
// Callers publish and remove memberships in the same mutation as the work record.
type RecoveryIndex struct {
	Root kv.Key
}

const recoveryAttemptTimeout = 30 * time.Second

func recoveryBucket(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:1])
}

func (index RecoveryIndex) directory() kv.Key {
	return append(append(kv.Key{"social-recovery-index"}, index.Root...), "buckets")
}

func (index RecoveryIndex) bucket(name string) kv.Key {
	return append(append(kv.Key{"social-recovery-index"}, index.Root...), "members", name)
}

// Add returns the memberships to commit atomically with a new work record.
func (index RecoveryIndex) Add(id string) []kv.SetMembers {
	bucket := recoveryBucket(id)
	return []kv.SetMembers{
		{Key: index.directory(), Members: []string{bucket}},
		{Key: index.bucket(bucket), Members: []string{id}},
	}
}

// Remove returns the membership to remove atomically with a completed record.
func (index RecoveryIndex) Remove(id string) []kv.SetMembers {
	return []kv.SetMembers{{Key: index.bucket(recoveryBucket(id)), Members: []string{id}}}
}

// IDs visits pending identities one bucket at a time. Callers must re-read each
// record because another worker may complete it after the set was read.
func (index RecoveryIndex) IDs(ctx context.Context, store kv.Store) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		buckets, err := recoveryMembers(ctx, store, index.directory())
		if err != nil {
			yield("", err)
			return
		}
		if len(buckets) > 256 {
			yield("", errors.New("social: invalid recovery bucket count"))
			return
		}
		slices.Sort(buckets)
		for _, bucket := range buckets {
			if err := ctx.Err(); err != nil {
				yield("", err)
				return
			}
			decoded, err := hex.DecodeString(bucket)
			if err != nil || len(decoded) != 1 || hex.EncodeToString(decoded) != bucket {
				if !yield("", errors.New("social: invalid recovery bucket")) {
					return
				}
				continue
			}
			ids, err := recoveryMembers(ctx, store, index.bucket(bucket))
			if err != nil {
				if !yield("", err) {
					return
				}
				continue
			}
			slices.Sort(ids)
			for _, id := range ids {
				if id == "" || recoveryBucket(id) != bucket {
					if !yield("", errors.New("social: invalid recovery member")) {
						return
					}
					continue
				}
				if !yield(id, nil) {
					return
				}
			}
		}
	}
}

func recoveryMembers(ctx context.Context, store kv.Store, key kv.Key) ([]string, error) {
	readCtx, cancel := context.WithTimeout(ctx, recoveryAttemptTimeout)
	defer cancel()
	return store.ListMembers(readCtx, key)
}

// Reconcile attempts each indexed identity independently with a 30-second
// deadline. Failed records stay indexed and do not hide later work. It returns
// a bounded error summary after the pass, or stops promptly on cancellation.
func (index RecoveryIndex) Reconcile(ctx context.Context, store kv.Store, reconcile func(context.Context, string) error) error {
	var firstErr error
	var failures int
	for id, err := range index.IDs(ctx, store) {
		if ctx.Err() != nil {
			return errors.Join(firstErr, ctx.Err())
		}
		if err == nil {
			attemptCtx, cancel := context.WithTimeout(ctx, recoveryAttemptTimeout)
			err = reconcile(attemptCtx, id)
			if err == nil {
				err = attemptCtx.Err()
			}
			cancel()
		}
		if err != nil {
			failures++
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return fmt.Errorf("social: %d recovery attempts failed: %w", failures, firstErr)
	}
	return ctx.Err()
}
