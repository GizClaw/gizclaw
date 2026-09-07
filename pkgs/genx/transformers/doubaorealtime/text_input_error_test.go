package doubaorealtime

import (
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"testing"
)

func TestTextInputRejectsFailedEOS(t *testing.T) {
	var input doubaoRealtimeTextInput
	_, err := input.push(&genx.MessageChunk{Part: genx.Text("unfinished"), Ctrl: &genx.StreamCtrl{StreamID: "question"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := input.push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "question", EndOfStream: true, Error: "input canceled"}})
	if err == nil || got != nil {
		t.Fatalf("failed EOS = %#v, %v; must not submit partial text", got, err)
	}
}
