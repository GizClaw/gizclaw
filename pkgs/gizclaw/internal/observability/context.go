package observability

import "context"

type outcomeContextKey struct{}

// WithOutcome attaches outcome to an existing request context.
func WithOutcome(ctx context.Context, outcome *Outcome) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, outcomeContextKey{}, outcome)
}

// FromContext returns the request outcome, if present.
func FromContext(ctx context.Context) *Outcome {
	if ctx == nil {
		return nil
	}
	outcome, _ := ctx.Value(outcomeContextKey{}).(*Outcome)
	return outcome
}

// Annotate adds one allowlisted safe domain identifier to the request outcome.
func Annotate(ctx context.Context, key AnnotationKey, value string) {
	FromContext(ctx).Annotate(key, value)
}

// SetErrorCode preserves a bounded domain error code at the transport boundary.
func SetErrorCode(ctx context.Context, code string) {
	FromContext(ctx).SetErrorCode(code)
}

// SetPeer preserves already-authenticated peer identity.
func SetPeer(ctx context.Context, publicKey, role string) {
	FromContext(ctx).SetPeer(publicKey, role)
}

// MarkPanic marks a panic already handled by an existing recovery boundary.
func MarkPanic(ctx context.Context) {
	FromContext(ctx).MarkPanic()
}
