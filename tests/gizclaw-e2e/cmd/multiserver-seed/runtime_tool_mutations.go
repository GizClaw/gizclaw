package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

func runtimeToolMutations(api *adminhttp.ClientWithResponses, prefix string, gate chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Instance  string `json:"instance"`
			Operation string `json:"operation"`
			Oracle    string `json:"oracle"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&request); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		id := prefix + "-" + request.Instance
		if err := customid.ValidateResourceID(id); err != nil {
			http.Error(w, "invalid instance", 400)
			return
		}
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		case <-r.Context().Done():
			http.Error(w, "coordination canceled", 500)
			return
		}
		status, err := mutateRuntimeTool(r.Context(), api, id, request.Operation, request.Oracle)
		if err != nil {
			http.Error(w, "test mutation failed", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"operation": request.Operation, "status": status})
	}
}

func mutateRuntimeTool(ctx context.Context, api *adminhttp.ClientWithResponses, id, operation, oracle string) (int, error) {
	current, err := api.GetRuntimeProfileWithResponse(ctx, id)
	if err != nil || current.JSON200 == nil {
		return 500, errors.New("test Profile unavailable")
	}
	spec := current.JSON200.Spec
	tools := *spec.Resources.Tools
	add := func(alias string, binding apitypes.RuntimeProfileToolBinding) {
		tools[alias] = binding
		workflow := spec.Workflows["assistant-10"]
		names := append([]string{}, (*workflow.Toolkit.ToolNames)...)
		names = append(names, alias)
		selection := *workflow.Toolkit
		selection.ToolNames = &names
		workflow.Toolkit = &selection
		spec.Workflows["assistant-10"] = workflow
	}
	switch operation {
	case "add-echo-http":
		if err := customid.ValidateResourceID(oracle); err != nil {
			return 400, nil
		}
		target := id + "-echo"
		resource := map[string]any{
			"apiVersion": "gizclaw.admin/v1alpha1", "kind": "Tool", "metadata": map[string]any{"id": target},
			"spec": map[string]any{
				"type": "http_request", "invoke_name": "private_" + target, "enabled": true,
				"input_schema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
				"http": map[string]any{"url": "https://postman-echo.com/get", "method": "GET", "auth": map[string]any{"method": "none"},
					"headers": map[string]any{"X-Giztest-Oracle": oracle}, "response_pointer": "/headers/x-giztest-oracle", "timeout": "10s", "max_response_bytes": 8192},
			},
		}
		encoded, err := json.Marshal(resource)
		if err != nil {
			return 500, err
		}
		var body adminhttp.ApplyResourceJSONRequestBody
		if err := json.Unmarshal(encoded, &body); err != nil {
			return 500, err
		}
		result, err := api.ApplyResourceWithResponse(ctx, body)
		if err != nil || result.JSON200 == nil {
			return 500, errors.New("echo HTTP resource creation failed")
		}
		tools["echo.lookup"] = apitypes.RuntimeProfileToolBinding{ResourceId: target, I18n: binding("", "Read actual HTTP resource identifier", "从 HTTPS 服务读取实际资源编号，不猜测或生成编号，只返回服务提供的值").I18n}
		workflow := spec.Workflows["assistant-10"]
		workflow.Toolkit = &apitypes.RuntimeProfileToolSelection{ToolNames: new([]string{"echo.lookup"})}
		spec.Workflows["http-probe"] = workflow
	case "add-http", "rebind-http", "disable-http":
		target := id + "-first"
		if operation != "add-http" {
			target = id + "-second"
		}
		resource := map[string]any{
			"apiVersion": "gizclaw.admin/v1alpha1", "kind": "Tool", "metadata": map[string]any{"id": target},
			"spec": map[string]any{
				"type": "http_request", "invoke_name": "private_" + target, "enabled": operation != "disable-http",
				"input_schema": map[string]any{"type": "object", "properties": map[string]any{"key": map[string]any{"type": "string"}}, "required": []string{"key"}, "additionalProperties": false},
				"http":         map[string]any{"url": "https://giztest.invalid/" + target, "method": "GET", "auth": map[string]any{"method": "none"}, "timeout": "1s", "max_response_bytes": 1024},
			},
		}
		data, err := json.Marshal(resource)
		if err != nil {
			return 500, err
		}
		var body adminhttp.ApplyResourceJSONRequestBody
		if err := json.Unmarshal(data, &body); err != nil {
			return 500, err
		}
		result, err := api.ApplyResourceWithResponse(ctx, body)
		if err != nil || result.JSON200 == nil {
			return 500, errors.New("HTTP resource mutation failed")
		}
		binding := apitypes.RuntimeProfileToolBinding{ResourceId: target, I18n: binding("", "Lookup", "查询").I18n}
		if _, exists := tools["lookup"]; exists {
			tools["lookup"] = binding
		} else {
			add("lookup", binding)
		}
	case "delete-http":
		result, err := api.DeleteResourceWithResponse(ctx, adminhttp.ResourceKind("Tool"), tools["lookup"].ResourceId)
		if err != nil || result.StatusCode() != 200 {
			return 500, errors.New("HTTP resource deletion failed")
		}
		return 200, nil
	case "unsupported-field":
		add("display.enabled", apitypes.RuntimeProfileToolBinding{I18n: binding("", "Display power", "屏幕开关").I18n, Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.main", Operation: "write", Fields: new([]string{"enabled"})}})
	case "unimplemented-procedure":
		add("restart", apitypes.RuntimeProfileToolBinding{I18n: binding("", "Restart", "重启").I18n, ClientTool: &apitypes.RuntimeProfileClientTool{Name: "device.reboot"}})
	case "unknown-field":
		add("invalid-field", apitypes.RuntimeProfileToolBinding{I18n: binding("", "Invalid", "非法").I18n, Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.main", Operation: "write", Fields: new([]string{"invented"})}})
	case "undeclared-instance":
		add("invalid-target", apitypes.RuntimeProfileToolBinding{I18n: binding("", "Invalid", "非法").I18n, Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.other", Operation: "read"}})
	case "mixed-source":
		add("invalid-source", apitypes.RuntimeProfileToolBinding{I18n: binding("", "Invalid", "非法").I18n, ResourceId: id + "-first", ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.play"}})
	case "alias-conflict":
		add("llm", apitypes.RuntimeProfileToolBinding{I18n: binding("", "Conflict", "冲突").I18n, ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.play"}})
	case "whitespace-alias":
		tools[" leading-space"] = apitypes.RuntimeProfileToolBinding{I18n: binding("", "Invalid", "非法").I18n, ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.stop"}}
	case "underscore-alias":
		tools["invalid_alias"] = apitypes.RuntimeProfileToolBinding{I18n: binding("", "Invalid", "非法").I18n, ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.stop"}}
	default:
		return 400, nil
	}
	result, err := api.PutRuntimeProfileWithResponse(ctx, id, adminhttp.RuntimeProfileUpsert{Id: id, Spec: spec})
	if err != nil {
		return 500, err
	}
	return result.StatusCode(), nil
}
