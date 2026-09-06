package socialutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"iter"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// RecoveryIndex partitions outstanding Social work across 256 exact sets.
// Root identifies one work kind. The directory contains at most 256 bucket
// names and intentionally retains empty buckets to avoid removal/addition races.
// Callers publish and remove memberships in the same mutation as the work record.
type RecoveryIndex struct {
	Root kv.Key
}

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
		buckets, err := store.ListMembers(ctx, index.directory())
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
			decoded, err := hex.DecodeString(bucket)
			if err != nil || len(decoded) != 1 || hex.EncodeToString(decoded) != bucket {
				yield("", errors.New("social: invalid recovery bucket"))
				return
			}
			ids, err := store.ListMembers(ctx, index.bucket(bucket))
			if err != nil {
				yield("", err)
				return
			}
			slices.Sort(ids)
			for _, id := range ids {
				if id == "" || recoveryBucket(id) != bucket {
					yield("", errors.New("social: invalid recovery member"))
					return
				}
				if !yield(id, nil) {
					return
				}
			}
		}
	}
}
