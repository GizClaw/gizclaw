package doubaorealtime

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
)

// A Peer may send both route and audio-channel BOS. External ASR turns that
// channel into text; the realtime Model must observe one route start, send no
// audio to its provider and remain usable for the next turn.
func TestExternalASRTextRoundsOnAudioSession(t *testing.T) {
	for _, mode := range []Mode{ModePushToTalk, ModeRealtime} {
		for _, controlBOS := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/control=%t", mode, controlBOS), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				session := &deviceTextSession{ctx: ctx, events: make(chan *doubaospeech.RealtimeEvent, 32)}
				core := newTransformer(nil, withMode(mode), withOutput(OutputText), withDoubaoRealtimeOpener(&fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}))
				dock, err := audiodock.New(audiodock.Config{Agent: core, ASR: deviceTextASR{}})
				if err != nil {
					t.Fatal(err)
				}
				input := newBufferStream(16)
				defer input.Close()
				output, err := dock.Transform(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				for turn := range 2 {
					id := fmt.Sprintf("mic-%d", turn)
					chunks := []*genx.MessageChunk{}
					if controlBOS {
						chunks = append(chunks, &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}})
					}
					chunks = append(chunks,
						&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}},
						&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}},
					)
					if controlBOS {
						chunks = append(chunks, &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
					}
					for _, chunk := range chunks {
						if err := input.Push(chunk); err != nil {
							t.Fatal(err)
						}
					}
					answer := false
					for {
						chunk, err := output.Next()
						if err != nil {
							t.Fatalf("turn %d: %v", turn, err)
						}
						if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
							t.Fatal(chunk.Ctrl.Error)
						}
						if chunk.Role == genx.RoleModel {
							if text, ok := chunk.Part.(genx.Text); ok {
								answer = answer || text == "answer"
								if chunk.IsEndOfStream() {
									break
								}
							}
						}
					}
					if !answer {
						t.Fatal("reply ended without text")
					}
				}
				session.mu.Lock()
				texts := slices.Clone(session.texts)
				audio := len(session.audio)
				session.mu.Unlock()
				if audio != 0 || len(texts) != 2 || texts[0] != "external ASR" || texts[1] != "external ASR" || session.endASRCount() != 0 {
					t.Fatalf("provider audio=%d texts=%q EndASR=%d", audio, texts, session.endASRCount())
				}
			})
		}
	}
}

type deviceTextASR struct{}

func (deviceTextASR) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	output := newBufferStream(16)
	go func() {
		defer output.Close()
		for {
			chunk, err := input.Next()
			if err != nil {
				return
			}
			if chunk.IsEndOfStream() {
				if err := output.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("external ASR"), Ctrl: &genx.StreamCtrl{StreamID: chunk.Ctrl.StreamID, Label: "transcript", BeginOfStream: true, EndOfStream: true}}); err != nil {
					return
				}
			}
		}
	}()
	return output, nil
}
