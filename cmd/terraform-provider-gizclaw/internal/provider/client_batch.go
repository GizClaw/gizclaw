package provider

import (
	"context"
	"time"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

const (
	readCoalesceWindow = 10 * time.Millisecond
	maxReadBatch       = 128
	readBatchTimeout   = 2 * time.Minute
)

type resourceRead struct {
	ctx    context.Context
	kind   string
	id     string
	result chan resourceReadResult
}

type resourceReadResult struct {
	resource resourceEnvelope
	err      error
}

// refresh groups concurrent Terraform reads into batches executed with
// adminresource.GetResources on the shared connection. It does not cache
// state between calls.
func (c *adminClient) refresh(ctx context.Context, kind, id string) (resourceEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return resourceEnvelope{}, err
	}
	request := resourceRead{ctx: ctx, kind: kind, id: id, result: make(chan resourceReadResult, 1)}
	c.readMu.Lock()
	first := len(c.pendingReads) == 0
	c.pendingReads = append(c.pendingReads, request)
	if first {
		time.AfterFunc(readCoalesceWindow, c.flushReads)
	}
	c.readMu.Unlock()
	select {
	case result := <-request.result:
		return result.resource, result.err
	case <-ctx.Done():
		return resourceEnvelope{}, ctx.Err()
	}
}

func (c *adminClient) flushReads() {
	c.readMu.Lock()
	pending := c.pendingReads
	c.pendingReads = nil
	c.readMu.Unlock()
	active := make([]resourceRead, 0, len(pending))
	for _, r := range pending {
		if r.ctx.Err() == nil {
			active = append(active, r)
		}
	}
	for len(active) > 0 {
		n := min(maxReadBatch, len(active))
		c.readBatch(active[:n])
		active = active[n:]
	}
}

func (c *adminClient) readBatch(requests []resourceRead) {
	// One canceled caller must not cancel unrelated reads in the same batch.
	ctx, cancel := context.WithTimeout(context.Background(), readBatchTimeout)
	defer cancel()
	refs := make([]adminresource.Reference, len(requests))
	parseErrs := make([]error, len(requests))
	valid := make([]adminresource.Reference, 0, len(requests))
	for i, r := range requests {
		refs[i], parseErrs[i] = adminresource.ParseReference(r.kind, r.id)
		if parseErrs[i] == nil {
			valid = append(valid, refs[i])
		}
	}
	results := adminresource.GetResources(ctx, c, valid)
	next := 0
	for i, r := range requests {
		if parseErrs[i] != nil {
			r.result <- resourceReadResult{err: parseErrs[i]}
			continue
		}
		result := results[next]
		next++
		if result.Err != nil {
			r.result <- resourceReadResult{err: result.Err}
			continue
		}
		envelope, err := toEnvelope(*result.Resource)
		r.result <- resourceReadResult{resource: envelope, err: err}
	}
}
