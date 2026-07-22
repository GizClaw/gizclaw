package streamkit

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

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
	if len(chunks) != 2 {
		t.Fatalf("got %d output chunks, want 2", len(chunks))
	}
	for index, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "input-stream" || chunk.Ctrl.Label != "speech" {
			t.Fatalf("chunk %d control = %#v", index, chunk.Ctrl)
		}
		if chunk.Role != genx.RoleUser || chunk.Name != "gear" {
			t.Fatalf("chunk %d metadata = role %q name %q", index, chunk.Role, chunk.Name)
		}
	}
	if !chunks[1].IsEndOfStream() {
		t.Fatalf("terminal chunk = %#v", chunks[1])
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
	if len(chunks) != 2 {
		t.Fatalf("got %d output chunks, want 2", len(chunks))
	}
	streamID := chunks[0].Ctrl.StreamID
	if streamID == "" || chunks[1].Ctrl.StreamID != streamID || !chunks[1].IsEndOfStream() {
		t.Fatalf("route controls = %#v / %#v", chunks[0].Ctrl, chunks[1].Ctrl)
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
	if len(chunks) != 1 || !chunks[0].IsEndOfStream() || chunks[0].Ctrl.StreamID != "input-stream" {
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

	var got []string
	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/ogg", func(_ context.Context, text string, meta TTSMeta, _ string, emit func([]byte) error) error {
		got = append(got, meta.StreamID+":"+text)
		return emit([]byte("audio"))
	})
	_ = collectTransformerChunks(t, output)
	want := []string{"s2:第二条消息已经来了，", "s1:好的，我来讲一个。"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("synthesized texts = %#v, want %#v", got, want)
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

func TestTTSStreamReturnsProviderFailureAsRouteEOS(t *testing.T) {
	wantErr := errors.New("provider failed")
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("hello."), Ctrl: &genx.StreamCtrl{StreamID: "failed"}},
	}, doneErr: io.EOF}
	output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/mpeg", func(context.Context, string, TTSMeta, string, func([]byte) error) error {
		return wantErr
	})
	chunks := collectTransformerChunks(t, output)
	if len(chunks) != 1 || !chunks[0].IsEndOfStream() || chunks[0].Ctrl.Error != wantErr.Error() {
		t.Fatalf("failure output = %#v", chunks)
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
	if len(chunks) != 1 || !chunks[0].IsEndOfStream() || chunks[0].Ctrl.Error != "caller interrupted" {
		t.Fatalf("interruption output = %#v", chunks)
	}
	if mimeType, ok := chunks[0].MIMEType(); !ok || mimeType != "audio/mpeg" {
		t.Fatalf("interruption MIME = %q, %t", mimeType, ok)
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
