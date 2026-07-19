package agent

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestOutputUsesFreshStreamIDForEachResponse(t *testing.T) {
	output := NewOutput(OutputConfig{})
	first, err := output.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin(first) error = %v", err)
	}
	if err := first.Push(&genx.MessageChunk{Part: genx.Text("first")}); err != nil {
		t.Fatalf("Push(first) error = %v", err)
	}
	if err := first.Finish(); err != nil {
		t.Fatalf("Finish(first) error = %v", err)
	}
	for range 2 {
		if _, err := output.Next(); err != nil {
			t.Fatalf("Next(first) error = %v", err)
		}
	}
	second, err := output.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin(second) error = %v", err)
	}
	if first.StreamID() == second.StreamID() {
		t.Fatalf("responses reused StreamID %q", first.StreamID())
	}
}

func TestOutputInterruptDiscardsUnpulledContentAndEmitsEOS(t *testing.T) {
	var observed []string
	output := NewOutput(OutputConfig{Observe: func(chunk *genx.MessageChunk) {
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			observed = append(observed, string(text))
		}
	}})
	response, err := output.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := response.Push(&genx.MessageChunk{Part: genx.Text("visible")}); err != nil {
		t.Fatalf("Push(visible) error = %v", err)
	}
	if err := response.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}}); err != nil {
		t.Fatalf("Push(audio) error = %v", err)
	}
	chunk, err := output.Next()
	if err != nil || chunk.Part != genx.Text("visible") {
		t.Fatalf("Next() = %#v, %v", chunk, err)
	}
	if err := response.Interrupt(); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	if cause := context.Cause(response.Context()); cause == nil || cause.Error() != Interrupted {
		t.Fatalf("response cause = %v", context.Cause(response.Context()))
	}

	var eos []*genx.MessageChunk
	for range 2 {
		chunk, err = output.Next()
		if err != nil {
			t.Fatalf("Next(interrupt) error = %v", err)
		}
		eos = append(eos, chunk)
	}
	if !slices.Equal(observed, []string{"visible"}) {
		t.Fatalf("observed content = %v", observed)
	}
	for _, chunk := range eos {
		if chunk.Ctrl.StreamID != response.StreamID() || !chunk.IsEndOfStream() || chunk.Ctrl.Error != Interrupted {
			t.Fatalf("interrupt chunk = %#v", chunk)
		}
	}
	if mime, _ := eos[0].MIMEType(); mime != "text/plain" {
		t.Fatalf("first interrupt MIME = %q, want text/plain", mime)
	}
	if mime, _ := eos[1].MIMEType(); mime != "audio/opus" {
		t.Fatalf("second interrupt MIME = %q, want audio/opus", mime)
	}
}

func TestOutputBeginInterruptsPreviousResponseBeforeNewContent(t *testing.T) {
	output := NewOutput(OutputConfig{})
	first, err := output.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Push(&genx.MessageChunk{Part: genx.Text("stale")}); err != nil {
		t.Fatal(err)
	}
	second, err := output.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Push(&genx.MessageChunk{Part: genx.Text("fresh")}); err != nil {
		t.Fatal(err)
	}

	chunk, err := output.Next()
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Ctrl.StreamID != first.StreamID() || chunk.Ctrl.Error != Interrupted || !chunk.IsEndOfStream() {
		t.Fatalf("first chunk = %#v, want interrupted first response", chunk)
	}
	chunk, err = output.Next()
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Ctrl.StreamID != second.StreamID() || chunk.Part != genx.Text("fresh") {
		t.Fatalf("second chunk = %#v, want fresh response", chunk)
	}
}

func TestOutputBufferLimitFailsWithoutBlocking(t *testing.T) {
	output := NewOutput(OutputConfig{MaxBufferedBytes: 10})
	response, err := output.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	err = response.Push(&genx.MessageChunk{Part: genx.Text("content larger than ten bytes")})
	if !errors.Is(err, ErrOutputBufferFull) {
		t.Fatalf("Push() error = %v, want ErrOutputBufferFull", err)
	}
	terminal, nextErr := output.Next()
	if nextErr != nil || terminal == nil || !terminal.IsEndOfStream() || !strings.Contains(terminal.Ctrl.Error, ErrOutputBufferFull.Error()) {
		t.Fatalf("overflow terminal = %#v, %v", terminal, nextErr)
	}
}

func TestOutputCloseDrainsQueuedContent(t *testing.T) {
	output := NewOutput(OutputConfig{})
	response, err := output.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Push(&genx.MessageChunk{Part: genx.Text("queued")}); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Next(); err != nil {
		t.Fatalf("Next(queued) error = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Next(done) error = %v, want EOF", err)
	}
}

func TestOutputFailAndCloseWithErrorAreObservable(t *testing.T) {
	output := NewOutput(OutputConfig{})
	response, err := output.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if response.Interrupted() {
		t.Fatal("new response is interrupted")
	}
	if err := response.Push(&genx.MessageChunk{Part: genx.Text("partial")}); err != nil {
		t.Fatal(err)
	}
	if err := response.Fail("model failed"); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Next(); err != nil {
		t.Fatal(err)
	}
	eos, err := output.Next()
	if err != nil || eos.Ctrl.Error != "model failed" || !eos.IsEndOfStream() {
		t.Fatalf("failed EOS = %#v, %v", eos, err)
	}
	wantErr := errors.New("transport failed")
	if err := output.CloseWithError(wantErr); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Next(); !errors.Is(err, wantErr) {
		t.Fatalf("Next() error = %v, want %v", err, wantErr)
	}
}

func TestIsStreamEndRecognizesSuccessfulTerminals(t *testing.T) {
	for _, err := range []error{io.EOF, genx.ErrDone} {
		if !IsStreamEnd(err) {
			t.Fatalf("IsStreamEnd(%v) = false", err)
		}
	}
	if IsStreamEnd(errors.New("failed")) {
		t.Fatal("IsStreamEnd(failure) = true")
	}
}
