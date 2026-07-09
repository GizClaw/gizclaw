package toolkit

import "encoding/json"

func testBuiltinTool(id string) Tool {
	return Tool{
		ID:          id,
		Name:        stringPtr(id),
		Description: stringPtr("test tool"),
		Source:      ToolSourceBuiltin,
		Enabled:     true,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Executor: ToolExecutor{
			Kind: ToolExecutorKindBuiltin,
			Name: stringPtr("music.play"),
		},
	}
}

func testDeviceTool(id, peer string) Tool {
	return Tool{
		ID:          id,
		Name:        stringPtr(id),
		Description: stringPtr("device tool"),
		Source:      ToolSourceDevice,
		Enabled:     true,
		OwnerPeer:   stringPtr(peer),
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Executor: ToolExecutor{
			Kind:   ToolExecutorKindDeviceRPC,
			Method: stringPtr("music.play"),
		},
	}
}

func stringPtr(value string) *string {
	return &value
}
