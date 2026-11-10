package asttranslate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type testProviderStartupError struct{ cause error }

func (e *testProviderStartupError) Error() string { return "provider startup: " + e.cause.Error() }
func (e *testProviderStartupError) Unwrap() error { return e.cause }
func (*testProviderStartupError) PublicError() (string, string, bool) {
	return "AST_STREAM_ERROR", "Speech translation could not start.", false
}

type testProviderErrorStream struct{ cause error }

func (s testProviderErrorStream) Next() (*genx.MessageChunk, error) { return nil, s.cause }
func (testProviderErrorStream) Close() error                        { return nil }
func (testProviderErrorStream) CloseWithError(error) error          { return nil }

func TestInterruptibleTransformerPreservesTypedProviderEOF(t *testing.T) {
	cause := fmt.Errorf("websocket connect: %w", io.EOF)
	providerError := &testProviderStartupError{cause: cause}
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.Close()
	transformer := interruptibleTransformer{Transformer: transformFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return testProviderErrorStream{cause: providerError}, nil
	})}
	output, err := transformer.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	_, err = output.Next()
	code, _, _, public := genx.PublicErrorDetails(err)
	if !errors.Is(err, cause) || !public || code != "AST_STREAM_ERROR" || errors.Is(err, errProviderOutputEndedWhileInputActive) {
		t.Fatalf("provider EOF lost its typed cause: %v", err)
	}
}

func TestTypedProviderFailureTakesPrecedenceOverStreamCompletion(t *testing.T) {
	for _, completion := range []error{io.EOF, genx.ErrDone} {
		wrapped := fmt.Errorf("stream context: %w", completion)
		if !isStreamDone(wrapped) {
			t.Fatalf("ordinary contextual completion changed: %v", wrapped)
		}
		if isStreamDone(&testProviderStartupError{cause: wrapped}) {
			t.Fatalf("typed provider failure treated as completion: %v", wrapped)
		}
	}
}
