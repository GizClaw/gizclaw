package audiodock

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestInputTextUsesOnlyFinalSegmentsForItsRoute(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var texts inputTexts
	current := texts.start("one")
	for _, chunk := range []*genx.MessageChunk{
		{Part: genx.Text("wrong interim"), Ctrl: &genx.StreamCtrl{StreamID: "one", TextInterim: true}},
		{Part: genx.Text("foreign"), Ctrl: &genx.StreamCtrl{StreamID: "other"}},
		{Part: genx.Text("first "), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
		{Ctrl: &genx.StreamCtrl{StreamID: "one", Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
		{Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
	} {
		texts.append(chunk)
	}
	if got, err := current.wait(ctx); err != nil || got != "first second" {
		t.Fatalf("final input text = %q, %v", got, err)
	}
	// A completed value remains immutable even after the next input arrives.
	texts.start("two")
	if got, err := current.wait(ctx); err != nil || got != "first second" {
		t.Fatalf("completed input was replaced: %q, %v", got, err)
	}
}

func TestInputTextInterruptionCancellationAndMissingEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var texts inputTexts
	first := texts.start("one")
	second := texts.start("two")
	if _, err := first.wait(ctx); err == nil {
		t.Fatal("replaced input was not interrupted")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := second.wait(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter: %v", err)
	}
	texts.append(&genx.MessageChunk{Part: genx.Text("late old text"), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}})
	select {
	case <-second.done:
		t.Fatal("late old transcript completed the replacement input")
	default:
	}
	texts.close(io.ErrUnexpectedEOF)
	if _, err := second.wait(ctx); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("missing ASR EOS: %v", err)
	}
	if _, err := texts.start("three").wait(ctx); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("input after ASR close: %v", err)
	}
}

func TestInputTextPropagatesTypedASRFailure(t *testing.T) {
	var texts inputTexts
	current := texts.start("one")
	cause := errors.New("recognition failed")
	texts.append(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true, ErrorCause: cause}})
	if _, err := current.wait(t.Context()); !errors.Is(err, cause) {
		t.Fatalf("ASR failure = %v", err)
	}
}
