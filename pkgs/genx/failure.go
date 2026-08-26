package genx

import (
	"context"
	"errors"
	"io"
)

// FailureClass is a payload-free provenance marker for a stream failure.
// Unknown or unclassified failures intentionally use the empty value.
type FailureClass string

const (
	FailureClassProvider  FailureClass = "provider"
	FailureClassTransform FailureClass = "transform"
)

type classifiedFailure struct {
	class FailureClass
	err   error
}

func (e classifiedFailure) Error() string { return e.err.Error() }
func (e classifiedFailure) Unwrap() error { return e.err }

// ClassifyFailure attaches a closed, payload-free provenance class while
// preserving the original error for errors.Is/errors.As. An existing valid
// class wins so downstream wrappers cannot overwrite the owning boundary.
func ClassifyFailure(err error, class FailureClass) error {
	if err == nil || !class.valid() {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, ErrDone) {
		return err
	}
	if _, ok := FailureClassOf(err); ok {
		return err
	}
	return classifiedFailure{class: class, err: err}
}

// FailureClassOf returns the first valid provenance class in an error chain.
func FailureClassOf(err error) (FailureClass, bool) {
	var classified classifiedFailure
	if !errors.As(err, &classified) || !classified.class.valid() {
		return "", false
	}
	return classified.class, true
}

func (c FailureClass) valid() bool {
	return c == FailureClassProvider || c == FailureClassTransform
}
