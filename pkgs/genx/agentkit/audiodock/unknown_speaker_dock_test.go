package audiodock

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestUnknownSpeakerConfigPolicy(t *testing.T) {
	agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) { return &sliceStream{}, nil })
	for _, tc := range []struct {
		policy                UnknownSpeakerPolicy
		backpressure, wantErr bool
	}{
		{"", false, false}, {UnknownSpeakerPreserve, false, false}, {UnknownSpeakerDefaultVoice, false, false}, {"bad", false, true}, {UnknownSpeakerDefaultVoice, true, true}, {UnknownSpeakerPreserve, true, false},
	} {
		_, err := New(Config{Agent: agent, UnknownSpeakerPolicy: tc.policy, Backpressure: tc.backpressure})
		if (err != nil) != tc.wantErr {
			t.Fatalf("policy=%q backpressure=%v err=%v", tc.policy, tc.backpressure, err)
		}
	}
}

func TestUnknownSpeakerDockUsesDefaultVoice(t *testing.T) {
	for _, tc := range []struct {
		name, input, wantText, wantAudio string
		voices                           map[string]string
		controlOnlyEOS                   bool
	}{
		{name: "unknown only", input: "【陌生人】你好", wantText: "你好", wantAudio: "default:你好"},
		{name: "known then unknown", input: "【狐】狐狸\n【陌生人】你好", wantText: "狐狸\n你好", wantAudio: "fox:狐狸\ndefault:你好", voices: map[string]string{"狐": "fox"}},
		{name: "unclosed marker on control EOS", input: "【陌", wantText: "【陌", wantAudio: "default:【陌", controlOnlyEOS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
				var chunks []*genx.MessageChunk
				for i := 0; i < len(tc.input); i++ {
					chunks = append(chunks, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(tc.input[i : i+1]), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: i == 0}})
				}
				terminal := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}}
				if tc.controlOnlyEOS {
					terminal.Part = nil
				}
				chunks = append(chunks, terminal)
				return &sliceStream{chunks: chunks}, nil
			})
			tts := muxFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
				var spoken strings.Builder
				for {
					chunk, err := input.Next()
					if chunk != nil {
						if text, ok := chunk.Part.(genx.Text); ok {
							spoken.WriteString(string(text))
						}
					}
					if err != nil {
						break
					}
				}
				return &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte(pattern + ":" + spoken.String())}, Ctrl: &genx.StreamCtrl{StreamID: "tts", BeginOfStream: true, EndOfStream: true}}}}, nil
			})
			dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: fixedVoice("default"), SpeakerVoices: tc.voices, UnknownSpeakerPolicy: UnknownSpeakerDefaultVoice})
			if err != nil {
				t.Fatal(err)
			}
			out, err := dock.Transform(t.Context(), emptyStream{})
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			var text, audio, history strings.Builder
			for _, chunk := range readAll(t, out) {
				switch part := chunk.Part.(type) {
				case genx.Text:
					text.WriteString(string(part))
					if chunk.HistoryText != nil {
						history.WriteString(string(*chunk.HistoryText))
					}
				case *genx.Blob:
					audio.Write(part.Data)
				}
			}
			if text.String() != tc.wantText || audio.String() != tc.wantAudio {
				t.Fatalf("text=%q audio=%q want text=%q audio=%q", text.String(), audio.String(), tc.wantText, tc.wantAudio)
			}
			if history.String() != tc.input {
				t.Fatalf("semantic history=%q want %q", history.String(), tc.input)
			}
		})
	}
}

func TestUnknownSpeakerDockEmptyResolvedVoiceStillDeliversText(t *testing.T) {
	agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleModel, Part: genx.Text("【陌生人】你好"), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: true, EndOfStream: true}}}}, nil
	})
	var synthesisCalls atomic.Int32
	tts := muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
		synthesisCalls.Add(1)
		return &sliceStream{}, nil
	})
	dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: fixedVoice(""), UnknownSpeakerPolicy: UnknownSpeakerDefaultVoice})
	if err != nil {
		t.Fatal(err)
	}
	out, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var text strings.Builder
	for _, chunk := range readAll(t, out) {
		if body, ok := chunk.Part.(genx.Text); ok {
			text.WriteString(string(body))
		}
	}
	if text.String() != "你好" || synthesisCalls.Load() != 0 {
		t.Fatalf("text=%q synthesis calls=%d", text.String(), synthesisCalls.Load())
	}
}
