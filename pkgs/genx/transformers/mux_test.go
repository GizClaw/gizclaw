package transformers

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type testTransformer struct {
	fn func(context.Context, string, genx.Stream) (genx.Stream, error)
}

func (t testTransformer) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return t.fn(ctx, pattern, input)
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
	err := m.Handle("foo/bar", testTransformer{fn: func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
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
	tf := testTransformer{fn: func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) { return input, nil }}
	if err := m.Handle("dup", tf); err != nil {
		t.Fatalf("first handle failed: %v", err)
	}
	if err := m.Handle("dup", tf); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate error, got: %v", err)
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

func TestBufferStreamConcurrentCloseIsIdempotent(t *testing.T) {
	stream := newBufferStream(1)
	wantErr := errors.New("closed with error")
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			if i%2 == 0 {
				_ = stream.Close()
				return
			}
			_ = stream.CloseWithError(wantErr)
		})
	}
	wg.Wait()
	select {
	case <-stream.Done():
	default:
		t.Fatal("Done() remained open after Close")
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) && !errors.Is(err, wantErr) {
		t.Fatalf("Next() error = %v, want EOF or close error", err)
	}
}
