package agenthost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolkitVerifierRejectsBeforeTransportAndReauthorizesAfterApproval(t *testing.T) {
	for _, mode := range []string{"no conversation", "empty current user", "rejected", "verification error", "revoked during verification"} {
		t.Run(mode, func(t *testing.T) {
			server := toolkittest.New(t)
			resource := agentHostBoundHTTPTool("private_control")
			resource.HTTP.Method = "POST"
			created := putAgentHostTool(t, server, resource)
			bindings := map[string]apitypes.RuntimeProfileToolBinding{"control": {ResourceId: created.ID}}
			profile := apitypes.RuntimeProfile{Revision: "current", Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Tools: &bindings}}}
			names := []string{"control"}
			calls, checks := 0, 0
			invoker := ToolkitInvoker{
				Catalog: &toolcatalog.Catalog{Tools: server},
				Owner:   func(context.Context) (string, error) { return "owner", nil },
				Scope:   func(context.Context, string) (apitypes.RuntimeProfile, []string, error) { return profile, names, nil },
				HTTP: giztools.HTTPExecutor{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				})},
				Verify: func(_ context.Context, tool toolcatalog.Tool, args json.RawMessage, conversation genx.ToolConversation) (string, error) {
					checks++
					if tool.Alias != "control" || string(args) != `{"level":7}` || conversation.CurrentUser != "requested control" {
						return "", fmt.Errorf("verification received wrong candidate or conversation")
					}
					switch mode {
					case "rejected":
						return "wrong target", nil
					case "verification error":
						return "", fmt.Errorf("checker unavailable")
					case "revoked during verification":
						names = []string{}
					}
					return "", nil
				},
			}
			ctx := t.Context()
			if mode != "no conversation" {
				currentUser := "requested control"
				if mode == "empty current user" {
					currentUser = ""
				}
				ctx = genx.WithToolConversation(ctx, genx.ToolConversation{CurrentUser: currentUser})
			}
			result, err := invoker.InvokeTool(ctx, "control", json.RawMessage(`{"level":7}`))
			wantChecks := 1
			if mode == "no conversation" || mode == "empty current user" {
				wantChecks = 0
			}
			if err != nil || calls != 0 || checks != wantChecks || !strings.Contains(string(result), `"error"`) {
				t.Fatalf("result=%s err=%v transport=%d verification=%d", result, err, calls, checks)
			}
		})
	}
}

func TestToolkitInvokerUsesCanonicalCurrentPeerScope(t *testing.T) {
	server := toolkittest.New(t)
	volume := putAgentHostTool(t, server, agentHostBoundHTTPTool("volume_set"))
	brightness := putAgentHostTool(t, server, agentHostBoundHTTPTool("brightness_set"))
	client := &recordingHTTPTools{result: json.RawMessage(`{"ok":true}`)}
	invoker := &testToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, HTTP: giztools.HTTPExecutor{Transport: client}, Request: toolkit.BuildRequest{AllowedTools: []string{brightness.ID, volume.ID}}}
	ctx := toolTestContext(t, map[string]string{
		"volume":     volume.ID,
		"brightness": brightness.ID,
	})

	definitions, err := prepareAliasTestInvoker(invoker).ResolveTools(ctx)
	if err != nil {
		t.Fatalf("ResolveTools() error = %v", err)
	}
	if len(definitions) != 2 || definitions[0].Name != "brightness" || definitions[1].Name != "volume" {
		t.Fatalf("ResolveTools() = %#v", definitions)
	}
	result, err := prepareAliasTestInvoker(invoker).InvokeTool(ctx, "volume", json.RawMessage(`{"level":7}`))
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("InvokeTool() = %s, %v", result, err)
	}
	if client.name != "volume_set" || string(client.args) != `{"level":7}` || client.calls != 1 {
		t.Fatalf("client invocation = name=%q args=%s calls=%d", client.name, client.args, client.calls)
	}
	aliasResult, err := prepareAliasTestInvoker(invoker).InvokeTool(ctx, "volume_set", json.RawMessage(`{"level":7}`))
	if err != nil || string(aliasResult) != `{"error":{"code":"unavailable","message":"tool is not authorized"}}` || client.calls != 1 {
		t.Fatalf("InvokeTool(alias) = %s, %v, calls=%d", aliasResult, err, client.calls)
	}
}

func TestToolkitInvokerReauthorizesResourceAtInvoke(t *testing.T) {
	server := toolkittest.New(t)
	tool := agentHostBoundHTTPTool("volume_set")
	created := putAgentHostTool(t, server, tool)
	invoker := &testToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, Request: toolkit.BuildRequest{AllowedTools: []string{created.ID}}}
	ctx := toolTestContext(t, map[string]string{"volume": created.ID})
	if _, err := prepareAliasTestInvoker(invoker).ResolveTools(ctx); err != nil {
		t.Fatal(err)
	}
	tool.Enabled = false
	if _, err := server.PutTool(t.Context(), created.ID, tool); err != nil {
		t.Fatalf("PutTool(%q) error = %v", tool.InvokeName, err)
	}
	result, err := prepareAliasTestInvoker(invoker).InvokeTool(ctx, "volume", json.RawMessage(`{"level":1}`))
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
	invoker := &testToolkitInvoker{
		Builder: &toolkit.Builder{Tools: server},
		HTTP:    giztools.HTTPExecutor{Transport: transport},
		Request: toolkit.BuildRequest{AllowedTools: []string{created.ID}},
	}
	ctx := toolTestContext(t, map[string]string{"weather": created.ID})
	result, err := prepareAliasTestInvoker(invoker).InvokeTool(ctx, "weather", json.RawMessage(`{"city":"Hangzhou"}`))
	if err != nil || string(result) != `{"temp":25}` {
		t.Fatalf("InvokeTool() = %s, %v", result, err)
	}
}

func TestToolkitInvokerReauthorizesAfterCredentialResolution(t *testing.T) {
	for _, mutation := range []string{"permissions", "binding", "resource"} {
		t.Run(mutation, func(t *testing.T) {
			server := toolkittest.New(t)
			initial := agentHostBoundHTTPTool("private_main")
			initial.HTTP.Auth = toolkit.HTTPAuth{Method: "volc_ark", Credential: new("fixture-credential")}
			first := putAgentHostTool(t, server, initial)
			second := putAgentHostTool(t, server, agentHostBoundHTTPTool("private_alternate"))
			bindings := map[string]apitypes.RuntimeProfileToolBinding{"volume": {ResourceId: first.ID}}
			profile := apitypes.RuntimeProfile{Revision: "current", Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Tools: &bindings}}}
			names := []string{"volume"}
			calls := 0
			invoker := ToolkitInvoker{
				Catalog: &toolcatalog.Catalog{Tools: server},
				Owner:   func(context.Context) (string, error) { return "workspace-owner", nil },
				Scope: func(context.Context, string) (apitypes.RuntimeProfile, []string, error) {
					return profile, names, nil
				},
				HTTP: giztools.HTTPExecutor{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				})},
			}
			invoker.Credentials = toolCredentialResolverFunc(func(context.Context, credential.HTTPAuthConfig) (giztools.HTTPAuthorizer, error) {
				return giztools.HTTPAuthorizerFunc(func(ctx context.Context, _ *http.Request) error {
					switch mutation {
					case "permissions":
						names = []string{}
					case "binding":
						bindings["volume"] = apitypes.RuntimeProfileToolBinding{ResourceId: second.ID}
					case "resource":
						initial.Enabled = false
						_, err := server.PutTool(ctx, first.ID, initial)
						return err
					}
					return nil
				}), nil
			})
			if definitions, err := invoker.ResolveTools(t.Context()); err != nil || len(definitions) != 1 {
				t.Fatalf("definitions = %v, error = %v", definitions, err)
			}
			result, err := invoker.InvokeTool(t.Context(), "volume", json.RawMessage(`{"level":1}`))
			if err != nil || calls != 0 || !strings.Contains(string(result), `"http_failure"`) {
				t.Fatalf("revoked call = %s, error = %v, HTTP calls = %d", result, err, calls)
			}
		})
	}
}

type toolCredentialResolverFunc func(context.Context, credential.HTTPAuthConfig) (giztools.HTTPAuthorizer, error)

func (f toolCredentialResolverFunc) HTTPAuthorizer(ctx context.Context, config credential.HTTPAuthConfig) (giztools.HTTPAuthorizer, error) {
	return f(ctx, config)
}

func TestToolkitInvokerConcurrentPeerScopesStayIsolated(t *testing.T) {
	server := toolkittest.New(t)
	firstTool := putAgentHostTool(t, server, agentHostBoundHTTPTool("peer_a"))
	secondTool := putAgentHostTool(t, server, agentHostBoundHTTPTool("peer_b"))
	recorder := &recordingHTTPTools{result: json.RawMessage(`{"ok":true}`)}
	invoker := &testToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, HTTP: giztools.HTTPExecutor{Transport: recorder}, Request: toolkit.BuildRequest{AllowedTools: []string{firstTool.ID, secondTool.ID}}}
	contexts := []context.Context{toolTestContext(t, map[string]string{"a": firstTool.ID}), toolTestContext(t, map[string]string{"b": secondTool.ID})}
	names := []string{"a", "b"}
	var wg sync.WaitGroup
	for i := range contexts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for range 25 {
				result, err := prepareAliasTestInvoker(invoker).InvokeTool(contexts[i], names[i], json.RawMessage(`{"level":3}`))
				if err != nil || string(result) != `{"ok":true}` {
					t.Errorf("invoke: %s %v", result, err)
				}
				result, err = prepareAliasTestInvoker(invoker).InvokeTool(contexts[i], names[1-i], json.RawMessage(`{"level":3}`))
				if err != nil || string(result) != `{"error":{"code":"unavailable","message":"tool is not authorized"}}` {
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

func TestWithToolExecutionPreservesAliasesToSameResource(t *testing.T) {
	bindings := map[string]apitypes.RuntimeProfileToolBinding{
		"one": {ResourceId: "volume_set"},
		"two": {ResourceId: "volume_set"},
	}
	ctx, err := WithToolExecution(t.Context(), &bindings)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := toolExecutionFromContext(ctx)
	if !ok || len(state.bindings) != 2 {
		t.Fatal("aliases were lost")
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
	bindings := make(map[string]apitypes.RuntimeProfileToolBinding, len(resources))
	for alias, name := range resources {
		bindings[alias] = apitypes.RuntimeProfileToolBinding{ResourceId: name}
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
	invoker := &testToolkitInvoker{Builder: &toolkit.Builder{Tools: server}, HTTP: giztools.HTTPExecutor{Transport: client}, Request: toolkit.BuildRequest{AllowedTools: []string{created.ID}}}
	ctx := toolTestContext(t, map[string]string{"volume": created.ID})
	if result, err := prepareAliasTestInvoker(invoker).InvokeTool(ctx, "volume", json.RawMessage(`{"level":"loud"}`)); err != nil || string(result) != `{"error":{"code":"invalid_arguments","message":"tool arguments do not match its schema"}}` {
		t.Fatalf("invalid arguments result=%s error=%v", result, err)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
}

// prepareAliasTestInvoker supplies a real catalog with per-call Profile scope;
// HTTP resource IDs in these legacy fixtures only select aliases in this test.
type testToolkitInvoker struct {
	Builder     *toolkit.Builder
	Request     toolkit.BuildRequest
	HTTP        giztools.HTTPExecutor
	Credentials toolCredentialResolver
}

func prepareAliasTestInvoker(source *testToolkitInvoker) *ToolkitInvoker {
	invoker := ToolkitInvoker{HTTP: source.HTTP, Credentials: source.Credentials}
	invoker.Catalog = &toolcatalog.Catalog{Tools: source.Builder.Tools}
	invoker.Owner = func(ctx context.Context) (string, error) {
		access, ok := resourceAccessFromContext(ctx)
		if !ok {
			return "", toolkit.ErrNotConfigured
		}
		return access.ownerPublicKey, nil
	}
	invoker.Scope = func(ctx context.Context, owner string) (apitypes.RuntimeProfile, []string, error) {
		scope, ok := toolExecutionFromContext(ctx)
		if !ok {
			return apitypes.RuntimeProfile{}, nil, toolkit.ErrNotConfigured
		}
		names := []string{}
		for alias, binding := range scope.bindings {
			if slices.Contains(source.Request.AllowedTools, binding.ResourceId) {
				names = append(names, alias)
			}
		}
		return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Tools: &scope.bindings}}}, names, nil
	}
	return &invoker
}

type verifyingMHSDevices struct {
	calls []string
}

func (*verifyingMHSDevices) Inspect(context.Context, string, apitypes.RuntimeProfile, toolcatalog.Tool) (toolcatalog.Availability, error) {
	return toolcatalog.Availability{Online: true, Supported: true}, nil
}

func (d *verifyingMHSDevices) Invoke(_ context.Context, _ string, _ apitypes.RuntimeProfile, tool toolcatalog.Tool, _ json.RawMessage) (json.RawMessage, error) {
	d.calls = append(d.calls, tool.Target["id"].(string))
	return json.RawMessage(`{"brightness_percent":50}`), nil
}

func TestToolkitVerifierRejectsWrongMHSReadBeforeDevice(t *testing.T) {
	devices := &verifyingMHSDevices{}
	bindings := map[string]apitypes.RuntimeProfileToolBinding{
		"lamp.read":   {Mhs: &apitypes.RuntimeProfileMhsTool{Id: "led.status", Operation: "read"}},
		"screen.read": {Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.main", Operation: "read"}},
	}
	profile := apitypes.RuntimeProfile{Revision: "current", Spec: apitypes.RuntimeProfileSpec{
		Resources: apitypes.RuntimeProfileResources{Tools: &bindings},
		Mhs:       &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "led.status", Hwd: "led"}, {Id: "display.main", Hwd: "display"}}}},
	}}
	checks := 0
	invoker := &ToolkitInvoker{
		Catalog: &toolcatalog.Catalog{Devices: devices},
		Owner:   func(context.Context) (string, error) { return "owner", nil },
		Scope: func(context.Context, string) (apitypes.RuntimeProfile, []string, error) {
			return profile, []string{"lamp.read", "screen.read"}, nil
		},
		Verify: func(_ context.Context, candidate toolcatalog.Tool, args json.RawMessage, conversation genx.ToolConversation) (string, error) {
			checks++
			if candidate.Target["operation"] != "read" || string(args) != `{}` || conversation.CurrentUser != "increase screen brightness" {
				t.Fatal("read verification omitted its target, parameters or actual user")
			}
			if candidate.Target["id"] != "display.main" {
				return "wrong target", nil
			}
			return "", nil
		},
	}
	ctx := genx.WithToolConversation(t.Context(), genx.ToolConversation{CurrentUser: "increase screen brightness"})
	result, err := invoker.InvokeTool(ctx, "lamp_read", json.RawMessage(`{}`))
	if err != nil || len(devices.calls) != 0 || !strings.Contains(string(result), `"intent_rejected"`) {
		t.Fatalf("wrong read reached device: result=%s error=%v calls=%v", result, err, devices.calls)
	}
	result, err = invoker.InvokeTool(ctx, "screen_read", json.RawMessage(`{}`))
	if err != nil || string(result) != `{"brightness_percent":50}` || checks != 2 || !slices.Equal(devices.calls, []string{"display.main"}) {
		t.Fatalf("correct read: result=%s error=%v checks=%d calls=%v", result, err, checks, devices.calls)
	}
}
