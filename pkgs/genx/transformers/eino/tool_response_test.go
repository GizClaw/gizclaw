package eino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/schema"
)

func TestVerifiedToolResponseRepairsMissingCallWithoutPublishingDrafts(t *testing.T) {
	var executions atomic.Int32
	chat := &scriptedChatModel{rounds: [][]*schema.Message{
		{{Content: "already done without a call"}},
		{{Role: schema.Assistant, Content: "unverified promise", ToolCalls: []schema.ToolCall{{ID: "private-provider-id", Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"requested"}`}}}}},
		{schema.AssistantMessage("actual result confirmed", nil)},
	}}
	config := chatConfig(&componentMapResolver{chat: chat})
	config.ToolInvoker = einoTestToolInvoker(func(value string) (any, error) {
		executions.Add(1)
		return map[string]string{"found": value}, nil
	})
	checks := 0
	config.VerifyToolResponse = func(_ context.Context, conversation genx.ToolConversation, reply string) (string, error) {
		checks++
		if conversation.CurrentUser != "requested" || conversation.ContinuationStart != 2 {
			t.Fatalf("current turn boundary = %#v", conversation)
		}
		if checks == 1 {
			if len(conversation.Messages) != conversation.ContinuationStart {
				t.Fatal("unexecuted draft acquired Tool proof")
			}
			return "a supported operation is missing", nil
		}
		var results int
		for _, message := range conversation.Messages[conversation.ContinuationStart:] {
			if message.Role == "tool" && message.Name == "lookup" && message.Content == `{"found":"requested"}` {
				results++
			}
		}
		encoded, _ := json.Marshal(conversation)
		if results != 1 || strings.Contains(string(encoded), "private-provider-id") || reply != "actual result confirmed" {
			t.Fatalf("current proof or final draft is wrong: %s; reply=%q", encoded, reply)
		}
		return "", nil
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	output, err := transformer.Transform(t.Context(), textInput("requested"))
	if err != nil {
		t.Fatal(err)
	}
	if got := joinedText(drain(t, output)); got != "actual result confirmed" || executions.Load() != 1 || checks != 2 {
		t.Fatalf("reply=%q executions=%d checks=%d", got, executions.Load(), checks)
	}
	if chat.inputs[1][2].Role != schema.Assistant {
		t.Fatal("role-free text draft was appended as an invalid model message")
	}
}

func TestVerifiedToolResponseFailsClosedAndBoundsCorrections(t *testing.T) {
	for _, providerFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "repeated rejection", true: "verification error"}[providerFailure], func(t *testing.T) {
			chat := &scriptedChatModel{rounds: [][]*schema.Message{
				{schema.AssistantMessage("unsafe draft one", nil)},
				{schema.AssistantMessage("unsafe draft two", nil)},
				{schema.AssistantMessage("unsafe draft three", nil)},
			}}
			config := chatConfig(&componentMapResolver{chat: chat})
			config.ToolInvoker = einoTestToolInvoker(func(string) (any, error) { t.Fatal("unexpected execution"); return nil, nil })
			checks := 0
			config.VerifyToolResponse = func(context.Context, genx.ToolConversation, string) (string, error) {
				checks++
				if providerFailure {
					return "", errors.New("checker unavailable")
				}
				return "missing actual proof", nil
			}
			transformer, err := New(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer transformer.Close()
			output, err := transformer.Transform(t.Context(), textInput("request"))
			if err != nil {
				t.Fatal(err)
			}
			chunks := drain(t, output)
			if got := joinedText(chunks); got != "" {
				t.Fatalf("unverified text escaped: %q", got)
			}
			var terminal string
			for _, chunk := range chunks {
				if chunk.IsEndOfStream() && chunk.Ctrl != nil {
					terminal = chunk.Ctrl.Error
				}
			}
			want := 3
			if providerFailure {
				want = 1
			}
			if terminal == "" || checks != want {
				t.Fatalf("terminal=%q checks=%d, want %d", terminal, checks, want)
			}
		})
	}
}

func TestVerifiedToolResponseConversationIsInvocationLocal(t *testing.T) {
	config := chatConfig(&componentMapResolver{chat: &concurrentToolChatModel{}})
	config.ToolInvoker = einoTestToolInvoker(func(value string) (any, error) { return map[string]string{"found": value}, nil })
	config.VerifyToolResponse = func(_ context.Context, conversation genx.ToolConversation, reply string) (string, error) {
		if reply != conversation.CurrentUser {
			return "", errors.New("another invocation's reply")
		}
		for _, message := range conversation.Messages[conversation.ContinuationStart:] {
			if message.Role == "tool" {
				var result map[string]string
				if err := json.Unmarshal([]byte(message.Content), &result); err != nil || result["found"] != reply {
					return "", errors.New("another invocation's result")
				}
				return "", nil
			}
		}
		return "", errors.New("missing current result")
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	var runs sync.WaitGroup
	for range 12 {
		runs.Go(func() {
			input := genx.NewStreamID()
			output, err := transformer.Transform(t.Context(), textInput(input))
			if err != nil {
				t.Error(err)
				return
			}
			if got := joinedText(drain(t, output)); got != input {
				t.Errorf("reply=%q, want %q", got, input)
			}
		})
	}
	runs.Wait()
}

func TestToolConversationDoesNotTreatHistoricalResultsAsCurrentProof(t *testing.T) {
	history := []*schema.Message{
		schema.UserMessage("old request"),
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "old-id", Function: schema.FunctionCall{Name: "lookup", Arguments: `{}`}}}},
		{Role: schema.Tool, ToolCallID: "old-id", ToolName: "lookup", Content: `{"found":"old"}`},
		schema.UserMessage("new request"),
	}
	conversation := toolConversation(history, len(history), "new request")
	if conversation.ContinuationStart != 5 || len(conversation.Messages[conversation.ContinuationStart:]) != 0 {
		t.Fatalf("historical Tool proof crossed current boundary: %#v", conversation)
	}
}

func TestVerifiedToolResponseRetainsOutputByteLimit(t *testing.T) {
	config := chatConfig(&componentMapResolver{chat: &fakeChatModel{chunks: []*schema.Message{{Content: "0123456789"}}}})
	config.ToolInvoker = einoTestToolInvoker(func(string) (any, error) { return nil, nil })
	config.Limits.MaxOutputBytes = 4
	config.VerifyToolResponse = func(context.Context, genx.ToolConversation, string) (string, error) {
		t.Error("oversized draft reached verification")
		return "", nil
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	output, err := transformer.Transform(t.Context(), textInput("request"))
	if err != nil {
		t.Fatal(err)
	}
	chunks := drain(t, output)
	if joinedText(chunks) != "" {
		t.Fatal("oversized unverified text escaped")
	}
	var terminal string
	for _, chunk := range chunks {
		if chunk.IsEndOfStream() && chunk.Ctrl != nil {
			terminal = chunk.Ctrl.Error
		}
	}
	if !strings.Contains(terminal, "buffered model reply exceeds MaxOutputBytes") {
		t.Fatalf("terminal=%q", terminal)
	}
}
