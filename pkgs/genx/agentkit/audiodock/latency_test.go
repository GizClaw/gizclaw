package audiodock

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

// A virtual provider delay makes the comparison exact and independent of CPU load.
func TestDockTextLatencyIndependentOfTTS(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		for _, voice := range []bool{false, true} {
			name := "streaming"
			if terminal {
				name = "text-with-eos"
			}
			if voice {
				name += "/tts"
			} else {
				name += "/text-only"
			}
			t.Run(name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					start := time.Now()
					chunk := &genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "model", BeginOfStream: true, EndOfStream: terminal}}
					chunks := []*genx.MessageChunk{chunk}
					if !terminal {
						chunks = append(chunks,
							&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "model"}},
							&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("last"), Ctrl: &genx.StreamCtrl{StreamID: "model", EndOfStream: true}},
						)
					}
					config := Config{Agent: fixedAgentOutput(chunks...)}
					if voice {
						config.ResolveVoice = fixedVoice("voice")
						config.TTS = muxFunc(func(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
							time.Sleep(3 * time.Second)
							return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{}, "audio/opus", func(_ context.Context, text string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
								time.Sleep(2 * time.Second)
								return emit([]byte(text))
							}), nil
						})
					}
					dock, err := New(config)
					if err != nil {
						t.Fatal(err)
					}
					output, err := dock.Transform(t.Context(), emptyStream{})
					if err != nil {
						t.Fatal(err)
					}
					defer output.Close()
					var times []time.Duration
					var text, audio string
					var textEOS, audioBOS, audioEOS int
					for {
						c, err := output.Next()
						if errors.Is(err, io.EOF) {
							break
						}
						if err != nil {
							t.Fatal(err)
						}
						if c.Role == genx.RoleModel && c.IsEndOfStream() && c.Ctrl.Error != "" {
							t.Errorf("unexpected terminal error: %+v", c.Ctrl)
						}
						if _, ok := c.Part.(genx.Text); ok && c.IsEndOfStream() {
							textEOS++
						}
						if part, ok := c.Part.(*genx.Blob); ok {
							audio += string(part.Data)
							if c.IsBeginOfStream() {
								audioBOS++
							}
							if c.IsEndOfStream() {
								audioEOS++
							}
						}
						if part, ok := c.Part.(genx.Text); ok && part != "" {
							times = append(times, time.Since(start))
							text += string(part)
						}
					}
					t.Logf("text=%q model_to_text=%v", text, times)
					if textEOS != 1 {
						t.Fatalf("text EOS=%d", textEOS)
					}
					if voice && (audio != text || audioBOS != 1 || audioEOS != 1) {
						t.Fatalf("TTS content/lifecycle changed: text=%q audio=%q BOS/EOS=%d/%d", text, audio, audioBOS, audioEOS)
					}
					if !voice && (audio != "" || audioBOS != 0 || audioEOS != 0) {
						t.Fatal("text-only run emitted audio")
					}

					for _, elapsed := range times {
						if elapsed != 0 {
							t.Errorf("model text delayed by TTS: %v", elapsed)
						}
					}
					if len(times) != len(chunks) {
						t.Fatalf("text chunks=%d, want %d", len(times), len(chunks))
					}
				})
			})
		}
	}
}

// A provider returning after explicit cancellation must have its handle closed
// without emitting audio, even when startup itself ignores the context.
func TestDockClosesLateTTSStartupAfterCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		late := streamkit.NewOutput(streamkit.OutputConfig{})
		startupContexts := make(chan context.Context, 1)
		release := make(chan struct{})
		dock, err := New(Config{
			Agent:        fixedAgentOutput(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "model", BeginOfStream: true, EndOfStream: true}}),
			ResolveVoice: fixedVoice("voice"),
			TTS: muxFunc(func(ctx context.Context, _ string, _ genx.Stream) (genx.Stream, error) {
				startupContexts <- ctx
				<-release
				return late, nil
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		output, err := dock.Transform(ctx, emptyStream{})
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		first, err := output.Next()
		if err != nil || first.Part != genx.Text("hello") {
			t.Fatalf("first=%#v error=%v", first, err)
		}
		startupContext := <-startupContexts
		cancel()
		synctest.Wait()
		if startupContext.Err() != context.Canceled {
			t.Fatal("startup was not cancelled")
		}
		close(release)
		synctest.Wait()
		for {
			chunk, err := output.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				break
			}
			if _, ok := chunk.Part.(*genx.Blob); ok {
				t.Fatalf("late startup emitted audio: %#v", chunk)
			}
		}
		if err := late.Push(&genx.MessageChunk{}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("late provider handle was not closed: %v", err)
		}
	})
}

func TestDockTextDoesNotWaitForVoiceResolution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dock, err := New(Config{
			Agent: fixedAgentOutput(
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "model"}},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("last"), Ctrl: &genx.StreamCtrl{StreamID: "model", EndOfStream: true}},
			),
			ResolveVoice: func(context.Context, VoiceRequest) (string, error) { time.Sleep(3 * time.Second); return "", nil },
			TTS: muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
				t.Error("empty voice must not open a provider")
				return nil, nil
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		output, err := dock.Transform(t.Context(), emptyStream{})
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		var text string
		for {
			c, err := output.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			part, ok := c.Part.(genx.Text)
			if !ok {
				t.Fatalf("unexpected route: %#v", c)
			}
			if part != "" {
				text += string(part)
				if time.Since(start) != 0 {
					t.Errorf("voice resolution delayed text: %v", time.Since(start))
				}
			}
		}
		if text != "firstlast" {
			t.Fatalf("text=%q", text)
		}
	})
}
