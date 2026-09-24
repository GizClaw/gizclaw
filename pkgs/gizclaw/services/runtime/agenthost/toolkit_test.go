package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolkitInvokerUsesCanonicalCurrentPeerScope(t *testing.T) {
	server := toolkittest.New(t)
	volume := putAgentHostTool(t, server, agentHostBoundHTTPTool("volume_set"))
	brightness := putAgentHostTool(t, server, agentHostBoundHTTPTool("brightness_set"))
	client := &recordingHTTPTools{result: json.RawMessage(`{"ok":true}`)}
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, HTTP: giztools.HTTPExecutor{Transport: client}}
	ctx := toolTestContext(t, map[string]string{
		"volume":     volume.ID,
		"brightness": brightness.ID,
	})

	definitions, err := invoker.ResolveTools(ctx)
	if err != nil {
		t.Fatalf("ResolveTools() error = %v", err)
	}
	if len(definitions) != 2 || definitions[0].Name != "brightness_set" || definitions[1].Name != "volume_set" {
		t.Fatalf("ResolveTools() = %#v", definitions)
	}
	result, err := invoker.InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":7}`))
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("InvokeTool() = %s, %v", result, err)
	}
	if client.name != "volume_set" || string(client.args) != `{"level":7}` || client.calls != 1 {
		t.Fatalf("client invocation = name=%q args=%s calls=%d", client.name, client.args, client.calls)
	}
	aliasResult, err := invoker.InvokeTool(ctx, "volume", json.RawMessage(`{"level":7}`))
	if err != nil || string(aliasResult) != `{"error":{"code":"unavailable","message":"tool is unavailable"}}` || client.calls != 1 {
		t.Fatalf("InvokeTool(alias) = %s, %v, calls=%d", aliasResult, err, client.calls)
	}
}

func TestToolkitInvokerReauthorizesResourceAtInvoke(t *testing.T) {
	server := toolkittest.New(t)
	tool := agentHostBoundHTTPTool("volume_set")
	created := putAgentHostTool(t, server, tool)
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}}
	ctx := toolTestContext(t, map[string]string{"volume": created.ID})
	if _, err := invoker.ResolveTools(ctx); err != nil {
		t.Fatal(err)
	}
	tool.Enabled = false
	if _, err := server.PutTool(t.Context(), created.ID, tool); err != nil {
		t.Fatalf("PutTool(%q) error = %v", tool.InvokeName, err)
	}
	result, err := invoker.InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":1}`))
	if err != nil || string(result) != `{"error":{"code":"unavailable","message":"tool is unavailable"}}` {
		t.Fatalf("InvokeTool(disabled) = %s, %v", result, err)
	}
}

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
	ctx := toolTestContext(t, map[string]string{"weather": created.ID})
	result, err := invoker.InvokeTool(ctx, "get_weather", json.RawMessage(`{"city":"Hangzhou"}`))
	if err != nil || string(result) != `{"temp":25}` {
		t.Fatalf("InvokeTool() = %s, %v", result, err)
	}
}

func TestToolkitInvokerConcurrentPeerScopesStayIsolated(t *testing.T) {
	server := toolkittest.New(t)
	firstTool := putAgentHostTool(t, server, agentHostBoundHTTPTool("peer_a"))
	secondTool := putAgentHostTool(t, server, agentHostBoundHTTPTool("peer_b"))
	recorder := &recordingHTTPTools{result: json.RawMessage(`{"ok":true}`)}
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, HTTP: giztools.HTTPExecutor{Transport: recorder}}
	contexts := []context.Context{toolTestContext(t, map[string]string{"a": firstTool.ID}), toolTestContext(t, map[string]string{"b": secondTool.ID})}
	names := []string{"peer_a", "peer_b"}
	var wg sync.WaitGroup
	for i := range contexts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for range 25 {
				result, err := invoker.InvokeTool(contexts[i], names[i], json.RawMessage(`{"level":3}`))
				if err != nil || string(result) != `{"ok":true}` {
					t.Errorf("invoke: %s %v", result, err)
				}
				result, err = invoker.InvokeTool(contexts[i], names[1-i], json.RawMessage(`{"level":3}`))
				if err != nil || string(result) != `{"error":{"code":"unavailable","message":"tool is unavailable"}}` {
					t.Errorf("cross-scope: %s %v", result, err)
				}
			}
		}(i)
	}
	wg.Wait()
	if recorder.calls != 50 {
		t.Fatalf("HTTP calls: %d", recorder.calls)
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

type recordingHTTPTools struct {
	mu     sync.Mutex
	name   string
	args   json.RawMessage
	result json.RawMessage
	err    error
	wait   bool
	calls  int
}

func (c *recordingHTTPTools) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	name := strings.TrimPrefix(request.URL.Path, "/")
	args := []byte(`{"level":` + request.URL.Query().Get("level") + `}`)
	if c.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.name = name
	c.args = append(c.args[:0], args...)
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(c.result)))}, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func toolTestContext(t *testing.T, resources map[string]string) context.Context {
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

func agentHostBoundHTTPTool(name string) toolkit.Tool {
	return toolkit.Tool{
		ID: name, InvokeName: name, Type: toolkit.ToolTypeHTTPRequest, Enabled: true,
		HTTP: &toolkit.HTTPRequest{URL: "https://tools.example/" + name, Method: "GET", Auth: toolkit.HTTPAuth{Method: "none"}, Query: []toolkit.HTTPArgumentBinding{{ArgumentPointer: "/level", Target: "level", Required: true}}, Timeout: time.Second, MaxResponseBytes: 1024},
		InputSchema: jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"level": {Type: "integer"},
			},
			Required: []string{"level"},
		},
	}
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
		HTTP: &toolkit.HTTPRequest{
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

func TestToolkitInvokerRejectsInvalidArgumentsBeforeHTTP(t *testing.T) {
	server := toolkittest.New(t)
	created := putAgentHostTool(t, server, agentHostBoundHTTPTool("volume_set"))
	client := &recordingHTTPTools{}
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, HTTP: giztools.HTTPExecutor{Transport: client}}
	ctx := toolTestContext(t, map[string]string{"volume": created.ID})
	if _, err := invoker.InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":"loud"}`)); !errors.Is(err, toolkit.ErrInvalidTool) {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
}
