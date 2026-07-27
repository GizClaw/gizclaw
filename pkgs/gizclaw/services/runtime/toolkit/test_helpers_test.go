package toolkit

import (
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

func testClientTool(name string) Tool {
	return Tool{
		Name:        name,
		Type:        ToolTypeClientRPC,
		Description: new("test client tool"),
		Enabled:     true,
		InputSchema: jsonschema.Schema{
			Type:                 "object",
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
	}
}

func testHTTPTool(name string) Tool {
	return Tool{
		Name:        name,
		Type:        ToolTypeHTTPRequest,
		Description: new("test HTTP tool"),
		Enabled:     true,
		InputSchema: jsonschema.Schema{Type: "object"},
		HTTP: &HTTPRequest{
			URL:              "https://example.com/weather",
			Method:           "GET",
			Auth:             HTTPAuth{Method: "none"},
			Timeout:          5 * time.Second,
			MaxResponseBytes: 4096,
		},
	}
}
