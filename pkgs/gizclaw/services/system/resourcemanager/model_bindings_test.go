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
		"provider_data":{"upstream_model":"chat",
		"request_bindings":[{"target":{"kind":"http_header","name":"X-Device-Metadata"},"source":{"kind":"peer_runtime_kv","key":"identity","pointer":"/id"},"required":false}]}}
	}`)
	created, err := manager.Apply(t.Context(), resource)
	if err != nil || created.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create: %v, %v", created, err)
	}
	input, err := resource.AsModelResource()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := input.Spec.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := manager.Get(t.Context(), apitypes.ResourceKindModel, "bound")
	if err != nil {
		t.Fatal(err)
	}
	output, err := stored.AsModelResource()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := output.Spec.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Get/Export dropped the binding: %v", err)
	}
	unchanged, err := manager.Apply(t.Context(), stored)
	if err != nil || unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("round-trip Apply: %v, %v", unchanged, err)
	}
	data, err := input.Spec.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil {
		t.Fatal(err)
	}
	data.RequestBindings = nil
	if err := input.Spec.ProviderData.FromOpenAITenantModelProviderData(data); err != nil {
		t.Fatal(err)
	}
	if err := resource.FromModelResource(input); err != nil {
		t.Fatal(err)
	}
	updated, err := manager.Put(t.Context(), resource)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := updated.AsModelResource()
	if err != nil {
		t.Fatal(err)
	}
	data, err = cleared.Spec.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil || data.RequestBindings != nil {
		t.Fatalf("Put retained removed bindings: %v", err)
	}
}
