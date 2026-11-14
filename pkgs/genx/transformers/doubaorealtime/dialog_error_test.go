package doubaorealtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	speech "github.com/GizClaw/doubao-speech-go"
)

func TestDialogCommonErrorInvalidatesSession(t *testing.T) {
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
