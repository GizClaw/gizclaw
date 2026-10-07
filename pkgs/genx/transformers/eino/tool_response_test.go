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
	"github.com/cloudwego/eino/components/model"
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
	if len(chat.inputs[1]) != 3 || chat.inputs[1][2].Role != schema.System || chat.inputs[1][2].Name != "tool_response_feedback" {
		t.Fatal("correction did not preserve the user turn and separate feedback")
	}
	for _, message := range chat.inputs[1] {
		if strings.Contains(message.Content, "already done without a call") {
			t.Fatal("rejected assistant draft entered model history")
		}
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

type malformedToolChatModel struct {
	*scriptedChatModel
	failures, attempts int
	observed           [][]*schema.Message
}

func (chat *malformedToolChatModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	chat.observed = append(chat.observed, cloneMessages(input))
	chat.attempts++
	if chat.attempts <= chat.failures {
		reader, writer := schema.Pipe[*schema.Message](1)
		writer.Send(nil, genx.ErrInvalidToolArguments)
		writer.Close()
		return reader, nil
	}
	return chat.scriptedChatModel.Stream(ctx, input, options...)
}

func TestMalformedToolArgumentsRegenerateWithoutDispatchOrParameterRepair(t *testing.T) {
	chat := &malformedToolChatModel{failures: 1, scriptedChatModel: &scriptedChatModel{rounds: [][]*schema.Message{
		{{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "valid-new-call", Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"requested"}`}}}}},
		{schema.AssistantMessage("actual success", nil)},
	}}}
	var calls atomic.Int32
	config := chatConfig(&componentMapResolver{chat: chat})
	config.ToolInvoker = einoTestToolInvoker(func(value string) (any, error) {
		calls.Add(1)
		if value != "requested" {
			t.Fatalf("recovery synthesized parameters: %q", value)
		}
		return map[string]bool{"ok": true}, nil
	})
	config.VerifyToolResponse = func(context.Context, genx.ToolConversation, string) (string, error) { return "", nil }
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	output, err := transformer.Transform(t.Context(), textInput("requested"))
	if err != nil {
		t.Fatal(err)
	}
	if text := joinedText(drain(t, output)); text != "actual success" || calls.Load() != 1 || chat.attempts != 3 {
		t.Fatalf("reply=%q executions=%d model attempts=%d", text, calls.Load(), chat.attempts)
	}
	feedback := chat.observed[1][len(chat.observed[1])-1]
	if feedback.Role != schema.System || feedback.Name != "tool_argument_feedback" || !strings.Contains(feedback.Content, "not executed") {
		t.Fatalf("syntax correction lost its non-execution boundary: %#v", feedback)
	}
}

func TestMalformedToolArgumentsRemainBoundedAndRequireVerification(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "verification absent", true: "bounded regeneration"}[enabled], func(t *testing.T) {
			chat := &malformedToolChatModel{failures: 10, scriptedChatModel: &scriptedChatModel{}}
			config := chatConfig(&componentMapResolver{chat: chat})
			config.ToolInvoker = einoTestToolInvoker(func(string) (any, error) { t.Fatal("invalid JSON dispatched"); return nil, nil })
			if enabled {
				config.VerifyToolResponse = func(context.Context, genx.ToolConversation, string) (string, error) {
					t.Fatal("invalid proposal became final reply")
					return "", nil
				}
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
			chunks := drain(t, output)
			if joinedText(chunks) != "" {
				t.Fatal("invalid proposal acquired published text")
			}
			wantAttempts := 1
			if enabled {
				wantAttempts = 3
			}
			if chat.attempts != wantAttempts {
				t.Fatalf("model attempts=%d want=%d", chat.attempts, wantAttempts)
			}
		})
	}
}
