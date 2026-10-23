package eino

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

func TestAudioTurnWithoutTranscriptKeepsReply(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("等于八。", nil)}}
	core, err := New(ctx, audioInputConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, audioInput("speech", [][]byte{{0x01}}))
	if err != nil {
		t.Fatal(err)
	}
	var reply strings.Builder
	sideband := 0
	for _, chunk := range drain(t, output) {
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("output error = %q", chunk.Ctrl.Error)
		}
		switch {
		case chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel:
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) != 0 {
				sideband++
			}
		case chunk.Role == genx.RoleUser && chunk.Part != nil:
			t.Fatalf("transcript route published without a transcript: %#v", chunk)
		case chunk.Role == genx.RoleModel:
			if text, ok := chunk.Part.(genx.Text); ok {
				reply.WriteString(string(text))
			}
		}
	}
	if reply.String() != "等于八。" || sideband != 0 {
		t.Fatalf("reply=%q user audio packets=%d", reply.String(), sideband)
	}
	history, err := core.history.load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Role != schema.Assistant || history[0].Content != "等于八。" {
		t.Fatalf("History = %#v, want only the assistant reply", history)
	}
}

func TestAudioInputWithoutAudioCapabilityIsNotATurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("answer", nil)}}
	core, err := New(ctx, chatConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, audioInput("speech", [][]byte{{0x01}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range drain(t, output) {
		if chunk.Role == genx.RoleModel {
			t.Fatalf("text-only Graph answered audio input: %#v", chunk)
		}
	}
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if len(chat.inputs) != 0 {
		t.Fatalf("ChatModel calls = %d, want 0", len(chat.inputs))
	}
}

func TestInterruptedAudioInputStartsNoTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("y", nil)}}
	core, err := New(ctx, audioInputConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true, Error: "interrupted"}},
	); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range drain(t, output) {
		if chunk.Role == genx.RoleModel || chunk.Role == genx.RoleUser && chunk.Part != nil {
			t.Fatalf("interrupted audio produced output: %#v", chunk)
		}
	}
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if len(chat.inputs) != 0 {
		t.Fatalf("ChatModel calls = %d, want 0", len(chat.inputs))
	}
}

func TestAudioInputFailureFailsSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	core, err := New(ctx, audioInputConfig(&componentMapResolver{chat: &fakeChatModel{}}))
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true, EndOfStream: true, Error: "microphone failed"}},
	); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	output, err := core.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err := output.Next()
		if err == nil {
			continue
		}
		if isStreamEnd(err) || errors.Is(err, context.Canceled) {
			t.Fatalf("session ended with %v, want the audio input failure", err)
		}
		if !strings.Contains(err.Error(), "microphone failed") {
			t.Fatalf("session error = %v", err)
		}
		return
	}
}

// An audio turn has no input.text when Memory recall runs. A recall bound to
// input.text is skipped, while a Graph can still recall with a query it
// derives from History.
func TestAudioTurnRecallsWithAQueryDerivedFromHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{

		schema.AssistantMessage("恐龙。", nil),
	}}
	memories := &recordingMemoryStore{events: &eventRecorder{}}
	config := audioInputConfig(&componentMapResolver{chat: chat})
	config.Memory = &MemoryConfig{Store: memories, Scope: memory.Scope{AppID: "app"}}
	config.Graph.State.Fields = append(config.Graph.State.Fields,
		StateField{Name: "recall_query", Type: StateString, Merge: MergeReplace},
		StateField{Name: "recalled", Type: StateString, Merge: MergeReplace},
		StateField{Name: "recalled_by_text", Type: StateString, Merge: MergeReplace},
	)
	config.Graph.Nodes = append(config.Graph.Nodes,
		NodeDefinition{
			ID:      "query",
			Inputs:  map[string]Binding{"text": {From: "input.text"}, "history": {From: "history.messages"}},
			Outputs: map[string]string{"query": "recall_query"},
			Script: &ScriptNode{
				Language: ScriptStarlark,
				Source: "def run(input):\n" +
					"  query = input[\"text\"]\n" +
					"  if query == \"\":\n" +
					"    for message in input[\"history\"]:\n" +
					"      if message[\"role\"] == \"user\" and message[\"content\"] != \"\":\n" +
					"        query = message[\"content\"]\n" +
					"  return {\"query\": query}\n",
				Limits: ScriptLimits{
					MaxExecutionSteps: 10_000, Timeout: time.Second,
					MaxInputBytes: 64 << 10, MaxOutputBytes: 1 << 10,
				},
			},
		},
		NodeDefinition{ID: "recall", MemoryRecall: &MemoryRecallNode{QueryFrom: "recall_query", Output: "recalled", TopK: 2}},
		NodeDefinition{ID: "recall_by_text", MemoryRecall: &MemoryRecallNode{QueryFrom: "input.text", Output: "recalled_by_text", TopK: 2}},
	)
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "query"}, {From: "query", To: "recall"}, {From: "recall", To: "recall_by_text"},
		{From: "recall_by_text", To: "prompt"}, {From: "prompt", To: "model"}, {From: "model", To: "end"},
	}
	core, err := New(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	queries := func() []string {
		memories.mu.Lock()
		defer memories.mu.Unlock()
		texts := make([]string, len(memories.queries))
		for index, query := range memories.queries {
			texts[index] = query.Text
		}
		return texts
	}

	output, err := core.Transform(ctx, textInput("我喜欢恐龙"))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, output)
	if got := queries(); !slices.Equal(got, []string{"我喜欢恐龙", "我喜欢恐龙"}) {
		t.Fatalf("text turn recall queries = %q", got)
	}

	output, err = core.Transform(ctx, audioInput("speech", [][]byte{{0x01}}))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, output)
	// Only the derived query recalls: it falls back to the previous user turn.
	if got := queries(); !slices.Equal(got, []string{"我喜欢恐龙", "我喜欢恐龙", "我喜欢恐龙"}) {
		t.Fatalf("recall queries after the audio turn = %q", got)
	}
}

// audioInputConfig answers through one root audio ChatModel node
// that receives the ordered input messages.
func audioInputConfig(resolver ComponentResolver) Config {
	config := chatConfig(resolver)
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "prompt", Inputs: map[string]Binding{"conversation": {From: "input.messages"}},
		Outputs: map[string]string{"messages": "messages"},
		Prompt: &PromptNode{
			Format: PromptFString,
			Messages: []PromptMessage{
				{Role: PromptSystem, Template: "system"},
				{Placeholder: "conversation"},
			},
		},
	}
	config.AudioInput = true
	return config
}

func audioInput(streamID string, packets [][]byte) genx.Stream {
	input := newInputBuilder()
	for index, packet := range packets {
		_ = input.Add(&genx.MessageChunk{
			Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: index == 0},
		})
	}
	_ = input.Add(&genx.MessageChunk{
		Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true},
	})
	_ = input.Done(genx.Usage{})
	return input.Stream()
}
