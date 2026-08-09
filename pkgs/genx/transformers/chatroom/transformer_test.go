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
	if first == nil || first.Name != transcriptLabel || first.Ctrl == nil || first.Ctrl.StreamID != "turn-a" || !first.IsBeginOfStream() || first.Part != genx.Text("hello") {
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
	if len(chunks) != 2 {
		t.Fatalf("output chunks = %d, want BOS/EOS: %#v", len(chunks), chunks)
	}
	if !chunks[0].IsBeginOfStream() {
		t.Fatalf("initial chunk = %#v, want transcript BOS", chunks[0])
	}
	chunk := chunks[1]
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

func TestTransformerPushToTalkKeepsOuterStreamAcrossTurns(t *testing.T) {
	asr := &recordingASR{text: "heard"}
	transformer, err := New(Config{
		ASR:               asr,
		TranscriptEnabled: true,
		ASRPattern:        "model/asr",
		InputMode:         InputModePushToTalk,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	for i, streamID := range []string{"turn-a", "turn-b"} {
		eosError := ""
		if i == 0 {
			eosError = "interrupted"
		}
		if err := input.Add(
			&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{byte(i + 1)}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
			&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true, Error: eosError}},
		); err != nil {
			t.Fatalf("input.Add(%s) error = %v", streamID, err)
		}
		waitForTranscriptEOS(t, output, streamID)
		if got := asr.Calls(); got != i+1 {
			t.Fatalf("ASR calls after %s = %d, want %d", streamID, got, i+1)
		}
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input.Done() error = %v", err)
	}
	waitForStreamDone(t, output)
}

func TestTransformerRealtimeKeepsOneASRSessionAcrossAudioRoutes(t *testing.T) {
	asr := newRouteAwareRealtimeASR()
	transformer, err := New(Config{
		ASR:               asr,
		TranscriptEnabled: true,
		ASRPattern:        "model/asr",
		InputMode:         InputModeRealtime,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	for _, streamID := range []string{"segment-a", "segment-b"} {
		if err := input.Add(
			&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}},
			&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
			&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
		); err != nil {
			t.Fatalf("input.Add(%s) error = %v", streamID, err)
		}
		waitForTranscriptEOS(t, output, streamID)
		if got := asr.Calls(); got != 1 {
			t.Fatalf("ASR calls after %s = %d, want 1", streamID, got)
		}
		select {
		case <-asr.inputDone:
			t.Fatalf("ASR input completed after local route %s", streamID)
		default:
		}
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input.Done() error = %v", err)
	}
	waitForStreamDone(t, output)
	select {
	case <-asr.inputDone:
	case <-time.After(time.Second):
		t.Fatal("ASR input did not complete with outer input")
	}
	if got := asr.LocalEOS(); got != 2 {
		t.Fatalf("ASR local EOS count = %d, want 2", got)
	}
}

func TestTransformerRealtimeRejectsPrematureASROutputCompletion(t *testing.T) {
	transformer, err := New(Config{
		ASR:               immediateDoneASR{},
		TranscriptEnabled: true,
		ASRPattern:        "model/asr",
		InputMode:         InputModeRealtime,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := &blockingInput{
		first: &genx.MessageChunk{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
			Ctrl: &genx.StreamCtrl{StreamID: "segment-a"},
		},
		closed: make(chan error, 1),
	}
	output, err := transformer.Transform(t.Context(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	if _, err := output.Next(); !errors.Is(err, errASROutputCompleted) {
		t.Fatalf("output.Next() error = %v, want %v", err, errASROutputCompleted)
	}
}

func TestTransformerPushToTalkRejectsASRCloseBeforeEOSDelivery(t *testing.T) {
	transformer, err := New(Config{
		ASR:               closeAfterAudioASR{},
		TranscriptEnabled: true,
		ASRPattern:        "model/asr",
		InputMode:         InputModePushToTalk,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-a"},
	}); err != nil {
		t.Fatalf("input.Add(audio) error = %v", err)
	}
	if _, err := output.Next(); err == nil || isStreamDone(err) || !strings.Contains(err.Error(), "chatroom: ASR") {
		t.Fatalf("output.Next() error = %v, want premature ASR failure before EOS", err)
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

type immediateDoneASR struct{}

func (immediateDoneASR) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	if err := output.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return output.Stream(), nil
}

type closeAfterAudioASR struct{}

func (closeAfterAudioASR) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	go func() {
		if _, err := input.Next(); err != nil {
			_ = output.Abort(err)
			return
		}
		_ = input.Close()
		_ = output.Done(genx.Usage{})
	}()
	return output.Stream(), nil
}

type recordingASR struct {
	mu      sync.Mutex
	calls   int
	pattern string
	audio   []byte
	text    string
}

func (a *recordingASR) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	a.mu.Lock()
	a.calls++
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
			&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(a.text), Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}},
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

func (a *recordingASR) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

type routeAwareRealtimeASR struct {
	mu        sync.Mutex
	calls     int
	localEOS  int
	inputDone chan struct{}
	doneOnce  sync.Once
}

func newRouteAwareRealtimeASR() *routeAwareRealtimeASR {
	return &routeAwareRealtimeASR{inputDone: make(chan struct{})}
}

func (a *routeAwareRealtimeASR) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	go func() {
		defer input.Close()
		defer a.doneOnce.Do(func() { close(a.inputDone) })
		seen := make(map[string]struct{})
		for {
			chunk, err := input.Next()
			if isStreamDone(err) {
				_ = output.Done(genx.Usage{})
				return
			}
			if err != nil {
				_ = output.Abort(err)
				return
			}
			if chunk == nil {
				continue
			}
			if chunk.IsEndOfStream() {
				a.mu.Lock()
				a.localEOS++
				a.mu.Unlock()
				continue
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || len(blob.Data) == 0 || chunk.Ctrl == nil {
				continue
			}
			streamID := chunk.Ctrl.StreamID
			if _, ok := seen[streamID]; ok {
				continue
			}
			seen[streamID] = struct{}{}
			if err := output.Add(
				&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("heard"), Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
			); err != nil {
				return
			}
		}
	}()
	return output.Stream(), nil
}

func (a *routeAwareRealtimeASR) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func (a *routeAwareRealtimeASR) LocalEOS() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.localEOS
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

func waitForTranscriptEOS(t *testing.T, output genx.Stream, streamID string) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		for {
			chunk, err := output.Next()
			if err != nil {
				result <- err
				return
			}
			if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID == streamID && chunk.IsEndOfStream() {
				result <- nil
				return
			}
		}
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("output.Next(%s) error = %v", streamID, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for transcript EOS %s", streamID)
	}
}

func waitForStreamDone(t *testing.T, output genx.Stream) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		_, err := output.Next()
		result <- err
	}()
	select {
	case err := <-result:
		if !isStreamDone(err) {
			t.Fatalf("output.Next() final error = %v, want done", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for outer output completion")
	}
}
