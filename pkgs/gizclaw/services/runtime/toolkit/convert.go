package toolkit

import (
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func FromAPI(tool apitypes.Tool) (Tool, error) {
	converted := Tool{
		ID:           tool.Id,
		Name:         tool.Name,
		Description:  tool.Description,
		Source:       ToolSource(tool.Source),
		Enabled:      tool.Enabled,
		OwnerPeer:    tool.OwnerPeer,
		Version:      tool.Version,
		InputSchema:  tool.InputSchema,
		OutputSchema: tool.OutputSchema,
		Executor: ToolExecutor{
			Kind:   ToolExecutorKind(tool.Executor.Kind),
			Name:   tool.Executor.Name,
			Method: tool.Executor.Method,
			PeerID: tool.Executor.PeerId,
		},
		CreatedAt: tool.CreatedAt,
		UpdatedAt: tool.UpdatedAt,
	}
	var err error
	if converted.Metadata, err = mapToRaw(tool.Metadata); err != nil {
		return Tool{}, err
	}
	if converted.Executor.Config, err = mapToRaw(tool.Executor.Config); err != nil {
		return Tool{}, err
	}
	if tool.Triggers != nil {
		if converted.Triggers, err = triggersFromAPI(*tool.Triggers); err != nil {
			return Tool{}, err
		}
	}
	return NormalizeTool(converted)
}

func ToAPI(tool Tool) (apitypes.Tool, error) {
	tool, err := NormalizeTool(tool)
	if err != nil {
		return apitypes.Tool{}, err
	}
	out := apitypes.Tool{
		Id:           tool.ID,
		Name:         tool.Name,
		Description:  tool.Description,
		Source:       apitypes.ToolSource(tool.Source),
		Enabled:      tool.Enabled,
		OwnerPeer:    tool.OwnerPeer,
		Version:      tool.Version,
		InputSchema:  tool.InputSchema,
		OutputSchema: tool.OutputSchema,
		Executor: apitypes.ToolExecutor{
			Kind:   apitypes.ToolExecutorKind(tool.Executor.Kind),
			Name:   tool.Executor.Name,
			Method: tool.Executor.Method,
			PeerId: tool.Executor.PeerID,
		},
		CreatedAt: tool.CreatedAt,
		UpdatedAt: tool.UpdatedAt,
	}
	if out.Metadata, err = rawToMap(tool.Metadata); err != nil {
		return apitypes.Tool{}, err
	}
	if out.Executor.Config, err = rawToMap(tool.Executor.Config); err != nil {
		return apitypes.Tool{}, err
	}
	if len(tool.Triggers) > 0 {
		triggers, err := triggersToAPI(tool.Triggers)
		if err != nil {
			return apitypes.Tool{}, err
		}
		out.Triggers = &triggers
	}
	return out, nil
}

func FromSpec(id string, spec apitypes.ToolSpec) (Tool, error) {
	enabled := true
	if spec.Enabled != nil {
		enabled = *spec.Enabled
	}
	return FromAPI(apitypes.Tool{
		Id:           id,
		Name:         spec.Name,
		Description:  spec.Description,
		Source:       spec.Source,
		Enabled:      enabled,
		OwnerPeer:    spec.OwnerPeer,
		Version:      spec.Version,
		InputSchema:  spec.InputSchema,
		OutputSchema: spec.OutputSchema,
		Triggers:     spec.Triggers,
		Executor:     spec.Executor,
		Metadata:     spec.Metadata,
	})
}

func ToSpec(tool Tool) (apitypes.ToolSpec, error) {
	value, err := ToAPI(tool)
	if err != nil {
		return apitypes.ToolSpec{}, err
	}
	enabled := value.Enabled
	return apitypes.ToolSpec{
		Name:         value.Name,
		Description:  value.Description,
		Source:       value.Source,
		Enabled:      &enabled,
		OwnerPeer:    value.OwnerPeer,
		Version:      value.Version,
		InputSchema:  value.InputSchema,
		OutputSchema: value.OutputSchema,
		Triggers:     value.Triggers,
		Executor:     value.Executor,
		Metadata:     value.Metadata,
	}, nil
}

func mapToRaw(in *map[string]interface{}) (json.RawMessage, error) {
	if in == nil {
		return nil, nil
	}
	return json.Marshal(*in)
}

func rawToMap(in json.RawMessage) (*map[string]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func triggersFromAPI(in []apitypes.ToolTrigger) ([]ToolTrigger, error) {
	out := make([]ToolTrigger, len(in))
	for i, trigger := range in {
		out[i] = ToolTrigger{Name: trigger.Name, Description: trigger.Description}
		var err error
		if out[i].Metadata, err = mapToRaw(trigger.Metadata); err != nil {
			return nil, err
		}
		if trigger.Patterns != nil {
			out[i].Patterns = append([]string(nil), (*trigger.Patterns)...)
		}
		if trigger.Examples != nil {
			out[i].Examples = make([]ToolTriggerExample, len(*trigger.Examples))
			for j, example := range *trigger.Examples {
				out[i].Examples[j] = ToolTriggerExample{Input: example.Input, Output: example.Output}
				if out[i].Examples[j].Args, err = mapToRaw(example.Args); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

func triggersToAPI(in []ToolTrigger) ([]apitypes.ToolTrigger, error) {
	out := make([]apitypes.ToolTrigger, len(in))
	for i, trigger := range in {
		out[i] = apitypes.ToolTrigger{Name: trigger.Name, Description: trigger.Description}
		if len(trigger.Patterns) > 0 {
			patterns := append([]string(nil), trigger.Patterns...)
			out[i].Patterns = &patterns
		}
		var err error
		if out[i].Metadata, err = rawToMap(trigger.Metadata); err != nil {
			return nil, err
		}
		if len(trigger.Examples) > 0 {
			examples := make([]apitypes.ToolTriggerExample, len(trigger.Examples))
			for j, example := range trigger.Examples {
				examples[j] = apitypes.ToolTriggerExample{Input: example.Input, Output: example.Output}
				if examples[j].Args, err = rawToMap(example.Args); err != nil {
					return nil, err
				}
			}
			out[i].Examples = &examples
		}
	}
	return out, nil
}

func triggersFromRPC(in []rpcapi.ToolTrigger) ([]ToolTrigger, error) {
	out := make([]ToolTrigger, len(in))
	for i, trigger := range in {
		out[i] = ToolTrigger{Name: trigger.Name, Description: trigger.Description}
		var err error
		if out[i].Metadata, err = mapToRaw(trigger.Metadata); err != nil {
			return nil, err
		}
		if trigger.Patterns != nil {
			out[i].Patterns = append([]string(nil), (*trigger.Patterns)...)
		}
		if trigger.Examples != nil {
			out[i].Examples = make([]ToolTriggerExample, len(*trigger.Examples))
			for j, example := range *trigger.Examples {
				out[i].Examples[j] = ToolTriggerExample{Input: example.Input, Output: example.Output}
				if out[i].Examples[j].Args, err = mapToRaw(example.Args); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

func triggersToRPC(in []ToolTrigger) ([]rpcapi.ToolTrigger, error) {
	out := make([]rpcapi.ToolTrigger, len(in))
	for i, trigger := range in {
		out[i] = rpcapi.ToolTrigger{Name: trigger.Name, Description: trigger.Description}
		if len(trigger.Patterns) > 0 {
			patterns := append([]string(nil), trigger.Patterns...)
			out[i].Patterns = &patterns
		}
		var err error
		if out[i].Metadata, err = rawToMap(trigger.Metadata); err != nil {
			return nil, err
		}
		if len(trigger.Examples) > 0 {
			examples := make([]rpcapi.ToolTriggerExample, len(trigger.Examples))
			for j, example := range trigger.Examples {
				examples[j] = rpcapi.ToolTriggerExample{Input: example.Input, Output: example.Output}
				if examples[j].Args, err = rawToMap(example.Args); err != nil {
					return nil, err
				}
			}
			out[i].Examples = &examples
		}
	}
	return out, nil
}
