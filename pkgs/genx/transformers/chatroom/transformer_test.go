package chatroom

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestNewValidatesTranscriptDependencies(t *testing.T) {
	for _, tt := range []struct {
		name    string
		config  Config
		wantErr string
	}{
		{name: "disabled transcript", config: Config{}},
		{name: "missing ASR", config: Config{TranscriptEnabled: true, ASRPattern: "model/asr"}, wantErr: "transformer is required"},
		{name: "missing pattern", config: Config{TranscriptEnabled: true, ASR: testMux{}}, wantErr: "transcript.asr_model is required"},
		{name: "invalid input mode", config: Config{InputMode: "unknown"}, wantErr: "unsupported input mode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := New(tt.config)
			if tt.wantErr == "" {
				if err != nil || transformer == nil {
					t.Fatalf("New() = %v, %v", transformer, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("New() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestASRPatternPreservesExistingQuery(t *testing.T) {
	transformer := &Transformer{config: Config{ASRPattern: "model/asr?language=zh-CN", InputMode: InputModeRealtime}}
	if got, want := transformer.asrPattern(), "model/asr?language=zh-CN&emit_interim=true"; got != want {
		t.Fatalf("asrPattern() = %q, want %q", got, want)
	}
}

func TestTransformerForwardsTextWithOneTranscriptRoute(t *testing.T) {
	transformer, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(context.Background(), &testStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "turn-a"}},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn-a", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	first, err := output.Next()
	if err != nil {
		t.Fatalf("output.Next() first error = %v", err)
	}
	if first == nil || first.Name != transcriptLabel || first.Ctrl == nil || first.Ctrl.StreamID != "turn-a" || first.Part != genx.Text("hello") {
		t.Fatalf("first output = %#v", first)
	}
	last, err := output.Next()
	if err != nil {
		t.Fatalf("output.Next() EOS error = %v", err)
	}
	if last == nil || !last.IsEndOfStream() || last.Ctrl == nil || last.Ctrl.StreamID != "turn-a" {
		t.Fatalf("EOS output = %#v", last)
	}
}

func TestTransformerTranscribesAudioInput(t *testing.T) {
	asr := &recordingASR{text: "hello"}
	transformer, err := New(Config{
		ASR:               asr,
		TranscriptEnabled: true,
		ASRPattern:        "model/asr",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(context.Background(), &testStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-a"}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-a", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	var transcript, transcriptEOS bool
	for {
		chunk, err := output.Next()
		if isStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("output.Next() error = %v", err)
		}
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.StreamID != "turn-a" || chunk.Name != transcriptLabel {
			t.Fatalf("output route = %#v", chunk)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "hello" && !chunk.IsEndOfStream() {
			transcript = true
		}
		if chunk.IsEndOfStream() {
			transcriptEOS = true
		}
	}
	if got := asr.Pattern(); got != "model/asr" {
		t.Fatalf("ASR pattern = %q, want model/asr", got)
	}
	if got := asr.Audio(); string(got) != string([]byte{1, 2, 3}) {
		t.Fatalf("ASR audio = %v", got)
	}
	if !transcript || !transcriptEOS {
		t.Fatalf("transcript output missing text=%t eos=%t", transcript, transcriptEOS)
	}
}

func TestTransformerConsumesEmptyASRCompletionWithoutError(t *testing.T) {
	asr := &recordingASR{}
	transformer, err := New(Config{
		ASR:               asr,
		TranscriptEnabled: true,
		ASRPattern:        "model/asr",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(context.Background(), &testStream{chunks: []*genx.MessageChunk{
		{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}},
			Ctrl: &genx.StreamCtrl{StreamID: "empty-turn"},
		},
		{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: "audio/opus"},
			Ctrl: &genx.StreamCtrl{StreamID: "empty-turn", EndOfStream: true},
		},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if isStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("output.Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 1 {
		t.Fatalf("output chunks = %d, want one terminal boundary: %#v", len(chunks), chunks)
	}
	chunk := chunks[0]
	if !chunk.IsEndOfStream() || chunk.Ctrl == nil || chunk.Ctrl.StreamID != "empty-turn" || chunk.Ctrl.Error != "" {
		t.Fatalf("terminal chunk = %#v, want successful empty-turn EOS", chunk)
	}
	if text, ok := chunk.Part.(genx.Text); !ok || strings.TrimSpace(string(text)) != "" {
		t.Fatalf("terminal part = %#v, want empty text", chunk.Part)
	}
}

func TestTransformerRealtimeEnablesASRInterimOutput(t *testing.T) {
	asr := &recordingASR{text: "hello"}
	transformer, err := New(Config{
		ASR:               asr,
		TranscriptEnabled: true,
		ASRPattern:        "model/asr?language=zh-CN",
		InputMode:         InputModeRealtime,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(context.Background(), &testStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-a", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	for {
		_, err := output.Next()
		if isStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("output.Next() error = %v", err)
		}
	}
	if got, want := asr.Pattern(), "model/asr?language=zh-CN&emit_interim=true"; got != want {
		t.Fatalf("ASR pattern = %q, want %q", got, want)
	}
}

func TestASRInputTransportConsumerCloseBeforeProducerDone(t *testing.T) {
	transport := newASRInputTransport(nil)
	consumer := transport.Stream()
	want := &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-a", EndOfStream: true}}
	if err := transport.Add(want); err != nil {
		t.Fatalf("transport.Add() error = %v", err)
	}
	got, err := consumer.Next()
	if err != nil {
		t.Fatalf("consumer.Next() error = %v", err)
	}
	if got == nil || !got.IsEndOfStream() || got.Ctrl.StreamID != "turn-a" {
		t.Fatalf("consumer.Next() = %#v, want audio EOS", got)
	}
	if err := consumer.Close(); err != nil {
		t.Fatalf("consumer.Close() error = %v", err)
	}
	if err := transport.Done(); err != nil {
		t.Fatalf("transport.Done() after consumer close error = %v", err)
	}
	if chunk, err := consumer.Next(); !isStreamDone(err) || chunk != nil {
		t.Fatalf("consumer.Next() after Done = %#v, %v; want done", chunk, err)
	}
}

func TestASRInputTransportAbortUnblocksPendingDone(t *testing.T) {
	transport := newASRInputTransport(nil)
	consumer := transport.Stream()
	for range 64 {
		if err := transport.Add(&genx.MessageChunk{Part: genx.Text("audio")}); err != nil {
			t.Fatalf("transport.Add() error = %v", err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- transport.Done() }()
	deadline := time.Now().Add(time.Second)
	for {
		transport.mu.Lock()
		completing := transport.completing != nil
		transport.mu.Unlock()
		if completing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("transport.Done() did not enter completion")
		}
		runtime.Gosched()
	}
	want := errors.New("consumer stopped")
	if err := consumer.CloseWithError(want); err != nil {
		t.Fatalf("consumer.CloseWithError() error = %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("transport.Done() error = %v, want %v", err, want)
		}
	case <-time.After(time.Second):
		t.Fatal("transport.Done() remained blocked after consumer error")
	}
}

func TestTransformerCancellationAbortsASRInput(t *testing.T) {
	asr := &blockingASR{started: make(chan struct{}), inputErr: make(chan error, 1)}
	transformer, err := New(Config{ASR: asr, TranscriptEnabled: true, ASRPattern: "model/asr"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, err := transformer.Transform(ctx, &blockingInput{closed: make(chan error, 1), first: &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-a"},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	select {
	case <-asr.started:
	case <-time.After(time.Second):
		t.Fatal("ASR did not start")
	}
	cancel()
	if _, err := output.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("output.Next() error = %v, want context canceled", err)
	}
	select {
	case err := <-asr.inputErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ASR input error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ASR input remained blocked after cancellation")
	}
}

type testMux struct{}

func (testMux) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return nil, errors.New("not used")
}

type recordingASR struct {
	mu      sync.Mutex
	pattern string
	audio   []byte
	text    string
}

func (a *recordingASR) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	a.mu.Lock()
	a.pattern = pattern
	a.mu.Unlock()
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	go func() {
		defer input.Close()
		streamID := defaultInputStreamID
		for {
			chunk, err := input.Next()
			if isStreamDone(err) {
				break
			}
			if err != nil {
				_ = output.Abort(err)
				return
			}
			if chunk == nil {
				continue
			}
			if chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
				streamID = chunk.Ctrl.StreamID
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) != 0 {
				a.mu.Lock()
				a.audio = append(a.audio, blob.Data...)
				a.mu.Unlock()
			}
		}
		if err := output.Add(
			&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(a.text), Ctrl: &genx.StreamCtrl{StreamID: streamID}},
			&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
		); err != nil {
			return
		}
		_ = output.Done(genx.Usage{})
	}()
	return output.Stream(), nil
}

func (a *recordingASR) Pattern() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pattern
}

func (a *recordingASR) Audio() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]byte(nil), a.audio...)
}

type blockingASR struct {
	started  chan struct{}
	inputErr chan error
}

func (a *blockingASR) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	go func() {
		defer input.Close()
		if _, err := input.Next(); err != nil {
			a.inputErr <- err
			_ = output.Abort(err)
			return
		}
		close(a.started)
		_, err := input.Next()
		a.inputErr <- err
		_ = output.Abort(err)
	}()
	return output.Stream(), nil
}

type testStream struct {
	chunks []*genx.MessageChunk
}

func (s *testStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (*testStream) Close() error { return nil }

func (*testStream) CloseWithError(error) error { return nil }

type blockingInput struct {
	first  *genx.MessageChunk
	closed chan error
	once   sync.Once
}

func (s *blockingInput) Next() (*genx.MessageChunk, error) {
	if s.first != nil {
		chunk := s.first
		s.first = nil
		return chunk, nil
	}
	return nil, <-s.closed
}

func (s *blockingInput) Close() error {
	return s.CloseWithError(io.EOF)
}

func (s *blockingInput) CloseWithError(err error) error {
	s.once.Do(func() { s.closed <- err })
	return nil
}
