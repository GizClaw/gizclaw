package chatroom

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type closeRecordingStream struct {
	mu       sync.Mutex
	closed   int
	closeErr error
}

func (*closeRecordingStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (s *closeRecordingStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
	return nil
}
func (s *closeRecordingStream) CloseWithError(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
	s.closeErr = err
	return nil
}

func TestASRInputTransportTerminalBranches(t *testing.T) {
	var callbacks []error
	transport := newASRInputTransport(func(err error) { callbacks = append(callbacks, err) })
	consumer := transport.Stream()
	if err := transport.Add(&genx.MessageChunk{Part: genx.Text("input")}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := transport.Done(); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	if err := transport.Done(); err != nil {
		t.Fatalf("second Done() error = %v", err)
	}
	if err := transport.Add(&genx.MessageChunk{}); !errors.Is(err, genx.ErrDone) {
		t.Fatalf("Add() after Done = %v, want ErrDone", err)
	}
	if err := consumer.CloseWithError(genx.ErrDone); err != nil {
		t.Fatalf("CloseWithError(ErrDone) error = %v", err)
	}
	if err := consumer.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if len(callbacks) != 1 || callbacks[0] != nil {
		t.Fatalf("consumer callbacks = %#v", callbacks)
	}
	if err := transport.Abort(errors.New("late")); err != nil {
		t.Fatalf("Abort() after Done error = %v", err)
	}

	aborted := newASRInputTransport(func(err error) { callbacks = append(callbacks, err) })
	view := aborted.Stream()
	if err := aborted.Abort(nil); err != nil {
		t.Fatalf("Abort(nil) close error = %v", err)
	}
	if err := aborted.Add(&genx.MessageChunk{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Add() after Abort = %v", err)
	}
	if err := view.Close(); err != nil {
		t.Fatalf("Close() after Abort error = %v", err)
	}
	if len(callbacks) != 1 {
		t.Fatalf("aborted consumer invoked callback: %#v", callbacks)
	}

	closeErr := errors.New("consumer failure")
	failing := newASRInputTransport(func(err error) { callbacks = append(callbacks, err) })
	if err := failing.Stream().CloseWithError(closeErr); err != nil {
		t.Fatalf("CloseWithError() close error = %v", err)
	}
	if len(callbacks) != 2 || !errors.Is(callbacks[1], closeErr) {
		t.Fatalf("failure callbacks = %#v", callbacks)
	}
}

func TestASRSessionLifecycleBranches(t *testing.T) {
	var nilSession *asrSession
	nilSession.allowCompletion()
	if nilSession.completionAllowed() {
		t.Fatal("nil session allowed completion")
	}
	if err := nilSession.wait(context.Background()); err != nil {
		t.Fatalf("nil wait() error = %v", err)
	}
	if err := nilSession.complete(context.Background(), true); err != nil {
		t.Fatalf("nil complete() error = %v", err)
	}
	nilSession.abort(errors.New("ignored"))
	nilSession.closeOutput(nil)
	nilSession.stopInputCancellation()

	input := newASRInputTransport(nil)
	input.markConsumerEOS()
	stopped := 0
	output := &closeRecordingStream{}
	session := &asrSession{
		input:             input,
		output:            output,
		readDone:          make(chan error, 1),
		routeCompletionOK: true,
		stopInputCancel: func() bool {
			stopped++
			return true
		},
	}
	if !session.completionAllowed() {
		t.Fatal("route EOS did not allow completion")
	}
	session.readDone <- nil
	if err := session.wait(context.Background()); err != nil {
		t.Fatalf("wait() error = %v", err)
	}
	if output.closed != 1 || output.closeErr != nil || stopped != 1 || session.readDone != nil || session.stopInputCancel != nil {
		t.Fatalf("completed session = output %#v stopped=%d readDone=%v", output, stopped, session.readDone)
	}

	errOutput := &closeRecordingStream{}
	errSession := &asrSession{output: errOutput}
	wantErr := errors.New("terminal")
	errSession.closeOutput(wantErr)
	errSession.closeOutput(nil)
	if errOutput.closed != 1 || !errors.Is(errOutput.closeErr, wantErr) {
		t.Fatalf("error close = %#v", errOutput)
	}

	expected := &asrSession{}
	expected.allowCompletion()
	if !expected.completionAllowed() {
		t.Fatal("allowCompletion() did not permit completion")
	}
}

func TestNormalizeASRTranscriptChunkBranches(t *testing.T) {
	if got := normalizeASRTranscriptChunk(nil, "fallback"); got != nil {
		t.Fatalf("normalize(nil) = %#v", got)
	}

	tests := []struct {
		name     string
		chunk    *genx.MessageChunk
		fallback string
		wantNil  bool
		wantText bool
	}{
		{name: "empty data", chunk: &genx.MessageChunk{}, fallback: "fallback", wantNil: true},
		{name: "BOS blob", chunk: &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}}, fallback: "fallback"},
		{name: "text", chunk: &genx.MessageChunk{Part: genx.Text("hello")}, fallback: "fallback", wantText: true},
		{name: "blob EOS", chunk: &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}}, wantText: true},
		{name: "history", chunk: &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{Label: genx.HistoryUserAudioLabel}}, fallback: "fallback"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeASRTranscriptChunk(test.chunk, test.fallback)
			if test.wantNil {
				if got != nil {
					t.Fatalf("normalize() = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.Ctrl == nil || got.Ctrl.StreamID == "" || got.Role != genx.RoleUser || got.Name != transcriptLabel || got.Ctrl.Label == "" {
				t.Fatalf("normalize() = %#v", got)
			}
			if test.wantText {
				if _, ok := got.Part.(genx.Text); !ok {
					t.Fatalf("normalized part = %T, want Text", got.Part)
				}
			}
		})
	}
}

func TestChatroomChunkHelpers(t *testing.T) {
	chunk := textChunk("", "hello", true, true)
	if chunk.Ctrl.StreamID != defaultInputStreamID || !chunk.IsBeginOfStream() || !chunk.IsEndOfStream() {
		t.Fatalf("textChunk() = %#v", chunk)
	}
	if !isAudioChunk(&genx.MessageChunk{Part: &genx.Blob{MIMEType: " Audio/L16; rate=16000 "}}) {
		t.Fatal("isAudioChunk() rejected parameterized audio")
	}
	if isAudioChunk(nil) || isAudioChunk(&genx.MessageChunk{Part: genx.Text("no")}) {
		t.Fatal("isAudioChunk() accepted non-audio")
	}
	if got := baseMIME(" Audio/L16; rate=16000 "); got != "audio/l16" {
		t.Fatalf("baseMIME() = %q", got)
	}
	if !isStreamDone(io.EOF) || !isStreamDone(genx.ErrDone) || isStreamDone(errors.New("other")) {
		t.Fatal("isStreamDone() classification mismatch")
	}

	transformer, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := transformer.Transform(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "input stream is required") {
		t.Fatalf("Transform(nil) error = %v", err)
	}
}
