//go:build gizclaw_e2e

package admin_test

import (
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestAdminAPIToolResourceLifecycle(t *testing.T) {
	env := newAdminAPIHarness(t)
	name := mutationName("tool-resource")
	var id string
	t.Cleanup(func() {
		if id != "" {
			_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindTool, id)
		}
	})

	auth := apitypes.ToolHTTPAuth{}
	if err := auth.FromToolHTTPAuthNone(apitypes.ToolHTTPAuthNone{
		Method: apitypes.ToolHTTPAuthNoneMethodNone,
	}); err != nil {
		t.Fatalf("build Tool auth: %v", err)
	}
	spec := apitypes.ToolSpec{}
	if err := spec.FromHTTPToolSpec(apitypes.HTTPToolSpec{
		Type:        apitypes.HTTPToolSpecTypeHttpRequest,
		InvokeName:  name,
		InputSchema: jsonschema.Schema{Type: "object", Required: []string{"city"}, Properties: map[string]*jsonschema.Schema{"city": {Type: "string"}}},
		Http: apitypes.ToolHTTPRequest{
			Url: "https://weather.example/v1", Method: apitypes.ToolHTTPMethodGET,
			Auth: auth, Timeout: "5s", MaxResponseBytes: 4096,
		},
	}); err != nil {
		t.Fatalf("build Tool spec: %v", err)
	}
	var resource apitypes.Resource
	if err := resource.FromToolResource(apitypes.ToolResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ToolResourceKindTool,
		Metadata:   apitypes.ResourceMetadata{Id: name},
		Spec:       spec,
	}); err != nil {
		t.Fatalf("build Tool resource: %v", err)
	}

	applied, err := env.api.ApplyResourceWithResponse(env.ctx, writableResource(t, resource))
	if err != nil {
		t.Fatalf("apply Tool resource: %v", err)
	}
	requireStatusOK(t, applied, applied.Body)
	if applied.JSON200 == nil || applied.JSON200.Id == nil || *applied.JSON200.Id != name || applied.JSON200.Kind != apitypes.ResourceKindTool {
		t.Fatalf("apply Tool resource = %#v", applied.JSON200)
	}
	id = *applied.JSON200.Id

	got, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindTool, id)
	if err != nil {
		t.Fatalf("get Tool resource: %v", err)
	}
	requireStatusOK(t, got, got.Body)
	tool, err := got.JSON200.AsToolResource()
	if err != nil {
		t.Fatalf("decode Tool resource: %v", err)
	}
	httpSpec, err := tool.Spec.AsHTTPToolSpec()
	if err != nil {
		t.Fatalf("decode HTTP Tool spec: %v", err)
	}
	if httpSpec.Enabled == nil || !*httpSpec.Enabled || httpSpec.InputSchema.Properties["city"].Type != "string" {
		t.Fatalf("Tool resource round trip = %#v", tool)
	}
	description := "updated by admin e2e"
	httpSpec.Description = &description
	if err := tool.Spec.FromHTTPToolSpec(httpSpec); err != nil {
		t.Fatalf("update HTTP Tool spec: %v", err)
	}
	tool.Kind = apitypes.ToolResourceKindTool
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("encode updated Tool resource: %v", err)
	}
	if err := json.Unmarshal(data, &resource); err != nil {
		t.Fatalf("build updated Tool resource: %v", err)
	}
	updated, err := env.api.PutResourceWithResponse(env.ctx, apitypes.ResourceKindTool, id, resource)
	if err != nil {
		t.Fatalf("put Tool resource: %v", err)
	}
	requireStatusOK(t, updated, updated.Body)
	updatedTool, err := updated.JSON200.AsToolResource()
	if err != nil {
		t.Fatalf("decode updated Tool resource: %v", err)
	}
	updatedSpec, err := updatedTool.Spec.AsHTTPToolSpec()
	if err != nil || updatedSpec.Description == nil || *updatedSpec.Description != description {
		t.Fatalf("put Tool resource = %#v, %v", updatedTool, err)
	}

	deleted, err := env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindTool, id)
	if err != nil {
		t.Fatalf("delete Tool resource: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
