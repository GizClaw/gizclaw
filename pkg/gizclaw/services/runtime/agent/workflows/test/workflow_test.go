package testworkflow

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/agent"
)

func TestWorkflowEchoesTextChunks(t *testing.T) {
	transformer, err := (&Workflow{}).NewAgent(context.Background(), agent.Spec{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	out, err := transformer.Transform(context.Background(), "demo", &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text("hello")},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	chunk, err := out.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if chunk.Role != genx.RoleModel {
		t.Fatalf("role = %q, want model", chunk.Role)
	}
	if got := string(chunk.Part.(genx.Text)); got != "test: hello" {
		t.Fatalf("text = %q, want test: hello", got)
	}

	chunk, err = out.Next()
	if err != nil {
		t.Fatalf("second Next() error = %v", err)
	}
	if blob := chunk.Part.(*genx.Blob); blob.MIMEType != "audio/pcm" || len(blob.Data) != 2 {
		t.Fatalf("blob = %#v", blob)
	}
	if _, err := out.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal Next() err = %v, want EOF", err)
	}
}

func TestWorkflowCustomPrefixAndClose(t *testing.T) {
	transformer, err := (&Workflow{Prefix: "echo: "}).NewAgent(context.Background(), agent.Spec{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := &closeRecordingStream{sliceStream: sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text("hello")},
	}}}
	out, err := transformer.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := out.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got := string(chunk.Part.(genx.Text)); got != "echo: hello" {
		t.Fatalf("text = %q, want echo: hello", got)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !input.closed {
		t.Fatal("input was not closed")
	}
}

func TestWorkflowErrorsAndCloseWithError(t *testing.T) {
	transformer, err := (&Workflow{}).NewAgent(context.Background(), agent.Spec{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if _, err := transformer.Transform(context.Background(), "demo", nil); err == nil {
		t.Fatal("Transform(nil) error = nil")
	}

	input := &closeRecordingStream{}
	out, err := transformer.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	wantErr := errors.New("boom")
	if err := out.CloseWithError(wantErr); !errors.Is(err, wantErr) {
		t.Fatalf("CloseWithError() error = %v, want %v", err, wantErr)
	}
	if !input.closedWithError {
		t.Fatal("input CloseWithError was not called")
	}

	input = &closeRecordingStream{}
	out, err = transformer.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := out.CloseWithError(io.EOF); err != nil {
		t.Fatalf("CloseWithError(EOF) error = %v", err)
	}
	if !input.closed {
		t.Fatal("input was not closed for EOF")
	}
}

func TestWorkflowContextCancellation(t *testing.T) {
	transformer, err := (&Workflow{}).NewAgent(context.Background(), agent.Spec{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out, err := transformer.Transform(ctx, "demo", &sliceStream{chunks: []*genx.MessageChunk{{Part: genx.Text("hello")}}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	cancel()
	if _, err := out.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next() error = %v, want context.Canceled", err)
	}
}

type sliceStream struct {
	chunks []*genx.MessageChunk
	idx    int
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if s.idx >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.idx]
	s.idx++
	return chunk, nil
}

func (s *sliceStream) Close() error               { return nil }
func (s *sliceStream) CloseWithError(error) error { return nil }

type closeRecordingStream struct {
	sliceStream
	closed          bool
	closedWithError bool
	err             error
}

func (s *closeRecordingStream) Close() error {
	s.closed = true
	return nil
}

func (s *closeRecordingStream) CloseWithError(err error) error {
	s.closedWithError = true
	s.err = err
	return err
}
