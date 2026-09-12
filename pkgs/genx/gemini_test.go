package genx

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/genai"
)

func TestGeminiConvSchemaTypeAndEnum(t *testing.T) {
	s := &jsonschema.Schema{Type: "string", Enum: []any{"a", 1}}
	got := geminiConvSchema(s)
	if got == nil {
		t.Fatal("expected schema")
	}
	if got.Type != genai.TypeString {
		t.Fatalf("unexpected type: %v", got.Type)
	}
	if len(got.Enum) != 2 || got.Enum[0] != "a" || got.Enum[1] != "1" {
		t.Fatalf("unexpected enum conversion: %#v", got.Enum)
	}
}

func TestGeminiConvMessageToolCallFallbackArgs(t *testing.T) {
	msg := &Message{
		Role: RoleModel,
		Payload: &ToolCall{
			ID: "fn",
			FuncCall: &FuncCall{
				Name:      "fn",
				Arguments: "not-json",
			},
		},
	}

	content, err := geminiConvMessage(nil, msg)
	if err != nil {
		t.Fatalf("geminiConvMessage failed: %v", err)
	}
	if content == nil || content.Role != "model" || len(content.Parts) != 1 {
		t.Fatalf("unexpected content: %#v", content)
	}
	fc := content.Parts[0].FunctionCall
	if fc == nil || fc.Args["text"] != "not-json" {
		t.Fatalf("expected fallback text args, got: %#v", fc)
	}
}

func TestGeminiPullHandlesFinishChunkWithoutContent(t *testing.T) {
	sb := NewStreamBuilder((&ModelContextBuilder{}).Build(), 8)

	seq := func(yield func(*genai.GenerateContentResponse, error) bool) {
		if !yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Index:   0,
				Content: &genai.Content{Parts: []*genai.Part{genai.NewPartFromText("hello")}},
			}},
		}, nil) {
			return
		}

		yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Index:        0,
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     1,
				CandidatesTokenCount: 1,
			},
		}, nil)
	}

	if err := geminiPull(sb, seq); err != nil {
		t.Fatalf("geminiPull failed: %v", err)
	}

	stream := sb.Stream()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("read first chunk failed: %v", err)
	}
	if chunk == nil || chunk.Part == nil {
		t.Fatalf("unexpected first chunk: %#v", chunk)
	}
	if text, ok := chunk.Part.(Text); !ok || string(text) != "hello" {
		t.Fatalf("unexpected first chunk text: %#v", chunk.Part)
	}

	if _, err := stream.Next(); !errors.Is(err, ErrDone) {
		t.Fatalf("expected ErrDone, got: %v", err)
	}
}

func TestGeminiConvMessageBranches(t *testing.T) {
	msg := &Message{Role: RoleUser, Payload: Contents{Text("hello"), &Blob{MIMEType: "audio/wav", Data: []byte{1, 2}}}}
	content, err := geminiConvMessage(nil, msg)
	if err != nil {
		t.Fatalf("geminiConvMessage user contents failed: %v", err)
	}
	if content == nil || content.Role != "user" || len(content.Parts) != 2 {
		t.Fatalf("unexpected content: %#v", content)
	}

	merged, err := geminiConvMessage(content, &Message{Role: RoleUser, Payload: Contents{Text("again")}})
	if err != nil {
		t.Fatalf("geminiConvMessage merge failed: %v", err)
	}
	if merged != nil || len(content.Parts) != 3 {
		t.Fatalf("expected parts to merge into last content, got merged=%#v parts=%d", merged, len(content.Parts))
	}

	if _, err := geminiConvMessage(nil, &Message{Role: RoleTool, Payload: Contents{Text("x")}}); err == nil {
		t.Fatal("expected role/type mismatch error")
	}

	if _, err := geminiConvMessage(nil, &Message{Payload: payloadStub{}}); err == nil {
		t.Fatal("expected unexpected payload type error")
	}

	if _, err := geminiConvMessage(nil, &Message{Role: RoleTool, Payload: &ToolResult{ID: "fn", Result: "not-json"}}); err != nil {
		t.Fatalf("tool result fallback conversion failed: %v", err)
	}
}

func TestGeminiConvModelContextAndSchema(t *testing.T) {
	tool := MustNewFuncTool[struct {
		N int `json:"n"`
	}]("sum", "sum")

	var mcb ModelContextBuilder
	mcb.PromptText("sys", "prompt")
	mcb.UserText("u", "hi")
	mcb.AddTool(tool)
	mcb.Params = &ModelParams{MaxTokens: 10, Temperature: 0.2, TopP: 0.8, TopK: 40}

	g := &GeminiGenerator{InvokeParams: &ModelParams{MaxTokens: 5}}
	cfg, contents, err := g.convModelContext(mcb.Build())
	if err != nil {
		t.Fatalf("convModelContext failed: %v", err)
	}
	if cfg == nil || contents == nil || len(contents) == 0 {
		t.Fatalf("unexpected convModelContext output: cfg=%#v contents=%#v", cfg, contents)
	}
	if cfg.MaxOutputTokens != 10 || cfg.Temperature == nil || *cfg.Temperature != 0.2 {
		t.Fatalf("unexpected params in config: %#v", cfg)
	}
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected one tool in config, got %d", len(cfg.Tools))
	}

	if _, _, err := g.convModelContext((&ModelContextBuilder{}).Build()); err == nil || !strings.Contains(err.Error(), "no contents") {
		t.Fatalf("expected no contents error, got: %v", err)
	}

	var badTools ModelContextBuilder
	badTools.UserText("u", "x")
	badTools.AddTool(&SearchWebTool{})
	if _, _, err := g.convModelContext(badTools.Build()); err == nil || !strings.Contains(err.Error(), "unexpected tool type") {
		t.Fatalf("expected unexpected tool type error, got: %v", err)
	}

	var declared ModelContextBuilder
	declared.UserText("u", "x")
	declared.AddTool(&FuncTool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"node":{"type":"string"}}}`)})
	cfg, _, err = g.convModelContext(declared.Build())
	if err != nil {
		t.Fatalf("convModelContext(declared parameters) error = %v", err)
	}
	if parameters := cfg.Tools[0].FunctionDeclarations[0].Parameters; parameters == nil || parameters.Properties["node"] == nil {
		t.Fatalf("declared parameters = %#v", parameters)
	}
	declared.Tools = []Tool{&FuncTool{Name: "lookup", Strict: true}}
	if _, _, err := g.convModelContext(declared.Build()); err == nil || !strings.Contains(err.Error(), "strict tool") {
		t.Fatalf("expected strict tool error, got: %v", err)
	}

	var badMsg ModelContextBuilder
	badMsg.Messages = append(badMsg.Messages, &Message{Payload: payloadStub{}})
	if _, _, err := g.convModelContext(badMsg.Build()); err == nil || !strings.Contains(err.Error(), "unexpected message type") {
		t.Fatalf("expected message conversion error, got: %v", err)
	}

	schema := geminiConvSchema(&jsonschema.Schema{
		Type:        "object",
		Description: "desc",
		Required:    []string{"name"},
		Properties: map[string]*jsonschema.Schema{
			"name": {Type: "string"},
			"age":  {Type: "integer", Enum: []any{1, "2"}},
		},
	})
	if schema == nil || schema.Type != genai.TypeObject || len(schema.Properties) != 2 {
		t.Fatalf("unexpected converted gemini schema: %#v", schema)
	}
	if geminiConvSchema(nil) != nil {
		t.Fatal("expected nil schema conversion for nil input")
	}
}

func TestGeminiPullBranches(t *testing.T) {
	ctx := (&ModelContextBuilder{}).Build()

	t.Run("stop", func(t *testing.T) {
		sb := NewStreamBuilder(ctx, 4)
		seq := func(yield func(*genai.GenerateContentResponse, error) bool) {
			if !yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Index: 0, Content: &genai.Content{Parts: []*genai.Part{genai.NewPartFromText("hello")}}}}}, nil) {
				return
			}
			yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Index: 0, FinishReason: genai.FinishReasonStop}}, UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 1, CandidatesTokenCount: 2}}, nil)
		}

		if err := geminiPull(sb, seq); err != nil {
			t.Fatalf("geminiPull stop failed: %v", err)
		}
		if _, err := sb.Stream().Next(); err != nil {
			t.Fatalf("expected first chunk before done: %v", err)
		}
	})

	t.Run("truncated_blocked_unexpected", func(t *testing.T) {
		sb := NewStreamBuilder(ctx, 2)
		maxSeq := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Index: 0, FinishReason: genai.FinishReasonMaxTokens}}}, nil)
		}
		if err := geminiPull(sb, maxSeq); err != nil {
			t.Fatalf("geminiPull max tokens failed: %v", err)
		}
		if _, err := sb.Stream().Next(); err == nil || !strings.Contains(err.Error(), "truncated") {
			t.Fatalf("expected truncated error, got: %v", err)
		}

		sb2 := NewStreamBuilder(ctx, 2)
		safeSeq := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Index: 0, FinishReason: genai.FinishReasonSafety, SafetyRatings: []*genai.SafetyRating{{Category: genai.HarmCategoryHarassment, Blocked: true}}}}}, nil)
		}
		if err := geminiPull(sb2, safeSeq); err != nil {
			t.Fatalf("geminiPull safety failed: %v", err)
		}
		if _, err := sb2.Stream().Next(); err == nil || !strings.Contains(err.Error(), "blocked") {
			t.Fatalf("expected blocked error, got: %v", err)
		}

		sb3 := NewStreamBuilder(ctx, 2)
		unexpectedSeq := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Index: 0, Content: &genai.Content{Parts: []*genai.Part{{}}}}}}, nil)
		}
		if err := geminiPull(sb3, unexpectedSeq); err == nil || !strings.Contains(err.Error(), "unexpected part type") {
			t.Fatalf("expected unexpected part error, got: %v", err)
		}
	})

	t.Run("iteration_error_and_no_finish", func(t *testing.T) {
		seqErr := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(nil, errors.New("stream err"))
		}
		if err := geminiPull(NewStreamBuilder(ctx, 2), seqErr); err == nil || !strings.Contains(err.Error(), "stream err") {
			t.Fatalf("expected iterator error, got: %v", err)
		}

		noFinish := func(yield func(*genai.GenerateContentResponse, error) bool) {
			yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Index: 0}}}, nil)
		}
		if err := geminiPull(NewStreamBuilder(ctx, 2), noFinish); err == nil || !strings.Contains(err.Error(), "no finish reason") {
			t.Fatalf("expected no finish reason error, got: %v", err)
		}
	})
}
