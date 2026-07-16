package asset

import "errors"

var (
	// ErrInvalid indicates that an asset request or persisted record is invalid.
	ErrInvalid = errors.New("asset: invalid")
	// ErrNotFound indicates that an asset does not exist or is not readable.
	ErrNotFound = errors.New("asset: not found")
	// ErrConflict indicates that an asset identifier or state conflicts with an operation.
	ErrConflict = errors.New("asset: conflict")
	// ErrInUse indicates that a live owner still references an asset.
	ErrInUse = errors.New("asset: in use")
	// ErrTooLarge indicates that an uploaded asset exceeded its configured limit.
	ErrTooLarge = errors.New("asset: too large")
	// ErrIntegrity indicates that stored bytes do not match their immutable metadata.
	ErrIntegrity = errors.New("asset: integrity check failed")
	// ErrResolverUnavailable indicates that an owner cannot be checked safely.
	ErrResolverUnavailable = errors.New("asset: owner resolver unavailable")
)
