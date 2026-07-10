package toolkit

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NormalizeTool(tool Tool) (Tool, error) {
	var err error
	tool.ID, err = normalizeToolID(tool.ID)
	if err != nil {
		return Tool{}, err
	}
	tool.Name = normalizedStringPtr(tool.Name)
	tool.Description = normalizedStringPtr(tool.Description)
	tool.OwnerPeer = normalizedStringPtr(tool.OwnerPeer)
	tool.Version = normalizedStringPtr(tool.Version)
	tool.Executor.Name = normalizedStringPtr(tool.Executor.Name)
	tool.Executor.Method = normalizedStringPtr(tool.Executor.Method)
	tool.Executor.PeerID = normalizedStringPtr(tool.Executor.PeerID)
	switch tool.Source {
	case ToolSourceBuiltin, ToolSourceDevice, ToolSourceAdmin:
	default:
		return Tool{}, fmt.Errorf("%w: unsupported source %q", ErrInvalidTool, tool.Source)
	}
	if err := validateInputSchema(tool.InputSchema.Type, tool.InputSchema.Types); err != nil {
		return Tool{}, err
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

func normalizeToolID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("%w: id is required", ErrInvalidTool)
	}
	if strings.Contains(id, ":") {
		return "", fmt.Errorf("%w: id must not contain ':'", ErrInvalidTool)
	}
	return id, nil
}

func validateInputSchema(single string, many []string) error {
	if single == "object" {
		return nil
	}
	for _, typ := range many {
		if typ == "object" {
			return nil
		}
	}
	if single == "" && len(many) == 0 {
		return fmt.Errorf("%w: input_schema type is required and must be object", ErrInvalidTool)
	}
	if single != "object" {
		return fmt.Errorf("%w: input_schema type must be object", ErrInvalidTool)
	}
	return nil
}

func validateToolArgs(_ Tool, args json.RawMessage) error {
	args = normalizeToolArgs(args)
	var values map[string]json.RawMessage
	if err := json.Unmarshal(args, &values); err != nil || values == nil {
		return fmt.Errorf("%w: tool arguments must be a JSON object", ErrInvalidTool)
	}
	return nil
}

func normalizeToolArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return json.RawMessage(`{}`)
	}
	return args
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
		if owner, peerID := trimPtr(tool.OwnerPeer), trimPtr(tool.Executor.PeerID); owner != "" && peerID != "" && owner != peerID {
			return fmt.Errorf("%w: owner_peer and executor.peer_id conflict", ErrInvalidTool)
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

func normalizedStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}
