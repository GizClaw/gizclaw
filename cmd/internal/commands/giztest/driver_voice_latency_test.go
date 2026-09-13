package giztestcmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
)

// Exercise the real Eino factory and realtime ASR branch with a static voice.
// Replacing input while the previous TTS is opening must not hold new text.
func TestEinoRealtimeTextDuringTTSStartup(t *testing.T) {
	data, err := os.ReadFile("../../../../tests/gizclaw-e2e/testdata/eino-voices/workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, voice := range []bool{false, true} {
		name := "text-only"
		if voice {
			name = "static-tts"
		}
		t.Run(name, func(t *testing.T) {
			var spec map[string]any
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}
			adapter := map[string]any{"asr_model": "fixture-asr"}
			if voice {
				adapter["default_voice"] = "story.default"
			}
			spec["voice_adapter"] = adapter
			configured, err := json.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			synctest.Test(t, func(t *testing.T) {
				entered := make(chan struct{}, 2)
				provider := &latencyVoiceProvider{
					voiceFixtureProvider: &voiceFixtureProvider{
						packets:   map[string][][]byte{"story.default": {{1}}},
						recognize: map[string]string{"fox-packet": "fox", "bird-packet": "bird"},
					},
					entered: entered,
				}
				resources := voiceFixtureResources{}
				service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: provider})
				agent := newVoiceFixtureAgent(t, "eino", configured, service, apitypes.WorkspaceInputModeRealtime)
				defer agent.(io.Closer).Close()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
				output, err := agent.Transform(ctx, input.Stream())
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				replies := make(chan *genx.MessageChunk, 16)
				go func() {
					defer close(replies)
					for {
						c, err := output.Next()
						if err != nil {
							return
						}
						if c.Role == genx.RoleModel {
							if text, ok := c.Part.(genx.Text); ok && text != "" {
								replies <- c
							}
						}
					}
				}()
				push := func(id, packet string) {
					t.Helper()
					// No audio EOS: fake ASR finalizes inside a still-open duplex stream.
					if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte(packet)}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}); err != nil {
						t.Fatal(err)
					}
				}
				start := time.Now()
				push("input-fox", "fox-packet")
				first := <-replies
				if first == nil || first.Part != genx.Text("fox") {
					t.Fatalf("first=%#v", first)
				}
				firstDelay := time.Since(start)
				if voice {
					<-entered
				}
				replacement := time.Now()
				push("input-bird", "bird-packet")
				second := <-replies
				if second == nil || second.Part != genx.Text("bird") {
					t.Fatalf("replacement=%#v", second)
				}
				delay := time.Since(replacement)
				t.Logf("first_text=%v replacement_first_text=%v (TTS startup=3s)", firstDelay, delay)
				if firstDelay != 0 || delay != 0 {
					t.Errorf("TTS delayed realtime text: first=%v replacement=%v", firstDelay, delay)
				}
				cancel()
				_ = input.Stream().Close()
			})
		})
	}
}

type latencyVoiceProvider struct {
	*voiceFixtureProvider
	entered chan struct{}
}

func (p *latencyVoiceProvider) BuildTransformer(ctx context.Context, c peergenx.TransformerConfig) (genx.Transformer, error) {
	next, err := p.voiceFixtureProvider.BuildTransformer(ctx, c)
	if err != nil || c.Voice == nil {
		return next, err
	}
	return latencyVoiceTransformer{entered: p.entered, next: next}, nil
}

type latencyVoiceTransformer struct {
	entered chan struct{}
	next    genx.Transformer
}

func (t latencyVoiceTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	t.entered <- struct{}{}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return t.next.Transform(ctx, input)
	}
}
