package genx

import (
	"errors"
	"io"
)

// IsStreamDone reports normal completion of a Stream, including contextual
// wrappers of EOF and ErrDone. A PublicError anywhere in the error tree marks
// an operational failure even when its underlying cause matches completion.
// Classification checks the marker without invoking its public detail method.
// Adapters can supply additional completion sentinels owned by their upstream.
func IsStreamDone(err error, additional ...error) bool {
	if err == nil {
		return false
	}
	if _, failed := errors.AsType[PublicError](err); failed {
		return false
	}
	if errors.Is(err, ErrDone) || errors.Is(err, io.EOF) {
		return true
	}
	for _, terminal := range additional {
		if errors.Is(err, terminal) {
			return true
		}
	}
	return false
}

func isDoneErr(err error) bool {
	return IsStreamDone(err)
}
