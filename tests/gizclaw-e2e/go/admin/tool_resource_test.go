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
	id := mutationName("tool-resource")
	t.Cleanup(func() {
		_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindTool, id)
	})

	name := "lookup_weather"
	executorName := "weather.lookup"
	var resource apitypes.Resource
	if err := resource.FromToolResource(apitypes.ToolResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ToolResourceKindTool,
		Metadata:   apitypes.ResourceMetadata{Name: id},
		Spec: apitypes.ToolSpec{
			Name:        &name,
			Source:      apitypes.ToolSourceAdmin,
			InputSchema: jsonschema.Schema{Type: "object", Required: []string{"city"}, Properties: map[string]*jsonschema.Schema{"city": {Type: "string"}}},
			Executor:    apitypes.ToolExecutor{Kind: apitypes.ToolExecutorKindBuiltin, Name: &executorName},
		},
	}); err != nil {
		t.Fatalf("build Tool resource: %v", err)
	}

	applied, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply Tool resource: %v", err)
	}
	requireStatusOK(t, applied, applied.Body)
	if applied.JSON200 == nil || applied.JSON200.Kind != apitypes.ResourceKindTool || applied.JSON200.Name != id {
		t.Fatalf("apply Tool resource = %#v", applied.JSON200)
	}

	got, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindTool, id)
	if err != nil {
		t.Fatalf("get Tool resource: %v", err)
	}
	requireStatusOK(t, got, got.Body)
	tool, err := got.JSON200.AsToolResource()
	if err != nil {
		t.Fatalf("decode Tool resource: %v", err)
	}
	if tool.Spec.Enabled == nil || !*tool.Spec.Enabled || tool.Spec.InputSchema.Properties["city"].Type != "string" {
		t.Fatalf("Tool resource round trip = %#v", tool)
	}
	description := "updated by admin e2e"
	tool.Spec.Description = &description
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
	if err != nil || updatedTool.Spec.Description == nil || *updatedTool.Spec.Description != description {
		t.Fatalf("put Tool resource = %#v, %v", updatedTool, err)
	}

	deleted, err := env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindTool, id)
	if err != nil {
		t.Fatalf("delete Tool resource: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
