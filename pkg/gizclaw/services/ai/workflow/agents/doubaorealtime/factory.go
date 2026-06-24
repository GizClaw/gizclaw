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

type realtimeWorkflowConfig struct {
	Model          string
	RealtimeModel  string
	Realtime       map[string]any
	RealtimeConfig map[string]any
}

func resolveRealtimeModelPattern(spec agenthost.Spec) (string, error) {
	if pattern := workflowRealtimeModelPattern(spec); pattern != "" {
		return normalizeModelPattern(pattern), nil
	}
	if spec.Workspace.Parameters != nil {
		typed, err := spec.Workspace.Parameters.AsDoubaoRealtimeWorkspaceParameters()
		if err != nil {
			return "", fmt.Errorf("doubaorealtime: decode workspace parameters: %w", err)
		}
		if typed.RealtimeModel != nil && strings.TrimSpace(*typed.RealtimeModel) != "" {
			params := mergeDoubaoRealtimeTypedParams(nil, typed)
			return normalizeModelPattern(appendPatternParams(*typed.RealtimeModel, params)), nil
		}
	}
	return "", fmt.Errorf("doubaorealtime: model is required")
}

func workflowRealtimeModelPattern(spec agenthost.Spec) string {
	workflowSpec := spec.Workflow.Spec.DoubaoRealtime
	if workflowSpec == nil {
		return ""
	}
	cfg := realtimeWorkflowConfig{
		Model:          stringPtrValue(workflowSpec.Model),
		RealtimeModel:  stringPtrValue(workflowSpec.RealtimeModel),
		RealtimeConfig: mapPtrValue(workflowSpec.RealtimeConfig),
		Realtime:       mapPtrValue(workflowSpec.Realtime),
	}
	pattern := firstNonEmpty(cfg.RealtimeModel, cfg.Model)
	if pattern == "" {
		return ""
	}
	params := realtimeWorkflowParams(cfg)
	if spec.Workspace.Parameters != nil {
		if typed, err := spec.Workspace.Parameters.AsDoubaoRealtimeWorkspaceParameters(); err == nil {
			params = mergeDoubaoRealtimeTypedParams(params, typed)
		}
	}
	return appendPatternParams(pattern, params)
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
	mergeRealtimeWorkflowParamsValue(params, cfg.RealtimeConfig)
	mergeRealtimeWorkflowParamsValue(params, cfg.Realtime)
	if len(params) == 0 {
		return nil
	}
	return params
}

func mapPtrValue(value *map[string]interface{}) map[string]any {
	if value == nil {
		return nil
	}
	return *value
}

func mergeDoubaoRealtimeTypedParams(params map[string]any, typed apitypes.DoubaoRealtimeWorkspaceParameters) map[string]any {
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
	if typed.Temperature != nil {
		params["temperature"] = *typed.Temperature
	}
	if typed.Session != nil {
		mergeRealtimeWorkspaceSession(params, *typed.Session)
	}
	if typed.Search != nil {
		mergeRealtimeWorkspaceSearch(params, *typed.Search)
	}
	if typed.Music != nil {
		mergeRealtimeWorkspaceMusic(params, *typed.Music)
	}
	if typed.Voice != nil {
		if internal, err := typed.Voice.AsDoubaoRealtimeInternalSpeakerParameters(); err == nil {
			if speaker := strings.TrimSpace(internal.RealtimeSpeakerId); speaker != "" {
				params["speaker"] = speaker
			}
		}
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func mergeRealtimeWorkspaceSession(params map[string]any, session apitypes.DoubaoRealtimeSessionParameters) {
	if value := stringPtrValue(session.BotName); value != "" {
		params["bot_name"] = value
	}
	if value := stringPtrValue(session.SystemRole); value != "" {
		params["system_role"] = value
	}
	if value := stringPtrValue(session.UpstreamModel); value != "" {
		params["upstream_model"] = value
	}
	if session.VadWindowMs != nil {
		params["vad_window_ms"] = *session.VadWindowMs
	}
	if value := stringPtrValue(session.SpeakingStyle); value != "" {
		params["speaking_style"] = value
	}
	if value := stringPtrValue(session.CharacterManifest); value != "" {
		params["character_manifest"] = value
	}
	if value := stringPtrValue(session.ResourceId); value != "" {
		params["resource_id"] = value
	}
}

func mergeRealtimeWorkspaceSearch(params map[string]any, search apitypes.DoubaoRealtimeSearchParameters) {
	if search.Enabled != nil {
		params["search_enabled"] = *search.Enabled
	}
	if value := stringPtrValue(search.Type); value != "" {
		params["search_type"] = value
	}
	if value := stringPtrValue(search.BotId); value != "" {
		params["search_bot_id"] = value
	}
	if search.ResultCount != nil {
		params["search_result_count"] = *search.ResultCount
	}
	if value := stringPtrValue(search.NoResultMessage); value != "" {
		params["search_no_result_message"] = value
	}
}

func mergeRealtimeWorkspaceMusic(params map[string]any, music apitypes.DoubaoRealtimeMusicParameters) {
	if music.Enabled != nil {
		params["music_enabled"] = *music.Enabled
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
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
	case float32:
		value := float64(typed)
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10), true
		}
		return strconv.FormatFloat(value, 'f', -1, 32), true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}
