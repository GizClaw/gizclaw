package peerresource

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestProjectToolExposesPeerNameDistinctFromInvocationName(t *testing.T) {
	t.Parallel()
	projected := projectTool(
		"device_volume",
		apitypes.RuntimeProfileBinding{},
		toolkit.Tool{
			InvokeName:  "client_volume_set",
			Type:        toolkit.ToolTypeClientRPC,
			InputSchema: jsonschema.Schema{Type: "object"},
		},
	)
	if projected.Name != "device_volume" || projected.InvokeName != "client_volume_set" {
		t.Fatalf("projectTool() identity = name %q, invoke_name %q", projected.Name, projected.InvokeName)
	}
}
