package eino

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	einojsonschema "github.com/eino-contrib/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestChatModelSendsDeclaredFunctionParameters(t *testing.T) {
	const declaration = `{"type":"object","required":["q"],"additionalProperties":false,"properties":{"q":{"type":"string"},"cursor":{"type":"string"},"filters":{"type":"object","additionalProperties":true,"properties":{"category":{"type":"string"}}}}}`
	var source einojsonschema.Schema
	if err := json.Unmarshal([]byte(declaration), &source); err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal([]byte(declaration), &want); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []struct {
				Function struct {
					Name       string         `json:"name"`
					Parameters map[string]any `json:"parameters"`
					Strict     bool           `json:"strict"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(body.Tools) != 1 || body.Tools[0].Function.Name != "catalog_search" || body.Tools[0].Function.Strict || !reflect.DeepEqual(body.Tools[0].Function.Parameters, want) {
			t.Errorf("provider function declaration = %#v, want the registered search schema", body.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}

data: [DONE]

`))
	}))
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
	bridge := genXChatModel{generator: &genx.OpenAIGenerator{Client: &client, Model: "fixture", SupportToolCalls: true, TextOnly: true}, pattern: "fixture"}
	_, err := bridge.Generate(t.Context(), []*schema.Message{schema.UserMessage("hello")}, model.WithTools([]*schema.ToolInfo{{
		Name: "catalog_search", ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&source),
	}}))
	if err != nil {
		t.Fatal(err)
	}
}
