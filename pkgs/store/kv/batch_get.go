package kv

import (
	"context"
	"sync"
)

const batchGetConcurrency = 8

// GetResult is the independently owned value or error for one requested key.
// Missing and expired keys report ErrNotFound, including in a mixed batch.
type GetResult struct {
	Value []byte
	Err   error
}

// BatchGetter is an optional bulk point-read capability. Results correspond
// one-to-one to keys in input order, including duplicates and failed reads.
// Implementations honor ctx and the same consistency policy as Get. A batch
// does not promise an atomic snapshot across all backends or across calls.
type BatchGetter interface {
	BatchGet(ctx context.Context, keys []Key) []GetResult
}

// BatchGet uses a backend's bulk reads when available, otherwise at most eight
// concurrent Get calls. All reads inherit ctx; queued keys skip I/O after
// cancellation. The compatibility path adds no retries or authority fallback;
// native implementations retain their backend's routing policy.
func BatchGet(ctx context.Context, store Store, keys []Key) []GetResult {
	if len(keys) == 0 {
		return []GetResult{}
	}
	if batch, ok := store.(BatchGetter); ok {
		return batch.BatchGet(ctx, keys)
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
				results[i].Value, results[i].Err = store.Get(ctx, keys[i])
			}
		})
	}
	done.Wait()
	return results
}
