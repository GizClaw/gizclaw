package toolkit

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NormalizeTool(tool Tool) (Tool, error) {
	tool.ID = strings.TrimSpace(tool.ID)
	if tool.ID == "" {
		return Tool{}, fmt.Errorf("%w: id is required", ErrInvalidTool)
	}
	if strings.Contains(tool.ID, ":") {
		return Tool{}, fmt.Errorf("%w: id must not contain ':'", ErrInvalidTool)
	}
	switch tool.Source {
	case ToolSourceBuiltin, ToolSourceDevice, ToolSourceAdmin:
	default:
		return Tool{}, fmt.Errorf("%w: unsupported source %q", ErrInvalidTool, tool.Source)
	}
	if len(tool.InputSchema) == 0 {
		return Tool{}, fmt.Errorf("%w: input_schema is required", ErrInvalidTool)
	}
	if !json.Valid(tool.InputSchema) {
		return Tool{}, fmt.Errorf("%w: input_schema must be valid JSON", ErrInvalidTool)
	}
	if len(tool.OutputSchema) > 0 && !json.Valid(tool.OutputSchema) {
		return Tool{}, fmt.Errorf("%w: output_schema must be valid JSON", ErrInvalidTool)
	}
	if len(tool.Metadata) > 0 && !json.Valid(tool.Metadata) {
		return Tool{}, fmt.Errorf("%w: metadata must be valid JSON", ErrInvalidTool)
	}
	if err := validateExecutor(tool); err != nil {
		return Tool{}, err
	}
	if err := validateTriggers(tool.Triggers); err != nil {
		return Tool{}, err
	}
	return cloneTool(tool), nil
}

func validateExecutor(tool Tool) error {
	switch tool.Executor.Kind {
	case ToolExecutorKindBuiltin:
		if trimPtr(tool.Executor.Name) == "" {
			return fmt.Errorf("%w: builtin executor name is required", ErrInvalidTool)
		}
	case ToolExecutorKindDeviceRPC:
		if trimPtr(tool.Executor.Method) == "" {
			return fmt.Errorf("%w: device_rpc executor method is required", ErrInvalidTool)
		}
		if trimPtr(tool.OwnerPeer) == "" && trimPtr(tool.Executor.PeerID) == "" {
			return fmt.Errorf("%w: device_rpc executor owner_peer or peer_id is required", ErrInvalidTool)
		}
	default:
		return fmt.Errorf("%w: unsupported executor kind %q", ErrInvalidTool, tool.Executor.Kind)
	}
	return nil
}

func validateTriggers(triggers []ToolTrigger) error {
	for i, trigger := range triggers {
		if strings.TrimSpace(trigger.Name) == "" {
			return fmt.Errorf("%w: triggers[%d].name is required", ErrInvalidTool, i)
		}
		if len(trigger.Metadata) > 0 && !json.Valid(trigger.Metadata) {
			return fmt.Errorf("%w: triggers[%d].metadata must be valid JSON", ErrInvalidTool, i)
		}
		for j, example := range trigger.Examples {
			if strings.TrimSpace(example.Input) == "" {
				return fmt.Errorf("%w: triggers[%d].examples[%d].input is required", ErrInvalidTool, i, j)
			}
			if len(example.Args) > 0 && !json.Valid(example.Args) {
				return fmt.Errorf("%w: triggers[%d].examples[%d].args must be valid JSON", ErrInvalidTool, i, j)
			}
		}
	}
	return nil
}

func trimPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
