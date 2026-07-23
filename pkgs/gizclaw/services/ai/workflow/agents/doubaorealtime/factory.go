package doubaorealtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "doubao-realtime"

type Factory struct {
	Transformer         genx.TransformerMux
	TransformerForOwner func(context.Context, string) (genx.TransformerMux, error)
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	transformer, err := resolveOwnerTransformer(ctx, spec.Workspace, f.Transformer, f.TransformerForOwner)
	if err != nil {
		return nil, err
	}
	if transformer == nil {
		return nil, fmt.Errorf("doubaorealtime: transformer is required")
	}
	pattern, err := resolveRealtimeModelPattern(spec)
	if err != nil {
		return nil, err
	}
	return agenthost.NewTransformerAgent(patternTransformer{Transformer: transformer, Pattern: pattern}), nil
}

func resolveOwnerTransformer(ctx context.Context, workspace apitypes.Workspace, fallback genx.TransformerMux, resolve func(context.Context, string) (genx.TransformerMux, error)) (genx.TransformerMux, error) {
	if workspace.OwnerPublicKey == nil || strings.TrimSpace(*workspace.OwnerPublicKey) == "" {
		return fallback, nil
	}
	owner := strings.TrimSpace(*workspace.OwnerPublicKey)
	if resolve == nil {
		return nil, fmt.Errorf("doubaorealtime: workspace %q owner transformer resolver is required", workspace.Name)
	}
	transformer, err := resolve(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("doubaorealtime: workspace %q owner runtime: %w", workspace.Name, err)
	}
	if transformer == nil {
		return nil, fmt.Errorf("doubaorealtime: workspace %q owner runtime returned no transformer", workspace.Name)
	}
	return transformer, nil
}

type patternTransformer struct {
	Transformer genx.TransformerMux
	Pattern     string
}

func (t patternTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t.Transformer == nil {
		return nil, fmt.Errorf("doubaorealtime: transformer is required")
	}
	return t.Transformer.Transform(ctx, t.Pattern, input)
}

func resolveRealtimeModelPattern(spec agenthost.Spec) (string, error) {
	workflowSpec := spec.Workflow.Spec.DoubaoRealtime
	if workflowSpec == nil {
		return "", fmt.Errorf("doubaorealtime: workflow doubao_realtime spec is required")
	}
	if err := rejectTools("workflow", workflowSpec.Tools); err != nil {
		return "", err
	}

	model := strings.TrimSpace(workflowSpec.Model)
	params := realtimeWorkflowParams(*workflowSpec)
	if spec.Workspace.Parameters != nil {
		typed, err := spec.Workspace.Parameters.AsDoubaoRealtimeWorkspaceParameters()
		if err != nil {
			return "", fmt.Errorf("doubaorealtime: decode workspace parameters: %w", err)
		}
		if typed.Model != nil && strings.TrimSpace(*typed.Model) != "" {
			model = strings.TrimSpace(*typed.Model)
		}
		if err := rejectTools("workspace", typed.Tools); err != nil {
			return "", err
		}
		params = mergeDoubaoRealtimeWorkspaceParams(params, typed)
	}
	if dialogID := strings.TrimSpace(spec.Runtime.DialogID); dialogID != "" {
		if params == nil {
			params = make(map[string]any)
		}
		params["dialog_id"] = dialogID
	}
	if model == "" {
		return "", fmt.Errorf("doubaorealtime: model is required")
	}
	return normalizeModelPattern(appendPatternParams(model, params)), nil
}

func normalizeModelPattern(pattern string) string {
	pattern = strings.Trim(strings.TrimSpace(pattern), "/")
	if pattern == "" || strings.Contains(pattern, "/") {
		return pattern
	}
	return "model/" + pattern
}

func realtimeWorkflowParams(spec apitypes.DoubaoRealtimeWorkflowSpec) map[string]any {
	params := make(map[string]any)
	if value := stringPtrValue(spec.Instructions); value != "" {
		params["instructions"] = value
	}
	if spec.Audio != nil {
		mergeDoubaoRealtimeAudioParams(params, *spec.Audio)
	}
	if spec.Extension != nil {
		params["extension"] = *spec.Extension
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func mergeDoubaoRealtimeWorkspaceParams(params map[string]any, typed apitypes.DoubaoRealtimeWorkspaceParameters) map[string]any {
	if params == nil {
		params = make(map[string]any)
	}
	if typed.Input != nil {
		switch *typed.Input {
		case apitypes.WorkspaceInputModePushToTalk:
			params["mode"] = "push-to-talk"
		case apitypes.WorkspaceInputModeRealtime:
			params["mode"] = "realtime"
		}
	}
	if value := stringPtrValue(typed.Instructions); value != "" {
		params["instructions"] = value
	}
	if typed.Audio != nil {
		mergeDoubaoRealtimeAudioParams(params, *typed.Audio)
	}
	if typed.Extension != nil {
		params["extension"] = *typed.Extension
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func rejectTools(scope string, tools *[]apitypes.DoubaoRealtimeFunctionTool) error {
	if tools == nil || len(*tools) == 0 {
		return nil
	}
	return fmt.Errorf("doubaorealtime: %s tools are unsupported until ToolCall is implemented", scope)
}

func mergeDoubaoRealtimeAudioParams(params map[string]any, audio apitypes.DoubaoRealtimeAudio) {
	params["input_format"] = string(audio.Input.Format.Type)
	params["input_sample_rate"] = audio.Input.Format.Rate
	params["output_format"] = string(audio.Output.Format.Type)
	params["output_sample_rate"] = audio.Output.Format.Rate
	if value := stringPtrValue(audio.Output.Voice); value != "" {
		params["output_voice"] = value
	}
	if audio.Output.Speed != nil {
		params["output_speed"] = *audio.Output.Speed
	}
	if audio.Output.Loudness != nil {
		params["output_loudness"] = *audio.Output.Loudness
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func appendPatternParams(pattern string, params map[string]any) string {
	if len(params) == 0 {
		return pattern
	}
	base, rawQuery, _ := strings.Cut(strings.TrimSpace(pattern), "?")
	query, _ := url.ParseQuery(rawQuery)
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if text, ok := workflowParamString(params[key]); ok {
			query.Set(key, text)
		}
	}
	encoded := query.Encode()
	if encoded == "" {
		return base
	}
	return base + "?" + encoded
}

func workflowParamString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		return text, text != ""
	case bool:
		return strconv.FormatBool(typed), true
	case int:
		return strconv.Itoa(typed), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10), true
		}
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case float32:
		value := float64(typed)
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10), true
		}
		return strconv.FormatFloat(value, 'f', -1, 32), true
	case json.Number:
		return typed.String(), true
	default:
		data, err := json.Marshal(typed)
		if err != nil || string(data) == "null" {
			return "", false
		}
		return string(data), true
	}
}
