package doubaorealtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/agenthost"
)

const Type = "doubao-realtime"

type Factory struct {
	Transformer genx.Transformer
}

func (f Factory) NewAgent(_ context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if f.Transformer == nil {
		return nil, fmt.Errorf("doubaorealtime: transformer is required")
	}
	pattern, err := resolveRealtimeModelPattern(spec)
	if err != nil {
		return nil, err
	}
	return agenthost.NewTransformerAgent(patternTransformer{Transformer: f.Transformer, Pattern: pattern}), nil
}

type patternTransformer struct {
	Transformer genx.Transformer
	Pattern     string
}

func (t patternTransformer) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
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
		params = mergeDoubaoRealtimeWorkspaceParams(params, typed)
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
	if spec.Tools != nil {
		params["tools"] = *spec.Tools
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
	if typed.Tools != nil {
		params["tools"] = *typed.Tools
	}
	if typed.Extension != nil {
		params["extension"] = *typed.Extension
	}
	if len(params) == 0 {
		return nil
	}
	return params
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
