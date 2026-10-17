package doubaorealtime

import (
	"bytes"
	"context"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type historySegmentTestSession struct {
	*fakeTransformerSession
	secondAudioSent chan struct{}
}

func (s *historySegmentTestSession) SendAudio(ctx context.Context, audio []byte) error {
	if err := s.fakeTransformerSession.SendAudio(ctx, audio); err != nil {
		return err
	}
	if s.audioCount() == 2 {
		close(s.secondAudioSent)
	}
	return nil
}

func TestTransformerRealtimeRecordsTwoUtterancesOnOneInputRoute(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	firstAudioSent, secondAudioSent, nextUtterance := make(chan struct{}), make(chan struct{}), make(chan struct{})
	session := &historySegmentTestSession{
		fakeTransformerSession: &fakeTransformerSession{
			firstAudioSent:   firstAudioSent,
			beforeRecv:       firstAudioSent,
			pauseBeforeEvent: 2,
			eventPaused:      nextUtterance,
			resumeEvents:     secondAudioSent,
			events: []*doubaospeech.RealtimeEvent{
				{Type: doubaospeech.EventASRResponse, Text: "first", IsFinal: true},
				{Type: doubaospeech.EventASREnded},
				{Type: doubaospeech.EventASRResponse, Text: "second", IsFinal: true},
				{Type: doubaospeech.EventASREnded},
				{Type: doubaospeech.EventSessionFinished},
			},
		},
		secondAudioSent: secondAudioSent,
	}
	transformer := newTransformer(nil, withMode(ModeRealtime), withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"))
	input, output := newBufferStream(4), newBufferStream(16)
	defer input.Close()
	first, second := []byte{1, 0}, []byte{2, 0}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: first}, Ctrl: &genx.StreamCtrl{StreamID: "microphone", BeginOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := transformer.processLoop(ctx, input, output, session)
		_ = output.Close()
		done <- err
	}()
	select {
	case <-nextUtterance:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: second}, Ctrl: &genx.StreamCtrl{StreamID: "microphone"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	chunks := drainRealtimeTestOutput(t, output)
	recordings := make(map[string][]byte)
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			if blob, ok := chunk.Part.(*genx.Blob); ok {
				recordings[chunk.Ctrl.StreamID] = append(recordings[chunk.Ctrl.StreamID], blob.Data...)
			}
		}
	}
	if !bytes.Equal(recordings["microphone:rt:1"], first) || !bytes.Equal(recordings["microphone:rt:2"], second) || len(recordings) != 2 {
		t.Fatalf("recordings were lost or crossed utterances: %v", recordings)
	}
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleUser, genx.HistoryUserAudioLabel, 2)
}

func TestTransformerRealtimePublishesUserRecordingWithTranscript(t *testing.T) {
	audioSent := make(chan struct{})
	session := &fakeTransformerSession{
		firstAudioSent: audioSent,
		beforeRecv:     audioSent,
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "question", IsFinal: true},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventTTSStarted},
			{Type: doubaospeech.EventChatResponse, Text: "answer"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 0}},
			{Type: doubaospeech.EventTTSFinished},
			{Type: doubaospeech.EventChatEnded},
			{Type: doubaospeech.EventSessionFinished},
		},
	}
	transformer := newTransformer(nil, withMode(ModeRealtime), withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"))
	input := newBufferStream(4)
	defer input.Close()
	data := []byte{1, 0, 2, 0}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: "microphone", BeginOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	output := newBufferStream(16)
	if err := runTransformerProcessLoop(t, transformer, input, output, session); err != nil {
		t.Fatal(err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	var recorded []byte
	audioEnd, textEnd := -1, -1
	for i, chunk := range chunks {
		if chunk.Role != genx.RoleUser || chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.StreamID != "microphone:rt:1" {
			t.Fatalf("recording and transcript used different identities: %#v", chunk.Ctrl)
		}
		if chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			if blob, ok := chunk.Part.(*genx.Blob); ok {
				recorded = append(recorded, blob.Data...)
			}
			if chunk.IsEndOfStream() {
				audioEnd = i
			}
		} else if chunk.IsEndOfStream() {
			textEnd = i
		}
	}
	if !bytes.Equal(recorded, data) || audioEnd < 0 || textEnd <= audioEnd {
		t.Fatalf("recording=%v, audio EOS=%d, transcript EOS=%d", recorded, audioEnd, textEnd)
	}
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleUser, genx.HistoryUserAudioLabel, 1)
}
