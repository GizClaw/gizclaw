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
	terminalErrors      []string
	assistantStreamID   string
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
	chunkErr := duplexChunkError(chunk)
	if chunkErr != nil {
		if label == duplexTranscriptLabel && !roundStreamMatches(chunkStreamID, streamID) {
			return nil
		}
		if label == duplexAssistantLabel && r.assistantStreamID != "" && chunkStreamID != r.assistantStreamID {
			return nil
		}
		if label == duplexAssistantLabel && r.assistantStreamID == "" && !roundStreamMatches(chunkStreamID, streamID) {
			return nil
		}
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
		if chunkErr != nil {
			r.terminalErrors = append(r.terminalErrors, chunkErr.Error())
		}
		return nil
	}
	if label != duplexAssistantLabel {
		return nil
	}
	if r.assistantStreamID == "" {
		if !chunk.IsBeginOfStream() || strings.TrimSpace(chunkStreamID) == "" {
			return fmt.Errorf("duplex assistant route started without BOS and StreamID: %#v", chunk)
		}
		r.assistantStreamID = chunkStreamID
	}
	if chunkStreamID != r.assistantStreamID {
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
	if chunkErr != nil {
		r.terminalErrors = append(r.terminalErrors, chunkErr.Error())
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
		t.Fatal("observe() current terminal error without BOS = nil")
	}
}

func TestDuplexRoundResultBindsProviderAssistantStreamID(t *testing.T) {
	var result duplexRoundResult
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "provider-response", Label: duplexAssistantLabel, BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "provider-response", Label: duplexAssistantLabel}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "provider-response", Label: duplexAssistantLabel, EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "stale-response", Label: duplexAssistantLabel, BeginOfStream: true}},
	} {
		if err := result.observe("input-turn", chunk); err != nil {
			t.Fatalf("observe() error = %v", err)
		}
	}
	if result.assistantStreamID != "provider-response" || result.assistantText.String() != "answer" || !result.assistantTextDone {
		t.Fatalf("assistant result = id %q text %q done %t", result.assistantStreamID, result.assistantText.String(), result.assistantTextDone)
	}
}

func TestDuplexRoundResultCollectsEveryRouteBeforeReturningTerminalError(t *testing.T) {
	var result duplexRoundResult
	chunks := []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text("question"), Ctrl: &genx.StreamCtrl{StreamID: "round-1:rt:1", Label: duplexTranscriptLabel, BeginOfStream: true}},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "round-1:rt:1", Label: duplexTranscriptLabel, EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "response-1", Label: duplexAssistantLabel, BeginOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "response-1", Label: duplexAssistantLabel, BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response-1", Label: duplexAssistantLabel, EndOfStream: true, Error: "DialogAudioIdleTimeoutError"}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "response-1", Label: duplexAssistantLabel, EndOfStream: true, Error: "DialogAudioIdleTimeoutError"}},
	}
	for index, chunk := range chunks {
		if err := result.observe("round-1", chunk); err != nil {
			t.Fatalf("observe(%d) error = %v", index, err)
		}
		if index == len(chunks)-2 && result.terminalComplete() {
			t.Fatal("result completed after text error EOS before audio error EOS")
		}
	}
	if !result.terminalComplete() {
		t.Fatalf("result = %#v, want all three routes complete", result.lifecycles.routes)
	}
	if err := result.terminalError(); err == nil || !strings.Contains(err.Error(), "DialogAudioIdleTimeoutError") {
		t.Fatalf("terminalError() = %v, want DialogAudioIdleTimeoutError", err)
	}
	result.lifecycles.assertComplete(t)
}

func (r *duplexRoundResult) done() bool {
	return strings.TrimSpace(r.transcript.String()) != "" &&
		r.transcriptDone &&
		strings.TrimSpace(r.assistantText.String()) != "" &&
		r.assistantAudioBytes > 0 &&
		r.assistantTextDone &&
		r.assistantAudioDone
}

func (r *duplexRoundResult) terminalComplete() bool {
	return len(r.terminalErrors) != 0 && r.lifecycles != nil &&
		len(r.lifecycles.routes) == 3 && r.lifecycles.allComplete()
}

func (r *duplexRoundResult) terminalError() error {
	if len(r.terminalErrors) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(r.terminalErrors, "; "))
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
