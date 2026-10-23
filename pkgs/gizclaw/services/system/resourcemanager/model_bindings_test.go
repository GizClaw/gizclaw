package resourcemanager

import (
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestModelRequestBindingsResourceRoundTrip(t *testing.T) {
	manager := newModelManager(t)
	resource := mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"bound"},
		"spec":{"kind":"llm","source":"manual","provider":{"kind":"openai-tenant","id":"main"},
		"provider_data":{"upstream_model":"chat"},
		"request_bindings":[{"target":{"kind":"http_header","name":"X-Device-Metadata"},"source":{"kind":"reported_kv","key":"identity","pointer":"/id"},"required":false}]}
	}`)
	created, err := manager.Apply(t.Context(), resource)
	if err != nil || created.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create: %v, %v", created, err)
	}
	input, err := resource.AsModelResource()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := manager.Get(t.Context(), apitypes.ResourceKindModel, "bound")
	if err != nil {
		t.Fatal(err)
	}
	output, err := stored.AsModelResource()
	if err != nil || !reflect.DeepEqual(output.Spec.RequestBindings, input.Spec.RequestBindings) {
		t.Fatalf("Get/Export dropped the binding: %v", err)
	}
	unchanged, err := manager.Apply(t.Context(), stored)
	if err != nil || unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("round-trip Apply: %v, %v", unchanged, err)
	}
	input.Spec.RequestBindings = nil
	if err := resource.FromModelResource(input); err != nil {
		t.Fatal(err)
	}
	updated, err := manager.Put(t.Context(), resource)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := updated.AsModelResource()
	if err != nil || cleared.Spec.RequestBindings != nil {
		t.Fatalf("Put retained removed bindings: %v", err)
	}
}
