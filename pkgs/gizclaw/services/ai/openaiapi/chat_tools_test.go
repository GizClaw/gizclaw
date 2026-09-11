package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/idy/ai-server-shell/backend"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const toolRoundTripBody = `{"model":"chat","messages":[
	{"role":"user","content":"which node is busy?"},
	{"role":"assistant","content":null,"tool_calls":[
		{"id":"call_a","type":"function","function":{"name":"node_snapshot","arguments":"{\"node\":\"a\"}"}},
		{"id":"call_b","type":"function","function":{"name":"node_snapshot","arguments":"{\"node\":\"b\"}"}}
	]},
	{"role":"tool","tool_call_id":"call_a","content":"{\"connections\":3}"},
	{"role":"tool","tool_call_id":"call_b","content":[{"type":"text","text":"{\"connections\":9}"}]}
],"tools":[{"type":"function","function":{"name":"node_snapshot","description":"Read one node.","parameters":{"type":"object","properties":{"node":{"type":"string"}},"required":["node"],"additionalProperties":false},"strict":true}}],
"tool_choice":"auto","parallel_tool_calls":true`

func TestHandleChatForwardsToolsAndReturnsToolCalls(t *testing.T) {
	key := mustKey(t)
	var checked []string
	server := &Server{
		Caller: key.Public,
		ToolCalls: toolCallSupportFunc(func(_ context.Context, pattern string) (bool, error) {
			checked = append(checked, pattern)
			return true, nil
		}),
		Generator: generatorFunc(func(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
			var tools []*genx.FuncTool
			for tool := range modelContext.Tools() {
				tools = append(tools, tool.(*genx.FuncTool))
			}
			if len(tools) != 1 || tools[0].Name != "node_snapshot" || tools[0].Description != "Read one node." || !tools[0].Strict ||
				!bytes.Contains(tools[0].Parameters, []byte(`"additionalProperties":false`)) {
				t.Fatalf("tools = %#v", tools)
			}
			var payloads []genx.Payload
			for message := range modelContext.Messages() {
				payloads = append(payloads, message.Payload)
			}
			if len(payloads) != 5 {
				t.Fatalf("messages = %#v", payloads)
			}
			if call, ok := payloads[2].(*genx.ToolCall); !ok || call.ID != "call_b" || call.FuncCall.Arguments != `{"node":"b"}` {
				t.Fatalf("second call = %#v", payloads[2])
			}
			if result, ok := payloads[4].(*genx.ToolResult); !ok || result.ID != "call_b" || result.Result != `{"connections":9}` {
				t.Fatalf("second result = %#v", payloads[4])
			}
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, ToolCall: &genx.ToolCall{ID: "call_c", FuncCall: &genx.FuncCall{Name: "node_snapshot", Arguments: `{"node":"c"}`}}},
			}}, nil
		}),
	}
	response, err := server.Handle(context.Background(), requestFor(
		key.Public, backend.CapabilityChat, "createChatCompletion", json.RawMessage(toolRoundTripBody+`}`),
	))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var completion struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   *string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(response.JSON, &completion); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	choice := completion.Choices[0]
	if choice.FinishReason != "tool_calls" || choice.Message.Content != nil || len(choice.Message.ToolCalls) != 1 ||
		choice.Message.ToolCalls[0].ID != "call_c" || choice.Message.ToolCalls[0].Type != "function" ||
		choice.Message.ToolCalls[0].Function.Arguments != `{"node":"c"}` {
		t.Fatalf("completion = %s", response.JSON)
	}
	if len(checked) != 1 || checked[0] != "model/chat" {
		t.Fatalf("capability checks = %#v", checked)
	}
}

func TestHandleChatStreamsToolCallsAndUsage(t *testing.T) {
	key := mustKey(t)
	server := &Server{
		Caller:    key.Public,
		ToolCalls: toolCallSupportFunc(func(context.Context, string) (bool, error) { return true, nil }),
		Generator: generatorFunc(func(context.Context, string, genx.ModelContext) (genx.Stream, error) {
			return &usageStream{sliceStream: sliceStream{chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Part: genx.Text("checking")},
				{Role: genx.RoleModel, ToolCall: &genx.ToolCall{ID: "call_a", FuncCall: &genx.FuncCall{Name: "node_snapshot", Arguments: `{}`}}},
				{Role: genx.RoleModel, ToolCall: &genx.ToolCall{ID: "call_b", FuncCall: &genx.FuncCall{Name: "node_snapshot", Arguments: `{}`}}},
			}}, usage: genx.Usage{PromptTokenCount: 7, GeneratedTokenCount: 5}}, nil
		}),
	}
	body := `{"model":"chat","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}],` +
		`"tools":[{"type":"function","function":{"name":"node_snapshot"}}]}`
	response, err := server.Handle(context.Background(), requestFor(key.Public, backend.CapabilityChat, "createChatCompletion", json.RawMessage(body)))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	events := collectEvents(t, response.Stream)
	if len(events) != 6 || string(events[5].Data) != "[DONE]" {
		t.Fatalf("events = %q", eventData(events))
	}
	for index, want := range []string{
		`"role":"assistant"`,
		`"tool_calls":[{"function":{"arguments":"{}","name":"node_snapshot"},"id":"call_a","index":0,"type":"function"}]`,
		`"id":"call_b","index":1`,
		`"finish_reason":"tool_calls"`,
		`"usage":{"completion_tokens":5,"prompt_tokens":7,"total_tokens":12}`,
	} {
		if !bytes.Contains(events[index].Data, []byte(want)) {
			t.Fatalf("event %d = %s, want %s", index, events[index].Data, want)
		}
	}
	if bytes.Contains(events[1].Data, []byte(`"role"`)) {
		t.Fatalf("role repeated: %s", events[1].Data)
	}
}

func TestHandleChatRejectsToolsForModelWithoutSupport(t *testing.T) {
	key := mustKey(t)
	for _, test := range []struct {
		name    string
		body    string
		support ToolCallSupport
		code    string
	}{
		{name: "declared tools", body: `{"model":"chat","messages":[],"tools":[{"type":"function","function":{"name":"fn"}}]}`, code: "unsupported_option",
			support: toolCallSupportFunc(func(context.Context, string) (bool, error) { return false, nil })},
		{name: "replayed tool result", body: `{"model":"chat","messages":[{"role":"tool","tool_call_id":"call","content":"ok"}]}`, code: "unsupported_option",
			support: toolCallSupportFunc(func(context.Context, string) (bool, error) { return false, nil })},
		{name: "no capability source", body: `{"model":"chat","messages":[],"tools":[{"type":"function","function":{"name":"fn"}}]}`, code: "tool_calls_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{
				Caller: key.Public, ToolCalls: test.support,
				Generator: generatorFunc(func(context.Context, string, genx.ModelContext) (genx.Stream, error) {
					t.Fatal("generator called")
					return nil, nil
				}),
			}
			_, err := server.Handle(context.Background(), requestFor(key.Public, backend.CapabilityChat, "createChatCompletion", json.RawMessage(test.body)))
			if backendErr, ok := errors.AsType[*backend.Error](err); !ok || backendErr.Code != test.code {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestHandleChatRejectsInvalidToolState(t *testing.T) {
	key := mustKey(t)
	for _, body := range []string{
		`{"model":"chat","messages":[],"tools":[{"type":"function","function":{"name":"bad name"}}]}`,
		`{"model":"chat","messages":[],"tools":[{"type":"function","function":{"name":"fn"}},{"type":"function","function":{"name":"fn"}}]}`,
		`{"model":"chat","messages":[],"tools":[{"type":"function","function":{"name":"fn","parameters":"object"}}]}`,
		`{"model":"chat","messages":[{"role":"assistant","tool_calls":[{"id":"","type":"function","function":{"name":"fn","arguments":"{}"}}]}]}`,
		`{"model":"chat","messages":[{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"fn"}}]}]}`,
		`{"model":"chat","messages":[{"role":"tool","content":"ok"}]}`,
		`{"model":"chat","messages":[],"stream_options":{"include_usage":true}}`,
		// Explicit nulls are present values and must satisfy the contract.
		`{"model":"chat","messages":[],"tools":[{"type":"function","function":{"name":"fn","strict":null}}]}`,
		`{"model":"chat","messages":[],"stream_options":null}`,
		`{"model":"chat","messages":[],"stream":true,"stream_options":null}`,
		`{"model":"chat","messages":[],"stream":true,"stream_options":{"include_usage":null}}`,
		`{"model":"chat","messages":[],"tool_choice":null}`,
		`{"model":"chat","messages":[],"parallel_tool_calls":null}`,
	} {
		_, err := (&Server{Caller: key.Public}).Handle(context.Background(), requestFor(
			key.Public, backend.CapabilityChat, "createChatCompletion", json.RawMessage(body),
		))
		if backendErr, ok := errors.AsType[*backend.Error](err); !ok || backendErr.Kind != backend.ErrorInvalid {
			t.Fatalf("body=%s error = %#v", body, err)
		}
	}
}

// usageStream ends like a provider stream that reported token usage.
type usageStream struct {
	sliceStream
	usage genx.Usage
}

func (s *usageStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.sliceStream.Next()
	if errors.Is(err, genx.ErrDone) {
		return nil, genx.Done(s.usage)
	}
	return chunk, err
}

type toolCallSupportFunc func(context.Context, string) (bool, error)

func (f toolCallSupportFunc) SupportsToolCalls(ctx context.Context, pattern string) (bool, error) {
	return f(ctx, pattern)
}

func eventData(events []backend.Event) []string {
	data := make([]string, 0, len(events))
	for _, event := range events {
		data = append(data, string(event.Data))
	}
	return data
}
