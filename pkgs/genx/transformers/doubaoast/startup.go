package doubaoast

import (
	"context"
	"errors"
	"io"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

// providerStartupError keeps an operational EOF distinct from stream completion.
// The original transport error remains available through errors.Is/errors.As.
type providerStartupError struct{ cause error }

func (e *providerStartupError) Error() string {
	return "doubao ast translate: provider startup: " + e.cause.Error()
}
func (e *providerStartupError) Unwrap() error { return e.cause }
func (e *providerStartupError) PublicError() (code, message string, retryable bool) {
	return "AST_STREAM_ERROR", "Speech translation provider session startup failed", errors.Is(e.cause, io.EOF)
}

// openProviderSession retries one opening EOF before accepting a session or
// sending any audio. Established sessions are never replayed by this path.
func (t *Transformer) openProviderSession(ctx context.Context, cfg doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
	open := t.openSession
	if t.newSession != nil {
		open = t.newSession
	}
	for attempt := 0; ; attempt++ {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		next, err := open(ctx, cfg)
		if cause := context.Cause(ctx); cause != nil {
			if next != nil {
				_ = next.Close()
			}
			return nil, cause
		}
		if err == nil && next != nil {
			return next, nil
		}
		if next != nil {
			_ = next.Close()
		}
		if err == nil {
			err = errors.New("provider returned no session")
		}
		if attempt == 0 && errors.Is(err, io.EOF) {
			continue
		}
		return nil, &providerStartupError{cause: err}
	}
}
