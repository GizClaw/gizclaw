package peerquota

import "errors"

type quotaError struct {
	message   string
	exhausted bool
}

func (e *quotaError) Error() string { return e.message }

// PublicError implements GenX's domain-neutral safe terminal error contract.
func (e *quotaError) PublicError() (code, message string, retryable bool) {
	if e.exhausted {
		return "QUOTA_EXHAUSTED", "Quota exhausted.", false
	}
	return "QUOTA_UNAVAILABLE", "Quota unavailable.", true
}

// ErrorDetails classifies only errors owned by quota enforcement. Other
// resource authorization failures must retain their own error contract.
func ErrorDetails(err error) (code, message string, retryable, ok bool) {
	if errors.Is(err, ErrDenied) {
		code, message, retryable = ErrDenied.PublicError()
		return code, message, retryable, true
	}
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrClosed) {
		code, message, retryable = ErrUnavailable.PublicError()
		return code, message, retryable, true
	}
	return "", "", false, false
}
