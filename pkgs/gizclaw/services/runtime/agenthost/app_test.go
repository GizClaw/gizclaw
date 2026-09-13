package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/google/jsonschema-go/jsonschema"
)

type testApps struct {
	apps   []apitypes.App
	result json.RawMessage
	err    error
	wait   bool
	calls  int
	mode   apitypes.AppMethodMode
}

func (c *testApps) ResolveApps(context.Context, []string) ([]apitypes.App, error) { return c.apps, nil }
func (c *testApps) InvokeApp(ctx context.Context, _ apitypes.App, method apitypes.AppMethod, _ json.RawMessage) (json.RawMessage, error) {
	c.calls++
	c.mode = method.Mode
	if c.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return c.result, c.err
}

func TestAppProjectionAndDispatch(t *testing.T) {
	server := toolkittest.New(t)
	invoker := &ToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, ClientTimeout: time.Millisecond}
	client := &testApps{apps: []apitypes.App{{AppName: "clock", Methods: []apitypes.AppMethod{{Name: "read", Mode: "call", InputSchema: jsonschema.Schema{Type: "object"}}, {Name: "show", Mode: "job", InputSchema: jsonschema.Schema{Type: "object"}}}}}, result: json.RawMessage(`{"ok":true}`)}
	ctx := WithAppExecution(toolTestContext(t, nil, nil), &map[string]apitypes.RuntimeProfileBinding{"clock": {ResourceId: "app"}}, client)
	definitions, err := invoker.ResolveTools(ctx)
	if err != nil || len(definitions) != 2 || definitions[0].Name != "clock__read" {
		t.Fatalf("definitions: %v %v", definitions, err)
	}
	result, err := invoker.InvokeTool(ctx, "clock__read", nil)
	if err != nil || string(result) != `{"ok":true}` || client.mode != "call" {
		t.Fatalf("call: %s %v", result, err)
	}
	client.result = json.RawMessage(`{"job_id":7}`)
	result, err = invoker.InvokeTool(ctx, "clock__show", nil)
	if err != nil || string(result) != `{"job_id":7}` || client.mode != "job" {
		t.Fatalf("job: %s %v", result, err)
	}
	before := client.calls
	if _, err := invoker.InvokeTool(ctx, "clock__read", json.RawMessage(`[]`)); err == nil || client.calls != before {
		t.Fatal("invalid arguments dispatched")
	}
	for _, result := range []string{"", `garbage`, `"` + strings.Repeat("x", 4095) + `"`} {
		client.result = json.RawMessage(result)
		if _, err := invoker.InvokeTool(ctx, "clock__read", nil); err == nil {
			t.Fatal("invalid result accepted")
		}
	}
	client.err = ErrAppUnavailable
	result, err = invoker.InvokeTool(ctx, "clock__read", nil)
	if err != nil || !strings.Contains(string(result), `"code":"unavailable"`) {
		t.Fatalf("unavailable: %s %v", result, err)
	}
	client.err = nil
	client.wait = true
	result, err = invoker.InvokeTool(ctx, "clock__read", nil)
	if err != nil || !strings.Contains(string(result), `"code":"timeout"`) {
		t.Fatalf("timeout: %s %v", result, err)
	}
	putAgentHostTool(t, server, agentHostHTTPTool("clock__read"))
	ctx = WithAppExecution(toolTestContext(t, map[string]string{"collision": "clock__read"}, nil), &map[string]apitypes.RuntimeProfileBinding{"clock": {ResourceId: "app"}}, client)
	if _, err := invoker.ResolveTools(ctx); err == nil {
		t.Fatal("collision accepted")
	}
	if _, err := invoker.InvokeTool(ctx, "clock__read", nil); err == nil {
		t.Fatal("collision dispatched")
	}
}

func TestAppJSONSizeBoundary(t *testing.T) {
	invoker := &ToolkitInvoker{}
	for _, mode := range []apitypes.AppMethodMode{"call", "job"} {
		t.Run(string(mode), func(t *testing.T) {
			method := apitypes.AppMethod{Name: "read", Mode: mode, InputSchema: jsonschema.Schema{Type: "object"}}
			for _, size := range []int{4096, 4097} {
				client := &testApps{result: json.RawMessage(`{}`)}
				args := json.RawMessage(`{"x":"` + strings.Repeat("x", size-8) + `"}`)
				_, err := invoker.invokeApp(t.Context(), client, apitypes.App{}, method, args)
				if size == 4096 && (err != nil || client.calls != 1) {
					t.Fatalf("4096 bytes: calls=%d err=%v", client.calls, err)
				}
				if size == 4097 && (!errors.Is(err, toolkit.ErrInvalidTool) || client.calls != 0) {
					t.Fatalf("4097 bytes: calls=%d err=%v", client.calls, err)
				}
				client.result = json.RawMessage(`"` + strings.Repeat("x", size-2) + `"`)
				_, err = invoker.invokeApp(t.Context(), client, apitypes.App{}, method, nil)
				if (err != nil) != (size > 4096) {
					t.Fatalf("result size %d: %v", size, err)
				}
			}
		})
	}
}
