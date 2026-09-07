package streamkit

import (
	"context"
	"errors"
	"io"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTTSStreamPreservesInputRouteMetadata(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{
			Role: genx.RoleUser,
			Name: "gear",
			Part: genx.Text("你好，世界。"),
			Ctrl: &genx.StreamCtrl{StreamID: "input-stream", Label: "speech"},
		},
		{
			Role: genx.RoleUser,
			Name: "gear",
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: "input-stream", Label: "speech", EndOfStream: true},
		},
	}, doneErr: io.EOF}

	var texts []string
	output := NewTTSStream(context.Background(), input, OutputConfig{InitialCapacity: 8}, "audio/mpeg", func(_ context.Context, text string, _ TTSMeta, _ string, emit func([]byte) error) error {
		texts = append(texts, text)
		return emit([]byte("audio:" + text))
	})

	chunks := collectTransformerChunks(t, output)
	if want := []string{"你好，世界。"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("synthesized texts = %#v, want %#v", texts, want)
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d output chunks, want BOS/data/EOS", len(chunks))
	}
	for index, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "input-stream" || chunk.Ctrl.Label != "speech" {
			t.Fatalf("chunk %d control = %#v", index, chunk.Ctrl)
		}
		if chunk.Role != genx.RoleUser || chunk.Name != "gear" {
			t.Fatalf("chunk %d metadata = role %q name %q", index, chunk.Role, chunk.Name)
		}
	}
	if !chunks[0].IsBeginOfStream() {
		t.Fatalf("initial chunk = %#v, want audio BOS", chunks[0])
	}
	if blob, ok := chunks[0].Part.(*genx.Blob); !ok || blob.MIMEType != "audio/mpeg" || len(blob.Data) != 0 {
		t.Fatalf("initial part = %#v, want empty audio/mpeg Blob", chunks[0].Part)
	}
	if blob, ok := chunks[1].Part.(*genx.Blob); !ok || len(blob.Data) == 0 || chunks[1].IsBeginOfStream() || chunks[1].IsEndOfStream() {
		t.Fatalf("data chunk = %#v", chunks[1])
	}
	if !chunks[2].IsEndOfStream() {
		t.Fatalf("terminal chunk = %#v", chunks[2])
	}
}

func TestTTSStreamCreatesStreamIDWhenInputOmitsOne(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("hello.")},
		genx.NewTextEndOfStream(),
	}, doneErr: io.EOF}

	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/mpeg", func(_ context.Context, _ string, _ TTSMeta, _ string, emit func([]byte) error) error {
		return emit([]byte("audio"))
	})
	chunks := collectTransformerChunks(t, output)
	if len(chunks) != 3 {
		t.Fatalf("got %d output chunks, want BOS/data/EOS", len(chunks))
	}
	streamID := chunks[0].Ctrl.StreamID
	if streamID == "" || !chunks[0].IsBeginOfStream() || chunks[1].Ctrl.StreamID != streamID || chunks[2].Ctrl.StreamID != streamID || !chunks[2].IsEndOfStream() {
		t.Fatalf("route controls = %#v / %#v / %#v", chunks[0].Ctrl, chunks[1].Ctrl, chunks[2].Ctrl)
	}
}

func TestTTSStreamSkipsUnreadableSegments(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text(`，。<node id="tool_call"><function name="noop"></function></node>（https://example.com）`), Ctrl: &genx.StreamCtrl{StreamID: "input-stream"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "input-stream", EndOfStream: true}},
	}, doneErr: io.EOF}

	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/ogg", func(_ context.Context, text string, _ TTSMeta, _ string, _ func([]byte) error) error {
		t.Fatalf("synthesizer called for unreadable text %q", text)
		return nil
	})
	chunks := collectTransformerChunks(t, output)
	if len(chunks) != 2 || !chunks[0].IsBeginOfStream() || !chunks[1].IsEndOfStream() || chunks[0].Ctrl.StreamID != "input-stream" || chunks[1].Ctrl.StreamID != "input-stream" {
		t.Fatalf("output chunks = %#v", chunks)
	}
}

func TestTTSStreamBuffersInterleavedStreamIDs(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("好的，"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text("第二条消息已经来了，"), Ctrl: &genx.StreamCtrl{StreamID: "s2"}},
		{Part: genx.Text("我来讲一个。"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", EndOfStream: true}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s2", EndOfStream: true}},
	}, doneErr: io.EOF}

	// Segments are synthesized concurrently, so the two streams race and only
	// the per-stream pairing of StreamID to accumulated text is deterministic.
	var mu sync.Mutex
	var got []string
	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/ogg", func(_ context.Context, text string, meta TTSMeta, _ string, emit func([]byte) error) error {
		mu.Lock()
		got = append(got, meta.StreamID+":"+text)
		mu.Unlock()
		return emit([]byte("audio"))
	})
	_ = collectTransformerChunks(t, output)
	mu.Lock()
	defer mu.Unlock()
	slices.Sort(got)
	want := []string{"s1:好的，我来讲一个。", "s2:第二条消息已经来了，"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("synthesized texts = %#v, want %#v", got, want)
	}
}

// Every segment costs a fresh provider request, so synthesizing them one after
// another puts a full first-audio latency of silence at each segment boundary.
// A segment must therefore start while its predecessor is still emitting.
func TestTTSStreamSynthesizesAheadWhilePriorSegmentEmits(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("第一句话已经说完了。"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text("第二句话也说完了。"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text("第三句话同样说完了。"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", EndOfStream: true}},
	}, doneErr: io.EOF}

	started := make(chan string, 4)
	release := make(chan struct{})
	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/ogg", func(_ context.Context, text string, _ TTSMeta, _ string, emit func([]byte) error) error {
		started <- text
		<-release
		return emit([]byte("audio:" + text))
	})

	await := func(what string) string {
		t.Helper()
		select {
		case text := <-started:
			return text
		case <-time.After(5 * time.Second):
			t.Fatalf("%s never started while an earlier segment was still held", what)
			return ""
		}
	}
	// The two syntheses run concurrently, so which goroutine records itself
	// first is scheduling, not contract. What matters is that both are in
	// flight while neither has been allowed to finish.
	inFlight := []string{await("first segment"), await("lookahead segment")}
	slices.Sort(inFlight)
	if want := []string{"第一句话已经说完了。", "第二句话也说完了。"}; !reflect.DeepEqual(inFlight, want) {
		t.Fatalf("in-flight syntheses = %#v, want %#v", inFlight, want)
	}
	// Lookahead is bounded: the third segment waits for the first to be emitted
	// rather than opening a third concurrent provider session.
	select {
	case text := <-started:
		t.Fatalf("third segment %q started beyond the lookahead bound", text)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	chunks := collectTransformerChunks(t, output)
	var audio []string
	for _, chunk := range chunks {
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			audio = append(audio, string(blob.Data))
		}
	}
	want := []string{"audio:第一句话已经说完了。", "audio:第二句话也说完了。", "audio:第三句话同样说完了。"}
	if !reflect.DeepEqual(audio, want) {
		t.Fatalf("emitted audio = %#v, want %#v in segment order", audio, want)
	}
}

func TestTTSStreamPassesThroughNonTextWithoutFlushing(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("好的，"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: &genx.Blob{MIMEType: "application/json", Data: []byte(`{"tool":true}`)}, Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text("我来讲一个。"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", EndOfStream: true}},
	}, doneErr: io.EOF}

	var texts []string
	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/ogg", func(_ context.Context, text string, _ TTSMeta, _ string, emit func([]byte) error) error {
		texts = append(texts, text)
		return emit([]byte("audio"))
	})
	chunks := collectTransformerChunks(t, output)
	if want := []string{"好的，我来讲一个。"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("synthesized texts = %#v, want %#v", texts, want)
	}
	for _, chunk := range chunks {
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "application/json" {
			return
		}
	}
	t.Fatalf("non-text chunk was not passed through: %#v", chunks)
}

func TestTTSStreamReturnsProviderFailureAsRouteEOSThenStreamError(t *testing.T) {
	wantErr := errors.New("provider failed")
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("hello."), Ctrl: &genx.StreamCtrl{StreamID: "failed"}},
	}, doneErr: io.EOF}
	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/mpeg", func(context.Context, string, TTSMeta, string, func([]byte) error) error {
		return wantErr
	})
	chunks, err := collectTransformerChunksUntilError(output)
	if len(chunks) != 2 || !chunks[0].IsBeginOfStream() || !chunks[1].IsEndOfStream() || chunks[1].Ctrl.Error != wantErr.Error() {
		t.Fatalf("failure output = %#v", chunks)
	}
	if chunks[1].Ctrl.FailureClass != genx.FailureClassProvider {
		t.Fatalf("failure class = %q, want provider", chunks[1].Ctrl.FailureClass)
	}
	// The route EOS carries the error for the device; the stream itself then
	// reports the same cause so composition layers do not mistake a provider
	// failure for a clean end of output.
	if !errors.Is(err, wantErr) {
		t.Fatalf("terminal stream error = %v, want %v", err, wantErr)
	}
	if class, ok := genx.FailureClassOf(err); !ok || class != genx.FailureClassProvider {
		t.Fatalf("terminal failure class = (%q, %t), want provider", class, ok)
	}
}

func TestTTSStreamInterruptDiscardsUnpulledAudio(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("hello."), Ctrl: &genx.StreamCtrl{StreamID: "interrupted"}},
		{Ctrl: &genx.StreamCtrl{StreamID: "interrupted", Error: "caller interrupted"}},
	}, doneErr: io.EOF}
	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/mpeg", func(_ context.Context, _ string, _ TTSMeta, _ string, emit func([]byte) error) error {
		return emit([]byte("unpulled audio"))
	})
	<-output.Done()

	chunks := collectTransformerChunks(t, output)
	if len(chunks) != 2 || !chunks[0].IsBeginOfStream() || !chunks[1].IsEndOfStream() || chunks[1].Ctrl.Error != "caller interrupted" {
		t.Fatalf("interruption output = %#v", chunks)
	}
	for index, chunk := range chunks {
		if mimeType, ok := chunk.MIMEType(); !ok || mimeType != "audio/mpeg" {
			t.Fatalf("interruption chunk %d MIME = %q, %t", index, mimeType, ok)
		}
		if blob := chunk.Part.(*genx.Blob); len(blob.Data) != 0 {
			t.Fatalf("interruption chunk %d retained discarded audio %q", index, blob.Data)
		}
	}
}

func collectTransformerChunksUntilError(stream genx.Stream) ([]*genx.MessageChunk, error) {
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			return chunks, err
		}
		chunks = append(chunks, chunk)
	}
}

func collectTransformerChunks(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
				return chunks
			}
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

type testStream struct {
	chunks  []*genx.MessageChunk
	index   int
	doneErr error
}

func (s *testStream) Next() (*genx.MessageChunk, error) {
	if s.index < len(s.chunks) {
		chunk := s.chunks[s.index]
		s.index++
		return chunk, nil
	}
	return nil, s.doneErr
}

func (*testStream) Close() error               { return nil }
func (*testStream) CloseWithError(error) error { return nil }
