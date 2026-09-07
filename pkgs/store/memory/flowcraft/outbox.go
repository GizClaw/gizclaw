package flowcraft

import (
	"context"

	"github.com/GizClaw/flowcraft/memory/recall"
)

// scopedSideEffectOutbox qualifies every enqueued job identity with its
// canonical scope key before the job reaches the durable outbox.
//
// Flowcraft names each Save batch by wall-clock nanoseconds and every
// outbox implementation derives the job identity from that batch name and
// the job kind, then treats an existing identity as an idempotent replay.
// Concurrent Saves in different scopes that share one outbox can therefore
// collide on the same nanosecond, and the second scope's projection job is
// silently dropped even though its canonical facts were appended. Prefixing
// the identity with the scope keeps replay dedupe inside one scope only.
type scopedSideEffectOutbox struct{ recall.SideEffectOutbox }

func (outbox scopedSideEffectOutbox) Enqueue(ctx context.Context, job recall.SideEffectJob) error {
	if job.ID == "" && job.RequestID != "" {
		job.ID = job.Scope.CanonicalKey() + "|" + job.RequestID + "|" + string(job.Kind)
	}
	return outbox.SideEffectOutbox.Enqueue(ctx, job)
}
