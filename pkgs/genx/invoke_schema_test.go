package genx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"google.golang.org/genai"
)

const optionalInvokeSchema = `{"type":"object","properties":{"mode":{"type":"string","enum":["research"]},"detail":{"type":"string"}},"required":["mode"],"additionalProperties":true}`

func TestOpenAIInvokePreservesDeclaredSchemaAcrossNativePaths(t *testing.T) {
	for _, path := range []string{"json", "function", "json-to-function"} {
		t.Run(path, func(t *testing.T) {
			requests := make(chan map[string]any, 2)
			attempt := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- body
				attempt++
				w.Header().Set("Content-Type", "application/json")
				if path == "json-to-function" && attempt == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"message":"This response_format type is unavailable now","type":"invalid_request_error"}`)
					return
				}
				if path == "json" {
					_, _ = io.WriteString(w, `{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"mode\":\"research\"}"}}]}`)
				} else {
					_, _ = io.WriteString(w, `{"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"intent","arguments":"{\"mode\":\"research\"}"}}]}}]}`)
				}
			}))
			defer server.Close()
			client := openai.NewClient(option.WithBaseURL(server.URL+"/"), option.WithAPIKey("fixture"), option.WithMaxRetries(0))
			generator := &OpenAIGenerator{Client: &client, Model: "fixture", SupportJSONOutput: path != "function", SupportToolCalls: path != "json", InvokeWithToolName: true}
			tool := &FuncTool{Name: "intent", Argument: &jsonschema.Schema{Type: "object"}, Parameters: json.RawMessage(optionalInvokeSchema), Strict: true}
			_, call, err := generator.Invoke(t.Context(), "", testOpenAIContext(false), tool)
			if err != nil || call == nil || call.Arguments != `{"mode":"research"}` {
				t.Fatalf("Invoke: call=%#v error=%v", call, err)
			}
			wantCount := 1
			if path == "json-to-function" {
				wantCount = 2
			}
			if len(requests) != wantCount {
				t.Fatalf("requests=%d want=%d", len(requests), wantCount)
			}
			for range wantCount {
				request := <-requests
				var actual any
				if format, ok := request["response_format"].(map[string]any); ok {
					actual = format["json_schema"].(map[string]any)["schema"]
				} else {
					actual = request["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"]
				}
				assertInvokeSchema(t, actual)
			}
		})
	}
}

func TestGeminiInvokePreservesDeclaredSchema(t *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"mode\":\"research\"}"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()
	client, err := genai.NewClient(t.Context(), &genai.ClientConfig{APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPOptions: genai.HTTPOptions{BaseURL: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	generator := &GeminiGenerator{Client: client, Model: "fixture"}
	tool := &FuncTool{Name: "intent", Argument: &jsonschema.Schema{Type: "object"}, Parameters: json.RawMessage(optionalInvokeSchema), Strict: true}
	_, call, err := generator.Invoke(t.Context(), "", testOpenAIContext(false), tool)
	if err != nil || call == nil || call.Arguments != `{"mode":"research"}` {
		t.Fatalf("Invoke: call=%#v error=%v", call, err)
	}
	configuration := (<-requests)["generationConfig"].(map[string]any)
	if _, rewritten := configuration["responseSchema"]; rewritten {
		t.Fatal("declared schema was converted through the typed schema formatter")
	}
	assertInvokeSchema(t, configuration["responseJsonSchema"])
}

func assertInvokeSchema(t testing.TB, actual any) {
	t.Helper()
	var want any
	if err := json.Unmarshal([]byte(optionalInvokeSchema), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("declared optional/non-null/additional-property semantics changed: actual=%#v want=%#v", actual, want)
	}
}
