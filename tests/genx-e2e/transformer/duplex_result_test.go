package transformer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const (
	duplexTranscriptLabel = "transcript"
	duplexAssistantLabel  = "assistant"
)

type duplexRoundResult struct {
	transcript          strings.Builder
	assistantText       strings.Builder
	lifecycles          *routeLifecycleTracker
	transcriptDone      bool
	assistantTextDone   bool
	assistantAudioDone  bool
	assistantAudioBytes int
}

func (r *duplexRoundResult) observe(streamID string, chunk *genx.MessageChunk) error {
	if chunk == nil {
		return nil
	}
	label := ""
	chunkStreamID := ""
	if chunk.Ctrl != nil {
		label = chunk.Ctrl.Label
		chunkStreamID = chunk.Ctrl.StreamID
	}
	if err := duplexChunkError(chunk); err != nil {
		if (label == duplexTranscriptLabel || label == duplexAssistantLabel) &&
			!roundStreamMatches(chunkStreamID, streamID) {
			return nil
		}
		return err
	}
	if label == duplexTranscriptLabel && roundStreamMatches(chunkStreamID, streamID) {
		if r.lifecycles == nil {
			r.lifecycles = newRouteLifecycleTracker()
		}
		if err := r.lifecycles.observe(chunk); err != nil {
			return err
		}
		if text, ok := chunk.Part.(genx.Text); ok && strings.TrimSpace(string(text)) != "" {
			r.transcript.WriteString(string(text))
		}
		if chunk.IsEndOfStream() {
			r.transcriptDone = true
		}
		return nil
	}
	if label != duplexAssistantLabel || !roundStreamMatches(chunkStreamID, streamID) {
		return nil
	}
	if r.lifecycles == nil {
		r.lifecycles = newRouteLifecycleTracker()
	}
	if err := r.lifecycles.observe(chunk); err != nil {
		return err
	}
	switch part := chunk.Part.(type) {
	case genx.Text:
		if strings.TrimSpace(string(part)) != "" {
			r.assistantText.WriteString(string(part))
		}
		if chunk.IsEndOfStream() {
			r.assistantTextDone = true
		}
	case *genx.Blob:
		if len(part.Data) > 0 {
			r.assistantAudioBytes += len(part.Data)
		}
		if chunk.IsEndOfStream() {
			r.assistantAudioDone = true
		}
	}
	return nil
}

func TestDuplexRoundResultIgnoresOtherStreamTerminalError(t *testing.T) {
	var result duplexRoundResult
	oldStreamError := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{
			StreamID:    "round-1:rt:1",
			Label:       duplexAssistantLabel,
			EndOfStream: true,
			Error:       "interrupted",
		},
	}
	if err := result.observe("round-2", oldStreamError); err != nil {
		t.Fatalf("observe() unrelated terminal error = %v", err)
	}

	currentStreamError := oldStreamError.Clone()
	currentStreamError.Ctrl.StreamID = "round-2:rt:1"
	if err := result.observe("round-2", currentStreamError); err == nil {
		t.Fatal("observe() current terminal error = nil")
	}
}

func (r *duplexRoundResult) done() bool {
	return strings.TrimSpace(r.transcript.String()) != "" &&
		r.transcriptDone &&
		strings.TrimSpace(r.assistantText.String()) != "" &&
		r.assistantAudioBytes > 0 &&
		r.assistantTextDone &&
		r.assistantAudioDone
}

func assertDuplexRound(t *testing.T, round int, result duplexRoundResult) {
	t.Helper()
	result.lifecycles.assertComplete(t)
	if len(result.lifecycles.routes) != 3 {
		t.Fatalf("round %d routes = %#v, want transcript text and assistant text/audio", round, result.lifecycles.routes)
	}
	if strings.TrimSpace(result.transcript.String()) == "" {
		t.Fatalf("round %d missing transcript", round)
	}
	if strings.TrimSpace(result.assistantText.String()) == "" {
		t.Fatalf("round %d missing assistant text", round)
	}
	if result.assistantAudioBytes == 0 {
		t.Fatalf("round %d missing assistant audio", round)
	}
}

func roundStreamMatches(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	return got == want || strings.HasPrefix(got, want+":")
}

func duplexChunkError(chunk *genx.MessageChunk) error {
	if chunk == nil || chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.Error) == "" {
		return nil
	}
	return fmt.Errorf("duplex stream %q label=%q returned error: %s", chunk.Ctrl.StreamID, chunk.Ctrl.Label, chunk.Ctrl.Error)
}
