package peerresource

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/google/jsonschema-go/jsonschema"
	"testing"
)

func TestProjectToolUsesAliasAndStableModelName(t *testing.T) {
	projected := projectCatalogTool(toolcatalog.Tool{Alias: "device.volume", FunctionName: "device_volume", Source: "mhs", Target: map[string]any{"id": "speaker.main"}, Schema: jsonschema.Schema{Type: "object"}})
	if projected.Name != "device.volume" || projected.InvokeName != "device_volume" || projected.Source != "mhs" || projected.Target["id"] != "speaker.main" {
		t.Fatalf("projection = %+v", projected)
	}
}
