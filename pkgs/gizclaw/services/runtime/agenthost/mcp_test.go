package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/mcpclient"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPInvokerSharedCacheToolsAndRevocation(t *testing.T) {
	var reads, calls atomic.Int32
	var version atomic.Value
	version.Store("old")
	var revoked atomic.Bool
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddResource(&mcp.Resource{URI: "fixture:///knowledge", Name: "knowledge"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		reads.Add(1)
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, Text: version.Load().(string)}}}, nil
	})
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "dynamic:///{name}", Name: "dynamic"}, func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		reads.Add(1)
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, Blob: []byte("binary")}}}, nil
	})
	for _, name := range []string{"echo", "forbidden"} {
		server.AddTool(&mcp.Tool{Name: name, Description: name, InputSchema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []string{"value"}, "additionalProperties": false}}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(request.Params.Arguments)}}, IsError: true}, nil
		})
	}
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	registry := mcpclient.NewRegistry()
	defer registry.Close()
	config := MCPConfig{Registry: registry, ProfileID: "profile", Servers: map[string]apitypes.RuntimeProfileMCPServer{"knowledge": {Endpoint: httpServer.URL, Headers: &map[string]string{"Authorization": "private-secret"}}}, Selection: map[string]apitypes.RuntimeProfileMCPSelection{"knowledge": {Resources: new(true), Tools: &[]string{"echo"}}}, Authorize: func(context.Context) error {
		if revoked.Load() {
			return errors.New("revoked")
		}
		return nil
	}}
	first, err := OpenMCP(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenMCP(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if first.sources["knowledge"] != second.sources["knowledge"] || reads.Load() != 1 {
		t.Fatal("Workflows did not reuse preloaded resources")
	}
	invoker := &ToolkitInvoker{MCP: first}
	tools, err := invoker.ResolveCatalog(t.Context())
	if err != nil || len(tools) != 4 {
		t.Fatalf("catalog = %v, %v", tools, err)
	}
	var readName, callName, templateName string
	for _, tool := range tools {
		encoded, _ := json.Marshal(tool)
		if strings.Contains(string(encoded), httpServer.URL) || strings.Contains(string(encoded), "private-secret") || strings.Contains(string(encoded), "forbidden") {
			t.Fatalf("private or unselected MCP data exposed: %s", encoded)
		}
		switch tool.Target["operation"] {
		case "read_resource":
			readName = tool.FunctionName
		case "call_tool":
			callName = tool.FunctionName
		case "list_resource_templates":
			templateName = tool.FunctionName
		}
	}
	ctx := t.Context()
	before, err := invoker.InvokeTool(ctx, readName, json.RawMessage(`{"uri":"fixture:///knowledge"}`))
	if err != nil || !strings.Contains(string(before), `"text":"old"`) {
		t.Fatalf("initial cached value = %s, %v", before, err)
	}
	version.Store("new")
	if err := first.sources["knowledge"].Cache.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := invoker.InvokeTool(ctx, readName, json.RawMessage(`{"uri":"fixture:///knowledge"}`))
	if err != nil || !strings.Contains(string(after), `"text":"new"`) {
		t.Fatalf("refreshed cached value = %s, %v", after, err)
	}
	shared, err := (&ToolkitInvoker{MCP: second}).InvokeTool(ctx, readName, json.RawMessage(`{"uri":"fixture:///knowledge"}`))
	if err != nil || !strings.Contains(string(shared), `"text":"new"`) {
		t.Fatalf("shared refreshed value = %s, %v", shared, err)
	}
	if reads.Load() != 2 {
		t.Fatalf("local reads reached remote %d times", reads.Load())
	}
	if templates, err := invoker.InvokeTool(ctx, templateName, nil); err != nil || !strings.Contains(string(templates), "dynamic:///{name}") {
		t.Fatalf("templates = %s, %v", templates, err)
	}
	for range 2 {
		body, err := invoker.InvokeTool(ctx, readName, json.RawMessage(`{"uri":"dynamic:///instance"}`))
		if err != nil || !strings.Contains(string(body), `"blob":"YmluYXJ5"`) {
			t.Fatalf("template read = %s, %v", body, err)
		}
	}
	if reads.Load() != 3 {
		t.Fatalf("template instance was not cached: %d", reads.Load())
	}
	if _, err := invoker.InvokeTool(ctx, callName, json.RawMessage(`{"value":1}`)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("invalid arguments reached remote tool")
	}
	for range 2 {
		body, err := invoker.InvokeTool(ctx, callName, json.RawMessage(`{"value":"hello"}`))
		if err != nil || !strings.Contains(string(body), `"isError":true`) {
			t.Fatalf("remote execution error lost: %s, %v", body, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("tool results were cached")
	}
	invoker.Verify = func(context.Context, toolcatalog.Tool, json.RawMessage, genx.ToolConversation) (string, error) {
		return "rejected", nil
	}
	verified := genx.WithToolConversation(ctx, genx.ToolConversation{CurrentUser: "echo"})
	body, err := invoker.InvokeTool(verified, callName, json.RawMessage(`{"value":"hello"}`))
	if err != nil || !strings.Contains(string(body), "intent_rejected") || calls.Load() != 2 {
		t.Fatalf("MCP bypassed semantic verification: %s, %v", body, err)
	}
	revoked.Store(true)
	if _, err := invoker.ResolveCatalog(ctx); err == nil {
		t.Fatal("revoked source remained available")
	}
	body, err = invoker.InvokeTool(ctx, readName, json.RawMessage(`{"uri":"fixture:///knowledge"}`))
	if err != nil || !strings.Contains(string(body), "unavailable") {
		t.Fatalf("cached data bypassed revocation: %s, %v", body, err)
	}
}

func TestMCPInvokerConstructionRollbackAndToolOnlySource(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	registry := mcpclient.NewRegistry()
	defer registry.Close()
	config := MCPConfig{Registry: registry, ProfileID: "profile", Servers: map[string]apitypes.RuntimeProfileMCPServer{"remote": {Endpoint: httpServer.URL}}, Selection: map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Tools: &[]string{"echo"}}}, Authorize: func(context.Context) error { return nil }}
	invoker, err := OpenMCP(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer invoker.Close()
	tools, err := invoker.ResolveCatalog(t.Context())
	if err != nil || len(tools) != 1 || tools[0].Source != "mcp_tool" {
		t.Fatalf("tool-only source = %v, %v", tools, err)
	}
	config.Selection["remote"] = apitypes.RuntimeProfileMCPSelection{Resources: new(true)}
	if _, err := OpenMCP(t.Context(), config); err == nil {
		t.Fatal("resources accepted unsupported server")
	}
	if _, err := invoker.ResolveCatalog(t.Context()); err != nil {
		t.Fatalf("failed constructor closed existing lease: %v", err)
	}
}
