package giztest

import (
	"context"
	"strings"
	"testing"
)

func TestSpeechTranscribeRejectsUntypedInputBeforeRPC(t *testing.T) {
	step := Step{ID: "transcribe", Speech: &SpeechOperation{Method: "server.speech.transcribe"}}
	_, err := invokeSpeech(context.Background(), nil, step, map[string]any{}, "not audio", VariableSpec{}, VariableSpec{})
	if err == nil || !strings.Contains(err.Error(), "audio bytes") {
		t.Fatalf("error = %v", err)
	}
}

func TestTranscriptionInputRequiresPCMLabelForDecodedOpus(t *testing.T) {
	spec := VariableSpec{Type: "audio", MediaType: "audio/ogg", Codec: "opus"}
	_, err := transcriptionInput([]byte("not-decoded"), spec, "audio/ogg")
	if err == nil || !strings.Contains(err.Error(), speechPCM16ContentType) {
		t.Fatalf("error = %v", err)
	}
}
