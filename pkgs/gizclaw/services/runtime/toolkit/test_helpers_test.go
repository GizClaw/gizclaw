package toolkit

import "github.com/google/jsonschema-go/jsonschema"

func testBuiltinTool(id string) Tool {
	return Tool{
		ID:          id,
		Name:        stringPtr(id),
		Description: stringPtr("test tool"),
		Source:      ToolSourceBuiltin,
		Enabled:     true,
		InputSchema: jsonschema.Schema{Type: "object"},
		Executor: ToolExecutor{
			Kind: ToolExecutorKindBuiltin,
			Name: stringPtr("music.play"),
		},
	}
}

func testDeviceTool(id, peer string) Tool {
	return Tool{
		ID:             id,
		Name:           stringPtr(id),
		Description:    stringPtr("device tool"),
		Source:         ToolSourceDevice,
		Enabled:        true,
		OwnerPeer:      stringPtr(peer),
		OwnerPublicKey: stringPtr(peer),
		InputSchema:    jsonschema.Schema{Type: "object"},
		Executor: ToolExecutor{
			Kind:   ToolExecutorKindDeviceRPC,
			Method: stringPtr("music.play"),
		},
	}
}

func stringPtr(value string) *string {
	return &value
}
