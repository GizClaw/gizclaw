package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/goccy/go-yaml"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type catalogPromptResolver struct{ generator genx.Generator }

func (r catalogPromptResolver) ResolveChatModel(context.Context, string) (model.BaseChatModel, error) {
	return genXChatModel{generator: r.generator, pattern: "model/llm"}, nil
}

func (r catalogPromptResolver) ResolveRetriever(context.Context, string) (retriever.Retriever, error) {
	return nil, errors.New("unexpected retriever")
}

type catalogPromptMemoryStore struct{ einoMemoryStore }

func (*catalogPromptMemoryStore) Wait(context.Context, memory.OperationRequest) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

type catalogPromptStateStore struct {
	mu      sync.Mutex
	value   genxeino.StateSnapshot
	version int
}

func (s *catalogPromptStateStore) Load(context.Context, string) (genxeino.StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, nil
}

func (s *catalogPromptStateStore) CompareAndSwap(_ context.Context, _ string, version string, fields map[string]any) (genxeino.StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value.Version != version {
		return genxeino.StateSnapshot{}, errors.New("stale state")
	}
	s.version++
	s.value = genxeino.StateSnapshot{Version: strconv.Itoa(s.version), Fields: fields}
	return s.value, nil
}

// Verify native prompt history through the Eino/GenX adapter and model HTTP.
func TestNativePromptPreservesTwoTurnModelHistory(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	type wireMessage struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	type request struct {
		Model               string        `json:"model"`
		Stream              bool          `json:"stream"`
		Messages            []wireMessage `json:"messages"`
		MaxCompletionTokens int           `json:"max_completion_tokens"`
	}
	captured := make(chan request, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" || req.Model != "fixture" || !req.Stream {
			t.Errorf("unexpected model request: %s %s model=%s stream=%v", r.Method, r.URL.Path, req.Model, req.Stream)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		select {
		case captured <- req:
		default:
			t.Error("unexpected additional model request")
			http.Error(w, "too many requests", http.StatusBadRequest)
			return
		}
		response := "fixture reply"
		delta, err := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": response}, "finish_reason": nil}}})
		if err != nil {
			t.Error(err)
			http.Error(w, "encoding fixture", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", delta, `{"id":"fixture","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)

	}))
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("local-fixture"))
	generator := &genx.OpenAIGenerator{Client: &client, Model: "fixture", TextOnly: true, PromptRole: genx.PromptRoleSystem}
	for _, test := range []struct {
		name, file, system string
		inputs             []string
	}{
		{"voice", "05-eino-basic.yaml", "你是一个友好的中文语音助手", []string{"第一轮：请回答数字一。", "第二轮：请回答数字二。"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			data, err := os.ReadFile(filepath.Join(root, "tests/gizclaw-e2e/testdata/resources/04-workflows", test.file))
			if err != nil {
				t.Fatal(err)
			}
			data, err = yaml.YAMLToJSON(data)
			if err != nil {
				t.Fatal(err)
			}
			var resource struct {
				Spec struct {
					Eino apitypes.EinoWorkflowSpec `json:"eino"`
				} `json:"spec"`
			}
			if err := json.Unmarshal(data, &resource); err != nil {
				t.Fatal(err)
			}
			graph, err := einoconfig.MapGraph(resource.Spec.Eino.Graph)
			if err != nil {
				t.Fatal(err)
			}
			config := genxeino.Config{Agent: genxeino.AgentConfig{ID: test.name, ContextID: test.name}, Graph: graph, Components: catalogPromptResolver{generator: generator}}
			config.Memory = &genxeino.MemoryConfig{Store: &catalogPromptMemoryStore{}, Scope: memory.Scope{AppID: "fixture", UserID: "fixture", AgentID: "fixture"}}
			field := "user-text"
			initial := any("")
			config.State = &genxeino.StatePersistenceConfig{Scope: test.name, Fields: []string{field}, Store: &catalogPromptStateStore{value: genxeino.StateSnapshot{Version: "0", Fields: map[string]any{field: initial}}}}
			transformer, err := genxeino.New(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				for len(captured) > 0 {
					<-captured
				}
			})
			t.Cleanup(func() { _ = transformer.Close() })
			for i, text := range test.inputs {
				builder := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
				if err := builder.Add(genx.NewBeginOfStream(fmt.Sprintf("probe-%s-%d", test.name, i)), &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text)}, genx.NewTextEndOfStream()); err != nil {
					t.Fatal(err)
				}
				if err := builder.Done(genx.Usage{}); err != nil {
					t.Fatal(err)
				}
				stream, err := transformer.Transform(ctx, builder.Stream())
				if err != nil {
					t.Fatal(err)
				}
				var delivered strings.Builder
				for {
					chunk, nextErr := stream.Next()
					err = nextErr
					if chunk != nil {
						if text, ok := chunk.Part.(genx.Text); ok {
							delivered.WriteString(string(text))
						}
						if failure := genx.StreamError(chunk.Ctrl); failure != nil {
							t.Fatal(failure)
						}
					}

					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				_ = stream.Close()
				if delivered.String() != "fixture reply" {
					t.Fatalf("unexpected model output: %q", delivered.String())
				}
				select {
				case req := <-captured:
					if len(req.Messages) < 2 || req.Messages[0].Role != "system" {
						t.Fatalf("missing system on wire: %+v", req.Messages)
					}
					if !strings.Contains(string(req.Messages[0].Content), test.system) {
						t.Fatalf("wrong system on wire: %s", req.Messages[0].Content)
					}
					wantTokens := 2048
					if req.MaxCompletionTokens != wantTokens {
						t.Fatalf("max completion tokens = %d, want %d", req.MaxCompletionTokens, wantTokens)
					}
					wantRoles := []string{"system", "user"}
					for range i {
						wantRoles = append(wantRoles, "assistant", "user")
					}
					if len(req.Messages) != len(wantRoles) {
						t.Fatalf("roles in round %d: got %d messages, want %v", i+1, len(req.Messages), wantRoles)
					}
					for index, role := range wantRoles {
						if req.Messages[index].Role != role {
							t.Fatalf("message %d role = %s, want %s", index, req.Messages[index].Role, role)
						}
					}
					last := req.Messages[len(req.Messages)-1]
					flattened := catalogPromptText(t, last.Content)
					if last.Role != "user" || !strings.Contains(flattened, text) {
						t.Fatalf("current user missing or wrong role: %+v", last)
					}
					if i > 0 {
						if !strings.Contains(catalogPromptText(t, req.Messages[1].Content), test.inputs[0]) || catalogPromptText(t, req.Messages[2].Content) != "fixture reply" {
							t.Fatal("earlier user/assistant turn was not preserved before current user")
						}
					}
					t.Logf("round %d: system preserved; current user last; messages=%d max_tokens=%d", i+1, len(req.Messages), req.MaxCompletionTokens)
				default:
					t.Fatal("no captured HTTP model request")
				}
			}
		})
	}
}

func catalogPromptText(t *testing.T, content json.RawMessage) string {
	t.Helper()
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &parts); err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	for _, part := range parts {
		if part.Type != "text" {
			t.Fatalf("unexpected content type %q", part.Type)
		}
		joined.WriteString(part.Text)
	}
	return joined.String()
}
