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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolkitInvokerUsesCanonicalCurrentPeerScope(t *testing.T) {
	server := &toolkit.Server{Store: kv.NewMemory(nil)}
	volume := putAgentHostTool(t, server, agentHostClientTool("volume_set"))
	brightness := putAgentHostTool(t, server, agentHostClientTool("brightness_set"))
	client := &recordingClientTools{result: json.RawMessage(`{"ok":true}`)}
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}}
	ctx := toolTestContext(t, map[string]string{
		"volume":     volume.ID,
		"brightness": brightness.ID,
	}, client)

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
	server := &toolkit.Server{Store: kv.NewMemory(nil)}
	tool := agentHostClientTool("volume_set")
	created := putAgentHostTool(t, server, tool)
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}}
	ctx := toolTestContext(t, map[string]string{"volume": created.ID}, &recordingClientTools{})
	if _, err := invoker.ResolveTools(ctx); err != nil {
		t.Fatal(err)
	}
	tool.Enabled = false
	if _, err := server.PutTool(t.Context(), created.ID, tool); err != nil {
		t.Fatalf("PutTool(%q) error = %v", tool.Name, err)
	}
	result, err := invoker.InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":1}`))
	if err != nil || string(result) != `{"error":{"code":"unavailable","message":"tool is unavailable"}}` {
		t.Fatalf("InvokeTool(disabled) = %s, %v", result, err)
	}
}

func TestToolkitInvokerClientRecoverableErrors(t *testing.T) {
	server := &toolkit.Server{Store: kv.NewMemory(nil)}
	created := putAgentHostTool(t, server, agentHostClientTool("volume_set"))
	invoker := &ToolkitInvoker{
		Builder:       &toolkit.Builder{Tools: server},
		ClientTimeout: time.Millisecond,
	}
	for _, test := range []struct {
		name   string
		client ClientToolInvoker
		want   string
	}{
		{
			name:   "unavailable",
			client: &recordingClientTools{err: giztools.ErrClientToolUnavailable},
			want:   `{"error":{"code":"unavailable","message":"client tool is unavailable"}}`,
		},
		{
			name:   "timeout with no parent deadline",
			client: &recordingClientTools{wait: true},
			want:   `{"error":{"code":"timeout","message":"tool execution timed out"}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := toolTestContext(t, map[string]string{"volume": created.ID}, test.client)
			result, err := invoker.InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":1}`))
			if err != nil || string(result) != test.want {
				t.Fatalf("InvokeTool() = %s, %v, want %s", result, err, test.want)
			}
		})
	}
}

func TestToolkitInvokerClientTimeoutIsEnforcedWithinLongerParentDeadline(t *testing.T) {
	server := &toolkit.Server{Store: kv.NewMemory(nil)}
	created := putAgentHostTool(t, server, agentHostClientTool("volume_set"))
	invoker := &ToolkitInvoker{
		Builder:       &toolkit.Builder{Tools: server},
		ClientTimeout: time.Millisecond,
	}
	ctx := toolTestContext(t, map[string]string{"volume": created.ID}, &recordingClientTools{wait: true})
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	result, err := invoker.InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":1}`))
	const want = `{"error":{"code":"timeout","message":"tool execution timed out"}}`
	if err != nil || string(result) != want {
		t.Fatalf("InvokeTool() = %s, %v, want %s", result, err, want)
	}
}

func TestToolkitInvokerHTTPDispatch(t *testing.T) {
	server := &toolkit.Server{Store: kv.NewMemory(nil)}
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

func TestToolkitInvokerConcurrentPeerScopesStayIsolated(t *testing.T) {
	server := &toolkit.Server{Store: kv.NewMemory(nil)}
	created := putAgentHostTool(t, server, agentHostClientTool("volume_set"))
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}}
	first := &recordingClientTools{result: json.RawMessage(`{"peer":"a"}`)}
	second := &recordingClientTools{result: json.RawMessage(`{"peer":"b"}`)}
	contexts := []context.Context{
		toolTestContext(t, map[string]string{"volume-a": created.ID}, first),
		toolTestContext(t, map[string]string{"volume-b": created.ID}, second),
	}
	wants := []string{`{"peer":"a"}`, `{"peer":"b"}`}
	var wg sync.WaitGroup
	for index := range contexts {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for range 25 {
				result, err := invoker.InvokeTool(contexts[index], "volume_set", json.RawMessage(`{"level":3}`))
				if err != nil || string(result) != wants[index] {
					t.Errorf("peer %d InvokeTool() = %s, %v", index, result, err)
				}
			}
		}(index)
	}
	wg.Wait()
	if first.calls != 25 || second.calls != 25 {
		t.Fatalf("client calls = %d, %d", first.calls, second.calls)
	}
}

func TestWithToolExecutionRejectsDuplicateCanonicalBindings(t *testing.T) {
	bindings := map[string]apitypes.RuntimeProfileBinding{
		"one": {ResourceId: "volume_set"},
		"two": {ResourceId: "volume_set"},
	}
	if _, err := WithToolExecution(t.Context(), &bindings, nil); err == nil {
		t.Fatal("WithToolExecution() accepted duplicate canonical bindings")
	}
}

type recordingClientTools struct {
	mu     sync.Mutex
	name   string
	args   json.RawMessage
	result json.RawMessage
	err    error
	wait   bool
	calls  int
}

func (c *recordingClientTools) InvokeClientTool(ctx context.Context, name string, args []byte) ([]byte, error) {
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
	return append([]byte(nil), c.result...), nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func toolTestContext(t *testing.T, resources map[string]string, client ClientToolInvoker) context.Context {
	t.Helper()
	bindings := make(map[string]apitypes.RuntimeProfileBinding, len(resources))
	for alias, name := range resources {
		bindings[alias] = apitypes.RuntimeProfileBinding{ResourceId: name}
	}
	ctx := WithResourceAccess(t.Context(), "workspace-owner", nil, nil)
	ctx, err := WithToolExecution(ctx, &bindings, client)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func putAgentHostTool(t *testing.T, server *toolkit.Server, tool toolkit.Tool) toolkit.Tool {
	t.Helper()
	created, err := server.CreateTool(t.Context(), tool)
	if err != nil {
		t.Fatalf("PutTool(%q) error = %v", tool.Name, err)
	}
	return created
}

func agentHostClientTool(name string) toolkit.Tool {
	return toolkit.Tool{
		Name: name, Type: toolkit.ToolTypeClientRPC, Enabled: true,
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
		Name: name, Type: toolkit.ToolTypeHTTPRequest, Enabled: true,
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

func TestToolkitInvokerRejectsInvalidArgumentsBeforeClientRPC(t *testing.T) {
	server := &toolkit.Server{Store: kv.NewMemory(nil)}
	created := putAgentHostTool(t, server, agentHostClientTool("volume_set"))
	client := &recordingClientTools{}
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}}
	ctx := toolTestContext(t, map[string]string{"volume": created.ID}, client)
	if _, err := invoker.InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":"loud"}`)); !errors.Is(err, toolkit.ErrInvalidTool) {
		t.Fatalf("InvokeTool() error = %v", err)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
}
