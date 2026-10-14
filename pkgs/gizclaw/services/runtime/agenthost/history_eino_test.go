package agenthost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestEinoReadsTheOnlyWorkspaceHistory(t *testing.T) {
	for _, test := range []struct {
		name   string
		docked bool
		noIDs  bool
	}{{name: "text"}, {name: "AudioDock", docked: true}, {name: "generatedIDs", docked: true, noIDs: true}} {
		t.Run(test.name, func(t *testing.T) { testEinoReadsTheOnlyWorkspaceHistory(t, test.docked, test.noIDs) })
	}
}

func testEinoReadsTheOnlyWorkspaceHistory(t *testing.T, docked, noIDs bool) {
	records := newTestHistoryLogStore(t)
	history := workspace.NewHistoryStore(records, newTestObjectStore(t), "conversation")
	config := genxeino.Config{
		Agent:   genxeino.AgentConfig{ID: "assistant"},
		History: &genxeino.HistoryConfig{Store: history, Scope: "conversation", Limit: 50},
		Graph: genxeino.GraphDefinition{
			Name:    "history",
			State:   genxeino.StateDefinition{Fields: []genxeino.StateField{{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace}}},
			Nodes:   []genxeino.NodeDefinition{{ID: "answer", Inputs: map[string]genxeino.Binding{"past": {From: "history.messages"}, "user": {From: "input.text"}}, Outputs: map[string]string{"text": "answer"}, Script: &genxeino.ScriptNode{Language: genxeino.ScriptStarlark, Source: "def run(input):\n  return {\"text\": str(len(input[\"past\"])) + \":\" + input[\"user\"]}\n", Limits: genxeino.ScriptLimits{Timeout: time.Second, MaxExecutionSteps: 1000, MaxInputBytes: 1 << 16, MaxOutputBytes: 1 << 16}}}},
			Edges:   []genxeino.EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
			Outputs: []genxeino.OutputDefinition{{Node: "answer", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}},
		},
	}
	transformer, err := genxeino.New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	var composed genx.Transformer = transformer
	if docked {
		composed, err = audiodock.New(audiodock.Config{Agent: transformer})
		if err != nil {
			t.Fatal(err)
		}
	}
	agent := wrapHistoryAgent(NewTransformerAgent(composed), history)
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	output, err := agent.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	addTurn := func(id, text string) {
		t.Helper()
		if noIDs {
			id = ""
		}
		if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}); err != nil {
			t.Fatal(err)
		}
	}
	addTurn("first-input", "first")
	first := ""
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Role == genx.RoleUser {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			first += string(text)
		}
		if chunk.IsEndOfStream() {
			break
		}
	}
	if first != "0:first" {
		t.Fatalf("first reply=%q", first)
	}
	addTurn("second-input", "second")
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	second := ""
	for {
		chunk, err := output.Next()
		if errors.Is(err, genx.ErrDone) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Role != genx.RoleUser {
			if text, ok := chunk.Part.(genx.Text); ok {
				second += string(text)
			}
		}
	}
	if second != "2:second" {
		t.Fatalf("second reply=%q; expected previous user and assistant, excluding current input", second)
	}
	page, err := records.Query(t.Context(), memory.HistoryQuery{ConversationID: "conversation", Limit: 10})
	if err != nil || len(page.Entries) != 4 {
		t.Fatalf("history duplicated or lost: %+v err=%v", page, err)
	}
	want := []string{"first", "0:first", "second", "2:second"}
	for index, entry := range page.Entries {
		if entry.Text != want[index] {
			t.Fatalf("history[%d]=%q want %q", index, entry.Text, want[index])
		}
	}
}

func TestEinoHistoryPreservesSpeakerContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		docked bool
		voices map[string]string
		policy audiodock.UnknownSpeakerPolicy
		live   string
	}{
		{name: "plain"},
		{name: "known speakers", docked: true, voices: map[string]string{"林知": "voice/a", "阿澈": "voice/b"}, live: "我支持四号。\n我反对四号。"},
		{name: "known and unknown speakers", docked: true, voices: map[string]string{"林知": "voice/a"}, policy: audiodock.UnknownSpeakerDefaultVoice, live: "我支持四号。\n阿澈: 我反对四号。"},
		{name: "unknown speakers only", docked: true, policy: audiodock.UnknownSpeakerDefaultVoice, live: "林知: 我支持四号。\n阿澈: 我反对四号。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			records := newTestHistoryLogStore(t)
			history := workspace.NewHistoryStore(records, newTestObjectStore(t), "conversation")
			config := genxeino.Config{Agent: genxeino.AgentConfig{ID: "assistant"}, History: &genxeino.HistoryConfig{Store: history, Scope: "conversation", Limit: 50}, Graph: genxeino.GraphDefinition{
				Name: "probe", State: genxeino.StateDefinition{Fields: []genxeino.StateField{{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace}}},
				Nodes: []genxeino.NodeDefinition{{ID: "answer", Inputs: map[string]genxeino.Binding{"past": {From: "history.messages"}, "user": {From: "input.text"}}, Outputs: map[string]string{"text": "answer"}, Script: &genxeino.ScriptNode{Language: genxeino.ScriptStarlark, Source: "def run(input):\n  if input[\"user\"] == \"first\":\n    return {\"text\": \"【林知】我支持四号。\\n【阿澈】我反对四号。\"}\n  marked = 0\n  for message in input[\"past\"]:\n    if \"【林知】\" in message.get(\"content\", \"\"):\n      marked += 1\n    if \"【阿澈】\" in message.get(\"content\", \"\"):\n      marked += 1\n  return {\"text\": \"past=\" + str(len(input[\"past\"])) + \";markers=\" + str(marked)}\n", Limits: genxeino.ScriptLimits{Timeout: time.Second, MaxExecutionSteps: 1000, MaxInputBytes: 1 << 16, MaxOutputBytes: 1 << 16}}}},
				Edges: []genxeino.EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}}, Outputs: []genxeino.OutputDefinition{{Node: "answer", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}},
			}}
			transformer, err := genxeino.New(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			defer transformer.Close()
			var composed genx.Transformer = transformer
			if tc.docked {
				composed, err = audiodock.New(audiodock.Config{Agent: transformer, SpeakerVoices: tc.voices, UnknownSpeakerPolicy: tc.policy})
				if err != nil {
					t.Fatal(err)
				}
			}
			agent := wrapHistoryAgent(NewTransformerAgent(composed), history)
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			output, err := agent.Transform(ctx, input.Stream())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			add := func(id, text string) {
				t.Helper()
				if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}); err != nil {
					t.Fatal(err)
				}
			}
			add("input-1", "first")
			first := ""
			for {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				if chunk.Role == genx.RoleUser {
					continue
				}
				if part, ok := chunk.Part.(genx.Text); ok {
					first += string(part)
				}
				if chunk.IsEndOfStream() {
					break
				}
			}
			add("input-2", "second")
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			second := ""
			for {
				chunk, err := output.Next()
				if errors.Is(err, genx.ErrDone) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if chunk.Role != genx.RoleUser {
					if part, ok := chunk.Part.(genx.Text); ok {
						second += string(part)
					}
				}
			}
			if tc.docked && first != tc.live {
				t.Fatalf("live speaker attribution=%q want %q", first, tc.live)
			}
			if second != "past=2;markers=2" {
				t.Fatalf("model history lost speaker markers: %s", second)
			}
			page, err := records.Query(t.Context(), memory.HistoryQuery{ConversationID: "conversation", Limit: 10})
			if err != nil || len(page.Entries) != 4 {
				t.Fatalf("speaker history duplicated or lost: %+v err=%v", page, err)
			}
			want := []string{"first", "【林知】我支持四号。\n【阿澈】我反对四号。", "second", "past=2;markers=2"}
			for index, entry := range page.Entries {
				if entry.Text != want[index] {
					t.Fatalf("history[%d]=%q want %q", index, entry.Text, want[index])
				}
			}
		})
	}
}
