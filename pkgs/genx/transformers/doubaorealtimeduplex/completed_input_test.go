package doubaorealtimeduplex

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Deliver the input tail only after processLoop has observed provider completion.
// This fixes the ordering without relying on scheduler luck or sleeps.
type completedDuplexInput struct {
	*bufferStream
	ctx     context.Context
	initial []*genx.MessageChunk
}

func (s *completedDuplexInput) NextOrDone(done <-chan struct{}) (*genx.MessageChunk, error, bool) {
	chunk, err, finished, _ := s.nextOrDoneOrKeepalive(done, nil)
	return chunk, err, finished
}

func (s *completedDuplexInput) nextOrDoneOrKeepalive(done <-chan struct{}, _ <-chan time.Time) (*genx.MessageChunk, error, bool, bool) {
	if len(s.initial) > 0 {
		chunk := s.initial[0]
		s.initial = s.initial[1:]
		return chunk, nil, false, false
	}
	select {
	case <-done:
		return nil, nil, true, false
	case <-s.ctx.Done():
		return nil, s.ctx.Err(), false, false
	}
}

func TestCompletedDuplexSessionDoesNotRestartForEmptyInputEOS(t *testing.T) {
	for _, test := range []struct {
		name string
		tail *genx.MessageChunk
		keep bool
	}{
		{name: "audio EOS", tail: &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}}},
		{name: "control EOS", tail: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}}},
		{name: "final audio data", tail: &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}}, keep: true},
		{name: "next turn BOS", tail: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "next", BeginOfStream: true}}, keep: true},
		{name: "next turn BOS and EOS", tail: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "next", BeginOfStream: true, EndOfStream: true}}, keep: true},
		{name: "input error", tail: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true, Error: "input failed"}}, keep: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			audioSent := make(chan struct{})
			session := &fakeDoubaoRealtimeDuplexSession{
				beforeRecv: audioSent, audioSent: audioSent,
				events: []*doubaospeech.RealtimeDuplexEvent{
					{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDelta, Delta: "done"},
					{Type: doubaospeech.RealtimeDuplexEventResponseOutputAudioStarted},
					{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDone},
					{Type: doubaospeech.RealtimeDuplexEventResponseOutputAudioDone},
				},
			}
			tail := newBufferStream(1)
			if err := tail.Push(test.tail); err != nil {
				t.Fatal(err)
			}
			if err := tail.Close(); err != nil {
				t.Fatal(err)
			}
			input := &completedDuplexInput{bufferStream: tail, ctx: ctx, initial: []*genx.MessageChunk{
				{Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
				{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn"}},
			}}
			output := newBufferStream(8)
			defer output.Close()
			transformer := newTransformer(nil, withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"))
			pending, err := transformer.processLoop(ctx, input, output, session, nil)
			if err != nil {
				t.Fatal(err)
			}
			if test.keep {
				if pending == nil || !reflect.DeepEqual(pending.Ctrl, test.tail.Ctrl) || !reflect.DeepEqual(pending.Part, test.tail.Part) {
					t.Fatalf("lost meaningful next input: got %#v, want %#v", pending, test.tail)
				}
			} else if pending != nil {
				t.Fatalf("completed session would restart for an empty EOS: %#v", pending)
			}
		})
	}
}
