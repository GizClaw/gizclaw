package doubaorealtime

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Text submitted on an audio session has no ASR events. Replies must own the
// submitted input route, and a completed text round must permit another round.
func TestDeviceTextRoundsOnAudioSession(t *testing.T) {
	for _, mode := range []Mode{ModePushToTalk, ModeRealtime, ModeText} {
		for _, outputMode := range []Output{OutputAudio, OutputText} {
			for _, fragmented := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/fragmented=%t", mode, outputMode, fragmented), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
					defer cancel()
					session := &deviceTextSession{events: make(chan *doubaospeech.RealtimeEvent, 32), ctx: ctx}
					tr := newTransformer(nil, withMode(mode), withOutput(outputMode), withFormat("pcm"), withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}))
					input := newBufferStream(16)
					defer input.Close()
					output, err := tr.transform(ctx, input)
					if err != nil {
						t.Fatal(err)
					}
					defer output.Close()
					for turn := 1; turn <= 2; turn++ {
						id := fmt.Sprintf("demo-%d", turn)
						if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: id, Label: "demo-home", BeginOfStream: true}}); err != nil {
							t.Fatal(err)
						}
						text := "complete question"
						if fragmented {
							if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("complete "), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "demo-home"}}); err != nil {
								t.Fatal(err)
							}
							text = "question"
						}
						if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "demo-home", EndOfStream: true}}); err != nil {
							t.Fatal(err)
						}
						textDone, audioDone, answer := false, outputMode == OutputText, false
						for !textDone || !audioDone {
							chunk, err := output.Next()
							if err != nil {
								t.Fatalf("turn %d: %v", turn, err)
							}
							if chunk.Role == genx.RoleUser {
								t.Fatalf("text turn synthesized ASR transcript: %#v", chunk)
							}
							if chunk.Ctrl == nil || !strings.HasPrefix(chunk.Ctrl.StreamID, id) {
								t.Fatalf("reply lost input ownership: %#v", chunk)
							}
							if chunk.Ctrl.Error != "" {
								t.Fatal(chunk.Ctrl.Error)
							}
							switch part := chunk.Part.(type) {
							case genx.Text:
								answer = answer || string(part) == "answer"
								textDone = textDone || chunk.IsEndOfStream()
							case *genx.Blob:
								audioDone = audioDone || chunk.IsEndOfStream()
							}
						}
						if !answer {
							t.Fatal("reply ended without assistant text")
						}
					}
					session.mu.Lock()
					sent := append([]string(nil), session.texts...)
					session.mu.Unlock()
					if len(sent) != 2 || sent[0] != "complete question" || sent[1] != "complete question" {
						t.Fatalf("SendText submissions: %q", sent)
					}
					if session.endASRCount() != 0 {
						t.Fatal("text turn called EndASR")
					}
				})
			}
		}
	}
}

type deviceTextSession struct {
	fakeTransformerSession
	events chan *doubaospeech.RealtimeEvent
	ctx    context.Context
}

func (s *deviceTextSession) SendText(ctx context.Context, text string) error {
	if err := s.fakeTransformerSession.SendText(ctx, text); err != nil {
		return err
	}
	s.mu.Lock()
	turn := len(s.texts)
	s.mu.Unlock()
	for _, kind := range []doubaospeech.RealtimeEventType{doubaospeech.EventTTSStarted, doubaospeech.EventChatResponse, doubaospeech.EventTTSAudioData, doubaospeech.EventTTSFinished, doubaospeech.EventChatEnded} {
		event := &doubaospeech.RealtimeEvent{Type: kind, Text: "answer", Audio: []byte{0, 32}, QuestionID: fmt.Sprintf("question-%d", turn), ReplyID: fmt.Sprintf("reply-%d", turn)}
		select {
		case s.events <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (s *deviceTextSession) Recv() iter.Seq2[*doubaospeech.RealtimeEvent, error] {
	return func(yield func(*doubaospeech.RealtimeEvent, error) bool) {
		for {
			select {
			case event := <-s.events:
				if !yield(event, nil) {
					return
				}
			case <-s.ctx.Done():
				return
			}
		}
	}
}

// Publishing the next input's response must progress while the receiver is
// matching the prior response, without giving its untagged audio to the new one.
func TestPTTTextResponseSubmissionPreservesAudioOwner(t *testing.T) {
	queue := &doubaoRealtimePTTResponses{}
	old := &doubaoRealtimePTTResponse{streamID: "old", identity: doubaoRealtimePTTResponseIdentity{replyID: "old"}, completion: newTransformerPTTCompletion()}
	queue.add(old)
	if got := queue.startAudio(old.identity); got != old {
		t.Fatal("old audio was not bound")
	}
	submitted := make(chan struct{})
	go func() {
		for index := range 1000 {
			queue.add(&doubaoRealtimePTTResponse{streamID: fmt.Sprintf("next-%d", index), identity: doubaoRealtimePTTResponseIdentity{replyID: fmt.Sprintf("next-%d", index)}})
		}
		close(submitted)
	}()
	for range 1000 {
		if got := queue.matchAudio(doubaoRealtimePTTResponseIdentity{}); got != old {
			t.Fatal("new text submission stole old binary audio")
		}
	}
	select {
	case <-submitted:
	case <-time.After(time.Second):
		t.Fatal("text submission blocked behind event matching")
	}
	old.ttsFinished = true
	old.chatEnded = true
	queue.finish(old)
	if got := queue.match(doubaoRealtimePTTResponseIdentity{replyID: "next-0"}); got == nil || got.streamID != "next-0" {
		t.Fatal("next text response was lost")
	}
}
