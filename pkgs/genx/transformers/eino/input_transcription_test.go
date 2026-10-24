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
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

func TestNativeAudioConsumesOnlyItsSuccessfulOuterEnd(t *testing.T) {
	config := audioInputConfig(&componentMapResolver{chat: &fakeChatModel{chunks: []*schema.Message{
		schema.AssistantMessage("reply", nil),
	}}})
	core, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	input := newInputBuilder()
	cause := errors.New("audio transport failed")
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true}},
		{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true, Error: "interrupted"}},
		{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true, ErrorCause: cause}},
		{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true, ErrorCode: "transport_unavailable"}},
		{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "unrelated", EndOfStream: true}},
		{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true}},
	} {
		if err := input.Add(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	foreign, interrupted, causeOnly, codeOnly := false, false, false, false
	for _, chunk := range drain(t, output) {
		if chunk.Part != nil || !chunk.IsEndOfStream() || chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			continue
		}
		if chunk.Ctrl.StreamID == "speech" && genx.StreamError(chunk.Ctrl) == nil {
			t.Fatal("consumed audio route EOS escaped into transcript output")
		}
		foreign = foreign || chunk.Ctrl.StreamID == "unrelated"
		interrupted = interrupted || chunk.Ctrl.StreamID == "speech" && chunk.Ctrl.Error == "interrupted"
		causeOnly = causeOnly || chunk.Ctrl.StreamID == "speech" && chunk.Ctrl.Error == "" && errors.Is(genx.StreamError(chunk.Ctrl), cause)
		codeOnly = codeOnly || chunk.Ctrl.StreamID == "speech" && chunk.Ctrl.Error == "" && chunk.Ctrl.ErrorCode == "transport_unavailable"
	}
	if !foreign || !interrupted || !causeOnly || !codeOnly {
		t.Fatalf("forwarded EOS: unrelated=%t interrupted=%t cause-only=%t code-only=%t", foreign, interrupted, causeOnly, codeOnly)
	}
}

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
	transcribe := func(ctx context.Context, audio []*genx.Blob) (string, error) {
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
		output, err := transcriptionDock(t, core, transcribe).Transform(t.Context(), input)
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
	transcribe := func(ctx context.Context, _ []*genx.Blob) (string, error) {
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
	output, err := transcriptionDock(t, core, transcribe).Transform(ctx, audioInput("speech", [][]byte{{1}}))
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

func TestInputTranscriptionFailureDoesNotExecuteGraph(t *testing.T) {
	config := textConfig()
	core, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	failure := errors.New("transcription unavailable")
	dock := transcriptionDock(t, core, func(context.Context, []*genx.Blob) (string, error) { return "", failure })
	output, err := dock.Transform(t.Context(), audioInput("speech", [][]byte{{1}}))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for {
		chunk, err := output.Next()
		if err != nil {
			if !errors.Is(err, failure) {
				t.Fatalf("error=%v", err)
			}
			return
		}
		if chunk.Role == genx.RoleModel {
			t.Fatalf("Graph executed after ASR failure: %+v", chunk)
		}
	}
}

// The fixture transcribes complete audio turns; AudioDock and the downstream
// Eino Graph are production components, including their cancellation boundary.
func transcriptionDock(t testing.TB, agent genx.Transformer, transcribe func(context.Context, []*genx.Blob) (string, error), native ...bool) *audiodock.Dock {
	t.Helper()
	asr := latencyTransformer(func(ctx context.Context, input genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{})
		go func() {
			defer output.Close()
			defer input.Close()
			var audio []*genx.Blob
			for {
				chunk, err := input.Next()
				if err != nil {
					return
				}
				if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) != 0 {
					audio = append(audio, blob)
				}
				if !chunk.IsEndOfStream() || len(audio) == 0 {
					continue
				}
				text, err := transcribe(ctx, audio)
				if err != nil {
					_ = output.CloseWithError(err)
					return
				}
				_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: chunk.Ctrl.StreamID, Label: "transcript", BeginOfStream: true, EndOfStream: true}})
				audio = nil
			}
		}()
		return output, nil
	})
	dock, err := audiodock.New(audiodock.Config{Agent: agent, ASR: asr, AgentAudioInput: len(native) != 0 && native[0]})
	if err != nil {
		t.Fatal(err)
	}
	return dock
}
