package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

// appJSONMaxBytes matches the RPC args_json and result_json capacity excluding NUL.
const appJSONMaxBytes = 4096

// ErrAppUnavailable indicates that the current connection cannot execute an App.
var ErrAppUnavailable = errors.New("agenthost: App unavailable")

// AppClient resolves installed Apps and executes methods on one current connection.
type AppClient interface {
	ResolveApps(context.Context, []string) ([]apitypes.App, error)
	InvokeApp(context.Context, apitypes.App, apitypes.AppMethod, json.RawMessage) (json.RawMessage, error)
}

type appContextKey struct{}
type appExecution struct {
	ids    []string
	client AppClient
}

// WithAppExecution attaches profile bindings and their connection-scoped executor.
func WithAppExecution(ctx context.Context, bindings *map[string]apitypes.RuntimeProfileBinding, client AppClient) context.Context {
	scope := appExecution{client: client}
	if bindings != nil {
		for _, binding := range *bindings {
			scope.ids = append(scope.ids, binding.ResourceId)
		}
	}
	sort.Strings(scope.ids)
	return context.WithValue(ctx, appContextKey{}, scope)
}

func (i *ToolkitInvoker) resolveApps(ctx context.Context) ([]apitypes.App, AppClient, error) {
	scope, ok := ctx.Value(appContextKey{}).(appExecution)
	if !ok || scope.client == nil || len(scope.ids) == 0 {
		return nil, nil, nil
	}
	timeout := i.ClientTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	apps, err := scope.client.ResolveApps(callCtx, scope.ids)
	return apps, scope.client, err
}

func appendAppDefinitions(definitions []genx.ToolDefinition, apps []apitypes.App) ([]genx.ToolDefinition, error) {
	names := map[string]bool{}
	for _, definition := range definitions {
		names[definition.Name] = true
	}
	for _, value := range apps {
		for _, method := range value.Methods {
			name := value.AppName + "__" + method.Name
			if names[name] {
				return nil, fmt.Errorf("agenthost: duplicate tool invocation name %q", name)
			}
			names[name] = true
			schema := method.InputSchema
			definitions = append(definitions, genx.ToolDefinition{Name: name, Description: method.Description, Argument: &schema})
		}
	}
	return definitions, nil
}

func (i *ToolkitInvoker) invokeApp(ctx context.Context, client AppClient, value apitypes.App, method apitypes.AppMethod, args json.RawMessage) (json.RawMessage, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var decoded map[string]any
	if len(args) > appJSONMaxBytes || json.Unmarshal(args, &decoded) != nil || decoded == nil {
		return nil, fmt.Errorf("%w: App arguments must be a JSON object within %d bytes", toolkit.ErrInvalidTool, appJSONMaxBytes)
	}
	schema, err := method.InputSchema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(decoded); err != nil {
		return nil, fmt.Errorf("%w: %v", toolkit.ErrInvalidTool, err)
	}
	timeout := i.ClientTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := client.InvokeApp(callCtx, value, method, args)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return recoverableToolError("timeout", "tool execution timed out"), nil
	}
	if errors.Is(err, ErrAppUnavailable) {
		return recoverableToolError("unavailable", "App is unavailable"), nil
	}
	if err != nil {
		return nil, err
	}
	if len(result) > appJSONMaxBytes || !json.Valid(result) {
		return nil, fmt.Errorf("agenthost: App result must be valid JSON within %d bytes", appJSONMaxBytes)
	}
	return result, nil
}
