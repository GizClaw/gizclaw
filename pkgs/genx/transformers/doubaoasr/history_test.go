package doubaoasr

import (
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type historyTestOutput struct {
	chunks []*genx.MessageChunk
	failAt int
}

func (o *historyTestOutput) Push(chunk *genx.MessageChunk) error {
	if o.failAt > 0 && len(o.chunks)+1 == o.failAt {
		return errors.New("push failed")
	}
	o.chunks = append(o.chunks, chunk)
	return nil
}

func TestHistoryUserAudioEOSChunkDefaultsStreamID(t *testing.T) {
	chunk := historyUserAudioEOSChunk("", "")
	if chunk.Role != genx.RoleUser || chunk.Name != "transcript" {
		t.Fatalf("history EOS route = %#v", chunk)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.Label != genx.HistoryUserAudioLabel || chunk.Ctrl.StreamID != "audio" || !chunk.IsEndOfStream() {
		t.Fatalf("history EOS ctrl = %#v", chunk)
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/pcm" {
		t.Fatalf("history EOS blob = %#v", chunk.Part)
	}
}

func TestTimestampedHistoryAudioBufferBranches(t *testing.T) {
	var nilBuffer *timestampedHistoryAudioBuffer
	nilBuffer.reset()
	nilBuffer.append(nil, "")
	if got := nilBuffer.segment(0, 0); got != nil {
		t.Fatalf("nil segment = %v", got)
	}

	buffer := &timestampedHistoryAudioBuffer{}
	buffer.append(nil, "history")
	buffer.append(&genx.MessageChunk{Part: genx.Text("skip")}, "history")
	buffer.append(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}}, "history")
	buffer.append(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}}, "history")
	for _, chunk := range []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}}, Ctrl: &genx.StreamCtrl{Timestamp: 1000}},
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}}, Ctrl: &genx.StreamCtrl{Timestamp: 1040}},
	} {
		buffer.append(chunk, "history")
	}
	if len(buffer.blocks) != 2 || buffer.blocks[0].endMS != 40 || buffer.blocks[1].startMS != 40 {
		t.Fatalf("blocks = %#v", buffer.blocks)
	}
	if got := buffer.segment(30, 45); len(got) != 2 {
		t.Fatalf("segment(30,45) = %d chunks", len(got))
	}
	if got := buffer.segment(0, 0); len(got) != 2 || buffer.flushedMS != 60 {
		t.Fatalf("flush = %d chunks, flushedMS=%d", len(got), buffer.flushedMS)
	}
	if got := buffer.segment(0, 0); got != nil {
		t.Fatalf("second flush = %d chunks", len(got))
	}
	buffer.reset()
	if len(buffer.blocks) != 0 || buffer.haveTS || buffer.cursorMS != 0 || buffer.flushedMS != 0 {
		t.Fatalf("reset buffer = %#v", buffer)
	}
}

func TestPushHistoryAudioSegmentBranches(t *testing.T) {
	if got := canonicalHistoryAudioMIME("not a mime"); got != "not a mime" {
		t.Fatalf("canonical fallback = %q", got)
	}
	if err := pushHistoryAudioSegment(&historyTestOutput{}, "route", nil); err != nil {
		t.Fatalf("empty segment error = %v", err)
	}
	chunks := []*genx.MessageChunk{{Part: &genx.Blob{MIMEType: " audio/pcm ", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{}}}
	output := &historyTestOutput{}
	if err := pushHistoryAudioSegment(output, "route", chunks); err != nil {
		t.Fatalf("pushHistoryAudioSegment() error = %v", err)
	}
	if len(output.chunks) != 3 || !output.chunks[0].IsBeginOfStream() || !output.chunks[2].IsEndOfStream() {
		t.Fatalf("history lifecycle = %#v", output.chunks)
	}
	for _, test := range []struct {
		name    string
		failAt  int
		wantErr string
	}{
		{name: "BOS", failAt: 1, wantErr: "bos"},
		{name: "data", failAt: 2, wantErr: "push failed"},
		{name: "EOS", failAt: 3, wantErr: "eos"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := pushHistoryAudioSegment(&historyTestOutput{failAt: test.failAt}, "route", chunks)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
