package doubaoast

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func runStartupRegression(t *testing.T, opener func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error)) error {
	t.Helper()
	input := newBufferStream(4)
	if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "startup", BeginOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "startup"}}); err != nil {
		t.Fatal(err)
	}
	_ = input.Close()
	transformer := newTransformer(doubaospeech.NewClient("test"), withRealtimePacing(false))
	transformer.newSession = opener
	output, err := transformer.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for {
		if _, err := output.Next(); err != nil {
			return err
		}
	}
}

func TestStartupEOFRecoversBeforeAnyAudioIsAccepted(t *testing.T) {
	var calls atomic.Int32
	session := &fakeASTTranslateSession{events: []*doubaospeech.ASTTranslateEvent{{Type: doubaospeech.ASTEventSessionFinished}}}
	err := runStartupRegression(t, func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		if calls.Add(1) == 1 {
			return nil, fmt.Errorf("websocket connect: %w", io.EOF)
		}
		return session, nil
	})
	if !errors.Is(err, io.EOF) || calls.Load() != 2 {
		t.Fatalf("startup result=%v, attempts=%d; want one recovered opening EOF", err, calls.Load())
	}
	if _, _, _, public := genx.PublicErrorDetails(err); public {
		t.Fatalf("successful completion retained a provider failure: %v", err)
	}
	if audio := session.sentAudioSnapshot(); len(audio) != 1 || len(audio[0]) != 2 {
		t.Fatalf("source audio uploaded %d times, want exactly once", len(audio))
	}
}

func TestStartupFailureRemainsTypedAfterBoundedRetry(t *testing.T) {
	for _, transient := range []bool{false, true} {
		t.Run(fmt.Sprint(transient), func(t *testing.T) {
			cause := errors.New("provider rejected startup")
			wantCalls := int32(1)
			if transient {
				cause = fmt.Errorf("websocket connect: %w", io.EOF)
				wantCalls = 2
			}
			var calls atomic.Int32
			err := runStartupRegression(t, func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
				calls.Add(1)
				return nil, cause
			})
			code, message, retryable, public := genx.PublicErrorDetails(err)
			if !errors.Is(err, cause) || !public || code != "AST_STREAM_ERROR" || message == "" || retryable != transient || calls.Load() != wantCalls {
				t.Fatalf("terminal=%v public=%t code=%q attempts=%d, want %d", err, public, code, calls.Load(), wantCalls)
			}
		})
	}
}

func TestStartupCancellationDoesNotRetryEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	input := newBufferStream(2)
	_ = input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}})
	_ = input.Close()
	var calls atomic.Int32
	transformer := newTransformer(doubaospeech.NewClient("test"), withRealtimePacing(false))
	transformer.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		calls.Add(1)
		cancel()
		return nil, fmt.Errorf("websocket connect: %w", io.EOF)
	}
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	_, err = output.Next()
	if calls.Load() != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup=%v, attempts=%d", err, calls.Load())
	}
}
