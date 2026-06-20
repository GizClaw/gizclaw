package transformers

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

func TestRunTTSTransformPreservesInputStreamIDAndNormalizesAudio(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{
			Role: genx.RoleUser,
			Name: "gear",
			Part: genx.Text("你好，世界。"),
			Ctrl: &genx.StreamCtrl{StreamID: "input-stream"},
		},
		{
			Role: genx.RoleUser,
			Name: "gear",
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: "input-stream", EndOfStream: true},
		},
	}, doneErr: io.EOF}

	output := newBufferStream(8)
	var texts []string
	runTTSTransform(context.Background(), input, output, "audio/mpeg", func(_ context.Context, text string, meta ttsChunkMeta, mimeType string, out *bufferStream) error {
		texts = append(texts, text)
		data := append(fakeID3Header(), []byte("audio:"+text)...)
		return pushTTSAudioChunk(out, meta, mimeType, data)
	})

	if want := []string{"你好，世界。"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("synthesized texts = %#v, want %#v", texts, want)
	}

	chunks := collectTransformerChunks(t, output)
	if len(chunks) != 2 {
		t.Fatalf("got %d output chunks, want 2", len(chunks))
	}
	for i, chunk := range chunks[:1] {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "input-stream" {
			t.Fatalf("audio chunk %d StreamID = %#v, want input-stream", i, chunk.Ctrl)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok {
			t.Fatalf("audio chunk %d part = %T, want *genx.Blob", i, chunk.Part)
		}
		if bytes.Contains(blob.Data, []byte("ID3")) {
			t.Fatalf("audio chunk %d still contains ID3 tag", i)
		}
		if chunk.Role != genx.RoleUser || chunk.Name != "gear" {
			t.Fatalf("audio chunk %d meta = role %q name %q", i, chunk.Role, chunk.Name)
		}
	}
	eos := chunks[1]
	if eos.Ctrl == nil || !eos.Ctrl.EndOfStream || eos.Ctrl.StreamID != "input-stream" {
		t.Fatalf("eos ctrl = %#v, want input-stream eos", eos.Ctrl)
	}
}

func TestRunTTSTransformDoesNotCreateStreamID(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("hello.")},
		genx.NewTextEndOfStream(),
	}, doneErr: io.EOF}

	output := newBufferStream(4)
	runTTSTransform(context.Background(), input, output, "audio/mpeg", func(_ context.Context, _ string, meta ttsChunkMeta, mimeType string, out *bufferStream) error {
		return pushTTSAudioChunk(out, meta, mimeType, []byte("audio"))
	})

	chunks := collectTransformerChunks(t, output)
	if len(chunks) != 2 {
		t.Fatalf("got %d output chunks, want 2", len(chunks))
	}
	if chunks[0].Ctrl != nil {
		t.Fatalf("audio chunk ctrl = %#v, want nil", chunks[0].Ctrl)
	}
	if chunks[1].Ctrl == nil || !chunks[1].Ctrl.EndOfStream || chunks[1].Ctrl.StreamID != "" {
		t.Fatalf("eos ctrl = %#v, want eos without stream id", chunks[1].Ctrl)
	}
}

func TestRunTTSTransformSkipsUnreadableSegments(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{
			Part: genx.Text(`，。<node id="tool_call"><function name="noop"></function></node>（https://example.com）`),
			Ctrl: &genx.StreamCtrl{StreamID: "input-stream"},
		},
		{
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: "input-stream", EndOfStream: true},
		},
	}, doneErr: io.EOF}

	output := newBufferStream(4)
	runTTSTransform(context.Background(), input, output, "audio/ogg", func(_ context.Context, text string, _ ttsChunkMeta, _ string, _ *bufferStream) error {
		t.Fatalf("synthesizer called for unreadable text %q", text)
		return nil
	})

	chunks := collectTransformerChunks(t, output)
	if len(chunks) != 1 {
		t.Fatalf("got %d output chunks, want only eos", len(chunks))
	}
	if chunks[0].Ctrl == nil || !chunks[0].Ctrl.EndOfStream || chunks[0].Ctrl.StreamID != "input-stream" {
		t.Fatalf("eos ctrl = %#v, want input-stream eos", chunks[0].Ctrl)
	}
}

func TestRunTTSTransformBuffersByStreamID(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("好的，"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text("第二条消息已经来了，"), Ctrl: &genx.StreamCtrl{StreamID: "s2"}},
		{Part: genx.Text("我来讲一个。"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", EndOfStream: true}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s2", EndOfStream: true}},
	}, doneErr: io.EOF}

	output := newBufferStream(8)
	var got []string
	runTTSTransform(context.Background(), input, output, "audio/ogg", func(_ context.Context, text string, meta ttsChunkMeta, mimeType string, out *bufferStream) error {
		got = append(got, meta.StreamID+":"+text)
		return pushTTSAudioChunk(out, meta, mimeType, []byte("audio"))
	})

	want := []string{
		"s2:第二条消息已经来了，",
		"s1:好的，我来讲一个。",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("synthesized texts = %#v, want %#v", got, want)
	}
}

func TestRunTTSTransformDoesNotFlushOnNonTextChunk(t *testing.T) {
	input := &testStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("好的，"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: &genx.Blob{MIMEType: "application/json", Data: []byte(`{"tool":true}`)}, Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text("我来讲一个。"), Ctrl: &genx.StreamCtrl{StreamID: "s1"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", EndOfStream: true}},
	}, doneErr: io.EOF}

	output := newBufferStream(8)
	var texts []string
	runTTSTransform(context.Background(), input, output, "audio/ogg", func(_ context.Context, text string, meta ttsChunkMeta, mimeType string, out *bufferStream) error {
		texts = append(texts, text)
		return pushTTSAudioChunk(out, meta, mimeType, []byte("audio"))
	})

	if want := []string{"好的，我来讲一个。"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("synthesized texts = %#v, want %#v", texts, want)
	}

	chunks := collectTransformerChunks(t, output)
	var sawJSON bool
	for _, chunk := range chunks {
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "application/json" {
			sawJSON = true
		}
	}
	if !sawJSON {
		t.Fatalf("non-text chunk was not passed through: %#v", chunks)
	}
}

func collectTransformerChunks(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			if err == io.EOF || err == genx.ErrDone {
				return chunks
			}
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
}
