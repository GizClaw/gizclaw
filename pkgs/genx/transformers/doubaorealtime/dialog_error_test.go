package doubaorealtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	speech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPTTDialogCommonErrorInvalidatesSession(t *testing.T) {
	for _, providerError := range []*speech.Error{nil, {Code: 55000000, Message: "local provider failure"}} {
		name := "without-payload"
		if providerError != nil {
			name = "structured-error"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			input := newDoubaoRealtimeInputReader(newBufferStream(1, ctx))
			defer input.Close()
			output := newBufferStream(8, ctx)
			defer output.Close()
			tr := newTransformer(nil)
			runtime := newDoubaoRealtimeRuntime(tr)
			defer runtime.close()
			session := &fakeTransformerSession{events: []*speech.RealtimeEvent{{
				Type: speech.EventDialogCommonError, StatusCode: "MODEL_BUSY", Error: providerError,
			}}}
			err := tr.processSession(ctx, input, output, session, runtime)
			if !isDoubaoRealtimeRecoverable(err) || !session.isClosed() {
				t.Fatalf("dialogue error must close and invalidate session: closed=%t err=%v", session.isClosed(), err)
			}
			if providerError == nil {
				if !strings.Contains(err.Error(), "provider reported a dialogue error") {
					t.Fatalf("missing error payload must retain a diagnostic cause: %v", err)
				}
			} else {
				var cause *speech.Error
				if !errors.As(err, &cause) || cause != providerError {
					t.Fatalf("lost structured provider cause: %v", err)
				}
			}
		})
	}
}

func TestDialogCommonErrorPreservesOtherModes(t *testing.T) {
	for _, mode := range []Mode{ModeRealtime, ModeText} {
		t.Run(string(mode), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			input := newBufferStream(1, ctx)
			defer input.Close()
			if err := input.Push(&genx.MessageChunk{Part: genx.Text("query")}); err != nil {
				t.Fatal(err)
			}
			reader := newDoubaoRealtimeInputReader(input)
			defer reader.Close()
			output := newBufferStream(8, ctx)
			textSent := make(chan struct{})
			session := &fakeTransformerSession{
				beforeRecv: textSent, firstTextSent: textSent,
				events: []*speech.RealtimeEvent{
					{Type: speech.EventDialogCommonError, StatusCode: "MODEL_BUSY", Error: &speech.Error{Code: 55000000, Message: "nonterminal"}},
					{Type: speech.EventChatResponse, Text: "continued reply"},
					{Type: speech.EventChatEnded},
					{Type: speech.EventSessionFinished},
				},
			}
			tr := newTransformer(nil, withMode(mode), withOutput(OutputText))
			runtime := newDoubaoRealtimeRuntime(tr)
			defer runtime.close()
			err := tr.processSession(ctx, reader, output, session, runtime)
			if !isDoubaoRealtimeRecoverable(err) || !strings.Contains(err.Error(), "session finished") {
				t.Fatalf("599 must retain the existing nonterminal policy in %s: %v", mode, err)
			}
			_ = output.Close()
			chunks := drainRealtimeTestOutput(t, output)
			if !hasRealtimeTestText(chunks, genx.RoleModel, "continued reply") {
				t.Fatalf("599 cut the following reply in %s", mode)
			}
		})
	}
}
