package genx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

func TestFormatOpenAISchemaObjectFieldsBecomeNullableAndRequired(t *testing.T) {
	s := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"name": {Type: "string"},
		},
	}

	out := FormatOpenAISchema(s)
	if out.AdditionalProperties == nil {
		t.Fatal("expected AdditionalProperties to be set")
	}
	if !slices.Contains(out.Required, "name") {
		t.Fatalf("expected required to contain name, got: %#v", out.Required)
	}
	if !slices.Contains(out.Properties["name"].Types, "null") {
		t.Fatalf("expected name to include null type, got: %#v", out.Properties["name"])
	}
}

func TestOpenAIPatchSchemaUsesCustomFormatter(t *testing.T) {
	in := &jsonschema.Schema{Type: "object"}
	g := &OpenAIGenerator{
		SchemaFormatter: func(m *jsonschema.Schema) *jsonschema.Schema {
			m.Description = "patched"
			return m
		},
	}

	out := g.patchSchema(in)
	if out == in {
		t.Fatal("expected patchSchema to clone input schema")
	}
	if out.Description != "patched" {
		t.Fatalf("unexpected description: %q", out.Description)
	}
}

type payloadStub struct{}

func (payloadStub) isPayload() {}

type fakeDecoder struct {
	events []ssestream.Event
	idx    int
	err    error
}

func (d *fakeDecoder) Event() ssestream.Event {
	return d.events[d.idx-1]
}

func (d *fakeDecoder) Next() bool {
	if d.idx >= len(d.events) {
		return false
	}
	d.idx++
	return true
}

func (d *fakeDecoder) Close() error { return nil }

func (d *fakeDecoder) Err() error { return d.err }

func newChunkStream(events ...string) *ssestream.Stream[openai.ChatCompletionChunk] {
	evts := make([]ssestream.Event, 0, len(events))
	for _, e := range events {
		evts = append(evts, ssestream.Event{Data: []byte(e)})
	}
	return ssestream.NewStream[openai.ChatCompletionChunk](&fakeDecoder{events: evts}, nil)
}

func testOpenAIContext(withTool bool) ModelContext {
	var mcb ModelContextBuilder
	mcb.PromptText("sys", "system prompt")
	mcb.UserText("u", "hello")
	mcb.ModelText("m", "world")
	mcb.Messages = append(mcb.Messages, &Message{Role: RoleModel, Payload: &ToolCall{ID: "id1", FuncCall: &FuncCall{Name: "fn", Arguments: `{"a":1}`}}})
	mcb.Messages = append(mcb.Messages, &Message{Role: RoleTool, Payload: &ToolResult{ID: "id1", Result: `{"ok":true}`}})
	if withTool {
		mcb.AddTool(MustNewFuncTool[struct {
			A int `json:"a"`
		}]("fn", "desc"))
	}
	return mcb.Build()
}

func TestOpenAIConversionHelpers(t *testing.T) {
	g := &OpenAIGenerator{Model: "gpt-test"}

	long := strings.Repeat("x", oaiMaxTextContentLength+10)
	prompts := g.convPrompt(&Prompt{Name: "n", Text: long})
	if len(prompts) != 2 || prompts[0].OfDeveloper == nil {
		t.Fatalf("unexpected prompt conversion: %#v", prompts)
	}

	g.PromptRole = PromptRoleSystem
	prompts = g.convPrompt(&Prompt{Name: "n", Text: "sys"})
	if len(prompts) != 1 || prompts[0].OfSystem == nil {
		t.Fatalf("expected system role prompt, got: %#v", prompts)
	}

	if _, err := g.convModelMessage(&Message{Role: RoleModel, Payload: Contents{&Blob{MIMEType: "x", Data: []byte{1}}}}); err == nil {
		t.Fatal("expected model message blob error")
	}
	if _, err := g.convModelMessage(&Message{Role: RoleModel, Payload: Contents{}}); err == nil {
		t.Fatal("expected empty model message error")
	}
	if _, err := g.convModelMessage(&Message{Role: RoleModel, Name: "assistant", Payload: Contents{Text("ok")}}); err != nil {
		t.Fatalf("convModelMessage text failed: %v", err)
	}

	g.TextOnly = true
	if _, err := g.convUserMessage(&Message{Role: RoleUser, Payload: Contents{&Blob{MIMEType: "audio/mp3", Data: []byte{1}}}}); err == nil {
		t.Fatal("expected text-only model error for audio")
	}
	if _, err := g.convUserMessage(&Message{Role: RoleUser, Payload: Contents{Text("ok")}}); err != nil {
		t.Fatalf("convUserMessage text failed: %v", err)
	}

	g.TextOnly = false
	if _, err := g.convUserMessage(&Message{Role: RoleUser, Payload: Contents{&Blob{MIMEType: "application/octet-stream", Data: []byte{1}}}}); err == nil {
		t.Fatal("expected unsupported mime error")
	}
	if _, err := g.convUserMessage(&Message{Role: RoleUser, Name: "user", Payload: Contents{Text("hi"), &Blob{MIMEType: "audio/mp3", Data: []byte{1}}}}); err != nil {
		t.Fatalf("convUserMessage mixed content failed: %v", err)
	}

	if _, err := g.convMessage(&Message{Role: RoleTool, Payload: Contents{Text("x")}}); err == nil {
		t.Fatal("expected content role mismatch error")
	}
	if _, err := g.convMessage(&Message{Payload: payloadStub{}}); err == nil {
		t.Fatal("expected unexpected payload type error")
	}
	if _, err := g.convMessage(&Message{Role: RoleModel, Payload: &ToolCall{ID: "id", FuncCall: &FuncCall{Name: "fn", Arguments: "{}"}}}); err != nil {
		t.Fatalf("convMessage tool call failed: %v", err)
	}
	if _, err := g.convMessage(&Message{Role: RoleTool, Payload: &ToolResult{ID: "id", Result: "ok"}}); err != nil {
		t.Fatalf("convMessage tool result failed: %v", err)
	}

	if _, err := g.convModelContext(ModelContexts((&ModelContextBuilder{}).Build(), (&ModelContextBuilder{Messages: []*Message{{Payload: payloadStub{}}}}).Build())); err == nil {
		t.Fatal("expected convModelContext to fail on unsupported payload")
	}

	g2 := &OpenAIGenerator{Model: "gpt-test", SupportToolCalls: true, ExtraFields: map[string]any{"foo": "bar"}}
	params, err := g2.chatCompletion(testOpenAIContext(true), &ModelParams{
		MaxTokens:        10,
		FrequencyPenalty: 1,
		N:                1,
		Temperature:      0.7,
		TopP:             0.9,
		PresencePenalty:  0.3,
	})
	if err != nil {
		t.Fatalf("chatCompletion failed: %v", err)
	}
	if params.Model != "gpt-test" || len(params.Messages) == 0 {
		t.Fatalf("unexpected chatCompletion params: %#v", params)
	}
	mcb := &ModelContextBuilder{Params: &ModelParams{ExtraFields: map[string]any{"reasoning_effort": "high"}}}
	params, err = g2.chatCompletion(mcb.Build(), &ModelParams{ExtraFields: map[string]any{"foo": "base"}})
	if err != nil {
		t.Fatalf("chatCompletion with context params failed: %v", err)
	}
	extra := params.ExtraFields()
	if extra["foo"] != "base" || extra["reasoning_effort"] != "high" {
		t.Fatalf("extra fields = %#v", extra)
	}

	if _, err := (&OpenAIGenerator{SupportToolCalls: true}).chatCompletion(ModelContexts((&ModelContextBuilder{Tools: []Tool{&SearchWebTool{}}}).Build()), nil); err == nil {
		t.Fatal("expected unsupported tool type error")
	}

	if _, _, err := (&OpenAIGenerator{}).Invoke(context.Background(), "", (&ModelContextBuilder{}).Build(), MustNewFuncTool[struct{}]("fn", "desc")); err == nil {
		t.Fatal("expected invoke mode error without json/tool-calls support")
	}
}

func TestOpenAIForwardsDeclaredToolsAndGroupsParallelCalls(t *testing.T) {
	var mcb ModelContextBuilder
	mcb.UserText("", "which node is busy?")
	mcb.ModelText("", "checking")
	for _, id := range []string{"call_a", "call_b"} {
		mcb.AddMessage(&Message{Role: RoleModel, Payload: &ToolCall{ID: id, FuncCall: &FuncCall{Name: "lookup", Arguments: "{}"}}})
	}
	mcb.AddMessage(&Message{Role: RoleTool, Payload: &ToolResult{ID: "call_a", Result: "3"}})
	mcb.AddMessage(&Message{Role: RoleTool, Payload: &ToolResult{ID: "call_b", Result: "9"}})
	mcb.AddTool(&FuncTool{
		Name: "lookup", Description: "Read one node.", Strict: true,
		Parameters: json.RawMessage(`{"type":"object","properties":{"node":{"type":"string"}}}`),
	})

	params, err := (&OpenAIGenerator{Model: "gpt-test", SupportToolCalls: true}).chatCompletion(mcb.Build(), nil)
	if err != nil {
		t.Fatalf("chatCompletion() error = %v", err)
	}
	if len(params.Messages) != 4 {
		t.Fatalf("messages = %d, want user, one assistant turn, two tool results", len(params.Messages))
	}
	assistant := params.Messages[1].OfAssistant
	if assistant == nil || assistant.Content.OfString.Value != "checking" || len(assistant.ToolCalls) != 2 || assistant.ToolCalls[1].ID != "call_b" {
		t.Fatalf("assistant = %#v", assistant)
	}
	function := params.Tools[0].Function
	// A declared schema is forwarded as-is: optional fields stay optional.
	if _, ok := function.Parameters["required"]; ok || !function.Strict.Value {
		t.Fatalf("function = %#v", function)
	}

	var broken ModelContextBuilder
	broken.AddMessage(&Message{Role: RoleModel, Payload: &ToolCall{ID: "call"}})
	if _, err := (&OpenAIGenerator{}).chatCompletion(broken.Build(), nil); err == nil {
		t.Fatal("expected error for a tool call without a function")
	}
}

func TestOAIPullerStates(t *testing.T) {
	ctx := (&ModelContextBuilder{}).Build()

	t.Run("stop", func(t *testing.T) {
		sb := NewStreamBuilder(ctx, 8)
		stream := newChunkStream(
			`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":""}]}`,
			`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
		)
		if err := (&oaiPuller{}).pull(sb, stream); err != nil {
			t.Fatalf("pull stop failed: %v", err)
		}
		out := sb.Stream()
		chunk, err := out.Next()
		if err != nil || chunk == nil || chunk.Part.(Text) != "hello" {
			t.Fatalf("unexpected first chunk: %#v err=%v", chunk, err)
		}
		if _, err := out.Next(); !errors.Is(err, ErrDone) {
			t.Fatalf("expected ErrDone, got: %v", err)
		}
	})

	t.Run("tool_calls", func(t *testing.T) {
		sb := NewStreamBuilder(ctx, 8)
		stream := newChunkStream(
			`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"sum","arguments":"{\"a\":"}}]},"finish_reason":""}]}`,
			`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"next","arguments":"{\"b\":2}"}}]},"finish_reason":"tool_calls"}]}`,
		)
		if err := (&oaiPuller{}).pull(sb, stream); err != nil {
			t.Fatalf("pull tool calls failed: %v", err)
		}
		out := sb.Stream()
		first, err := out.Next()
		if err != nil || first == nil || first.ToolCall == nil {
			t.Fatalf("expected first tool call chunk, got %#v err=%v", first, err)
		}
		second, err := out.Next()
		if err != nil || second == nil || second.ToolCall == nil {
			t.Fatalf("expected second tool call chunk, got %#v err=%v", second, err)
		}
		if _, err := out.Next(); !errors.Is(err, ErrDone) {
			t.Fatalf("expected ErrDone after tool calls, got: %v", err)
		}
	})

	t.Run("truncated_and_blocked", func(t *testing.T) {
		sb := NewStreamBuilder(ctx, 4)
		lengthStream := newChunkStream(`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`)
		if err := (&oaiPuller{}).pull(sb, lengthStream); err != nil {
			t.Fatalf("pull length failed: %v", err)
		}
		if _, err := sb.Stream().Next(); err == nil || !strings.Contains(err.Error(), "truncated") {
			t.Fatalf("expected truncated error, got: %v", err)
		}

		sb2 := NewStreamBuilder(ctx, 4)
		blockedStream := newChunkStream(`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"refusal":"policy"},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
		if err := (&oaiPuller{}).pull(sb2, blockedStream); err != nil {
			t.Fatalf("pull blocked failed: %v", err)
		}
		if _, err := sb2.Stream().Next(); err == nil || !strings.Contains(err.Error(), "blocked") {
			t.Fatalf("expected blocked error, got: %v", err)
		}
	})

	t.Run("blocked_without_refusal_detail", func(t *testing.T) {
		sb := NewStreamBuilder(ctx, 4)
		stream := newChunkStream(`{"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
		if err := (&oaiPuller{}).pull(sb, stream); err != nil {
			t.Fatal(err)
		}
		_, err := sb.Stream().Next()
		var state *State
		if !errors.As(err, &state) || state.Status() != StatusBlocked || !strings.Contains(err.Error(), "finish_reason=content_filter") || !strings.Contains(err.Error(), "not supplied") {
			t.Fatalf("missing provider filter diagnostic: %v", err)
		}
	})

	t.Run("decoder_error", func(t *testing.T) {
		sb := NewStreamBuilder(ctx, 2)
		st := ssestream.NewStream[openai.ChatCompletionChunk](&fakeDecoder{events: nil, err: errors.New("decode")}, nil)
		if err := (&oaiPuller{}).pull(sb, st); err == nil || !strings.Contains(err.Error(), "decode") {
			t.Fatalf("expected decoder error, got: %v", err)
		}
	})

	t.Run("commit_tool_nil", func(t *testing.T) {
		if err := (&oaiPuller{}).commitTool(NewStreamBuilder(ctx, 1)); err != nil {
			t.Fatalf("commitTool nil should be no-op, got: %v", err)
		}
	})
}

func TestOpenAIInvokeAndGenerateStreamWithHTTP(t *testing.T) {
	fn := MustNewFuncTool[struct {
		A int `json:"a"`
	}]("fn", "desc")

	mkResp := func(choice any) string {
		payload := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "gpt-test",
			"choices": []any{choice},
			"usage": map[string]any{
				"prompt_tokens":     1,
				"completion_tokens": 1,
				"total_tokens":      2,
			},
		}
		b, _ := json.Marshal(payload)
		return string(b)
	}

	t.Run("invoke_json_output", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, mkResp(map[string]any{
				"index":         0,
				"finish_reason": "stop",
				"logprobs":      map[string]any{"content": []any{}, "refusal": []any{}},
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"ok":true}`,
				},
			}))
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL+"/"))
		g := &OpenAIGenerator{Client: &client, Model: "gpt-test", SupportJSONOutput: true}

		_, call, err := g.Invoke(context.Background(), "", testOpenAIContext(false), fn)
		if err != nil {
			t.Fatalf("invoke json output failed: %v", err)
		}
		if call == nil || !strings.Contains(call.Arguments, "ok") {
			t.Fatalf("unexpected func call: %#v", call)
		}
	})

	t.Run("invoke_tool_calls", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, mkResp(map[string]any{
				"index":         0,
				"finish_reason": "tool_calls",
				"logprobs":      map[string]any{"content": []any{}, "refusal": []any{}},
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "fn",
							"arguments": `{"a":1}`,
						},
					}},
				},
			}))
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL+"/"))
		g := &OpenAIGenerator{Client: &client, Model: "gpt-test", SupportToolCalls: true, InvokeWithToolName: true}

		_, call, err := g.Invoke(context.Background(), "", testOpenAIContext(true), fn)
		if err != nil {
			t.Fatalf("invoke tool call failed: %v", err)
		}
		if call == nil || !strings.Contains(call.Arguments, "\"a\":1") {
			t.Fatalf("unexpected tool call result: %#v", call)
		}
	})

	t.Run("invoke_json_output_falls_back_to_tool_calls_when_response_format_unsupported", func(t *testing.T) {
		var requests int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			w.Header().Set("Content-Type", "application/json")
			if requests == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"message":"This response_format type is unavailable now","type":"invalid_request_error","param":null,"code":"invalid_request_error"}`)
				return
			}
			_, _ = io.WriteString(w, mkResp(map[string]any{
				"index":         0,
				"finish_reason": "tool_calls",
				"logprobs":      map[string]any{"content": []any{}, "refusal": []any{}},
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "fn",
							"arguments": `{"a":1}`,
						},
					}},
				},
			}))
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL+"/"))
		g := &OpenAIGenerator{Client: &client, Model: "gpt-test", SupportJSONOutput: true, SupportToolCalls: true, InvokeWithToolName: true}

		_, call, err := g.Invoke(context.Background(), "", testOpenAIContext(true), fn)
		if err != nil {
			t.Fatalf("invoke fallback failed: %v", err)
		}
		if requests != 2 {
			t.Fatalf("requests = %d, want 2", requests)
		}
		if call == nil || !strings.Contains(call.Arguments, "\"a\":1") {
			t.Fatalf("unexpected fallback result: %#v", call)
		}
	})

	t.Run("invoke_tool_calls_with_stop_finish_reason", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, mkResp(map[string]any{
				"index":         0,
				"finish_reason": "stop",
				"logprobs":      map[string]any{"content": []any{}, "refusal": []any{}},
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "fn",
							"arguments": `{"a":1}`,
						},
					}},
				},
			}))
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL+"/"))
		g := &OpenAIGenerator{Client: &client, Model: "gpt-test", SupportToolCalls: true, InvokeWithToolName: true}

		_, call, err := g.Invoke(context.Background(), "", testOpenAIContext(true), fn)
		if err != nil {
			t.Fatalf("invoke tool call with stop finish reason failed: %v", err)
		}
		if call == nil || !strings.Contains(call.Arguments, "\"a\":1") {
			t.Fatalf("unexpected tool call result: %#v", call)
		}
	})

	t.Run("generate_stream", func(t *testing.T) {
		sse := strings.Join([]string{
			`data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":""}]}`,
			"",
			`data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, sse)
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL+"/"))
		g := &OpenAIGenerator{Client: &client, Model: "gpt-test"}

		stream, err := g.GenerateStream(context.Background(), "", testOpenAIContext(false))
		if err != nil {
			t.Fatalf("GenerateStream failed: %v", err)
		}

		first, err := stream.Next()
		if err != nil {
			t.Fatalf("stream first next failed: %v", err)
		}
		if first == nil || first.Part.(Text) != "hi" {
			t.Fatalf("unexpected streamed chunk: %#v", first)
		}
		if _, err := stream.Next(); !errors.Is(err, ErrDone) {
			t.Fatalf("expected ErrDone from generated stream, got: %v", err)
		}
	})
}

func TestOpenAIGeneratorReportsProviderUsage(t *testing.T) {
	var requests []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, strings.Join([]string{
				`data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":""}]}`,
				`data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[],"usage":{"prompt_tokens":30,"completion_tokens":5,"total_tokens":35,"prompt_tokens_details":{"cached_tokens":10}}}`,
				"data: [DONE]",
				"",
			}, "\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"2","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"a\":1}"}}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`)
	}))
	defer srv.Close()

	var records []UsageRecord
	ctx := WithUsageRecorder(context.Background(), func(record UsageRecord) { records = append(records, record) })
	client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL+"/"))
	g := &OpenAIGenerator{Client: &client, Provider: "deepseek", Model: "chat-model", SupportJSONOutput: true}

	stream, err := g.GenerateStream(ctx, "", testOpenAIContext(false))
	if err != nil {
		t.Fatalf("GenerateStream failed: %v", err)
	}
	if chunk, err := stream.Next(); err != nil || chunk.Part.(Text) != "hi" {
		t.Fatalf("first chunk = %#v, %v", chunk, err)
	}
	_, err = stream.Next()
	var state *State
	if !errors.As(err, &state) || state.Status() != StatusDone {
		t.Fatalf("terminal error = %v, want done state", err)
	}
	if want := (Usage{PromptTokenCount: 30, CachedContentTokenCount: 10, GeneratedTokenCount: 5}); state.Usage() != want {
		t.Fatalf("terminal usage = %+v, want %+v", state.Usage(), want)
	}
	options, _ := requests[0]["stream_options"].(map[string]any)
	if options["include_usage"] != true {
		t.Fatalf("stream_options = %v, want include_usage", requests[0]["stream_options"])
	}

	fn := MustNewFuncTool[struct {
		A int `json:"a"`
	}]("fn", "desc")
	usage, _, err := g.Invoke(ctx, "", testOpenAIContext(false), fn)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if want := (Usage{PromptTokenCount: 7, GeneratedTokenCount: 3}); usage != want {
		t.Fatalf("Invoke usage = %+v, want %+v", usage, want)
	}

	want := []UsageRecord{
		{Provider: "deepseek", Model: "chat-model", Modality: UsageModalityText, Unit: UsageUnitToken, Input: 20, CachedInput: 10, Output: 5},
		{Provider: "deepseek", Model: "chat-model", Modality: UsageModalityText, Unit: UsageUnitToken, Input: 7, Output: 3},
	}
	if !slices.Equal(records, want) {
		t.Fatalf("records = %+v, want %+v", records, want)
	}
}

func TestOpenAIGenerateStreamRetriesWithoutRejectedStreamOptions(t *testing.T) {
	var requests []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		if _, ok := body["stream_options"]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"Unrecognized request argument supplied: stream_options","type":"invalid_request_error","param":null,"code":null}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":""}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			"data: [DONE]",
			"",
		}, "\n\n"))
	}))
	defer srv.Close()

	var records []UsageRecord
	ctx := WithUsageRecorder(context.Background(), func(record UsageRecord) { records = append(records, record) })
	client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL+"/"), option.WithMaxRetries(0))
	g := &OpenAIGenerator{Client: &client, Provider: "legacy", Model: "chat-model"}

	stream, err := g.GenerateStream(ctx, "", testOpenAIContext(false))
	if err != nil {
		t.Fatalf("GenerateStream failed: %v", err)
	}
	if chunk, err := stream.Next(); err != nil || chunk.Part.(Text) != "hi" {
		t.Fatalf("first chunk = %#v, %v", chunk, err)
	}
	if _, err := stream.Next(); !errors.Is(err, ErrDone) {
		t.Fatalf("terminal error = %v, want ErrDone", err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want the rejected request and one retry", len(requests))
	}
	if _, ok := requests[1]["stream_options"]; ok {
		t.Fatalf("retry sent stream_options: %v", requests[1]["stream_options"])
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v, want none when the endpoint reports no usage", records)
	}
}
