package agenthost

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestEinoNamedResponseReleasesNextHistoryRead(t *testing.T) {
	records := newTestHistoryLogStore(t)
	history := workspace.NewHistoryStore(records, newTestObjectStore(t), "conversation")
	transformer, err := genxeino.New(t.Context(), genxeino.Config{
		Agent:   genxeino.AgentConfig{ID: "named-history"},
		History: &genxeino.HistoryConfig{Store: history, Scope: "conversation", Limit: 50},
		Graph: genxeino.GraphDefinition{
			Name:    "named-history",
			Compile: genxeino.GraphCompileConfig{PrimaryOutputMode: genxeino.PrimaryFirstOutput},
			State:   genxeino.StateDefinition{Fields: []genxeino.StateField{{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace}}},
			Nodes: []genxeino.NodeDefinition{{
				ID: "reply", Inputs: map[string]genxeino.Binding{"past": {From: "history.messages"}, "user": {From: "input.text"}}, Outputs: map[string]string{"text": "answer"},
				Script: &genxeino.ScriptNode{Language: genxeino.ScriptStarlark, Source: "def run(input):\n    return {\"text\": str(len(input[\"past\"])) + \":\" + input[\"user\"]}\n", Limits: genxeino.ScriptLimits{Timeout: time.Second, MaxExecutionSteps: 1000, MaxInputBytes: 1 << 16, MaxOutputBytes: 1 << 16}},
			}},
			Edges:   []genxeino.EdgeDefinition{{From: "start", To: "reply"}, {From: "reply", To: "end"}},
			Outputs: []genxeino.OutputDefinition{{Node: "reply", Field: "answer", Name: "rejected", MIMEType: "text/plain", Primary: true}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	output, err := wrapHistoryAgent(NewTransformerAgent(transformer), history).Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for index, text := range []string{"first", "second"} {
		id := text + "-input"
		if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true, EndOfStream: true}}); err != nil {
			t.Fatal(err)
		}
		var reply string
		for {
			chunk, err := output.Next()
			if err != nil {
				t.Fatalf("turn %d did not complete: %v", index+1, err)
			}
			if chunk.Role == genx.RoleUser {
				continue
			}
			if value, ok := chunk.Part.(genx.Text); ok {
				reply += string(value)
			}
			if chunk.Ctrl != nil && (chunk.Ctrl.Error != "" || chunk.Ctrl.ErrorCode != "") {
				t.Fatalf("turn %d failed: %+v", index+1, chunk.Ctrl)
			}
			if chunk.IsEndOfStream() {
				break
			}
		}
		want := "0:first"
		if index == 1 {
			want = "2:second"
		}
		if reply != want {
			t.Fatalf("turn %d reply=%q want %q", index+1, reply, want)
		}
	}
	page, err := records.Query(ctx, memory.HistoryQuery{ConversationID: "conversation", Limit: 10})
	if err != nil || len(page.Entries) != 4 || page.Entries[1].Name != "rejected" || page.Entries[1].Text != "0:first" {
		t.Fatalf("named reply History was lost: %+v, %v", page, err)
	}
}

func TestHistoryNamedResponseEpochCompletesAllMIMEChannels(t *testing.T) {
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	recorder := newHistoryRecorder(history, "", nil)
	observe := func(part genx.Part, messageID string, eos, epochEnd bool) {
		t.Helper()
		if err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: "rejected", Part: part, Ctrl: &genx.StreamCtrl{StreamID: "response", Label: "assistant", MessageID: messageID, EndOfStream: eos, ResponseEpochEnd: epochEnd}}); err != nil {
			t.Fatal(err)
		}
	}
	observe(genx.Text("visible reply"), "message", false, false)
	observe(&genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}, "message", false, false)
	observe(genx.Text(""), "", true, false)
	if len(recorder.pendingText) != 1 {
		t.Fatal("sibling audio ended History early")
	}
	observe(&genx.Blob{MIMEType: "audio/opus"}, "", true, true)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := recorder.waitForText(ctx); err != nil {
		t.Fatalf("completed response retained its text barrier: %v", err)
	}
	page, err := history.ListPage(t.Context(), apitypes.PeerRunHistoryListRequest{})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "visible reply" || len(page.Entries[0].Assets) != 1 {
		t.Fatalf("paired text/audio History changed: %+v, %v", page, err)
	}
}

func TestHistoryNamedResponseEpochPreservesOtherRoutesAndFinalPayload(t *testing.T) {
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	recorder := newHistoryRecorder(history, "", nil)
	observe := func(stream, label, name, message, text string, eos, epochEnd bool) {
		t.Helper()
		if err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: name, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: stream, Label: label, MessageID: message, EndOfStream: eos, ResponseEpochEnd: epochEnd}}); err != nil {
			t.Fatal(err)
		}
	}
	observe("reply", "assistant", "part", "message", "visible ", false, false)
	observe("reply", "other", "side", "message", "side", false, false)
	observe("another", "assistant", "part", "message", "another", false, false)
	observe("reply", "assistant", "part", "message", "suffix", true, true)
	page, err := history.ListPage(t.Context(), apitypes.PeerRunHistoryListRequest{})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "visible suffix" || len(recorder.pendingText) != 2 {
		t.Fatalf("response completion crossed route boundaries or lost final data: %+v, %v", page, err)
	}
	observe("reply", "other", "side", "", "", true, true)
	observe("another", "assistant", "part", "", "", true, true)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := recorder.waitForText(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryNamedMemberEOSDoesNotCloseItsSibling(t *testing.T) {
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	recorder := newHistoryRecorder(history, "", nil)
	for _, name := range []string{"first", "second"} {
		if err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: name, Part: genx.Text(name), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", MessageID: name}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: "first", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", MessageID: "first", EndOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	page, err := history.ListPage(t.Context(), apitypes.PeerRunHistoryListRequest{})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Text != "first" || len(recorder.pendingText) != 1 {
		t.Fatalf("member EOS prematurely ended a sibling: %+v, %v", page, err)
	}
	if err := recorder.ObserveOutput(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Name: "second", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", EndOfStream: true, ResponseEpochEnd: true}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := recorder.waitForText(ctx); err != nil {
		t.Fatal(err)
	}
}
