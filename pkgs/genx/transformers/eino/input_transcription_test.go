package eino

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestInputTranscriptionPrecedesScriptsMemoryAndHistory(t *testing.T) {
	memories := &recordingMemoryStore{events: &eventRecorder{}}
	config := textConfig()
	config.Graph.State.Fields = []StateField{
		{Name: "recalled", Type: StateString, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
		{Name: "observation", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{{
		ID: "control", Inputs: map[string]Binding{"text": {From: "input.text"}, "messages": {From: "input.messages"}},
		Outputs: map[string]string{"text": "answer", "observation": "observation"},
		Script: &ScriptNode{Language: ScriptStarlark,
			Limits: ScriptLimits{MaxExecutionSteps: 10_000, Timeout: time.Second, MaxInputBytes: 64 << 10, MaxOutputBytes: 1 << 10},
			Source: `def run(input):
  current = input["text"]
  if current == "" or input["messages"][-1]["content"] != current:
    fail("control did not receive the current transcription")
  return {"text": current + "|" + str(len(input["messages"])), "observation": current}
`},
	}}
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "control"}, {From: "control", To: "end"}}
	config.Graph.Outputs[0].Node = "control"
	config.Memory = &MemoryConfig{Store: memories, Scope: memory.Scope{AppID: "app"},
		Recall:  []RecallDefinition{{QueryFrom: "input.text", Output: "recalled", TopK: 2}},
		Observe: ObservePolicy{Enabled: true, Facts: []ObserveDefinition{{TextFrom: "observation"}}},
	}
	transcriptions := 0
	config.TranscribeInput = func(ctx context.Context, audio []*genx.Blob) (string, error) {
		transcriptions++
		if len(audio) != 2 || audio[0].MIMEType != "audio/opus" || !slices.Equal(audio[1].Data, []byte{2}) {
			return "", fmt.Errorf("unexpected audio input: %v", audio)
		}
		return fmt.Sprintf("heard %d", transcriptions), ctx.Err()
	}
	core, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	for index, want := range []string{"heard 1", "heard 2", "typed"} {
		input := textInput(want)
		if index < 2 {
			_ = input.Close()
			input = audioInput(fmt.Sprintf("speech-%d", index), [][]byte{{1}, {2}})
		}
		output, err := core.Transform(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		var reply, transcript strings.Builder
		for _, chunk := range drain(t, output) {
			if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				t.Fatalf("turn %d: %s", index, chunk.Ctrl.Error)
			}
			if text, ok := chunk.Part.(genx.Text); ok {
				if chunk.Role == genx.RoleModel {
					reply.WriteString(string(text))
				} else if chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" {
					transcript.WriteString(string(text))
				}
			}
		}
		if reply.String() != fmt.Sprintf("%s|%d", want, 2*index+1) || index < 2 && transcript.String() != want {
			t.Fatalf("turn %d: reply=%q transcript=%q", index, reply.String(), transcript.String())
		}
	}
	if transcriptions != 2 {
		t.Fatalf("transcriptions = %d; text input must bypass transcription", transcriptions)
	}
	history, err := core.history.load(t.Context())
	if err != nil || len(history) != 6 {
		t.Fatalf("History = %v, error = %v", history, err)
	}
	memories.mu.Lock()
	defer memories.mu.Unlock()
	if len(memories.queries) != 3 || len(memories.observations) != 3 {
		t.Fatalf("recalls = %v, observations = %v", memories.queries, memories.observations)
	}
	for index, want := range []string{"heard 1", "heard 2", "typed"} {
		observation := memories.observations[index]
		if memories.queries[index].Text != want || observation.Turns[0].Text != want || observation.Facts[0].Text != want || history[2*index].Content != want {
			t.Fatalf("turn %d lost user text: query=%v observation=%v History=%v", index, memories.queries[index], observation, history)
		}
	}
}

func TestInputTranscriptionCancellationStopsBeforeGraph(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started, exited := make(chan struct{}), make(chan struct{})
	config := textConfig()
	config.TranscribeInput = func(ctx context.Context, _ []*genx.Blob) (string, error) {
		close(started)
		defer close(exited)
		<-ctx.Done()
		return "", context.Cause(ctx)
	}
	core, err := New(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	output, err := core.Transform(ctx, audioInput("speech", [][]byte{{1}}))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("transcription did not start")
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("transcription survived output close")
	}
	history, err := core.history.load(t.Context())
	if err != nil || len(history) != 0 {
		t.Fatalf("canceled transcription entered History: %v, %v", history, err)
	}
}

func TestInputTranscriptionEmptyAndFailureDoNotExecuteGraph(t *testing.T) {
	for _, failure := range []error{nil, errors.New("transcription unavailable")} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			config := textConfig()
			config.TranscribeInput = func(context.Context, []*genx.Blob) (string, error) { return "", failure }
			core, err := New(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			output, err := core.Transform(t.Context(), audioInput("speech", [][]byte{{1}}))
			if err != nil {
				t.Fatal(err)
			}
			eos, failed := false, false
			for _, chunk := range drain(t, output) {
				if chunk.Role == genx.RoleModel {
					if text, ok := chunk.Part.(genx.Text); ok && text != "" {
						t.Fatalf("Graph replied without a transcript: %q", text)
					}
					eos = eos || chunk.IsEndOfStream()
					failed = failed || chunk.Ctrl != nil && strings.Contains(chunk.Ctrl.Error, "transcription unavailable")
				}
			}
			if !eos || failed != (failure != nil) {
				t.Fatalf("EOS=%v failed=%v error=%v", eos, failed, failure)
			}
		})
	}
}
