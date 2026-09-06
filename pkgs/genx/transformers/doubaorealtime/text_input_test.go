package doubaorealtime

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTextInputReplacesIncompleteTurnAndIgnoresDuplicateEOS(t *testing.T) {
	var input doubaoRealtimeTextInput
	push := func(id, text string, end bool) *genx.MessageChunk {
		t.Helper()
		got, err := input.push(&genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: end}})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := push("old", "unfinished", false); got != nil {
		t.Fatal("submitted incomplete text")
	}
	if got := push("new", "complete", true); got == nil || got.Part != genx.Text("complete") {
		t.Fatalf("replacement = %#v", got)
	}
	if got := push("new", "", true); got != nil {
		t.Fatal("submitted duplicate EOS")
	}
}

func TestTextInputBoundsAccumulatedBytes(t *testing.T) {
	var input doubaoRealtimeTextInput
	chunk := &genx.MessageChunk{Part: genx.Text(strings.Repeat("a", doubaoRealtimeTextInputLimit)), Ctrl: &genx.StreamCtrl{StreamID: "question"}}
	if _, err := input.push(chunk); err != nil {
		t.Fatal(err)
	}
	chunk.Part = genx.Text("b")
	if _, err := input.push(chunk); err == nil {
		t.Fatal("accepted oversized text")
	}
}
