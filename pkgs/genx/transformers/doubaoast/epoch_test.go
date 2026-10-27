package doubaoast

import (
	"context"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerPreservesInputEpochAcrossReusedStreamID(t *testing.T) {
	input := newBufferStream(8)
	oldEpoch, newEpoch := genx.NewResponseEpoch("reused"), genx.NewResponseEpoch("reused")
	first := &fakeASTTranslateSession{closeCh: make(chan struct{})}
	second := &fakeASTTranslateSession{events: []*doubaospeech.ASTTranslateEvent{
		{Type: doubaospeech.ASTEventTranslationSubtitleStart},
		{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "new translation"},
		{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "new translation"},
		{Type: doubaospeech.ASTEventSessionFinished},
	}}
	sessions := []doubaoASTTranslateSession{first, second}
	transformer := newTransformer(doubaospeech.NewClient("test"), withRealtimePacing(false))
	transformer.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		next := sessions[0]
		sessions = sessions[1:]
		return next, nil
	}
	out, err := transformer.transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	for _, epoch := range []*genx.ResponseEpoch{oldEpoch, newEpoch} {
		_ = input.Push(&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "reused", BeginOfStream: true, ResponseEpoch: epoch}})
		_ = input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "reused", ResponseEpoch: epoch}})
	}
	_ = input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "reused", EndOfStream: true, ResponseEpoch: newEpoch}})
	_ = input.Close()
	var translated, interrupted bool
	for _, chunk := range readAllASTTranslateChunks(t, out) {
		if chunk.Role != genx.RoleModel {
			continue
		}
		if chunk.Ctrl.ResponseEpoch == oldEpoch {
			interrupted = interrupted || chunk.Ctrl.Error == "interrupted"
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				t.Fatal("old epoch produced translation")
			}
		} else {
			if chunk.Ctrl.ResponseEpoch != newEpoch {
				t.Fatal("new output adopted an old epoch")
			}
			translated = translated || chunk.Part == genx.Text("new translation")
		}
	}
	if !translated || !interrupted || len(sessions) != 0 {
		t.Fatalf("translated=%v interrupted=%v unused sessions=%d", translated, interrupted, len(sessions))
	}
}

func TestTextStateHandlesCumulativeAndPerSubtitleFinalsAcrossSegments(t *testing.T) {
	output := &recordingASTTranslateOutput{}
	state := astTranslateTextState{role: genx.RoleModel, label: "assistant", streamID: "one"}
	for _, step := range []struct{ id, token, final string }{
		{"one", "Hello", "Hello"},
		{"two", "world.", "Hello world."},
		{"three", "Again.", "Again."},
	} {
		state.streamID = step.id
		if err := state.addToken(output, step.token); err != nil {
			t.Fatal(err)
		}
		if err := state.addFinal(output, step.final); err != nil {
			t.Fatal(err)
		}
		if err := state.close(output, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, step := range []struct{ id, want string }{{"one", "Hello"}, {"two", "world."}, {"three", "Again."}} {
		if got := collectASTTranslateText(output.snapshot(), genx.RoleModel, "assistant", step.id); got != step.want {
			t.Fatalf("segment %s text=%q, want %q", step.id, got, step.want)
		}
	}
}
