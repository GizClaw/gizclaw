package doubaorealtime

import (
	"context"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerSemanticTurnEmptyAssistantIsTerminalError(t *testing.T) {
	for _, mode := range []Mode{ModePushToTalk, ModeRealtime} {
		for _, ttsFirst := range []bool{false, true} {
			for _, ttsStarted := range []bool{false, true} {
				name := string(mode)
				if ttsFirst {
					name += "/tts-first"
				} else {
					name += "/chat-first"
				}
				if !ttsStarted {
					name += "/without-tts-start"
				}
				t.Run(name, func(t *testing.T) {
					ready := make(chan struct{})
					drained := make(chan struct{})
					events := []*doubaospeech.RealtimeEvent{
						{Type: doubaospeech.EventASRResponse, Text: "question", QuestionID: "q-1"},
						{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
					}
					if ttsStarted {
						events = append(events, &doubaospeech.RealtimeEvent{Type: doubaospeech.EventTTSStarted, QuestionID: "q-1", ReplyID: "r-1"})
					}
					chat := &doubaospeech.RealtimeEvent{Type: doubaospeech.EventChatEnded, QuestionID: "q-1", ReplyID: "r-1"}
					tts := &doubaospeech.RealtimeEvent{Type: doubaospeech.EventTTSFinished, QuestionID: "q-1", ReplyID: "r-1"}
					if ttsFirst {
						events = append(events, tts, chat)
					} else {
						events = append(events, chat, tts)
					}
					session := &fakeTransformerSession{
						beforeRecv: ready, eventsDrained: drained,
						blockAfterEvents: make(chan struct{}), events: events,
					}
					if mode == ModePushToTalk {
						session.endASR = ready
					} else {
						session.firstAudioSent = ready
					}
					transformer := newTransformer(nil,
						withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}),
						withMode(mode), withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"),
					)
					ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
					defer cancel()
					input := newBufferStream(8)
					defer input.Close()
					output, err := transformer.Transform(ctx, input)
					if err != nil {
						t.Fatal(err)
					}
					defer output.Close()
					pushPTTTestTurn(t, input, "turn-1", 1)
					select {
					case <-drained:
					case <-ctx.Done():
						t.Fatal("provider terminals were not consumed")
					}
					var terminals, errorTerminals int
					for terminals < 2 {
						chunk, err := output.Next()
						if err != nil {
							t.Fatal(err)
						}
						if chunk == nil || chunk.Role != genx.RoleModel || !chunk.IsEndOfStream() {
							continue
						}
						terminals++
						if strings.Contains(chunk.Ctrl.Error, "completed without assistant content") {
							errorTerminals++
						}
					}
					if terminals != 2 || errorTerminals == 0 {
						t.Fatalf("empty semantic response: assistant terminals=%d error terminals=%d; want paired EOS with an explicit failure", terminals, errorTerminals)
					}
					if err := input.Close(); err != nil {
						t.Fatal(err)
					}
					for _, chunk := range drainRealtimeTestOutput(t, output) {
						if chunk != nil && chunk.Role == genx.RoleModel && chunk.IsEndOfStream() {
							t.Fatal("input closure duplicated the completed response terminal")
						}
					}
				})
			}
		}
	}
}

func TestTransformerEmptySemanticReplyKeepsNextTurnUsable(t *testing.T) {
	for _, mode := range []Mode{ModePushToTalk, ModeRealtime} {
		t.Run(string(mode), func(t *testing.T) {
			ready := make(chan struct{})
			paused := make(chan struct{})
			resume := make(chan struct{})
			session := &fakeTransformerSession{
				beforeRecv: ready, pauseBeforeEvent: 5, eventPaused: paused, resumeEvents: resume,
				blockAfterEvents: make(chan struct{}),
				events: []*doubaospeech.RealtimeEvent{
					{Type: doubaospeech.EventASRResponse, Text: "first question", QuestionID: "q-1"},
					{Type: doubaospeech.EventASREnded, QuestionID: "q-1"},
					{Type: doubaospeech.EventTTSStarted, QuestionID: "q-1", ReplyID: "r-1"},
					{Type: doubaospeech.EventChatEnded, QuestionID: "q-1", ReplyID: "r-1"},
					{Type: doubaospeech.EventTTSFinished, QuestionID: "q-1", ReplyID: "r-1"},
					{Type: doubaospeech.EventASRResponse, Text: "second question", QuestionID: "q-2"},
					{Type: doubaospeech.EventASREnded, QuestionID: "q-2"},
					{Type: doubaospeech.EventTTSStarted, Text: "answer", QuestionID: "q-2", ReplyID: "r-2"},
					{Type: doubaospeech.EventTTSAudioData, Audio: []byte{2, 0}},
					{Type: doubaospeech.EventChatEnded, QuestionID: "q-2", ReplyID: "r-2"},
					{Type: doubaospeech.EventTTSFinished, QuestionID: "q-2", ReplyID: "r-2"},
				},
			}
			if mode == ModePushToTalk {
				session.endASR = ready
			} else {
				session.firstAudioSent = ready
			}
			opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
			transformer := newTransformer(nil,
				withDoubaoRealtimeOpener(opener), withMode(mode),
				withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"),
			)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			input := newBufferStream(16)
			defer input.Close()
			output, err := transformer.Transform(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			pushPTTTestTurn(t, input, "turn-1", 1)
			var firstID string
			var errorTerminals int
			for terminals := 0; terminals < 2; {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				if chunk == nil || chunk.Role != genx.RoleModel || !chunk.IsEndOfStream() {
					continue
				}
				firstID = chunk.Ctrl.StreamID
				terminals++
				if chunk.Ctrl.Error != "" {
					errorTerminals++
				}
			}
			if errorTerminals == 0 {
				t.Fatal("empty semantic turn was reported as successful")
			}
			select {
			case <-paused:
			case <-ctx.Done():
				t.Fatal("provider did not reach the next-turn barrier")
			}
			pushPTTTestTurn(t, input, "turn-2", 2)
			if mode == ModePushToTalk && !session.waitForEndASRCount(2, time.Second) {
				t.Fatal("next input did not reach provider ASR completion")
			}
			if mode == ModeRealtime {
				deadline := time.Now().Add(time.Second)
				for session.audioCount() < 2 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if session.audioCount() < 2 {
					t.Fatal("next realtime audio did not reach the provider")
				}
			}
			close(resume)
			var chunks []*genx.MessageChunk
			var terminals int
			for terminals < 2 {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				chunks = append(chunks, chunk)
				if chunk == nil || chunk.Role != genx.RoleModel || !chunk.IsEndOfStream() {
					continue
				}
				if chunk.Ctrl.StreamID == firstID || chunk.Ctrl.Error != "" {
					t.Fatalf("previous empty turn poisoned next response: %#v", chunk.Ctrl)
				}
				terminals++
			}
			if !hasRealtimeTestText(chunks, genx.RoleModel, "answer") {
				t.Fatal("next turn did not produce its answer")
			}
			if terminals != 2 || opener.callCount() != 1 {
				t.Fatalf("next turn terminals=%d sessions=%d; want two successful terminals on the same provider session", terminals, opener.callCount())
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			drainRealtimeTestOutput(t, output)
		})
	}
}
