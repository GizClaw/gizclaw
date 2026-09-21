package audiodock

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestDockCompletesSlowTTSAfterTextEOS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var textEOSAt time.Time
		dock, err := New(Config{
			Agent: fixedAgentOutput(&genx.MessageChunk{
				Role: genx.RoleModel, Part: genx.Text("hello"),
				Ctrl: &genx.StreamCtrl{StreamID: "model", BeginOfStream: true, EndOfStream: true},
			}),
			ResolveVoice: fixedVoice("voice"),
			TTS: muxFunc(func(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
				return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{}, "audio/opus", func(ctx context.Context, _ string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
					// The unpunctuated input is flushed only when text EOS arrives.
					textEOSAt = time.Now()
					for i := range 8 {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-time.After(30 * time.Second):
						}
						if err := emit([]byte{byte(i)}); err != nil {
							return err
						}
					}
					return nil
				}), nil
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		output, err := dock.Transform(t.Context(), emptyStream{})
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		var audio []byte
		var text string
		var textEOS, audioBOS, audioEOS, epochEnd int
		for {
			chunk, err := output.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if chunk.Ctrl != nil {
				if chunk.Ctrl.Error != "" {
					t.Errorf("unexpected terminal error at %v: %s", time.Since(start), chunk.Ctrl.Error)
				}
				if chunk.Ctrl.ResponseEpochEnd {
					epochEnd++
				}
			}
			switch part := chunk.Part.(type) {
			case genx.Text:
				text += string(part)
				if part != "" && time.Since(start) != 0 {
					t.Errorf("text delayed by synthesis: %v", time.Since(start))
				}
				if chunk.IsEndOfStream() {
					textEOS++
				}
			case *genx.Blob:
				audio = append(audio, part.Data...)
				if chunk.IsBeginOfStream() {
					audioBOS++
				}
				if chunk.IsEndOfStream() {
					audioEOS++
				}
			}
		}
		synctest.Wait()
		if textEOSAt != start || time.Since(textEOSAt) != 4*time.Minute {
			t.Errorf("synthesis after text EOS = %v, want 4m0s (EOS delay %v)", time.Since(textEOSAt), textEOSAt.Sub(start))
		}
		if !slices.Equal(audio, []byte{0, 1, 2, 3, 4, 5, 6, 7}) {
			t.Errorf("audio = %v, want all 8 chunks in order", audio)
		}
		if text != "hello" || textEOS != 1 || audioBOS != 1 || audioEOS != 1 || epochEnd != 1 {
			t.Errorf("text=%q text EOS=%d audio BOS/EOS=%d/%d epoch end=%d", text, textEOS, audioBOS, audioEOS, epochEnd)
		}
		t.Logf("synthesis after text EOS=%v, audio=%v, text/audio EOS=%d/%d, epoch end=%d", time.Since(textEOSAt), audio, textEOS, audioEOS, epochEnd)
	})
}

func TestDockCancelsPendingTTSAfterTextEOS(t *testing.T) {
	for _, closeOutput := range []bool{false, true} {
		name := "context"
		if closeOutput {
			name = "output-close"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				providerOutput := streamkit.NewOutput(streamkit.OutputConfig{})
				started := make(chan context.Context, 1)
				dock, err := New(Config{
					Agent: fixedAgentOutput(&genx.MessageChunk{
						Role: genx.RoleModel, Part: genx.Text("hello"),
						Ctrl: &genx.StreamCtrl{StreamID: "model", BeginOfStream: true, EndOfStream: true},
					}),
					ResolveVoice: fixedVoice("voice"),
					TTS: muxFunc(func(ctx context.Context, _ string, _ genx.Stream) (genx.Stream, error) {
						started <- ctx
						return providerOutput, nil
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
				providerContext := <-started
				synctest.Wait()
				time.Sleep(4 * time.Minute)
				if err := providerContext.Err(); err != nil {
					t.Fatalf("pending TTS cancelled before caller action: %v", err)
				}
				if closeOutput {
					if err := output.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					cancel()
				}
				synctest.Wait()
				if providerContext.Err() != context.Canceled {
					t.Fatalf("TTS context error = %v, want cancellation", providerContext.Err())
				}
				if err := providerOutput.Push(&genx.MessageChunk{}); !errors.Is(err, context.Canceled) {
					t.Fatalf("TTS output was not closed: %v", err)
				}
			})
		})
	}
}
