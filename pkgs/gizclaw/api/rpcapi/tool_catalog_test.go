package rpcapi

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolCatalogPreservesResourceAndInvocationNames(t *testing.T) {
	t.Parallel()
	var payload RPCPayload
	response := ToolGetResponse{
		Value: Tool{
			Name:        "device_volume",
			InvokeName:  "client_volume_set",
			InputSchema: jsonschema.Schema{Type: "object"},
		},
	}
	if err := payload.FromToolGetResponse(response); err != nil {
		t.Fatalf("FromToolGetResponse() error = %v", err)
	}
	decoded, err := payload.AsToolGetResponse()
	if err != nil {
		t.Fatalf("AsToolGetResponse() error = %v", err)
	}
	if decoded.Value.Name != response.Value.Name || decoded.Value.InvokeName != response.Value.InvokeName {
		t.Fatalf("Tool identity = name %q, invoke_name %q", decoded.Value.Name, decoded.Value.InvokeName)
	}
}
