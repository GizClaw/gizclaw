package doubaorealtimeduplex

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "doubao-realtime-duplex"

// Factory projects persisted Workflow and Workspace options into the existing
// Doubao Realtime Duplex transformer selected by peergenx.
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
		return nil, fmt.Errorf("doubaorealtimeduplex: build transformer: %w", err)
	}
	return agenthost.NewTransformerAgent(transformer), nil
}

func (f Factory) serviceForWorkspace(ctx context.Context, spec agenthost.Spec) (*peergenx.Service, error) {
	if spec.Workspace.OwnerPublicKey == nil || strings.TrimSpace(*spec.Workspace.OwnerPublicKey) == "" {
		if f.GenX == nil {
			return nil, fmt.Errorf("doubaorealtimeduplex: GenX service is required")
		}
		return f.GenX, nil
	}
	if f.GenXForOwner == nil {
		return nil, fmt.Errorf("doubaorealtimeduplex: workspace %q owner GenX resolver is required", spec.Workspace.Name)
	}
	service, err := f.GenXForOwner(ctx, strings.TrimSpace(*spec.Workspace.OwnerPublicKey))
	if err != nil {
		return nil, fmt.Errorf("doubaorealtimeduplex: workspace %q owner runtime: %w", spec.Workspace.Name, err)
	}
	if service == nil {
		return nil, fmt.Errorf("doubaorealtimeduplex: workspace %q owner runtime returned no service", spec.Workspace.Name)
	}
	return service, nil
}

func resolvePattern(spec agenthost.Spec) (string, error) {
	public := spec.Workflow.Spec.DoubaoRealtimeDuplex
	if public == nil {
		return "", fmt.Errorf("doubaorealtimeduplex: workflow spec.doubao_realtime_duplex is required")
	}
	model := strings.TrimSpace(public.Model)
	values := make(url.Values)
	setString(values, "output_voice", public.Voice)
	setString(values, "instructions", public.Instructions)
	setEnum(values, "format", public.Format)
	setInt(values, "sample_rate", enumInt(public.SampleRate))
	setEnum(values, "input_format", public.InputFormat)
	setInt(values, "input_sample_rate", public.InputSampleRate)
	setInt(values, "input_channels", public.InputChannels)
	setBool(values, "input_transcode", public.InputTranscode)
	setInt(values, "output_speed", public.OutputSpeed)
	setInt(values, "output_loudness", public.OutputLoudness)

	if spec.Workspace.Parameters != nil {
		params, err := spec.Workspace.Parameters.AsDoubaoRealtimeDuplexWorkspaceParameters()
		if err != nil {
			return "", fmt.Errorf("doubaorealtimeduplex: decode workspace parameters: %w", err)
		}
		if params.Model != nil && strings.TrimSpace(*params.Model) != "" {
			model = strings.TrimSpace(*params.Model)
		}
		overrideString(values, "output_voice", params.Voice)
		overrideString(values, "instructions", params.Instructions)
		overrideEnum(values, "format", params.Format)
		overrideInt(values, "sample_rate", enumInt(params.SampleRate))
		overrideEnum(values, "input_format", params.InputFormat)
		overrideInt(values, "input_sample_rate", params.InputSampleRate)
		overrideInt(values, "input_channels", params.InputChannels)
		overrideBool(values, "input_transcode", params.InputTranscode)
		overrideInt(values, "output_speed", params.OutputSpeed)
		overrideInt(values, "output_loudness", params.OutputLoudness)
	}
	if model == "" {
		return "", fmt.Errorf("doubaorealtimeduplex: model is required")
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

func enumInt[T ~int](value *T) *int {
	if value == nil {
		return nil
	}
	result := int(*value)
	return &result
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
