package rpcapi

import "net/http"

// StatusCodeFromHTTP maps an HTTP status onto the canonical status code that
// carries the same meaning, following the google.rpc.Code mapping. A status
// with no canonical counterpart becomes Unknown for 4xx and Internal for
// everything else, so an unmapped status is never silently reported as success.
func StatusCodeFromHTTP(status int) StatusCode {
	switch status {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return StatusCodeOK
	case http.StatusBadRequest:
		return StatusCodeInvalidArgument
	case http.StatusUnauthorized:
		return StatusCodeUnauthenticated
	case http.StatusForbidden:
		return StatusCodePermissionDenied
	case http.StatusNotFound:
		return StatusCodeNotFound
	case http.StatusConflict:
		return StatusCodeAborted
	case http.StatusGone:
		return StatusCodeNotFound
	case http.StatusPreconditionFailed:
		return StatusCodeFailedPrecondition
	case http.StatusRequestedRangeNotSatisfiable:
		return StatusCodeOutOfRange
	case http.StatusTooManyRequests:
		return StatusCodeResourceExhausted
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return StatusCodeDeadlineExceeded
	case http.StatusNotImplemented:
		return StatusCodeUnimplemented
	case http.StatusServiceUnavailable:
		return StatusCodeUnavailable
	case 499:
		return StatusCodeCancelled
	}
	if status >= 400 && status < 500 {
		return StatusCodeUnknown
	}
	return StatusCodeInternal
}

// HTTPStatus maps a canonical status code onto the HTTP status that carries
// the same meaning, following the google.rpc.Code mapping. It replaces the
// hand-written switches that previously had to enumerate a subset of codes and
// silently reported every code they had not listed as a bad gateway.
func (e StatusCode) HTTPStatus() int {
	switch e {
	case StatusCodeOK:
		return http.StatusOK
	case StatusCodeCancelled:
		return 499
	case StatusCodeInvalidArgument, StatusCodeOutOfRange:
		return http.StatusBadRequest
	case StatusCodeDeadlineExceeded:
		return http.StatusGatewayTimeout
	case StatusCodeNotFound:
		return http.StatusNotFound
	case StatusCodeAlreadyExists, StatusCodeAborted:
		return http.StatusConflict
	case StatusCodePermissionDenied:
		return http.StatusForbidden
	case StatusCodeUnauthenticated:
		return http.StatusUnauthorized
	case StatusCodeResourceExhausted:
		return http.StatusTooManyRequests
	case StatusCodeFailedPrecondition:
		return http.StatusPreconditionFailed
	case StatusCodeUnimplemented:
		return http.StatusNotImplemented
	case StatusCodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
