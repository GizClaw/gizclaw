package rpcapi

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolCatalogPreservesAliasAndCanonicalName(t *testing.T) {
	t.Parallel()
	var payload RPCPayload
	response := ToolGetResponse{
		Value: Tool{
			Alias:       "device_volume",
			Name:        "client_volume_set",
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
	if decoded.Value.Alias != response.Value.Alias || decoded.Value.Name != response.Value.Name {
		t.Fatalf("Tool identity = alias %q, name %q", decoded.Value.Alias, decoded.Value.Name)
	}
}
