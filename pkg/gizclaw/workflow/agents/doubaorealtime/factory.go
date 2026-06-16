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
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/agenthost"
)

const Type = "doubao-realtime"

const (
	workspaceAgentTypeParameter     = "agent_type"
	workspaceRealtimeModelParameter = "realtime_model"
)

type Factory struct {
	Transformer genx.Transformer
}

func (f Factory) NewAgent(_ context.Context, spec agenthost.Spec) (genx.Transformer, error) {
	if f.Transformer == nil {
		return nil, fmt.Errorf("doubaorealtime: transformer is required")
	}
	pattern, err := resolveRealtimeModelPattern(spec)
	if err != nil {
		return nil, err
	}
	return patternTransformer{Transformer: f.Transformer, Pattern: pattern}, nil
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

type realtimeWorkflowConfig struct {
	Spec struct {
		Model          string `json:"model"`
		RealtimeModel  string `json:"realtime_model"`
		Realtime       map[string]any
		RealtimeConfig map[string]any `json:"realtime_config"`
	} `json:"spec"`
}

func resolveRealtimeModelPattern(spec agenthost.Spec) (string, error) {
	if pattern := workflowRealtimeModelPattern(spec); pattern != "" {
		return normalizeModelPattern(pattern), nil
	}
	pattern, err := workspaceRealtimeModelPattern(spec.Workspace.Parameters)
	if err != nil {
		return "", err
	}
	if pattern != "" {
		return normalizeModelPattern(pattern), nil
	}
	return "", fmt.Errorf("doubaorealtime: model is required")
}

func workflowRealtimeModelPattern(spec agenthost.Spec) string {
	data, err := json.Marshal(spec.Workflow)
	if err != nil {
		return ""
	}
	var cfg realtimeWorkflowConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	pattern := firstNonEmpty(cfg.Spec.RealtimeModel, cfg.Spec.Model)
	if pattern == "" {
		return ""
	}
	params := realtimeWorkflowParams(cfg)
	params = mergeWorkspaceRealtimeParams(params, spec.Workspace.Parameters)
	return appendPatternParams(pattern, params)
}

func workspaceRealtimeModelPattern(parameters *map[string]any) (string, error) {
	if parameters == nil {
		return "", nil
	}
	for _, key := range []string{workspaceRealtimeModelParameter, "model"} {
		value, ok := (*parameters)[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("doubaorealtime: workspace parameter %q must be a string", key)
		}
		if strings.TrimSpace(text) != "" {
			params := mergeWorkspaceRealtimeParams(nil, parameters)
			return appendPatternParams(text, params), nil
		}
	}
	return "", nil
}

func normalizeModelPattern(pattern string) string {
	pattern = strings.Trim(strings.TrimSpace(pattern), "/")
	if pattern == "" || strings.Contains(pattern, "/") {
		return pattern
	}
	return "model/" + pattern
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func realtimeWorkflowParams(cfg realtimeWorkflowConfig) map[string]any {
	params := make(map[string]any)
	mergeRealtimeWorkflowParamsValue(params, cfg.Spec.RealtimeConfig)
	mergeRealtimeWorkflowParamsValue(params, cfg.Spec.Realtime)
	if len(params) == 0 {
		return nil
	}
	return params
}

func mergeWorkspaceRealtimeParams(params map[string]any, parameters *map[string]any) map[string]any {
	if parameters == nil {
		return params
	}
	if params == nil {
		params = make(map[string]any)
	}
	for key, value := range *parameters {
		switch key {
		case workspaceAgentTypeParameter, workspaceRealtimeModelParameter, "model":
			continue
		case "realtime", "realtime_config":
			mergeRealtimeWorkflowParamsValue(params, value)
		default:
			mergeRealtimeWorkflowParam(params, key, value)
		}
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func mergeRealtimeWorkflowParamsValue(params map[string]any, value any) {
	values, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, value := range values {
		mergeRealtimeWorkflowParam(params, key, value)
	}
}

func mergeRealtimeWorkflowParam(params map[string]any, key string, value any) {
	switch key {
	case "session":
		mergeRealtimeWorkflowMap(params, value, map[string]string{
			"model": "upstream_model",
		})
	case "input":
		return
	case "output":
		mergeRealtimeWorkflowAllowedMap(params, value, map[string]string{
			"speaker": "speaker",
			"voice":   "speaker",
		})
	default:
		params[key] = value
	}
}

func mergeRealtimeWorkflowMap(params map[string]any, value any, aliases map[string]string) {
	values, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, value := range values {
		if alias := aliases[key]; alias != "" {
			key = alias
		}
		params[key] = value
	}
}

func mergeRealtimeWorkflowAllowedMap(params map[string]any, value any, keys map[string]string) {
	values, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, value := range values {
		if target := keys[key]; target != "" {
			params[target] = value
		}
	}
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
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}
