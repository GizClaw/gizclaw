package dashscoperealtime

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "dashscope-realtime"

// Factory projects persisted Workflow and Workspace options into the existing
// DashScope Realtime transformer selected by peergenx.
type Factory struct {
	GenX         *peergenx.Service
	GenXForOwner func(context.Context, string) (*peergenx.Service, error)
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	service, err := f.serviceForWorkspace(ctx, spec)
	if err != nil {
		return nil, err
	}
	pattern, err := resolvePattern(spec)
	if err != nil {
		return nil, err
	}
	transformer, err := service.BuildTransformerWithToolInvoker(ctx, pattern, spec.ToolInvoker)
	if err != nil {
		return nil, fmt.Errorf("dashscoperealtime: build transformer: %w", err)
	}
	return agenthost.NewTransformerAgent(transformer), nil
}

func (f Factory) serviceForWorkspace(ctx context.Context, spec agenthost.Spec) (*peergenx.Service, error) {
	if spec.Workspace.OwnerPublicKey == nil || strings.TrimSpace(*spec.Workspace.OwnerPublicKey) == "" {
		if f.GenX == nil {
			return nil, fmt.Errorf("dashscoperealtime: GenX service is required")
		}
		return f.GenX, nil
	}
	if f.GenXForOwner == nil {
		return nil, fmt.Errorf("dashscoperealtime: workspace %q owner GenX resolver is required", spec.Workspace.Name)
	}
	service, err := f.GenXForOwner(ctx, strings.TrimSpace(*spec.Workspace.OwnerPublicKey))
	if err != nil {
		return nil, fmt.Errorf("dashscoperealtime: workspace %q owner runtime: %w", spec.Workspace.Name, err)
	}
	if service == nil {
		return nil, fmt.Errorf("dashscoperealtime: workspace %q owner runtime returned no service", spec.Workspace.Name)
	}
	return service, nil
}

func resolvePattern(spec agenthost.Spec) (string, error) {
	public := spec.Workflow.Spec.DashscopeRealtime
	if public == nil {
		return "", fmt.Errorf("dashscoperealtime: workflow spec.dashscope_realtime is required")
	}
	model := strings.TrimSpace(public.Model)
	values := make(url.Values)
	setString(values, "output_voice", public.Voice)
	setString(values, "instructions", public.Instructions)
	if public.Modalities != nil {
		modalities := make([]string, 0, len(*public.Modalities))
		for _, modality := range *public.Modalities {
			if value := strings.TrimSpace(string(modality)); value != "" {
				modalities = append(modalities, value)
			}
		}
		if len(modalities) != 0 {
			values.Set("modalities", strings.Join(modalities, ","))
		}
	}
	setEnum(values, "vad", public.Vad)
	setFloat(values, "temperature", public.Temperature)
	setInt(values, "max_output_tokens", public.MaxOutputTokens)
	setBool(values, "enable_asr", public.EnableAsr)
	setString(values, "asr_model", public.AsrModel)
	setEnum(values, "input_audio_format", public.InputAudioFormat)
	setEnum(values, "output_audio_format", public.OutputAudioFormat)

	if spec.Workspace.Parameters != nil {
		params, err := spec.Workspace.Parameters.AsDashScopeRealtimeWorkspaceParameters()
		if err != nil {
			return "", fmt.Errorf("dashscoperealtime: decode workspace parameters: %w", err)
		}
		if params.Model != nil && strings.TrimSpace(*params.Model) != "" {
			model = strings.TrimSpace(*params.Model)
		}
		overrideString(values, "output_voice", params.Voice)
		overrideString(values, "instructions", params.Instructions)
		if params.Modalities != nil {
			modalities := make([]string, 0, len(*params.Modalities))
			for _, modality := range *params.Modalities {
				if value := strings.TrimSpace(string(modality)); value != "" {
					modalities = append(modalities, value)
				}
			}
			values.Set("modalities", strings.Join(modalities, ","))
		}
		overrideEnum(values, "vad", params.Vad)
		overrideFloat(values, "temperature", params.Temperature)
		overrideInt(values, "max_output_tokens", params.MaxOutputTokens)
		overrideBool(values, "enable_asr", params.EnableAsr)
		overrideString(values, "asr_model", params.AsrModel)
		overrideEnum(values, "input_audio_format", params.InputAudioFormat)
		overrideEnum(values, "output_audio_format", params.OutputAudioFormat)
	}
	if model == "" {
		return "", fmt.Errorf("dashscoperealtime: model is required")
	}
	pattern := "model/" + model
	if query := values.Encode(); query != "" {
		pattern += "?" + query
	}
	return pattern, nil
}

func setString(values url.Values, key string, value *string) {
	if value != nil && strings.TrimSpace(*value) != "" {
		values.Set(key, strings.TrimSpace(*value))
	}
}

func overrideString(values url.Values, key string, value *string) {
	if value != nil {
		values.Set(key, strings.TrimSpace(*value))
	}
}

func setEnum[T ~string](values url.Values, key string, value *T) {
	if value != nil && strings.TrimSpace(string(*value)) != "" {
		values.Set(key, strings.TrimSpace(string(*value)))
	}
}

func overrideEnum[T ~string](values url.Values, key string, value *T) {
	if value != nil {
		values.Set(key, strings.TrimSpace(string(*value)))
	}
}

func setFloat(values url.Values, key string, value *float32) {
	if value != nil {
		values.Set(key, strconv.FormatFloat(float64(*value), 'g', -1, 32))
	}
}

func overrideFloat(values url.Values, key string, value *float32) {
	setFloat(values, key, value)
}

func setInt(values url.Values, key string, value *int) {
	if value != nil {
		values.Set(key, strconv.Itoa(*value))
	}
}

func overrideInt(values url.Values, key string, value *int) {
	setInt(values, key, value)
}

func setBool(values url.Values, key string, value *bool) {
	if value != nil {
		values.Set(key, strconv.FormatBool(*value))
	}
}

func overrideBool(values url.Values, key string, value *bool) {
	setBool(values, key, value)
}
