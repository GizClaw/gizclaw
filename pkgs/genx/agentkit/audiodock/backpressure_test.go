package audiodock

import (
	"context"
	"io"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

type gatedSpeechInput struct {
	genx.Stream
	ready <-chan struct{}
}

func (s gatedSpeechInput) Next() (*genx.MessageChunk, error) {
	<-s.ready
	return s.Stream.Next()
}

func TestBackpressurePreservesWhitespaceOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ready := make(chan struct{})
		dock, err := New(Config{
			Backpressure: true, ResolveVoice: fixedVoice("host"),
			TTS: muxFunc(func(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
				return streamkit.NewTTSStream(ctx, gatedSpeechInput{Stream: input, ready: ready}, streamkit.OutputConfig{}, "audio/opus", func(_ context.Context, _ string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
					return emit([]byte{1})
				}), nil
			}),
			Agent: fixedAgentOutput(
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("First sentence."), Ctrl: &genx.StreamCtrl{StreamID: "playback", MessageID: "generation", BeginOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("\n\n"), Ctrl: &genx.StreamCtrl{StreamID: "playback", MessageID: "generation"}},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("Second sentence."), Ctrl: &genx.StreamCtrl{StreamID: "playback", MessageID: "generation"}},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "playback", MessageID: "generation", MessageEnd: true}},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "playback", EndOfStream: true}},
			),
		})
		if err != nil {
			t.Fatal(err)
		}
		output, err := dock.Transform(t.Context(), emptyStream{})
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		published := make(chan string, 8)
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer close(published)
			for {
				chunk, err := output.Next()
				if err != nil {
					if err != io.EOF {
						t.Error(err)
					}
					return
				}
				if text, ok := chunk.Part.(genx.Text); ok && text != "" {
					published <- string(text)
				}
			}
		}()
		synctest.Wait()
		select {
		case text := <-published:
			close(ready)
			<-done
			t.Fatalf("text bypassed pending TTS input: %q", text)
		default:
		}
		close(ready)
		<-done
		var text strings.Builder
		for fragment := range published {
			text.WriteString(fragment)
		}
		if got := text.String(); got != "First sentence.\n\nSecond sentence." {
			t.Fatalf("narration text was reordered: %q", got)
		}
	})
}

func TestBackpressureWithDisabledVoiceCompletesMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dock, err := New(Config{
			Backpressure: true, ResolveVoice: fixedVoice(""),
			TTS: muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
				t.Error("disabled Voice opened TTS")
				return emptyStream{}, nil
			}),
			Agent: fixedAgentOutput(
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "playback", MessageID: "generation", BeginOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "playback", MessageID: "generation", MessageEnd: true}},
				&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "playback", EndOfStream: true}},
			),
		})
		if err != nil {
			t.Fatal(err)
		}
		output, err := dock.Transform(t.Context(), emptyStream{})
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		boundaries := 0
		var text strings.Builder
		for _, chunk := range readAll(t, output) {
			if part, ok := chunk.Part.(genx.Text); ok {
				text.WriteString(string(part))
			}
			if chunk.Ctrl != nil && chunk.Ctrl.MessageEnd {
				boundaries++
			}
		}
		if text.String() != "hello" || boundaries != 1 {
			t.Fatalf("text=%q message boundaries=%d", text.String(), boundaries)
		}
	})
}
