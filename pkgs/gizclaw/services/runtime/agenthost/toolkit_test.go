package agenthost

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolkitInvokerHTTPDispatch(t *testing.T) {
	server := toolkittest.New(t)
	created := putAgentHostTool(t, server, agentHostHTTPTool("get_weather"))
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://weather.example/v1?city=Hangzhou" {
			t.Fatalf("HTTP URL = %s", request.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":{"temp":25}}`)),
		}, nil
	})
	invoker := &ToolkitInvoker{
		Builder: &toolkit.Builder{Tools: server},
		HTTP:    giztools.HTTPExecutor{Transport: transport},
	}
	ctx := toolTestContext(t, map[string]string{"weather": created.ID}, nil)
	result, err := invoker.InvokeTool(ctx, "get_weather", json.RawMessage(`{"city":"Hangzhou"}`))
	if err != nil || string(result) != `{"temp":25}` {
		t.Fatalf("InvokeTool() = %s, %v", result, err)
	}
}

func TestWithToolExecutionRejectsDuplicateCanonicalBindings(t *testing.T) {
	bindings := map[string]apitypes.RuntimeProfileBinding{
		"one": {ResourceId: "volume_set"},
		"two": {ResourceId: "volume_set"},
	}
	if _, err := WithToolExecution(t.Context(), &bindings); err == nil {
		t.Fatal("WithToolExecution() accepted duplicate canonical bindings")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func toolTestContext(t *testing.T, resources map[string]string, client AppClient) context.Context {
	t.Helper()
	bindings := make(map[string]apitypes.RuntimeProfileBinding, len(resources))
	for alias, name := range resources {
		bindings[alias] = apitypes.RuntimeProfileBinding{ResourceId: name}
	}
	ctx := WithResourceAccess(t.Context(), "workspace-owner", nil, nil)
	ctx, err := WithToolExecution(ctx, &bindings)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func putAgentHostTool(t *testing.T, server *toolkit.Server, tool toolkit.Tool) toolkit.Tool {
	t.Helper()
	created, err := server.CreateTool(t.Context(), tool)
	if err != nil {
		t.Fatalf("PutTool(%q) error = %v", tool.InvokeName, err)
	}
	return created
}

func agentHostHTTPTool(name string) toolkit.Tool {
	pointer := "/data"
	return toolkit.Tool{
		ID: name, InvokeName: name, Type: toolkit.ToolTypeHTTPRequest, Enabled: true,
		InputSchema: jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"city": {Type: "string"},
			},
			Required: []string{"city"},
		},
		HTTP: toolkit.HTTPRequest{
			URL: "https://weather.example/v1", Method: http.MethodGet,
			Auth: toolkit.HTTPAuth{Method: "none"},
			Query: []toolkit.HTTPArgumentBinding{{
				ArgumentPointer: "/city", Target: "city", Required: true,
			}},
			ResponsePointer: &pointer, SuccessStatusCodes: []int{http.StatusOK},
			Timeout: time.Second, MaxResponseBytes: 1024,
		},
	}
}

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
