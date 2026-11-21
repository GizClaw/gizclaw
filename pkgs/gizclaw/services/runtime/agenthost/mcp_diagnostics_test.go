package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/mcpclient"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPDiagnosticsSummarizeOnlyPreDispatchRejections(t *testing.T) {
	const private = "private-marker"
	const configuredTool = "private-marker-tool"
	var calls atomic.Int32
	var authorizeCalls atomic.Int32
	var authorizeLimit atomic.Int32
	remote := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	for _, name := range []string{"search_catalog", configuredTool} {
		remote.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{
			"type": "object", "required": []string{"q"}, "additionalProperties": false,
			"properties": map[string]any{"q": map[string]any{"type": "string"}},
		}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: private + "-output"}}}, nil
		})
	}
	server := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()
	registry := mcpclient.NewRegistry()
	defer registry.Close()
	mcpInvoker, err := OpenMCP(t.Context(), MCPConfig{
		Registry: registry, ProfileID: "fixture",
		Servers:   map[string]apitypes.RuntimeProfileMCPServer{"litelink": {Endpoint: server.URL}},
		Selection: map[string]apitypes.RuntimeProfileMCPSelection{"litelink": {Tools: &[]string{"search_catalog", configuredTool}}},
		Authorize: func(context.Context) error {
			if count, limit := authorizeCalls.Add(1), authorizeLimit.Load(); limit > 0 && count >= limit {
				return errors.New(private + "-authorization-error")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mcpInvoker.Close()
	invoker := &ToolkitInvoker{MCP: mcpInvoker}
	catalog, err := invoker.ResolveCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	functions := make(map[string]string)
	for _, tool := range catalog {
		functions[tool.Alias] = tool.FunctionName
	}
	search := functions["litelink.call_tool.search_catalog"]
	unknown := functions["litelink.call_tool."+configuredTool]
	if search == "" || unknown == "" {
		t.Fatal("fixture tools were not exposed")
	}

	logs := &agenthostLogStore{}
	logger, closeLogger, err := gizlog.NewLogger(gizlog.Config{
		Sinks: []gizlog.SinkConfig{{Kind: gizlog.SinkStore, Store: "logs"}},
	}, agenthostLogResolver{store: logs})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLogger() })
	oldLogger := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(oldLogger) })
	peerCtx := gizlog.WithSessionID(gizlog.WithPeerPublicKey(t.Context(), "fixture-peer"), "fixture-session")
	turnCtx, cancelTurn := context.WithCancel(peerCtx)
	defer cancelTurn()
	ctx, finish := invoker.BeginToolTurn(turnCtx, "fixture-turn")
	assertCall := func(ctx context.Context, name, args, want string) {
		t.Helper()
		body, err := invoker.InvokeTool(ctx, name, json.RawMessage(args))
		if err != nil || !strings.Contains(string(body), want) {
			t.Fatalf("tool result = %s, error = %v, want %q", body, err, want)
		}
	}
	for range 2 {
		assertCall(ctx, search, `{"q":123,"extra":"private-marker-args"}`, `"code":"invalid_arguments"`)
	}
	assertCall(ctx, unknown, `{"q":123}`, `"code":"invalid_arguments"`)
	assertCall(ctx, mcpFunctionPrefix+private, `{}`, `"code":"unavailable"`)
	if calls.Load() != 0 {
		t.Fatal("rejected calls dispatched")
	}

	assertCall(ctx, search, `{"q":"private-marker-args"}`, private+"-output")
	if calls.Load() != 1 {
		t.Fatal("valid call did not dispatch exactly once")
	}
	// Reject at the final authorization check after catalog resolution.
	authorizeLimit.Store(authorizeCalls.Load() + 2)
	assertCall(ctx, search, `{"q":"synthetic"}`, "MCP authorization changed")
	// The next call fails at the initial catalog/authorization check.
	assertCall(ctx, search, `{"q":"synthetic"}`, "MCP authorization is unavailable")
	if calls.Load() != 1 {
		t.Fatal("authorization rejection dispatched")
	}
	cancelTurn()
	finish()
	// A late sibling result and a second finish must not change the summary.
	recordMCPReject(ctx, mcpOperation{}, mcpSchemaInvalid)
	finish()
	authorizeLimit.Store(0)
	next, finishNext := invoker.BeginToolTurn(peerCtx, "fixture-next-turn")
	assertCall(next, search, `{"q":"synthetic"}`, private+"-output")
	finishNext()
	if calls.Load() != 2 {
		t.Fatal("successful turn did not dispatch exactly once")
	}
	if err := closeLogger(); err != nil {
		t.Fatal(err)
	}
	wantFields := map[string]string{
		"peer_public_key": "fixture-peer",
		"session_id":      "fixture-session",
		"stream_id":       "fixture-turn",
		"rejections.schema_invalid.litelink.call_tool.search_catalog":              "2",
		"rejections.schema_invalid.unknown":                                        "1",
		"rejections.catalog_or_auth_unavailable.litelink.call_tool.search_catalog": "2",
		"rejections.catalog_or_auth_unavailable.unknown":                           "1",
	}
	var summaries int
	for _, record := range logs.records {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{private, search, unknown, server.URL} {
			if strings.Contains(string(encoded), value) {
				t.Fatalf("log exposed unapproved data: %s", encoded)
			}
		}
		if record.Message == "agenthost: MCP tool rejections" {
			summaries++
			fields := maps.Clone(record.Attributes)
			for _, key := range []string{"path", "source", "source_file", "source_line"} {
				delete(fields, key)
			}
			if !maps.Equal(fields, wantFields) {
				t.Fatalf("summary fields = %v, want %v", fields, wantFields)
			}
		}
	}
	if summaries != 1 {
		t.Fatalf("got %d diagnostics summaries, want 1", summaries)
	}
}

func TestMCPDiagnosticsCountConcurrentRejectionsWithinTurn(t *testing.T) {
	invoker := &ToolkitInvoker{MCP: &MCPInvoker{}}
	ctx, _ := invoker.BeginToolTurn(t.Context(), "fixture-turn")
	diagnostics := ctx.Value(mcpDiagnosticsKey{}).(*mcpDiagnostics)
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() { recordMCPReject(ctx, mcpOperation{}, mcpSchemaInvalid) })
	}
	group.Wait()
	if count := diagnostics.counts[mcpSchemaInvalid][0]; count != 32 {
		t.Fatalf("concurrent rejections = %d, want 32", count)
	}
}
