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
