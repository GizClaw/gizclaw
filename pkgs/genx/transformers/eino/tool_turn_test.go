package eino

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/schema"
)

type testToolTurnKey struct{}

type testObservedToolInvoker struct {
	genx.ToolInvoker
	began, finished, attempts int
	streamID                  string
}

func (i *testObservedToolInvoker) BeginToolTurn(ctx context.Context, streamID string) (context.Context, func()) {
	i.began++
	i.streamID = streamID
	return context.WithValue(ctx, testToolTurnKey{}, i), func() { i.finished++ }
}

func (i *testObservedToolInvoker) InvokeTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if ctx.Value(testToolTurnKey{}) == i {
		i.attempts++
	}
	return i.ToolInvoker.InvokeTool(ctx, name, args)
}

func TestToolTurnObservationClosesAcrossGraphContinuations(t *testing.T) {
	for _, budgetExhausted := range []bool{false, true} {
		name := "success"
		if budgetExhausted {
			name = "budget_exhausted"
		}
		t.Run(name, func(t *testing.T) {
			chat := &scriptedChatModel{rounds: [][]*schema.Message{
				{newsToolProposal("first", "first")}, {schema.AssistantMessage("First report.", nil)},
				{newsToolProposal("second", "second")}, {schema.AssistantMessage("Second report.", nil)},
			}}
			if budgetExhausted {
				chat.rounds[3] = []*schema.Message{newsToolProposal("third", "third")}
			}
			invoker := &testObservedToolInvoker{ToolInvoker: einoTestToolInvoker(func(query string) (any, error) {
				return map[string]string{"headline": query}, nil
			})}
			config := continuationToolConfig(chat, true, 2)
			config.ToolInvoker = invoker
			config.MaxToolCalls = 1
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
			if invoker.began != 1 || invoker.finished != 1 || invoker.attempts != 2 {
				t.Fatalf("turn scope began=%d finished=%d attempts=%d", invoker.began, invoker.finished, invoker.attempts)
			}
			if invoker.streamID == "" || chunks[0].Ctrl.StreamID != invoker.streamID {
				t.Fatal("observation was not correlated with the turn's output route")
			}
			terminal := chunks[len(chunks)-1].Ctrl.Error
			if budgetExhausted && !strings.Contains(terminal, "ToolCall limit exceeded") || !budgetExhausted && terminal != "" {
				t.Fatalf("unexpected terminal error %q", terminal)
			}
		})
	}
}
