package agenthost

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type outputCompletionFailure struct{ cause error }

func (e *outputCompletionFailure) Error() string { return "provider failed: " + e.cause.Error() }
func (e *outputCompletionFailure) Unwrap() error { return e.cause }
func (*outputCompletionFailure) PublicError() (string, string, bool) {
	return "PROVIDER_FAILED", "provider failed", true
}

func TestMixerOutputPreservesTypedProviderCompletionFailure(t *testing.T) {
	for _, cause := range []error{io.EOF, genx.ErrDone} {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/failure=%t", cause, failed), func(t *testing.T) {
				stream := NewInputStream(1)
				terminal := fmt.Errorf("output: %w", cause)
				var failure *outputCompletionFailure
				if failed {
					failure = &outputCompletionFailure{terminal}
					terminal = fmt.Errorf("agent: %w", failure)
				}
				if err := stream.CloseWithError(terminal); err != nil {
					t.Fatal(err)
				}
				err := (MixerOutput{}).ConsumeAgentOutput(t.Context(), stream)
				if failed {
					if !errors.Is(err, failure) || !errors.Is(err, cause) {
						t.Fatalf("consumer lost failure: %v", err)
					}
				} else if err != nil {
					t.Fatalf("completion failed: %v", err)
				}
			})
		}
	}
}
