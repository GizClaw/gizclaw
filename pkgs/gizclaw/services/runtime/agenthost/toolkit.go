package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
)

type toolCredentialResolver interface {
	HTTPAuthorizer(context.Context, credential.HTTPAuthConfig) (giztools.HTTPAuthorizer, error)
}

// ToolkitInvoker is an AgentHost-owned ToolInvoker. It resolves the current
// Peer's RuntimeProfile on every call and keeps resource storage and transport
// details out of workflow Transformers.
type ToolkitInvoker struct {
	Catalog     *toolcatalog.Catalog
	Owner       func(context.Context) (string, error)
	Scope       func(context.Context, string) (apitypes.RuntimeProfile, []string, error)
	Credentials toolCredentialResolver
	HTTP        giztools.HTTPExecutor
	// Verify validates a mutating proposal against its actual conversation.
	// It never selects another alias or changes the supplied arguments.
	Verify func(context.Context, toolcatalog.Tool, json.RawMessage, genx.ToolConversation) (string, error)
}

var _ genx.ToolInvoker = (*ToolkitInvoker)(nil)

// ResolveCatalog returns the current authorized catalog for product consumers.
// HTTP executor data is private; model inputs must project only public metadata.
func (i *ToolkitInvoker) ResolveCatalog(ctx context.Context) ([]toolcatalog.Tool, error) {
	if i == nil || i.Catalog == nil || i.Owner == nil || i.Scope == nil {
		return nil, toolkit.ErrNotConfigured
	}
	owner, err := i.Owner(ctx)
	if err != nil {
		return nil, err
	}
	profile, names, err := i.Scope(ctx, owner)
	if err != nil {
		return nil, err
	}
	return i.Catalog.Resolve(ctx, owner, profile, &names)
}

func (i *ToolkitInvoker) ResolveTools(ctx context.Context) ([]genx.ToolDefinition, error) {
	tools, err := i.ResolveCatalog(ctx)
	if err != nil {
		return nil, err
	}
	definitions := make([]genx.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if !tool.Available {
			continue
		}
		schema := tool.Schema
		definitions = append(definitions, genx.ToolDefinition{Name: tool.FunctionName, Description: tool.Description, Argument: &schema})
	}
	return definitions, nil
}

func (i *ToolkitInvoker) InvokeTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if i == nil || i.Catalog == nil || i.Owner == nil || i.Scope == nil {
		return recoverableToolError("unavailable", "tool authorization is unavailable"), nil
	}
	owner, err := i.Owner(ctx)
	if err != nil {
		return recoverableToolError("unavailable", "tool authorization is unavailable"), nil
	}
	profile, names, err := i.Scope(ctx, owner)
	if err != nil {
		return recoverableToolError("unavailable", "tool authorization is unavailable"), nil
	}
	selected := []string{}
	for _, alias := range names {
		function, err := toolcatalog.FunctionName(alias)
		if err == nil && function == name {
			selected = append(selected, alias)
		}
	}
	if len(selected) != 1 {
		slog.WarnContext(ctx, "agenthost: Tool call rejected", "reason", "function_not_authorized")
		return recoverableToolError("unavailable", "tool is not authorized"), nil
	}
	// Resolve only the requested binding immediately before execution. Unrelated
	// devices do not gain traffic or affect this operation's authorization.
	tools, err := i.Catalog.Resolve(ctx, owner, profile, &selected)
	if err != nil {
		return recoverableToolError("unavailable", "tool authorization is unavailable"), nil
	}
	for _, tool := range tools {
		if tool.FunctionName != name {
			continue
		}
		slog.InfoContext(ctx, "agenthost: Tool execution requested", "tool_alias", tool.Alias, "source", tool.Source)
		if !tool.Available {
			slog.WarnContext(ctx, "agenthost: Tool unavailable", "tool_alias", tool.Alias, "reason", tool.Reason)
			return recoverableToolError("unavailable", "tool is unavailable"), nil
		}
		if err := toolcatalog.ValidateArguments(tool, args); err != nil {
			slog.WarnContext(ctx, "agenthost: Tool arguments rejected", "tool_alias", tool.Alias, "reason", "invalid_arguments")
			return recoverableToolError("invalid_arguments", "tool arguments do not match its schema"), nil
		}
		if i.Verify != nil && mutatingTool(tool) {
			conversation, ok := genx.ToolConversationFromContext(ctx)
			if !ok || strings.TrimSpace(conversation.CurrentUser) == "" {
				return recoverableToolError("intent_unverified", "the actual user conversation is unavailable; do not execute or claim completion"), nil
			}
			reason, err := i.Verify(ctx, tool, args, conversation)
			if err != nil {
				return recoverableToolError("intent_unverified", "request verification failed; no change was executed"), nil
			}
			if reason != "" {
				return recoverableToolError("intent_rejected", reason+"; no change was executed; clarify missing information and never substitute another target or claim completion"), nil
			}
		}
		tool.Authorize = func(ctx context.Context) error {
			current, names, err := i.Scope(ctx, owner)
			if err != nil || current.Revision != profile.Revision || !slices.Contains(names, tool.Alias) || current.Spec.Resources.Tools == nil {
				return errors.New("Tool permissions changed")
			}
			if binding, ok := (*current.Spec.Resources.Tools)[tool.Alias]; !ok || !reflect.DeepEqual(binding, tool.Binding) {
				return errors.New("Tool binding changed")
			}
			if tool.HTTP != nil {
				resource, err := i.Catalog.Tools.GetToolByID(ctx, tool.HTTP.ID)
				if err != nil || !resource.Enabled || !reflect.DeepEqual(resource, *tool.HTTP) {
					return errors.New("HTTP Tool changed")
				}
			}
			return nil
		}
		if tool.HTTP != nil {
			return i.invokeHTTP(ctx, *tool.HTTP, args, tool.Authorize)
		}
		result, err := i.Catalog.Devices.Invoke(ctx, owner, profile, tool, args)
		if err != nil {
			reason := "device_operation_failed"
			if failure, ok := errors.AsType[*toolcatalog.InvocationError](err); ok {
				reason = failure.Code
			}
			slog.WarnContext(ctx, "agenthost: device Tool failed", "tool_alias", tool.Alias, "reason", reason)
			return recoverableToolError("device_failure", "device operation did not succeed"), nil
		}
		return result, nil
	}
	return recoverableToolError("unavailable", "tool is not authorized"), nil
}

func mutatingTool(tool toolcatalog.Tool) bool {
	if tool.Source == "mhs" {
		return tool.Binding.Mhs.Operation == "write"
	}
	if tool.Source == "client_tool" {
		switch tool.Binding.ClientTool.Name {
		case "info.get", "identifiers.get", "device.status.get", "audioplayer.get", "audioplayer.playlist.get", "wifi.scan", "wifi.saved.list":
			return false
		default:
			return true
		}
	}
	return tool.HTTP != nil && tool.HTTP.HTTP != nil && tool.HTTP.HTTP.Method != "GET"
}

func (i *ToolkitInvoker) invokeHTTP(
	ctx context.Context,
	tool toolkit.Tool,
	args json.RawMessage,
	authorize func(context.Context) error,
) (json.RawMessage, error) {
	if tool.HTTP == nil {
		return nil, fmt.Errorf("agenthost: Tool %q has no HTTP operation", tool.InvokeName)
	}
	authorizer, err := i.httpAuthorizer(ctx, tool.HTTP.Auth)
	if err != nil {
		return recoverableToolError("authorization_failure", "tool authorization did not succeed"), nil
	}
	result, err := i.HTTP.Invoke(ctx, httpOperation(*tool.HTTP), args, giztools.HTTPAuthorizerFunc(func(callCtx context.Context, request *http.Request) error {
		if authorizer != nil {
			if err := authorizer.Authorize(callCtx, request); err != nil {
				return err
			}
		}
		return authorize(callCtx)
	}))
	if errors.Is(err, context.DeadlineExceeded) {
		return recoverableToolError("timeout", "tool execution timed out"), nil
	}
	if err != nil {
		return recoverableToolError("http_failure", "HTTP tool operation did not succeed"), nil
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
