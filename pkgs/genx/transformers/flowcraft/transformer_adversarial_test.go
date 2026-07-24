package flowcraft

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerRejectsNilSurfacesAndAcceptsNilContext(t *testing.T) {
	t.Parallel()
	var nilAgent *Agent
	if _, err := nilAgent.Transform(t.Context(), textInput("input")); err == nil ||
		!strings.Contains(err.Error(), "Transformer is nil") {
		t.Fatalf("nil Agent Transform() error = %v", err)
	}
	if _, err := (&Agent{}).Transform(t.Context(), textInput("input")); err == nil ||
		!strings.Contains(err.Error(), "Transformer is nil") {
		t.Fatalf("empty Agent Transform() error = %v", err)
	}
	transformer, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := transformer.Transform(t.Context(), nil); err == nil ||
		!strings.Contains(err.Error(), "input Stream is required") {
		t.Fatalf("Transform(nil input) error = %v", err)
	}
	output, err := transformer.Transform(nil, textInput("nil context"))
	if err != nil {
		t.Fatalf("Transform(nil context) error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "reply: nil context" {
		t.Fatalf("nil-context output = %q", got)
	}
}

func TestTransformerStreamAndTerminationHelpersAreTotal(t *testing.T) {
	t.Parallel()
	var stream *sessionStream
	if err := stream.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
	if err := stream.CloseWithError(nil); err != nil {
		t.Fatalf("nil CloseWithError() error = %v", err)
	}
	if !isStreamEnd(nil) || !isStreamEnd(io.EOF) || !isStreamEnd(genx.Done(genx.Usage{})) {
		t.Fatal("known terminal errors were not recognized")
	}
	if isStreamEnd(genx.Error(genx.Usage{}, errors.New("failed"))) ||
		isStreamEnd(errors.New("ordinary failure")) {
		t.Fatal("non-terminal error was recognized as stream end")
	}
	if messageStreamID(nil) != "" ||
		messageStreamID(&genx.MessageChunk{}) != "" ||
		messageStreamID(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: " id "}}) != "id" {
		t.Fatal("messageStreamID did not normalize nil/whitespace cases")
	}

	session := &session{}
	session.observeOutput(nil)
	session.observeOutput(&genx.MessageChunk{})
}
