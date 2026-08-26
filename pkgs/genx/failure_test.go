package genx

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestClassifyFailurePreservesCauseAndOwningClass(t *testing.T) {
	cause := errors.New("provider payload")
	classified := ClassifyFailure(cause, FailureClassProvider)
	if !errors.Is(classified, cause) {
		t.Fatalf("classified error = %v, want wrapped cause", classified)
	}
	if class, ok := FailureClassOf(classified); !ok || class != FailureClassProvider {
		t.Fatalf("FailureClassOf() = (%q, %t), want provider", class, ok)
	}
	classified = ClassifyFailure(classified, FailureClassTransform)
	if class, _ := FailureClassOf(classified); class != FailureClassProvider {
		t.Fatalf("downstream classification replaced owner with %q", class)
	}
}

func TestClassifyFailureLeavesLifecycleTerminalsUnclassified(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, io.EOF, ErrDone} {
		classified := ClassifyFailure(err, FailureClassProvider)
		if _, ok := FailureClassOf(classified); ok {
			t.Fatalf("FailureClassOf(%v) unexpectedly returned a class", err)
		}
	}
}
