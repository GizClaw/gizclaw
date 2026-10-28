package genx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestOpenAIProtectedStreamTerminalStates(t *testing.T) {
	for _, row := range []struct {
		name, finish string
		protected    bool
		refusal      bool
		want         Status
	}{
		{"legacy refusal", "content_filter", false, true, StatusBlocked},
		{"protected refusal", "", true, true, StatusBlocked},
		{"protected filter", "content_filter", true, true, StatusBlocked},
		{"protected truncated", "length", true, false, StatusTruncated},
		{"protected done", "stop", true, false, StatusDone},
	} {
		t.Run(row.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				delta := map[string]string{}
				if row.refusal {
					delta["refusal"] = "legacy policy detail"
					if row.protected {
						delta["refusal"] = r.Header.Get("X-Proxy-Fixture")
					}
				}
				chunk, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": row.finish}}})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n", chunk)
			}))
			defer provider.Close()
			client := openai.NewClient(option.WithAPIKey("local-fixture"), option.WithBaseURL(provider.URL+"/"), option.WithMaxRetries(0))
			g := &OpenAIGenerator{Client: &client, Model: "local-fixture"}
			if row.protected {
				g.RequestHeaders = func(context.Context) (http.Header, error) {
					return http.Header{"X-Proxy-Fixture": {"private-refusal-fixture"}}, nil
				}
			}
			stream, err := g.GenerateStream(t.Context(), "", testOpenAIContext(false))
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			_, err = stream.Next()
			state, ok := errors.AsType[*State](err)
			if !ok || state.Status() != row.want || state.Usage() != (Usage{PromptTokenCount: 3, GeneratedTokenCount: 2}) {
				t.Fatalf("terminal status or usage changed: %v", err)
			}
			_, _, _, public := PublicErrorDetails(err)
			if public != (row.protected && row.want != StatusDone) || strings.Contains(err.Error(), "private-refusal-fixture") {
				t.Fatalf("unsafe terminal error: %v", err)
			}
			if row.want == StatusDone && !errors.Is(err, ErrDone) {
				t.Fatal("normal completion lost ErrDone")
			}
			if !row.protected && !strings.Contains(err.Error(), "legacy policy detail") {
				t.Fatal("legacy refusal diagnostic changed")
			}
		})
	}
}
