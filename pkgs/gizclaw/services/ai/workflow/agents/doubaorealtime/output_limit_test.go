package doubaorealtime

import (
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestOutputLimitStreamTruncatesUnicodePerAssistantStream(t *testing.T) {
	source := &chunkListStream{chunks: []*genx.MessageChunk{
		textChunk("a", "assistant", genx.RoleModel, "你", true, false, ""),
		textChunk("a", "assistant", genx.RoleModel, "好🙂呀", false, false, ""),
		textChunk("b", "assistant", genx.RoleModel, "AB", true, false, ""),
		textChunk("a", "assistant", genx.RoleModel, "ignored", false, false, ""),
		textChunk("a", "assistant", genx.RoleModel, "", false, true, "provider terminal"),
		textChunk("b", "assistant", genx.RoleModel, "C", false, true, ""),
	}}
	stream := &outputLimitStream{source: source, limit: 3, counts: make(map[string]int)}

	var got []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		got = append(got, chunk)
	}
	if len(got) != 5 {
		t.Fatalf("chunks = %d, want 5: %#v", len(got), got)
	}
	if text := got[1].Part.(genx.Text); text != "好🙂" {
		t.Fatalf("crossing chunk = %q, want %q", text, "好🙂")
	}
	if text := got[3].Part.(genx.Text); text != "" || !got[3].IsEndOfStream() || got[3].Ctrl.Error != "provider terminal" {
		t.Fatalf("terminal chunk = %#v, want empty provider error EOS", got[3])
	}
	if text := got[4].Part.(genx.Text); text != "C" || !got[4].IsEndOfStream() {
		t.Fatalf("independent stream terminal = %#v", got[4])
	}
}

func TestOutputLimitStreamLeavesOtherRoutesUnchanged(t *testing.T) {
	transcript := textChunk("turn", "transcript", genx.RoleUser, "用户输入", true, true, "")
	audio := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant"},
	}
	stream := &outputLimitStream{
		source: &chunkListStream{chunks: []*genx.MessageChunk{transcript, audio}},
		limit:  1,
		counts: make(map[string]int),
	}
	for _, want := range []*genx.MessageChunk{transcript, audio} {
		got, err := stream.Next()
		if err != nil || got != want {
			t.Fatalf("Next() = %#v, %v; want original %#v", got, err, want)
		}
	}
}

func TestOutputLimitStreamResetsSameIDAtNextBOS(t *testing.T) {
	stream := &outputLimitStream{
		source: &chunkListStream{chunks: []*genx.MessageChunk{
			textChunk("turn", "assistant", genx.RoleModel, "one", true, true, ""),
			textChunk("turn", "assistant", genx.RoleModel, "two", true, true, ""),
		}},
		limit:  2,
		counts: make(map[string]int),
	}
	for _, want := range []genx.Text{"on", "tw"} {
		chunk, err := stream.Next()
		if err != nil || chunk.Part != want {
			t.Fatalf("Next() = %#v, %v; want %q", chunk, err, want)
		}
	}
}

func textChunk(streamID, label string, role genx.Role, text string, bos, eos bool, errText string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: role,
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{
			StreamID: streamID, Label: label, BeginOfStream: bos, EndOfStream: eos, Error: errText,
		},
	}
}

type chunkListStream struct {
	chunks []*genx.MessageChunk
}

func (s *chunkListStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *chunkListStream) Close() error { return nil }

func (s *chunkListStream) CloseWithError(error) error { return nil }
