package streamkit

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type testStreamingTTS struct {
	send   func(string) error
	flush  func() error
	finish func() error
	done   chan struct{}
	err    error
	once   sync.Once
}

func (s *testStreamingTTS) SendText(_ context.Context, text string) error { return s.send(text) }
func (s *testStreamingTTS) Flush(context.Context) error                   { return s.flush() }
func (s *testStreamingTTS) Finish(context.Context) error {
	if s.finish != nil {
		if err := s.finish(); err != nil {
			return err
		}
	}
	s.once.Do(func() { close(s.done) })
	return nil
}
func (s *testStreamingTTS) Close() error {
	s.once.Do(func() { s.err = context.Canceled; close(s.done) })
	return nil
}
func (s *testStreamingTTS) Done() <-chan struct{} { return s.done }
func (s *testStreamingTTS) Err() error            { return s.err }

func TestStreamingTTSSendsIncompleteTextWithoutClientSegmentation(t *testing.T) {
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	sent := make(chan string, 8)
	session := &testStreamingTTS{done: make(chan struct{}), send: func(text string) error { sent <- text; return nil }, flush: func() error { return nil }}
	opens := 0
	output := NewStreamingTTSStream(t.Context(), input.Stream(), OutputConfig{}, "audio/pcm", func(_ context.Context, meta TTSMeta, _ func(context.Context, []byte) error) (TTSStreamingSession, error) {
		opens++
		if meta.MessageID != "message" {
			t.Errorf("message identity lost: %+v", meta)
		}
		return session, nil
	})
	defer output.Close()
	for _, text := range []string{"1,", "000", "元。他说：“", "Hello", " ", "world。”"} {
		if err := input.Add(&genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "route", MessageID: "message"}}); err != nil {
			t.Fatal(err)
		}
		if text == "1," {
			// Client segmentation would still be waiting for the rest of
			// this number. A native streaming provider receives it now.
			select {
			case got := <-sent:
				if got != text {
					t.Fatalf("first text = %q", got)
				}
			case <-t.Context().Done():
				t.Fatal("incomplete input was buffered by a client segmenter")
			}
		}
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	collectTransformerChunks(t, output)
	close(sent)
	var rest strings.Builder
	for text := range sent {
		rest.WriteString(text)
	}
	if opens != 1 || rest.String() != "000元。他说：“Hello world。”" {
		t.Fatalf("opens=%d text=%q", opens, rest.String())
	}
}

func TestStreamingTTSFlushKeepsSessionAndMessageIdentity(t *testing.T) {
	opens, flushes := 0, 0
	input := &testStream{doneErr: io.EOF, chunks: []*genx.MessageChunk{
		{Part: genx.Text("第一轮"), Ctrl: &genx.StreamCtrl{StreamID: "route", MessageID: "first", MessageEnd: true}},
		{Part: genx.Text("第二轮"), Ctrl: &genx.StreamCtrl{StreamID: "route", MessageID: "second", EndOfStream: true}},
	}}
	output := NewStreamingTTSStream(t.Context(), input, OutputConfig{}, "audio/pcm", func(ctx context.Context, _ TTSMeta, emit func(context.Context, []byte) error) (TTSStreamingSession, error) {
		opens++
		var pending strings.Builder
		session := &testStreamingTTS{done: make(chan struct{})}
		session.send = func(text string) error { pending.WriteString(text); return nil }
		drain := func() error { text := pending.String(); pending.Reset(); return emit(ctx, []byte(text)) }
		session.flush = func() error { flushes++; return drain() }
		session.finish = drain
		return session, nil
	})
	chunks := collectTransformerChunks(t, output)
	audio := 0
	for _, chunk := range chunks {
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			continue
		}
		wantID, wantText := "first", "第一轮"
		if audio == 1 {
			wantID, wantText = "second", "第二轮"
		}
		if chunk.Ctrl.MessageID != wantID || string(blob.Data) != wantText {
			t.Fatalf("message audio identity changed: %+v %q", chunk.Ctrl, blob.Data)
		}
		audio++
	}
	if opens != 1 || flushes != 1 || audio != 2 {
		t.Fatalf("opens=%d flushes=%d audio=%d", opens, flushes, audio)
	}
}

func TestStreamingTTSProviderFailureReleasesStalledInput(t *testing.T) {
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	opened := make(chan *testStreamingTTS, 1)
	output := NewStreamingTTSStream(t.Context(), input.Stream(), OutputConfig{}, "audio/pcm", func(context.Context, TTSMeta, func(context.Context, []byte) error) (TTSStreamingSession, error) {
		session := &testStreamingTTS{done: make(chan struct{}), send: func(string) error { return nil }, flush: func() error { return nil }}
		opened <- session
		return session, nil
	})
	if err := input.Add(&genx.MessageChunk{Part: genx.Text("正文"), Ctrl: &genx.StreamCtrl{StreamID: "route"}}); err != nil {
		t.Fatal(err)
	}
	session := <-opened
	session.once.Do(func() { session.err = errors.New("provider failed"); close(session.done) })
	chunks, err := collectTransformerChunksUntilError(output)
	if err == nil || !strings.Contains(err.Error(), "provider failed") || len(chunks) < 2 {
		t.Fatalf("provider failure disappeared: chunks=%d err=%v", len(chunks), err)
	}
}

func TestStreamingTTSPreservesProviderNewlineBoundaries(t *testing.T) {
	input := &testStream{doneErr: io.EOF, chunks: []*genx.MessageChunk{
		{Part: genx.Text("第一行\r"), Ctrl: &genx.StreamCtrl{StreamID: "route"}},
		{Part: genx.Text("\n第二行\n"), Ctrl: &genx.StreamCtrl{StreamID: "route"}},
	}}
	var sent strings.Builder
	output := NewStreamingTTSStream(t.Context(), input, OutputConfig{}, "audio/pcm", func(context.Context, TTSMeta, func(context.Context, []byte) error) (TTSStreamingSession, error) {
		return &testStreamingTTS{done: make(chan struct{}), send: func(text string) error { sent.WriteString(text); return nil }, flush: func() error { return nil }}, nil
	})
	collectTransformerChunks(t, output)
	if sent.String() != "第一行\n第二行\n" {
		t.Fatalf("provider sentence punctuation changed: %q", sent.String())
	}
}
