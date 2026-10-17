package doubaorealtime

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestRealtimeHistoryAudioOwnsEachTranscriptSegment(t *testing.T) {
	history := &realtimeHistoryAudio{}
	data := []byte{1, 0, 2, 0}
	chunk := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: "microphone", BeginOfStream: true}}
	if err := history.append(chunk, "microphone"); err != nil {
		t.Fatal(err)
	}
	data[0] = 9
	first := history.take("microphone:rt:1")
	if len(first) != 1 || !bytes.Equal(first[0].Part.(*genx.Blob).Data, []byte{1, 0, 2, 0}) || first[0].Ctrl.StreamID != "microphone:rt:1" {
		t.Fatalf("first recording = %#v", first)
	}
	if chunk.Ctrl.StreamID != "microphone" || !chunk.IsBeginOfStream() {
		t.Fatal("recording mutated the live input")
	}
	if got := history.take("microphone:rt:2"); len(got) != 0 {
		t.Fatal("an utterance reused the previous recording")
	}
	if err := history.append(chunk, "microphone"); err != nil {
		t.Fatal(err)
	}
	second := history.take("microphone:rt:2")
	if len(second) != 1 || second[0].Part.(*genx.Blob).Data[0] != 9 || second[0].Ctrl.StreamID != "microphone:rt:2" {
		t.Fatalf("second recording = %#v", second)
	}
	if err := history.append(chunk, "microphone"); err != nil {
		t.Fatal(err)
	}
	if err := history.append(chunk, "replacement"); err != nil {
		t.Fatal(err)
	}
	if got := history.take("replacement:rt:1"); len(got) != 1 {
		t.Fatalf("replacement retained %d chunks, want only its own input", len(got))
	}
}

func TestRealtimeHistoryAudioRejectsOversizedRecordingUntilNextSegment(t *testing.T) {
	history := &realtimeHistoryAudio{}
	tooLarge := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, realtimeHistoryAudioMaxBytes+1)}}
	if err := history.append(tooLarge, "input"); err == nil {
		t.Fatal("oversized input was accepted")
	}
	next := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}}
	if err := history.append(next, "input"); err != nil {
		t.Fatal(err)
	}
	if got := history.take("input:rt:1"); len(got) != 0 {
		t.Fatal("an incomplete recording was presented as complete")
	}
	if err := history.append(next, "input"); err != nil {
		t.Fatal(err)
	}
	if got := history.take("input:rt:2"); len(got) != 1 {
		t.Fatal("recording did not recover for the next utterance")
	}
}

func TestRealtimeHistoryAudioConcurrentInputAndTranscriptCompletion(t *testing.T) {
	history := &realtimeHistoryAudio{}
	const count = 1000
	ready := make(chan error)
	go func() {
		defer close(ready)
		for i := range count {
			ready <- history.append(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{byte(i), byte(i >> 8)}}}, "input")
		}
	}()
	seen := make(map[int]bool)
	collect := func(chunks []*genx.MessageChunk) {
		for _, chunk := range chunks {
			data := chunk.Part.(*genx.Blob).Data
			id := int(data[0]) | int(data[1])<<8
			if seen[id] {
				t.Errorf("audio frame %d was recorded twice", id)
			}
			seen[id] = true
		}
	}
	for err := range ready {
		if err != nil {
			t.Error(err)
		}
		collect(history.take("input:rt:1"))
	}
	collect(history.take("input:rt:2"))
	if len(seen) != count {
		t.Fatalf("recorded %d distinct frames, want %d", len(seen), count)
	}
}

type failingHistoryOutput struct {
	chunks []*genx.MessageChunk
	failAt int
}

func (o *failingHistoryOutput) Push(chunk *genx.MessageChunk) error {
	if o.failAt > 0 && len(o.chunks)+1 == o.failAt {
		return errors.New("push failed")
	}
	o.chunks = append(o.chunks, chunk)
	return nil
}

func TestHistoryAudioRouteDefaultsAndErrors(t *testing.T) {
	if got := canonicalHistoryAudioMIME("not a mime"); got != "audio/pcm" {
		t.Fatalf("canonicalHistoryAudioMIME() = %q", got)
	}
	bos := historyUserAudioBOSChunk("", "")
	eos := historyUserAudioEOSChunk("", "")
	if bos.Ctrl.StreamID != "audio" || eos.Ctrl.StreamID != "audio" || !bos.IsBeginOfStream() || !eos.IsEndOfStream() {
		t.Fatalf("default history lifecycle = %#v / %#v", bos, eos)
	}

	var nilRoutes *doubaoRealtimeHistoryRoutes
	if err := nilRoutes.push(nil, nil, ""); err != nil {
		t.Fatalf("nil routes push() error = %v", err)
	}
	if err := nilRoutes.close(nil, "", ""); err != nil {
		t.Fatalf("nil routes close() error = %v", err)
	}

	source := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}}
	for _, test := range []struct {
		name    string
		failAt  int
		close   bool
		wantErr string
	}{
		{name: "BOS", failAt: 1, wantErr: "bos"},
		{name: "data", failAt: 2, wantErr: "data"},
		{name: "close BOS", failAt: 1, close: true, wantErr: "bos"},
		{name: "close EOS", failAt: 2, close: true, wantErr: "eos"},
	} {
		t.Run(test.name, func(t *testing.T) {
			routes := newDoubaoRealtimeHistoryRoutes()
			output := &failingHistoryOutput{failAt: test.failAt}
			var err error
			if test.close {
				err = routes.close(output, "route", "audio/opus")
			} else {
				err = routes.push(output, source, "route")
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestHistoryAudioRoutesCloseMatchingChannels(t *testing.T) {
	routes := newDoubaoRealtimeHistoryRoutes()
	output := &failingHistoryOutput{}
	for _, mimeType := range []string{"audio/opus", "audio/pcm"} {
		chunk := &genx.MessageChunk{Part: &genx.Blob{MIMEType: mimeType, Data: []byte{1}}}
		if err := routes.push(output, chunk, "route"); err != nil {
			t.Fatalf("push(%s) error = %v", mimeType, err)
		}
	}
	if len(routes.open) != 2 {
		t.Fatalf("open routes = %d, want 2", len(routes.open))
	}
	if err := routes.close(output, " route ", " AUDIO/OPUS "); err != nil {
		t.Fatalf("close(audio/opus) error = %v", err)
	}
	if len(routes.open) != 1 {
		t.Fatalf("open routes after filtered close = %d, want 1", len(routes.open))
	}
	if err := routes.close(output, "route", ""); err != nil {
		t.Fatalf("close(all) error = %v", err)
	}
	if len(routes.open) != 0 {
		t.Fatalf("open routes after close = %d", len(routes.open))
	}
}

func TestTimestampedHistoryAudioBuffer(t *testing.T) {
	var nilBuffer *timestampedHistoryAudioBuffer
	nilBuffer.reset()
	nilBuffer.append(nil, "")
	if got := nilBuffer.segment(0, 0); got != nil {
		t.Fatalf("nil buffer segment = %v", got)
	}

	buffer := &timestampedHistoryAudioBuffer{}
	buffer.append(nil, "history")
	buffer.append(&genx.MessageChunk{Part: genx.Text("skip")}, "history")
	buffer.append(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}}, "history")
	buffer.append(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}}, "history")
	if len(buffer.blocks) != 0 {
		t.Fatalf("invalid appends created %d blocks", len(buffer.blocks))
	}

	first := &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}},
		Ctrl: &genx.StreamCtrl{Timestamp: 1000, BeginOfStream: true, EndOfStream: true, Error: "source"},
	}
	second := &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}},
		Ctrl: &genx.StreamCtrl{Timestamp: 1040},
	}
	buffer.append(first, "history")
	buffer.append(second, "history")
	if len(buffer.blocks) != 2 || buffer.blocks[0].startMS != 0 || buffer.blocks[0].endMS != 40 ||
		buffer.blocks[1].startMS != 40 || buffer.blocks[1].endMS != 60 {
		t.Fatalf("timestamped blocks = %#v", buffer.blocks)
	}
	if first.Ctrl.StreamID != "" || !first.IsBeginOfStream() || !first.IsEndOfStream() || first.Ctrl.Error != "source" {
		t.Fatalf("append mutated source = %#v", first.Ctrl)
	}

	explicit := buffer.segment(30, 45)
	if len(explicit) != 2 {
		t.Fatalf("segment(30,45) = %d chunks, want 2", len(explicit))
	}
	flushed := buffer.segment(0, 0)
	if len(flushed) != 2 || buffer.flushedMS != 60 {
		t.Fatalf("flush segment = %d chunks, flushedMS = %d", len(flushed), buffer.flushedMS)
	}
	if got := buffer.segment(0, 0); got != nil {
		t.Fatalf("second flush = %d chunks, want none", len(got))
	}
	if got := buffer.segment(50, 50); got != nil {
		t.Fatalf("empty explicit range = %d chunks", len(got))
	}
	buffer.reset()
	if len(buffer.blocks) != 0 || buffer.baseTS != 0 || buffer.haveTS || buffer.cursorMS != 0 || buffer.flushedMS != 0 {
		t.Fatalf("reset buffer = %#v", buffer)
	}
}

func TestPushHistoryAudioSegment(t *testing.T) {
	if err := pushHistoryAudioSegment(&failingHistoryOutput{}, "route", nil); err != nil {
		t.Fatalf("empty segment error = %v", err)
	}
	chunks := []*genx.MessageChunk{
		nil,
		{Part: &genx.Blob{MIMEType: " audio/pcm ", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{}},
		nil,
	}
	output := &failingHistoryOutput{}
	if err := pushHistoryAudioSegment(output, "route", chunks); err != nil {
		t.Fatalf("pushHistoryAudioSegment() error = %v", err)
	}
	if len(output.chunks) != 3 || !output.chunks[0].IsBeginOfStream() || !output.chunks[2].IsEndOfStream() {
		t.Fatalf("history segment lifecycle = %#v", output.chunks)
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
			err := pushHistoryAudioSegment(&failingHistoryOutput{failAt: test.failAt}, "route", chunks)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
