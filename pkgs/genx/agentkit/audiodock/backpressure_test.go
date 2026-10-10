package audiodock

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

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
