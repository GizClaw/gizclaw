package genx

import "errors"

// PublicError supplies stable, sanitized terminal details for an error. Domain
// owners implement this interface; GenX does not interpret domain error codes.
type PublicError interface {
	error
	PublicError() (code, message string, retryable bool)
}

// PublicErrorDetails finds safe details through a wrapped error chain.
func PublicErrorDetails(err error) (code, message string, retryable, ok bool) {
	public, ok := errors.AsType[PublicError](err)
	if !ok {
		return "", "", false, false
	}
	code, message, retryable = public.PublicError()
	return code, message, retryable, true
}

// SetStreamError retains the cause and projects its public details onto an EOS.
// Call it before publishing the control object to another goroutine.
func SetStreamError(ctrl *StreamCtrl, err error) {
	ctrl.ErrorCause = err
	ctrl.Error, ctrl.ErrorCode, ctrl.ErrorRetryable = "", "", false
	if err == nil {
		return
	}
	ctrl.Error = err.Error()
	if code, message, retryable, ok := PublicErrorDetails(err); ok {
		ctrl.Error, ctrl.ErrorCode, ctrl.ErrorRetryable = message, code, retryable
	}
}

// StreamError returns the original terminal cause when it is available. An
// untyped wire error remains an ordinary error; its text is never classified.
func StreamError(ctrl *StreamCtrl) error {
	if ctrl == nil {
		return nil
	}
	if ctrl.ErrorCause != nil {
		return ctrl.ErrorCause
	}
	if ctrl.Error != "" {
		return errors.New(ctrl.Error)
	}
	if ctrl.ErrorCode != "" {
		return errors.New(ctrl.ErrorCode)
	}
	return nil
}
