package eino

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestWrapAudioUnknownSpeakerPolicy(t *testing.T) {
	preserve, normalize := apitypes.VoiceAdapterUnknownSpeakerPolicyPreserve, apitypes.VoiceAdapterUnknownSpeakerPolicyDefaultVoice
	for _, tc := range []struct {
		name                  string
		policy                *apitypes.VoiceAdapterUnknownSpeakerPolicy
		nodeVoice             bool
		wantText, wantPattern string
	}{
		{"legacy", nil, false, "【陌生人】你好", "voice/story.default"},
		{"explicit preserve", &preserve, false, "【陌生人】你好", "voice/story.default"},
		{"default fallback", &normalize, false, "你好", "voice/story.default?format=ogg_opus"},
		{"node fallback", &normalize, true, "你好", "voice/story.node?format=ogg_opus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var patterns []string
			mux := einoTestMux(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
				mu.Lock()
				patterns = append(patterns, pattern)
				mu.Unlock()
				output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
				go func() {
					defer input.Close()
					for {
						if _, err := input.Next(); err != nil {
							break
						}
					}
					_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("OggS")}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}}, &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/ogg"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}})
					_ = output.Done(genx.Usage{})
				}()
				return output.Stream(), nil
			})
			core := einoTestTransformer(func(context.Context, genx.Stream) (genx.Stream, error) {
				output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
				_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("【陌生"), Ctrl: &genx.StreamCtrl{StreamID: "reply"}}, &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("人】你好"), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}})
				_ = output.Done(genx.Usage{})
				return output.Stream(), nil
			})
			fallback := "story.default"
			voice := apitypes.VoiceAdapter{DefaultVoice: &fallback, UnknownSpeakerPolicy: tc.policy}
			if tc.nodeVoice {
				nodes := map[string]string{"answer": "story.node"}
				voice.NodeVoices = &nodes
			}
			outputs := []apitypes.EinoOutput{{Name: "assistant", Node: "answer", MimeType: "text/plain"}}
			dock, err := wrapAudio(mux, core, nil, voice, outputs, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			output, err := dock.Transform(t.Context(), genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1).Stream())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			var text strings.Builder
			for {
				chunk, err := output.Next()
				if chunk != nil {
					if body, ok := chunk.Part.(genx.Text); ok {
						text.WriteString(string(body))
					}
				}
				if err != nil {
					if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
						t.Fatal(err)
					}
					break
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if text.String() != tc.wantText || len(patterns) != 1 || patterns[0] != tc.wantPattern {
				t.Fatalf("text=%q patterns=%q want %q %q", text.String(), patterns, tc.wantText, tc.wantPattern)
			}
		})
	}
}
