package audiodock

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type providerCompletionError struct{ cause error }

func (e *providerCompletionError) Error() string { return "provider failed: " + e.cause.Error() }
func (e *providerCompletionError) Unwrap() error { return e.cause }
func (*providerCompletionError) PublicError() (string, string, bool) {
	return "PROVIDER_FAILED", "provider failed", true
}

func TestStreamDonePreservesCompletionAndRejectsProviderFailures(t *testing.T) {
	for _, terminal := range []error{io.EOF, genx.ErrDone} {
		for _, wrapped := range []error{terminal, fmt.Errorf("completed input: %w", terminal)} {
			if !streamDone(wrapped) {
				t.Errorf("completion %v classified as failure", wrapped)
			}
			failure := fmt.Errorf("adapter: %w", &providerCompletionError{cause: wrapped})
			if !errors.Is(failure, terminal) {
				t.Fatal("test lost original completion cause")
			}
			if streamDone(failure) {
				t.Errorf("typed provider failure %v classified as completion", failure)
			}
		}
	}
	if streamDone(errors.New("provider EOF")) {
		t.Fatal("classified error text as completion")
	}
}
