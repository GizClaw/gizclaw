package chatroom

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

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

type testMux struct{}

func (testMux) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return nil, errors.New("not used")
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
