package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestMCPModelResourceReadAndContinuation(t *testing.T) {
	var reads atomic.Int32
	remote := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	remote.AddResource(&mcp.Resource{URI: "fixture:///answer", Name: "answer"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		reads.Add(1)
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, Text: "cached answer"}}}, nil
	})
	mcpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer mcpServer.Close()
	registry := mcpclient.NewRegistry()
	defer registry.Close()
	config := &agenthost.MCPConfig{Registry: registry, ProfileID: "profile", Servers: map[string]apitypes.RuntimeProfileMCPServer{"knowledge": {Endpoint: mcpServer.URL}}, Selection: map[string]apitypes.RuntimeProfileMCPSelection{"knowledge": {Resources: new(true)}}, Authorize: func(context.Context) error { return nil }}
	capabilities, err := agenthost.OpenMCP(t.Context(), *config)
	if err != nil {
		t.Fatal(err)
	}
	defer capabilities.Close()
	catalog, err := capabilities.ResolveCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	readName := ""
	for _, tool := range catalog {
		if tool.Target["operation"] == "read_resource" {
			readName = tool.FunctionName
		}
	}
	var rounds atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		found := false
		for _, tool := range request.Tools {
			if tool.Function.Name == readName {
				found = true
			}
		}
		if !found {
			t.Error("MCP resource reader not exposed to model")
		}
		var delta map[string]any
		if rounds.Add(1) == 1 {
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "resource-call", "type": "function", "function": map[string]any{"name": readName, "arguments": `{"uri":"fixture:///answer"}`}}}}
		} else {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || !strings.Contains(string(last.Content), "cached answer") {
				t.Errorf("resource result missing from model continuation: %+v", last)
			}
			delta = map[string]any{"role": "assistant", "content": "answer from MCP"}
		}
		encoded, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
		finish := "stop"
		if delta["tool_calls"] != nil {
			finish = "tool_calls"
		}
		terminal, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}})
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", encoded, terminal)
	}))
	defer modelServer.Close()
	client := openai.NewClient(option.WithBaseURL(modelServer.URL), option.WithAPIKey("fixture"))
	generator := &genx.OpenAIGenerator{Client: &client, Model: "fixture", TextOnly: true, SupportToolCalls: true, PromptRole: genx.PromptRoleSystem}
	transformer, err := genxeino.New(t.Context(), genxeino.Config{
		Agent:      genxeino.AgentConfig{ID: "mcp-integration", ContextID: "mcp-integration"},
		Graph:      genxeino.GraphDefinition{Name: "mcp", State: genxeino.StateDefinition{Fields: []genxeino.StateField{{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace}}}, Nodes: []genxeino.NodeDefinition{{ID: "model", Inputs: map[string]genxeino.Binding{"messages": {From: "input.messages"}}, Outputs: map[string]string{"text": "answer"}, ChatModel: &genxeino.ChatModelNode{Model: "chat"}}}, Edges: []genxeino.EdgeDefinition{{From: "start", To: "model"}, {From: "model", To: "end"}}, Outputs: []genxeino.OutputDefinition{{Node: "model", Field: "answer", Name: "answer", MIMEType: "text/plain", Primary: true}}},
		Components: catalogPromptResolver{generator: generator}, ToolInvoker: &agenthost.ToolkitInvoker{MCP: capabilities},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	if err := input.Add(genx.NewBeginOfStream("mcp-turn"), &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("read the resource")}, genx.NewTextEndOfStream()); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var text strings.Builder
	for {
		chunk, err := output.Next()
		if chunk != nil {
			if failure := genx.StreamError(chunk.Ctrl); failure != nil {
				t.Fatal(failure)
			}
			if value, ok := chunk.Part.(genx.Text); ok {
				text.WriteString(string(value))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if text.String() != "answer from MCP" || rounds.Load() != 2 || reads.Load() != 1 {
		t.Fatalf("continuation output=%q model_rounds=%d remote_reads=%d", text.String(), rounds.Load(), reads.Load())
	}
	// The product factory must also acquire/release the same shared source,
	// rather than only the generic test config.
	var authorized atomic.Int32
	config.Authorize = func(context.Context) error { authorized.Add(1); return nil }
	spec := einoFactorySpec(t)
	spec.MCP = config
	agent, err := (Factory{GenX: &peergenx.Service{}}).NewAgent(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := agent.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("MCP factory did not return a managed agent")
	}
	if authorized.Load() == 0 || reads.Load() != 1 {
		t.Fatal("factory did not share the prepared MCP source")
	}
}
