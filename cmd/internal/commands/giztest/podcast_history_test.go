package giztestcmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// Exercise the production Workspace recorder and SQLite persistence. Topic
// Each generated message becomes a History entry while playback stays open.
func TestContinuousPodcastWorkspaceHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tr, err := genxeino.New(ctx, genxeino.Config{
		Agent: genxeino.AgentConfig{ID: "podcast"}, ContinueFrom: "continue",
		Graph: genxeino.GraphDefinition{
			Name: "podcast-history", Compile: genxeino.GraphCompileConfig{NodeTriggerMode: genxeino.NodeTriggerAnyPredecessor, MaxRunSteps: 4},
			State: genxeino.StateDefinition{Fields: []genxeino.StateField{
				{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace},
				{Name: "continue", Type: genxeino.StateBoolean, Merge: genxeino.MergeReplace},
			}},
			Nodes: []genxeino.NodeDefinition{{
				ID: "narrate", Inputs: map[string]genxeino.Binding{"text": {From: "input.text"}},
				Outputs: map[string]string{"text": "answer", "continue": "continue"},
				Script: &genxeino.ScriptNode{
					Language: genxeino.ScriptStarlark,
					Source:   "def run(input):\n    stop = input[\"text\"] == \"stop\"\n    return {\"text\": \"stopped\" if stop else \"A new topic. \", \"continue\": not stop}\n",
					Limits:   genxeino.ScriptLimits{MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 4096, MaxOutputBytes: 4096},
				},
			}},
			Edges:   []genxeino.EdgeDefinition{{From: "start", To: "narrate"}, {From: "narrate", To: "end"}},
			Outputs: []genxeino.OutputDefinition{{Node: "narrate", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	agent := openHistoryRegressionAgent(t, ctx, tr)
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.Close()
	output, err := agent.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	push := func(id, text string) {
		t.Helper()
		for _, chunk := range textInputChunks(&giztest.PeerStreamOperation{TextDone: true}, id, text) {
			if err := input.Push(ctx, chunk); err != nil {
				t.Fatal(err)
			}
		}
	}
	push("start", "podcast")
	var delivered strings.Builder
	firstID, sent, interrupted := "", false, false
	completed := 0
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if firstID == "" {
			firstID = chunk.Ctrl.StreamID
		}
		if chunk.Ctrl.StreamID == firstID {
			if chunk.Ctrl.MessageEnd {
				completed++
			}
			if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
				delivered.WriteString(string(text))
			}
			if chunk.IsEndOfStream() {
				if !sent || chunk.Ctrl.Error != "interrupted" {
					t.Fatalf("podcast ended without user interruption: %+v", chunk.Ctrl)
				}
				interrupted = true
			}
			if completed >= 3 && !sent {
				history, err := agent.ListHistory(ctx, apitypes.PeerRunHistoryListRequest{})
				if err != nil {
					t.Fatal(err)
				}
				agents := 0
				for _, item := range history.Items {
					if item.Type == apitypes.PeerRunHistoryEntryTypeAgent {
						agents++
						if item.Text != "A new topic. " {
							t.Fatalf("History combined generations: %+v", item)
						}
					}
				}
				if agents < 3 {
					t.Fatalf("only %d generated messages persisted before interruption", agents)
				}
				sent = true
				push("stop", "stop")
			}
		} else if chunk.IsEndOfStream() {
			if !interrupted || chunk.Ctrl.Error != "" {
				t.Fatalf("replacement lifecycle: interrupted=%v ctrl=%+v", interrupted, chunk.Ctrl)
			}
			break
		}
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		history, err := agent.ListHistory(ctx, apitypes.PeerRunHistoryListRequest{})
		if err != nil {
			t.Fatal(err)
		}
		agents, prefix, replacement := 0, 0, 0
		for _, item := range history.Items {
			if item.Type != apitypes.PeerRunHistoryEntryTypeAgent {
				continue
			}
			agents++
			if item.Text == "A new topic. " {
				prefix++
			}
			if item.Text == "stopped" {
				replacement++
			}
		}
		if replacement == 1 {
			if prefix != strings.Count(delivered.String(), "A new topic.") || agents != prefix+1 || history.HasNext {
				t.Fatalf("combined or incomplete History: %+v", history.Items)
			}
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("Workspace History was not persisted")
		}
	}
}
