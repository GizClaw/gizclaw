package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
)

const clientToolTimeout = 10 * time.Second

type toolCredentialResolver interface {
	HTTPAuthorizer(context.Context, credential.HTTPAuthConfig) (giztools.HTTPAuthorizer, error)
}

// ToolkitInvoker is an AgentHost-owned ToolInvoker. It resolves the current
// Peer's RuntimeProfile on every call and keeps resource storage and transport
// details out of workflow Transformers.
type ToolkitInvoker struct {
	Builder       *toolkit.Builder
	Credentials   toolCredentialResolver
	HTTP          giztools.HTTPExecutor
	Request       toolkit.BuildRequest
	ClientTimeout time.Duration
}

var _ genx.ToolInvoker = (*ToolkitInvoker)(nil)

func (i *ToolkitInvoker) ResolveTools(ctx context.Context) ([]genx.ToolDefinition, error) {
	request, _, err := i.requestForContext(ctx)
	if err != nil {
		return nil, err
	}
	kit, err := i.Builder.Build(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("agenthost: resolve Tools: %w", err)
	}
	definitions := make([]genx.ToolDefinition, 0, len(kit.Tools))
	for index := range kit.Tools {
		tool := kit.Tools[index]
		schema := tool.InputSchema
		definitions = append(definitions, genx.ToolDefinition{
			Name:        tool.Name,
			Description: stringValue(tool.Description),
			Argument:    &schema,
		})
	}
	return definitions, nil
}

func (i *ToolkitInvoker) InvokeTool(
	ctx context.Context,
	name string,
	args json.RawMessage,
) (json.RawMessage, error) {
	request, scope, err := i.requestForContext(ctx)
	if err != nil {
		return nil, err
	}
	tool, arguments, err := i.Builder.ResolveInvoke(ctx, toolkit.InvokeRequest{
		Build: request,
		Name:  name,
		Args:  args,
	})
	if err != nil {
		if errors.Is(err, toolkit.ErrToolNotFound) {
			return recoverableToolError("unavailable", "tool is unavailable"), nil
		}
		return nil, fmt.Errorf("agenthost: authorize Tool invocation: %w", err)
	}
	switch tool.Type {
	case toolkit.ToolTypeHTTPRequest:
		return i.invokeHTTP(ctx, tool, arguments)
	case toolkit.ToolTypeClientRPC:
		return invokeClientTool(ctx, scope.client, tool.Name, arguments, i.ClientTimeout)
	default:
		return nil, fmt.Errorf("agenthost: unsupported Tool type %q", tool.Type)
	}
}

func (i *ToolkitInvoker) requestForContext(ctx context.Context) (toolkit.BuildRequest, toolExecutionContext, error) {
	if i == nil || i.Builder == nil {
		return toolkit.BuildRequest{}, toolExecutionContext{}, toolkit.ErrNotConfigured
	}
	scope, ok := toolExecutionFromContext(ctx)
	if !ok {
		return toolkit.BuildRequest{}, toolExecutionContext{}, errors.New("agenthost: current Peer Tool context is required")
	}
	request := i.Request
	request.ProfileTools = append([]string(nil), scope.profileTools...)
	if access, ok := resourceAccessFromContext(ctx); ok {
		request.CallerPublicKey = access.ownerPublicKey
	}
	return request, scope, nil
}

func (i *ToolkitInvoker) invokeHTTP(
	ctx context.Context,
	tool toolkit.Tool,
	args json.RawMessage,
) (json.RawMessage, error) {
	if tool.HTTP == nil {
		return nil, fmt.Errorf("agenthost: Tool %q has no HTTP operation", tool.Name)
	}
	authorizer, err := i.httpAuthorizer(ctx, tool.HTTP.Auth)
	if err != nil {
		return nil, fmt.Errorf("agenthost: Tool %q auth: %w", tool.Name, err)
	}
	result, err := i.HTTP.Invoke(ctx, httpOperation(*tool.HTTP), args, authorizer)
	if errors.Is(err, context.DeadlineExceeded) {
		return recoverableToolError("timeout", "tool execution timed out"), nil
	}
	if err != nil {
		return nil, fmt.Errorf("agenthost: invoke HTTP Tool %q: %w", tool.Name, err)
	}
	return result, nil
}

func (i *ToolkitInvoker) httpAuthorizer(
	ctx context.Context,
	auth toolkit.HTTPAuth,
) (giztools.HTTPAuthorizer, error) {
	switch auth.Method {
	case "none":
		return nil, nil
	case "bearer":
		return headerAuthorizer("Authorization", "Bearer "+pointerValue(auth.BearerToken)), nil
	case "header_api_key":
		return headerAuthorizer(pointerValue(auth.Header), pointerValue(auth.APIKey)), nil
	case "volc_ark", "volc_search", "volc_openapi", "aliyun_app_code", "aliyun_openapi_v3":
		if i.Credentials == nil {
			return nil, errors.New("credential resolver is not configured")
		}
		return i.Credentials.HTTPAuthorizer(ctx, credential.HTTPAuthConfig{
			Method:     auth.Method,
			Credential: pointerValue(auth.Credential),
			Region:     pointerValue(auth.Region),
			Service:    pointerValue(auth.Service),
			Action:     pointerValue(auth.Action),
			Version:    pointerValue(auth.Version),
		})
	default:
		return nil, fmt.Errorf("unsupported auth method %q", auth.Method)
	}
}

func invokeClientTool(
	ctx context.Context,
	client ClientToolInvoker,
	name string,
	args json.RawMessage,
	timeout time.Duration,
) (json.RawMessage, error) {
	if client == nil {
		return recoverableToolError("unavailable", "client tool is unavailable"), nil
	}
	if timeout <= 0 {
		timeout = clientToolTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := client.InvokeClientTool(callCtx, name, args)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(callCtx.Err(), context.DeadlineExceeded):
			return recoverableToolError("timeout", "tool execution timed out"), nil
		case errors.Is(err, giztools.ErrClientToolUnavailable):
			return recoverableToolError("unavailable", "client tool is unavailable"), nil
		default:
			return nil, fmt.Errorf("agenthost: invoke client Tool %q: %w", name, err)
		}
	}
	if len(result) == 0 {
		return json.RawMessage(`null`), nil
	}
	if len(result) > 64<<10 {
		return nil, fmt.Errorf("agenthost: client Tool %q result exceeds 65536 bytes", name)
	}
	if !json.Valid(result) {
		return nil, fmt.Errorf("agenthost: client Tool %q returned invalid JSON", name)
	}
	return json.RawMessage(append([]byte(nil), result...)), nil
}

func recoverableToolError(code, message string) json.RawMessage {
	value, _ := json.Marshal(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
	return value
}

func httpOperation(source toolkit.HTTPRequest) giztools.HTTPOperation {
	return giztools.HTTPOperation{
		URL:                source.URL,
		Method:             source.Method,
		Headers:            cloneStringMap(source.Headers),
		Query:              httpBindings(source.Query),
		Body:               httpBindings(source.Body),
		ResponsePointer:    cloneString(source.ResponsePointer),
		SuccessStatusCodes: append([]int(nil), source.SuccessStatusCodes...),
		Timeout:            source.Timeout,
		MaxResponseBytes:   source.MaxResponseBytes,
	}
}

func httpBindings(source []toolkit.HTTPArgumentBinding) []giztools.HTTPBinding {
	result := make([]giztools.HTTPBinding, 0, len(source))
	for _, binding := range source {
		result = append(result, giztools.HTTPBinding{
			ArgumentPointer: binding.ArgumentPointer,
			Target:          binding.Target,
			Required:        binding.Required,
		})
	}
	return result
}

func headerAuthorizer(name, value string) giztools.HTTPAuthorizer {
	return giztools.HTTPAuthorizerFunc(func(_ context.Context, request *http.Request) error {
		request.Header.Set(name, value)
		return nil
	})
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	maps.Copy(result, source)
	return result
}

func cloneString(source *string) *string {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
