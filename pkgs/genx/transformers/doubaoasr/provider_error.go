package doubaoasr

import (
	"errors"
	"io"
)

// providerStartupError distinguishes a failed provider open from normal stream
// completion, while retaining the transport cause for errors.Is and errors.As.
type providerStartupError struct {
	cause error
}

func (e *providerStartupError) Error() string {
	return "doubao asr: provider startup: " + e.cause.Error()
}
func (e *providerStartupError) Unwrap() error { return e.cause }
func (e *providerStartupError) PublicError() (code, message string, retryable bool) {
	return "ASR_STREAM_ERROR", "ASR provider session startup failed", errors.Is(e.cause, io.EOF)
}
