package gizclaw

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaotts"
)

type peerTTSAgent struct{}

func (peerTTSAgent) Transform(context.Context, genx.Stream) (genx.Stream, error) {
	return &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text("Sentence number 1!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("Sentence number 2!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant"}},
		{Role: genx.RoleModel, Part: genx.Text("Sentence number 3!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", EndOfStream: true}},
	}}, nil
}

type peerTTSMux struct{ transformer genx.Transformer }

func (m peerTTSMux) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return m.transformer.Transform(ctx, input)
}

func TestPeerTTSSegmentFailureLifecycle(t *testing.T) {
	for _, failed := range []string{"1", "2", "3", "123"} {
		t.Run("failed_"+failed, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Params struct {
						Text string `json:"text"`
					} `json:"req_params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				text := strings.TrimSpace(request.Params.Text)
				segment := text[len(text)-2 : len(text)-1]
				if strings.Contains(failed, segment) {
					w.Header().Set("Content-Length", "1000")
					_, _ = io.WriteString(w, "{\"code\":0,\"reqid\":\"req-failed\"}\n")
					return
				}
				pcm := []byte{0, segment[0], 0, segment[0], 0, segment[0]}
				_, _ = fmt.Fprintf(w, "{\"code\":0,\"data\":%q}\n{\"code\":20000000}\n", base64.StdEncoding.EncodeToString(pcm))
			}))
			defer server.Close()
			tts, err := doubaotts.NewSeedV2(doubaotts.SeedV2Config{
				Client:  doubaospeech.NewClient("test", doubaospeech.WithAPIKey("test"), doubaospeech.WithBaseURL(server.URL)),
				Speaker: "test", Format: "pcm",
			})
			if err != nil {
				t.Fatal(err)
			}
			dock, err := audiodock.New(audiodock.Config{Agent: peerTTSAgent{}, TTS: peerTTSMux{tts}, ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) { return "voice/test", nil }})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			output, err := dock.Transform(ctx, &peerStreamSliceStream{})
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			var events bytes.Buffer
			broker := newPeerStreamEventBroker()
			unsubscribe, err := broker.Subscribe(&events)
			if err != nil {
				t.Fatal(err)
			}
			defer unsubscribe()
			tracks := &peerStreamFakeTracks{createdCh: make(chan struct{}, 1)}
			defer func() {
				if tracks.mixer != nil {
					_ = tracks.mixer.Close()
				}
			}()
			done := make(chan error, 1)
			go func() {
				done <- (peerAgentOutput{Events: broker, Tracks: tracks}).ConsumeAgentOutput(ctx, output)
			}()
			select {
			case <-tracks.createdCh:
				err = waitPeerAgentOutputDrain(t, tracks.mixer, done)
			case err = <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err != nil {
				t.Fatal(err)
			}
			var audio bytes.Buffer
			if tracks.track != nil {
				for _, chunk := range tracks.track.chunks {
					if _, err := chunk.WriteTo(&audio); err != nil {
						t.Fatal(err)
					}
				}
			}
			var wantAudio []byte
			for _, segment := range "123" {
				if !strings.ContainsRune(failed, segment) {
					wantAudio = append(wantAudio, 0, byte(segment), 0, byte(segment), 0, byte(segment))
				}
			}
			if !bytes.Equal(audio.Bytes(), wantAudio) {
				t.Errorf("client track audio = %v, want %v", audio.Bytes(), wantAudio)
			}
			bos, eos := 0, 0
			var text strings.Builder
			for events.Len() > 0 {
				event, err := readPeerStreamEvent(&events)
				if err != nil {
					t.Fatal(err)
				}
				if event.GetBos() != nil {
					bos++
				}
				if delta := event.GetTextDelta(); delta != nil {
					text.WriteString(delta.Text)
				}
				if end := event.GetEos(); end != nil {
					eos++
					if failed == "123" {
						if failure := end.GetError(); failure == nil || failure.Code != "STREAM_ERROR" || failure.Retryable || !strings.Contains(failure.Message, "tts stream truncated before final frame") {
							t.Errorf("all-failed wire EOS = %+v", end)
						}
					} else if end.GetError() != nil {
						t.Errorf("partial reply wire EOS = %+v", end)
					}
				}
			}
			wantEOS := 1 // successful text completion is TEXT_DONE, plus one audio EOS
			if failed == "123" {
				wantEOS = 3 // text error EOS, audio error EOS, response epoch error EOS
			}
			if bos != 2 || eos != wantEOS {
				t.Errorf("wire BOS=%d EOS=%d, want 2 and %d", bos, eos, wantEOS)
			}
			if text.String() != "Sentence number 1!Sentence number 2!Sentence number 3!" {
				t.Errorf("client text = %q", text.String())
			}
		})
	}
}
