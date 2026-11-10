package doubaorealtime

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type failedPTTCaptureSession struct {
	*fakeTransformerSession
	receiverJoined chan struct{}
}

func (s *failedPTTCaptureSession) Recv() iter.Seq2[*doubaospeech.RealtimeEvent, error] {
	return func(yield func(*doubaospeech.RealtimeEvent, error) bool) {
		defer close(s.receiverJoined)
		<-s.closedSignal()
		// These events were already in flight when the capture was aborted.
		for _, event := range []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventASRResponse, Text: "stale failed transcript"},
			{Type: doubaospeech.EventASREnded},
			{Type: doubaospeech.EventChatResponse, Text: "stale failed answer"},
		} {
			if !yield(event, nil) {
				return
			}
		}
	}
}

type failedPTTCaptureOpener struct {
	*fakeTransformerOpener
	retired *failedPTTCaptureSession
}

func (o *failedPTTCaptureOpener) OpenSession(ctx context.Context, cfg *doubaospeech.RealtimeConfig) (doubaoRealtimeSession, error) {
	if o.retired != nil && o.callCount() == 1 {
		select {
		case <-o.retired.receiverJoined:
		default:
			return nil, errors.New("replacement opened before failed capture receiver joined")
		}
	}
	return o.fakeTransformerOpener.OpenSession(ctx, cfg)
}

func TestFailedPTTAudioEOSRetiresTurnBeforeNextBOS(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	for _, uploaded := range []bool{false, true} {
		for _, mimeEOS := range []bool{false, true} {
			t.Run(fmt.Sprintf("uploaded=%t/mimeEOS=%t", uploaded, mimeEOS), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				endASR := make(chan struct{})
				responding := &fakeTransformerSession{
					beforeRecv: endASR, endASR: endASR, blockAfterEvents: make(chan struct{}),
					events: []*doubaospeech.RealtimeEvent{
						{Type: doubaospeech.EventASRResponse, Text: "second question", QuestionID: "second"},
						{Type: doubaospeech.EventASREnded, QuestionID: "second"},
						{Type: doubaospeech.EventTTSStarted, QuestionID: "second", ReplyID: "second"},
						{Type: doubaospeech.EventChatResponse, Text: "second answer", ReplyID: "second"},
						{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}, ReplyID: "second"},
						{Type: doubaospeech.EventTTSFinished, ReplyID: "second"},
						{Type: doubaospeech.EventChatEnded, ReplyID: "second"},
					},
				}
				first := responding
				var retired *failedPTTCaptureSession
				results := []fakeTransformerOpenResult{{session: responding}}
				firstBytes := 80 // Less than one Opus frame: entirely local pending PCM.
				if uploaded {
					firstBytes += 640 // One uploaded frame plus the same failed tail.
					first = &fakeTransformerSession{}
					retired = &failedPTTCaptureSession{fakeTransformerSession: first, receiverJoined: make(chan struct{})}
					results = []fakeTransformerOpenResult{{session: retired}, {session: responding}}
				}
				opener := &failedPTTCaptureOpener{fakeTransformerOpener: &fakeTransformerOpener{results: results}, retired: retired}
				transformer := newTransformer(nil, withDoubaoRealtimeOpener(opener), withInputFormat("speech_opus"), withFormat("pcm"))
				cause := errors.New("interrupted capture")
				failed := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "first", EndOfStream: true}}
				genx.SetStreamError(failed.Ctrl, cause)
				if mimeEOS {
					failed.Part = &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 640)}
				}
				input := &sliceRealtimeStream{chunks: []*genx.MessageChunk{
					{Ctrl: &genx.StreamCtrl{StreamID: "first", BeginOfStream: true}},
					{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, firstBytes)}, Ctrl: &genx.StreamCtrl{StreamID: "first"}},
					failed,
					{Ctrl: &genx.StreamCtrl{StreamID: "first", EndOfStream: true}},
					{Ctrl: &genx.StreamCtrl{StreamID: "first", EndOfStream: true}},
					{Ctrl: &genx.StreamCtrl{StreamID: "second", BeginOfStream: true}},
					{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 640)}, Ctrl: &genx.StreamCtrl{StreamID: "second"}},
					{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "second", EndOfStream: true}},
					{Ctrl: &genx.StreamCtrl{StreamID: "second", EndOfStream: true}},
				}}
				output, err := transformer.transform(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				chunks := drainRealtimeTestOutput(t, output)
				if !hasRealtimeTestText(chunks, genx.RoleUser, "second question") || !hasRealtimeTestText(chunks, genx.RoleModel, "second answer") {
					t.Fatal("next PTT turn did not finish on the same outer stream")
				}
				if got := opener.callCount(); got != len(results) {
					t.Fatalf("provider sessions = %d, want %d", got, len(results))
				}
				if responding.audioCount() != 1 || responding.endASRCount() != 1 || first.interruptCount() != 0 {
					t.Fatalf("second audio=%d EndASR=%d; first Interrupt=%d", responding.audioCount(), responding.endASRCount(), first.interruptCount())
				}
				if uploaded && (first.audioCount() != 1 || first.endASRCount() != 0 || !first.isClosed()) {
					t.Fatalf("failed capture audio=%d EndASR=%d closed=%t", first.audioCount(), first.endASRCount(), first.isClosed())
				}
				terminals := make(map[string]int)
				for _, chunk := range chunks {
					if chunk.Part == genx.Text("stale failed transcript") || chunk.Part == genx.Text("stale failed answer") {
						t.Fatal("retired capture published stale output")
					}
					if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "first" || !chunk.IsEndOfStream() {
						continue
					}
					terminals[chunk.Ctrl.Label]++
					if !errors.Is(genx.StreamError(chunk.Ctrl), cause) {
						t.Fatalf("failed route terminal lost cause: %#v", chunk.Ctrl)
					}
				}
				if terminals[genx.HistoryUserAudioLabel] != 1 || terminals[doubaoRealtimeTranscriptLabel] != 1 {
					t.Fatalf("failed route terminals = %v", terminals)
				}
				requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleUser, genx.HistoryUserAudioLabel, 2)
			})
		}
	}
}
