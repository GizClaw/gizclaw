package eino

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolrun"
	"github.com/cloudwego/eino/schema"
)

func continuationToolConfig(chat *scriptedChatModel, nested bool, iterations int) Config {
	var generations int
	config := continuationConfig(func(_ context.Context, input map[string]any) (map[string]any, error) {
		generations++
		return map[string]any{"text": input["text"], "continue": generations < iterations}, nil
	})
	config.Components = &componentMapResolver{chat: chat}
	config.Graph.Compile.MaxRunSteps = 8
	config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "news", Type: StateString, Merge: MergeReplace})
	config.Graph.Nodes[0].Inputs["text"] = Binding{From: "news"}
	search := NodeDefinition{
		ID: "search", Inputs: map[string]Binding{"messages": {From: "input.messages"}},
		Outputs: map[string]string{"text": "news"}, ChatModel: &ChatModelNode{Model: "chat"},
	}
	if nested {
		child := GraphDefinition{
			Name: "search-child", State: StateDefinition{Fields: []StateField{{Name: "news", Type: StateString, Merge: MergeReplace}}},
			Nodes:   []NodeDefinition{search},
			Edges:   []EdgeDefinition{{From: "start", To: "search"}, {From: "search", To: "end"}},
			Outputs: []OutputDefinition{{Node: "search", Field: "news", Name: "news", MIMEType: "text/plain", Primary: true}},
		}
		search.ChatModel = nil
		search.Outputs = map[string]string{"news": "news"}
		search.Subgraph = &SubgraphNode{Graph: child}
	}
	config.Graph.Nodes = append([]NodeDefinition{search}, config.Graph.Nodes...)
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "search"}, {From: "search", To: "answer"}, {From: "answer", To: "end"}}
	return config
}

func newsToolProposal(id, query string) *schema.Message {
	return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
		ID: id, Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: fmt.Sprintf(`{"value":%q}`, query)},
	}}}
}

func TestContinuationToolBudgetAndIDsResetPerGeneration(t *testing.T) {
	t.Parallel()
	const iterations = genx.DefaultMaxToolCalls + 8
	for _, nested := range []bool{false, true} {
		for _, maximum := range []int{0, 1} {
			for _, reuseID := range []bool{false, true} {
				t.Run(fmt.Sprintf("nested=%t/maximum=%d/reuse-ID=%t", nested, maximum, reuseID), func(t *testing.T) {
					t.Parallel()
					chat := &scriptedChatModel{}
					var queries []string
					var want strings.Builder
					for generation := range iterations {
						id := fmt.Sprintf("search-%d", generation+1)
						if reuseID {
							id = "search"
						}
						query := fmt.Sprintf("news %d", generation+1)
						queries = append(queries, query)
						text := query + ". "
						want.WriteString(text)
						chat.rounds = append(chat.rounds, []*schema.Message{newsToolProposal(id, query)}, []*schema.Message{schema.AssistantMessage(text, nil)})
					}
					var invoked []string
					config := continuationToolConfig(chat, nested, iterations)
					config.ToolInvoker = einoTestToolInvoker(func(query string) (any, error) {
						invoked = append(invoked, query)
						return map[string]string{"headline": query}, nil
					})
					config.MaxToolCalls = maximum
					store := &recordingHistoryStore{events: &eventRecorder{}}
					config.History = &HistoryConfig{Store: store, Scope: "news", Limit: iterations + 1}
					transformer, err := New(t.Context(), config)
					if err != nil {
						t.Fatal(err)
					}
					defer transformer.Close()
					output, err := transformer.Transform(t.Context(), textInput("news"))
					if err != nil {
						t.Fatal(err)
					}
					defer output.Close()
					chunks := drain(t, output)
					for _, chunk := range chunks {
						if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
							t.Fatalf("continuous news ended after %d tool invocations: %s", len(invoked), chunk.Ctrl.Error)
						}
					}
					if !slices.Equal(invoked, queries) || joinedText(chunks) != want.String() {
						t.Fatalf("queries=%v output=%q, want %d searched sections", invoked, joinedText(chunks), iterations)
					}
					streamID, begins, ends := "", 0, 0
					messageIDs := make(map[string]bool)
					for _, chunk := range chunks {
						if chunk.Ctrl == nil {
							t.Fatal("missing output control")
						}
						if streamID == "" {
							streamID = chunk.Ctrl.StreamID
						}
						if chunk.Ctrl.StreamID != streamID {
							t.Fatal("tool budget reset split the playback route")
						}
						if chunk.IsBeginOfStream() {
							begins++
						}
						if chunk.IsEndOfStream() {
							ends++
						}
						if text, ok := chunk.Part.(genx.Text); ok && text != "" {
							messageIDs[chunk.Ctrl.MessageID] = true
						}
					}
					if begins != 1 || ends != 1 || len(messageIDs) != iterations || messageIDs[""] {
						t.Fatalf("BOS/EOS=%d/%d message IDs=%v", begins, ends, messageIDs)
					}
					messages := continuationRecordedMessages(t, store)
					if len(messages) != iterations+1 || messages[0].Role != schema.User {
						t.Fatalf("History has %d entries, want one user and %d sections", len(messages), iterations)
					}
					for generation, message := range messages[1:] {
						if message.Role != schema.Assistant || message.Content != queries[generation]+". " {
							t.Fatalf("section %d History=%+v", generation+1, message)
						}
					}
				})
			}
		}
	}
}

func TestGraphGenerationSharesToolGuardsWithNestedNodes(t *testing.T) {
	t.Parallel()
	for _, continuous := range []bool{false, true} {
		for _, nested := range []bool{false, true} {
			for _, duplicate := range []bool{false, true} {
				t.Run(fmt.Sprintf("continuous=%t/nested=%t/duplicate=%t", continuous, nested, duplicate), func(t *testing.T) {
					t.Parallel()
					secondID, wantError, maximum := "second", "ToolCall limit exceeded", 1
					if duplicate {
						secondID, wantError, maximum = "first", "duplicate ToolCall ID", 2
					}
					chat := &scriptedChatModel{rounds: [][]*schema.Message{
						{newsToolProposal("first", "first")}, {schema.AssistantMessage("first", nil)},
						{newsToolProposal(secondID, "second")}, {schema.AssistantMessage("second", nil)},
					}}
					config := continuationToolConfig(chat, nested, 1)
					if !continuous {
						config.ContinueFrom = ""
					}
					config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "first_result", Type: StateString, Merge: MergeReplace})
					config.Graph.Nodes = append([]NodeDefinition{{
						ID: "first", Inputs: map[string]Binding{"messages": {From: "input.messages"}},
						Outputs: map[string]string{"text": "first_result"}, ChatModel: &ChatModelNode{Model: "chat"},
					}}, config.Graph.Nodes...)
					config.Graph.Edges = append([]EdgeDefinition{{From: "start", To: "first"}, {From: "first", To: "search"}}, config.Graph.Edges[1:]...)
					var invoked atomic.Int32
					config.ToolInvoker = einoTestToolInvoker(func(query string) (any, error) {
						invoked.Add(1)
						return map[string]string{"headline": query}, nil
					})
					config.MaxToolCalls = maximum
					transformer, err := New(t.Context(), config)
					if err != nil {
						t.Fatal(err)
					}
					defer transformer.Close()
					output, err := transformer.Transform(t.Context(), textInput("news"))
					if err != nil {
						t.Fatal(err)
					}
					defer output.Close()
					var terminal string
					chunks := drain(t, output)
					for _, chunk := range chunks {
						if chunk.IsEndOfStream() && chunk.Ctrl != nil {
							terminal = chunk.Ctrl.Error
						}
					}
					if !strings.Contains(terminal, wantError) || invoked.Load() != 1 || joinedText(chunks) != "" {
						t.Fatalf("terminal=%q invocations=%d output=%q", terminal, invoked.Load(), joinedText(chunks))
					}
				})
			}
		}
	}
}

func TestContinuationPreservesInheritedToolBudget(t *testing.T) {
	t.Parallel()
	chat := &scriptedChatModel{rounds: [][]*schema.Message{
		{newsToolProposal("first", "first")}, {schema.AssistantMessage("first", nil)},
		{newsToolProposal("second", "second")},
	}}
	var invoked atomic.Int32
	invoker := einoTestToolInvoker(func(query string) (any, error) {
		invoked.Add(1)
		return map[string]string{"headline": query}, nil
	})
	config := continuationToolConfig(chat, true, 2)
	config.ToolInvoker = invoker
	config.MaxToolCalls = 10
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	ctx := toolrun.WithContext(t.Context(), toolrun.New(invoker, 1))
	output, err := transformer.Transform(ctx, textInput("news"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var terminal string
	for _, chunk := range drain(t, output) {
		if chunk.IsEndOfStream() && chunk.Ctrl != nil {
			terminal = chunk.Ctrl.Error
		}
	}
	if !strings.Contains(terminal, "ToolCall limit exceeded") || invoked.Load() != 1 {
		t.Fatalf("inherited budget bypassed: terminal=%q invocations=%d", terminal, invoked.Load())
	}
}
