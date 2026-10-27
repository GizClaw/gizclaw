package eino

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestStructuredModelSendsNativeSchemaAudioHistoryAndOptions(t *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"mode\":\"research\"}"}}]}`))
	}))
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL+"/"), option.WithAPIKey("fixture"), option.WithMaxRetries(0))
	bridge := genXChatModel{generator: &genx.OpenAIGenerator{
		Client: &client, Model: "fixture-native", SupportJSONOutput: true,
		ExtraFields: map[string]any{"thinking": map[string]any{"type": "disabled"}},
	}, pattern: "fixture"}
	var definition jsonschema.Schema
	if err := json.Unmarshal([]byte(`{"type":"object","additionalProperties":true,"required":["mode"],"properties":{"mode":{"type":"string","enum":["plot","research"]},"detail":{"type":"string"}}}`), &definition); err != nil {
		t.Fatal(err)
	}
	audio := base64.StdEncoding.EncodeToString(pcm.L16Mono16K.WAV([]byte{1, 0, 2, 0}))
	message, err := bridge.GenerateStructured(t.Context(), []*schema.Message{
		schema.SystemMessage("Classify the current turn."), schema.UserMessage("earlier question"),
		schema.AssistantMessage("earlier reply", nil),
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{{
			Type:  schema.ChatMessagePartTypeAudioURL,
			Audio: &schema.MessageInputAudio{MessagePartCommon: schema.MessagePartCommon{Base64Data: &audio, MIMEType: "audio/wav"}},
		}}},
	}, genxeino.StructuredOutput{Name: "turn_intent", Schema: &definition}, model.WithTemperature(0), model.WithMaxTokens(64))
	if err != nil || message == nil || message.Content != `{"mode":"research"}` {
		t.Fatalf("structured reply: %#v, %v", message, err)
	}
	request := <-requests
	format := request["response_format"].(map[string]any)
	contract := format["json_schema"].(map[string]any)
	if format["type"] != "json_schema" || contract["name"] != "turn_intent" || contract["strict"] != true {
		t.Fatalf("native schema was not sent: %#v", format)
	}
	encoded, err := json.Marshal(&definition)
	if err != nil {
		t.Fatal(err)
	}
	var declared any
	if err := json.Unmarshal(encoded, &declared); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract["schema"], declared) {
		t.Fatal("optional fields, nullability or additional properties changed before native Invoke")
	}
	if request["stream"] == true || request["tools"] != nil || request["temperature"] != float64(0) || request["max_completion_tokens"] != float64(64) {
		t.Fatalf("wrong structured options: %#v", request)
	}
	if request["thinking"].(map[string]any)["type"] != "disabled" {
		t.Fatal("thinking was enabled")
	}
	messages := request["messages"].([]any)
	if len(messages) != 4 || structuredWireText(messages[1]) != "earlier question" || structuredWireText(messages[2]) != "earlier reply" {
		t.Fatal("original History changed")
	}
	last := messages[3].(map[string]any)
	part := last["content"].([]any)[0].(map[string]any)
	if last["role"] != "user" || part["type"] != "input_audio" || part["input_audio"].(map[string]any)["data"] != audio {
		t.Fatal("original audio was replaced with text or moved out of the final user message")
	}
}

func structuredWireText(message any) string {
	switch content := message.(map[string]any)["content"].(type) {
	case string:
		return content
	case []any:
		var text string
		for _, part := range content {
			if item := part.(map[string]any); item["type"] == "text" {
				text += item["text"].(string)
			}
		}
		return text
	default:
		return ""
	}
}

func TestStructuredModelPropagatesInvokeFailureWithoutTextFallback(t *testing.T) {
	want := errors.New("native structured output unavailable")
	bridge := genXChatModel{generator: structuredUnavailableGenerator{err: want}}
	if _, err := bridge.GenerateStructured(t.Context(), []*schema.Message{schema.UserMessage("classify")},
		genxeino.StructuredOutput{Name: "intent", Schema: &jsonschema.Schema{Type: "object"}}); !errors.Is(err, want) {
		t.Fatalf("native Invoke failure was hidden: %v", err)
	}
}

type structuredUnavailableGenerator struct{ err error }

func (g structuredUnavailableGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, g.err
}

func (structuredUnavailableGenerator) GenerateStream(context.Context, string, genx.ModelContext) (genx.Stream, error) {
	panic("structured output must not fall back to ordinary text generation")
}
