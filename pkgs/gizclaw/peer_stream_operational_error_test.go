package gizclaw

import (
	"fmt"
	"io"
	"testing"
)

type lifecycleOperationalError struct{ cause error }

func (e lifecycleOperationalError) Error() string {
	return "private provider failure: " + e.cause.Error()
}
func (e lifecycleOperationalError) Unwrap() error { return e.cause }
func (lifecycleOperationalError) PublicError() (string, string, bool) {
	panic("lifecycle classification must not invoke domain callbacks")
}

func TestPeerStreamLifecycleDoesNotLogOperationalEOFAsClosed(t *testing.T) {
	for _, completion := range []error{io.EOF, io.ErrClosedPipe} {
		err := fmt.Errorf("adapter: %w", lifecycleOperationalError{cause: completion})
		result, reason := peerStreamLifecycleResult(err)
		if result != "runtime_error" || reason != "internal_error" {
			t.Fatalf("operational failure logged as %q/%q", result, reason)
		}
	}
}
