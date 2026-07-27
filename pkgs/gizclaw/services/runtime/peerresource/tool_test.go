package peerresource

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestProjectToolExposesCanonicalNameDistinctFromRuntimeAlias(t *testing.T) {
	t.Parallel()
	projected := projectTool(
		"device_volume",
		apitypes.RuntimeProfileBinding{},
		toolkit.Tool{
			Name:        "client_volume_set",
			Type:        toolkit.ToolTypeClientRPC,
			InputSchema: jsonschema.Schema{Type: "object"},
		},
	)
	if projected.Alias != "device_volume" || projected.Name != "client_volume_set" {
		t.Fatalf("projectTool() identity = alias %q, name %q", projected.Alias, projected.Name)
	}
}
