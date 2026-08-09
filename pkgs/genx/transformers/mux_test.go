package transformers

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type testTransformer struct {
	fn func(context.Context, genx.Stream) (genx.Stream, error)
}

func (t testTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.fn(ctx, input)
}

type testStream struct {
	chunks  []*genx.MessageChunk
	idx     int
	doneErr error
}

func (s *testStream) Next() (*genx.MessageChunk, error) {
	if s.idx < len(s.chunks) {
		v := s.chunks[s.idx]
		s.idx++
		return v, nil
	}
	if s.doneErr == nil {
		return nil, genx.ErrDone
	}
	return nil, s.doneErr
}

func (s *testStream) Close() error               { return nil }
func (s *testStream) CloseWithError(error) error { return nil }

func TestMuxTransformRoutesToRegisteredTransformer(t *testing.T) {
	m := NewMux()
	called := false
	err := m.Handle("foo/bar", testTransformer{fn: func(_ context.Context, input genx.Stream) (genx.Stream, error) {
		called = true
		return input, nil
	}})
	if err != nil {
		t.Fatalf("handle failed: %v", err)
	}

	out, err := m.Transform(context.Background(), "foo/bar", &testStream{chunks: []*genx.MessageChunk{{Part: genx.Text("ok")}}})
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	if !called {
		t.Fatal("expected transformer to be called")
	}

	chunk, err := out.Next()
	if err != nil {
		t.Fatalf("next failed: %v", err)
	}
	if chunk == nil || chunk.Part.(genx.Text) != "ok" {
		t.Fatalf("unexpected chunk: %#v", chunk)
	}
}

func TestMuxHandleRejectsDuplicate(t *testing.T) {
	m := NewMux()
	tf := testTransformer{fn: func(_ context.Context, input genx.Stream) (genx.Stream, error) { return input, nil }}
	if err := m.Handle("dup", tf); err != nil {
		t.Fatalf("first handle failed: %v", err)
	}
	if err := m.Handle("dup", tf); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}

func TestDefaultMuxFacade(t *testing.T) {
	previous := DefaultMux
	DefaultMux = NewMux()
	t.Cleanup(func() { DefaultMux = previous })

	if _, err := Transform(context.Background(), "missing", &testStream{}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Transform(missing) error = %v", err)
	}
	if err := Handle("nil", nil); err != nil {
		t.Fatalf("Handle(nil) error = %v", err)
	}
	if _, err := Transform(context.Background(), "nil", &testStream{}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Transform(nil) error = %v", err)
	}

	want := &testStream{chunks: []*genx.MessageChunk{{Part: genx.Text("ok")}}}
	if err := Handle("default", testTransformer{fn: func(_ context.Context, input genx.Stream) (genx.Stream, error) {
		if input != want {
			t.Fatalf("input = %p, want %p", input, want)
		}
		return input, nil
	}}); err != nil {
		t.Fatalf("Handle(default) error = %v", err)
	}
	if got, err := Transform(context.Background(), "default", want); err != nil || got != want {
		t.Fatalf("Transform(default) = (%p, %v), want %p", got, err, want)
	}
}

func TestStreamToReaderTreatsErrDoneAsEnd(t *testing.T) {
	r := streamToReader(&testStream{chunks: []*genx.MessageChunk{{Part: genx.Text("hello")}}, doneErr: genx.ErrDone})
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read all failed: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected reader content: %q", string(b))
	}
}

func TestStreamToReaderPropagatesNonDoneError(t *testing.T) {
	wantErr := errors.New("boom")
	r := streamToReader(&testStream{doneErr: wantErr})
	_, err := io.ReadAll(r)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestStreamToReaderSkipsNilAndNonTextChunks(t *testing.T) {
	r := streamToReader(&testStream{chunks: []*genx.MessageChunk{
		nil,
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}},
		{Part: genx.Text("text")},
	}})
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got := string(b); got != "text" {
		t.Fatalf("ReadAll() = %q", got)
	}
}
