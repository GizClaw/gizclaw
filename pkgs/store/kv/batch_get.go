package kv

import (
	"context"
	"errors"
	"sync"
)

const batchGetConcurrency = 8

// GetResult is the independently owned value or error for one requested key.
// Missing, expired and collection keys report ErrNotFound in a mixed batch.
type GetResult struct {
	Value []byte
	Err   error
}

// batchGet implements backend-owned batches using at most eight point reads.
// It preserves ctx, never retries or changes read authority, and skips queued
// reads after cancellation. Native backends implement BatchGet directly.
func batchGet(ctx context.Context, keys []Key, get func(context.Context, Key) ([]byte, error)) []GetResult {
	if len(keys) == 0 {
		return []GetResult{}
	}
	results := make([]GetResult, len(keys))
	workers := min(len(keys), batchGetConcurrency)
	var done sync.WaitGroup
	for worker := range workers {
		done.Go(func() {
			for i := worker; i < len(keys); i += workers {
				if err := ctx.Err(); err != nil {
					results[i].Err = err
					continue
				}
				results[i].Value, results[i].Err = get(ctx, keys[i])
				if errors.Is(results[i].Err, ErrWrongType) {
					results[i] = GetResult{Err: ErrNotFound}
				}
			}
		})
	}
	done.Wait()
	return results
}
