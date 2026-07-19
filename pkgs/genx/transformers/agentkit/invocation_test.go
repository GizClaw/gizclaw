package agentkit

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestInvocationInterruptKeepsPulledPrefixAndDiscardsBufferedSuffix(t *testing.T) {
	var observed []string
	invocation := NewInvocation(context.Background(), OutputConfig{Observe: func(chunk *genx.MessageChunk) {
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			observed = append(observed, string(text))
		}
	}})
	defer invocation.Close()
	response, err := invocation.StartResponse("text/plain", "audio/opus")
	if err != nil {
		t.Fatalf("StartResponse() error = %v", err)
	}
	for _, chunk := range []*genx.MessageChunk{
		{Part: genx.Text("pulled")},
		{Part: genx.Text("discarded")},
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}},
	} {
		if err := invocation.Emit(response, chunk); err != nil {
			t.Fatalf("Emit() error = %v", err)
		}
	}
	prefix, err := invocation.Output().Next()
	if err != nil || prefix.Part != genx.Text("pulled") {
		t.Fatalf("Next() = (%#v, %v), want pulled prefix", prefix, err)
	}
	if err := invocation.Interrupt(response, "assistant"); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{Part: genx.Text("late")}); !errors.Is(err, ErrInactiveResponse) {
		t.Fatalf("late Emit() error = %v, want ErrInactiveResponse", err)
	}

	for _, wantMIME := range []string{"audio/opus", "text/plain"} {
		chunk, err := invocation.Output().Next()
		if err != nil {
			t.Fatalf("EOS Next() error = %v", err)
		}
		mimeType, ok := chunk.MIMEType()
		if !ok || mimeType != wantMIME {
			t.Fatalf("EOS MIME = %q, want %q", mimeType, wantMIME)
		}
		if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "interrupted" || chunk.Ctrl.StreamID != response.StreamID() {
			t.Fatalf("EOS ctrl = %#v", chunk.Ctrl)
		}
	}
	if len(observed) != 1 || observed[0] != "pulled" {
		t.Fatalf("observed text = %v, want only pulled prefix", observed)
	}
	replacement, err := invocation.StartResponse("text/plain")
	if err != nil {
		t.Fatalf("replacement StartResponse() error = %v", err)
	}
	if replacement.StreamID() == response.StreamID() {
		t.Fatalf("replacement reused StreamID %q", replacement.StreamID())
	}
}

func TestInvocationInterruptReplacesDiscardedRouteEOS(t *testing.T) {
	invocation := NewInvocation(context.Background(), OutputConfig{})
	response, err := invocation.StartResponse("text/plain", "audio/pcm")
	if err != nil {
		t.Fatalf("StartResponse() error = %v", err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("visible")}); err != nil {
		t.Fatalf("Emit(text) error = %v", err)
	}
	visible, err := invocation.Output().Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if visible.Part != genx.Text("visible") {
		t.Fatalf("visible part = %#v", visible.Part)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: response.StreamID(), EndOfStream: true},
	}); err != nil {
		t.Fatalf("Emit(text EOS) error = %v", err)
	}
	if err := invocation.Interrupt(response, "assistant"); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}

	for _, wantMIME := range []string{"audio/pcm", "text/plain"} {
		chunk, err := invocation.Output().Next()
		if err != nil {
			t.Fatalf("Next(%s EOS) error = %v", wantMIME, err)
		}
		mimeType, ok := chunk.MIMEType()
		if !ok || mimeType != wantMIME || !chunk.IsEndOfStream() || chunk.Ctrl.Error != "interrupted" {
			t.Fatalf("%s EOS = %#v", wantMIME, chunk)
		}
	}
}

func TestInvocationInterruptDoesNotRepeatPulledRouteEOS(t *testing.T) {
	invocation := NewInvocation(context.Background(), OutputConfig{})
	response, err := invocation.StartResponse("text/plain", "audio/pcm")
	if err != nil {
		t.Fatalf("StartResponse() error = %v", err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: response.StreamID(), EndOfStream: true},
	}); err != nil {
		t.Fatalf("Emit(text EOS) error = %v", err)
	}
	textEOS, err := invocation.Output().Next()
	if err != nil || !textEOS.IsEndOfStream() {
		t.Fatalf("Next(text EOS) = (%#v, %v)", textEOS, err)
	}
	if err := invocation.Interrupt(response, "assistant"); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}

	audioEOS, err := invocation.Output().Next()
	if err != nil {
		t.Fatalf("Next(audio EOS) error = %v", err)
	}
	mimeType, ok := audioEOS.MIMEType()
	if !ok || mimeType != "audio/pcm" || audioEOS.Ctrl.Error != "interrupted" {
		t.Fatalf("audio EOS = %#v", audioEOS)
	}
	if err := invocation.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := invocation.Output().Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal error = %v, want EOF", err)
	}
}

func TestInvocationCancellationDoesNotCrossInvocationBoundary(t *testing.T) {
	first := NewInvocation(context.Background(), OutputConfig{})
	second := NewInvocation(context.Background(), OutputConfig{})
	defer first.Close()
	defer second.Close()
	firstResponse, _ := first.StartResponse("text/plain")
	secondResponse, _ := second.StartResponse("text/plain")
	if err := first.Emit(firstResponse, &genx.MessageChunk{Part: genx.Text("discarded")}); err != nil {
		t.Fatalf("first Emit() error = %v", err)
	}
	if err := first.Cancel(context.Canceled); err != nil {
		t.Fatalf("first Cancel() error = %v", err)
	}
	if err := second.Emit(secondResponse, &genx.MessageChunk{Part: genx.Text("independent")}); err != nil {
		t.Fatalf("second Emit() after first cancellation error = %v", err)
	}
	firstEOS, err := first.Output().Next()
	if err != nil {
		t.Fatalf("first Next() error = %v", err)
	}
	if firstEOS.Ctrl == nil || firstEOS.Ctrl.Error != context.Canceled.Error() || !firstEOS.Ctrl.EndOfStream {
		t.Fatalf("first EOS = %#v", firstEOS.Ctrl)
	}
	if _, err := first.Output().Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("first terminal error = %v, want EOF", err)
	}
	secondChunk, err := second.Output().Next()
	if err != nil || secondChunk.Part != genx.Text("independent") {
		t.Fatalf("second Next() = (%#v, %v)", secondChunk, err)
	}
}

func TestInvocationParentCancellationClosesOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	invocation := NewInvocation(ctx, OutputConfig{})
	defer invocation.Close()
	response, _ := invocation.StartResponse("text/plain")
	cancel()
	chunk, err := invocation.Output().Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID != response.StreamID() || chunk.Ctrl.Error != context.Canceled.Error() {
		t.Fatalf("cancellation EOS = %#v", chunk.Ctrl)
	}
}

func TestInvocationStartsClosedForCancelledParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	invocation := NewInvocation(ctx, OutputConfig{})
	if _, err := invocation.StartResponse("text/plain"); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("StartResponse() error = %v, want closed pipe", err)
	}
	if _, err := invocation.Output().Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Next() error = %v, want EOF", err)
	}
}

func TestInvocationOutputCloseCancelsInvocation(t *testing.T) {
	invocation := NewInvocation(context.Background(), OutputConfig{})
	response, err := invocation.StartResponse("text/plain")
	if err != nil {
		t.Fatalf("StartResponse() error = %v", err)
	}
	if err := invocation.Output().Close(); err != nil {
		t.Fatalf("Output().Close() error = %v", err)
	}
	select {
	case <-invocation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("invocation context remained active after output close")
	}
	if err := invocation.Emit(response, &genx.MessageChunk{Part: genx.Text("late")}); !errors.Is(err, ErrInactiveResponse) {
		t.Fatalf("Emit() error = %v, want ErrInactiveResponse", err)
	}
	if _, err := invocation.StartResponse("text/plain"); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("StartResponse() error = %v, want closed pipe", err)
	}
}

func TestInvocationFinishAndResponseExclusivity(t *testing.T) {
	invocation := NewInvocation(context.Background(), OutputConfig{})
	defer invocation.Close()
	response, err := invocation.StartResponse("text/plain")
	if err != nil {
		t.Fatalf("StartResponse() error = %v", err)
	}
	if _, err := invocation.StartResponse("audio/opus"); !errors.Is(err, ErrResponseActive) {
		t.Fatalf("second StartResponse() error = %v", err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("wrong"),
		Ctrl: &genx.StreamCtrl{StreamID: "another-response"},
	}); !errors.Is(err, ErrInactiveResponse) {
		t.Fatalf("cross-response Emit() error = %v", err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("ok")}); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	if err := invocation.FinishResponse(response, "assistant", ""); err != nil {
		t.Fatalf("FinishResponse() error = %v", err)
	}
	data, err := invocation.Output().Next()
	if err != nil || data.Ctrl == nil || data.Ctrl.StreamID != response.StreamID() {
		t.Fatalf("data = (%#v, %v)", data, err)
	}
	eos, err := invocation.Output().Next()
	if err != nil || eos.Ctrl == nil || !eos.Ctrl.EndOfStream {
		t.Fatalf("EOS = (%#v, %v)", eos, err)
	}
	if _, err := invocation.StartResponse("audio/opus"); err != nil {
		t.Fatalf("replacement StartResponse() error = %v", err)
	}
}

func TestInvocationOutputLimitCancelsOnlyAffectedInvocation(t *testing.T) {
	invocation := NewInvocation(context.Background(), OutputConfig{MaxBytes: 1})
	response, _ := invocation.StartResponse("text/plain")
	err := invocation.Emit(response, &genx.MessageChunk{Part: genx.Text("too large")})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("Emit() error = %v, want ErrOutputLimit", err)
	}
	select {
	case <-invocation.Context().Done():
	default:
		t.Fatal("invocation context remained active after overflow")
	}
	if _, err := invocation.StartResponse("text/plain"); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("StartResponse() error = %v, want closed pipe", err)
	}
	if _, err := invocation.Output().Next(); !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("Next() error = %v, want ErrOutputLimit", err)
	}
}
