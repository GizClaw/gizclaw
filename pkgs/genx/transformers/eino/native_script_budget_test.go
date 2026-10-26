package eino

import (
	"bytes"
	"context"
	"encoding/base64"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

func TestNativeScriptBudgetPreservesAudioAndHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("reply", nil)}}
	config := audioInputConfig(&componentMapResolver{chat: chat})
	config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "heard", Type: StateString, Merge: MergeReplace})
	config.Graph.Nodes = append(config.Graph.Nodes, NodeDefinition{
		ID: "before", Inputs: map[string]Binding{"text": {From: "input.text"}, "messages": {From: "input.messages"}},
		Outputs: map[string]string{"value": "heard"},
		Script: &ScriptNode{Language: ScriptStarlark, Source: `def run(input):
    if input["messages"][-1]["role"] != "user":
        fail("missing current message")
    return {"value": input["text"]}
`, Limits: ScriptLimits{MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 262144, MaxOutputBytes: 4096}},
	})
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "before"}, {From: "before", To: "prompt"}, {From: "prompt", To: "model"}, {From: "model", To: "end"}}
	memories := &recordingMemoryStore{events: &eventRecorder{}}
	config.Memory = &MemoryConfig{Store: memories, Scope: memory.Scope{AppID: "app"}, Observe: ObservePolicy{Enabled: true, TextFrom: "heard"}}
	core, err := New(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	previous, err := core.Transform(ctx, textInput("earlier context"))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, previous)
	dock := transcriptionDock(t, core, func(context.Context, []*genx.Blob) (string, error) {
		return "current final transcript", nil
	}, true)
	audio := bytes.Repeat([]byte{1, 2, 3, 4}, 100000)
	output, err := dock.Transform(ctx, audioInput("long-speech", [][]byte{audio}))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	drain(t, output)
	chat.mu.Lock()
	input := chat.inputs[len(chat.inputs)-1]
	chat.mu.Unlock()
	last := input[len(input)-1]
	if len(input) < 4 || input[1].Content != "earlier context" || len(last.UserInputMultiContent) != 1 {
		t.Fatal("native model lost conversation history or the audio part")
	}
	data, err := base64.StdEncoding.DecodeString(*last.UserInputMultiContent[0].Audio.Base64Data)
	if err != nil || !slices.Equal(data, audio) {
		t.Fatalf("native audio changed after the Script node: decode=%v bytes=%d", err, len(data))
	}
	memories.mu.Lock()
	defer memories.mu.Unlock()
	if len(memories.observations) == 0 || memories.observations[len(memories.observations)-1].Text != "current final transcript" {
		t.Fatal("Script did not receive the final transcript")
	}
}
