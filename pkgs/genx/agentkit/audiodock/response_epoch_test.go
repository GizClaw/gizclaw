package audiodock

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

// The real input router acknowledges the replacement before forwarding its
// BOS to the Agent. The producer's first old response arrives only afterwards.
func TestDockPreservesDelayedProducerEpoch(t *testing.T) {
	for _, owner := range []string{"first-input", ""} {
		for _, audio := range []bool{false, true} {
			name := owner
			if name == "" {
				name = "unowned"
			}
			if audio {
				name += "/tts"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				epoch := genx.NewResponseEpoch(owner)
				agent := transformerFunc(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
					out := streamkit.NewOutput(streamkit.OutputConfig{})
					go func() {
						defer out.Close()
						begins := 0
						for begins < 2 {
							c, err := input.Next()
							if err != nil {
								return
							}
							if c != nil && c.IsBeginOfStream() {
								begins++
							}
						}
						_ = out.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("delayed reply"), Ctrl: &genx.StreamCtrl{StreamID: "source-response", Label: "assistant", BeginOfStream: true, EndOfStream: true, ResponseEpoch: epoch, ResponseEpochEnd: true}})
					}()
					return out, nil
				})
				config := Config{Agent: agent}
				if audio {
					config.ResolveVoice = fixedVoice("voice/local")
					config.TTS = muxFunc(func(_ context.Context, _ string, source genx.Stream) (genx.Stream, error) {
						out := streamkit.NewOutput(streamkit.OutputConfig{})
						go func() {
							defer out.Close()
							defer source.Close()
							for {
								if _, err := source.Next(); err != nil {
									break
								}
							}
							_ = out.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "tts-child", BeginOfStream: true}})
							_ = out.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "tts-child", EndOfStream: true}})
						}()
						return out, nil
					})
				}
				dock, err := New(config)
				if err != nil {
					t.Fatal(err)
				}
				input := streamkit.NewOutput(streamkit.OutputConfig{})
				defer input.Close()
				output, err := dock.Transform(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				for _, id := range []string{"first-input", "replacement-input"} {
					if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}); err != nil {
						t.Fatal(err)
					}
				}
				textSeen, audioSeen, completed := false, false, 0
				for {
					chunk, err := output.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if chunk == nil {
						continue
					}
					if chunk.Ctrl == nil || chunk.Ctrl.ResponseEpoch != epoch {
						t.Fatalf("immutable producer epoch was replaced: %#v", chunk.Ctrl)
					}
					if text, ok := chunk.Part.(genx.Text); ok && text == "delayed reply" {
						textSeen = true
					}
					if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
						audioSeen = true
					}
					if chunk.Ctrl.ResponseEpochEnd {
						completed++
					}
				}
				if !textSeen || audioSeen != audio || completed != 1 {
					t.Fatalf("incomplete response: text=%t audio=%t epoch completions=%d", textSeen, audioSeen, completed)
				}

			})
		}

	}
}
