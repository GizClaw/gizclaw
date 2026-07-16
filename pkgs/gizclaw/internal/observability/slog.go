package observability

import (
	"context"
	"log/slog"
)

const CompletionMessage = "gizclaw: request completed"

// Log emits one scalar structured completion record through the global logger.
func Log(ctx context.Context, outcome *Outcome) {
	if outcome == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	level, attrs := outcome.logRecord()
	slog.LogAttrs(context.WithoutCancel(ctx), level, CompletionMessage, attrs...)
}
