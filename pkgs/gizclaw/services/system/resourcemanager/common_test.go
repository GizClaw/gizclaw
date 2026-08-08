package resourcemanager

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func metadataID(t *testing.T, metadata apitypes.ResourceMetadata) string {
	t.Helper()
	if metadata.Id == "" {
		t.Fatal("metadata.id is nil")
	}
	return metadata.Id
}

func TestSemanticEqualNormalizesJSONValues(t *testing.T) {
	left := map[string]any{
		"nested": map[string]any{
			"enabled": true,
		},
		"items": []string{"a", "b"},
	}
	right := struct {
		Items  []string       `json:"items"`
		Nested map[string]any `json:"nested"`
	}{
		Items: []string{"a", "b"},
		Nested: map[string]any{
			"enabled": true,
		},
	}

	same, err := semanticEqual(left, right)
	if err != nil {
		t.Fatalf("semanticEqual returned error: %v", err)
	}
	if !same {
		t.Fatal("semanticEqual = false, want true")
	}
}

func TestResponseErrorExtractsErrorResponse(t *testing.T) {
	err := responseError(409, "FALLBACK", "fallback message", apitypes.ErrorResponse{
		Error: apitypes.ErrorPayload{
			Code:    "RESOURCE_CONFLICT",
			Message: "resource already exists",
		},
	})

	if err.StatusCode != 409 {
		t.Fatalf("StatusCode = %d, want 409", err.StatusCode)
	}
	if err.Code != "RESOURCE_CONFLICT" {
		t.Fatalf("Code = %q, want RESOURCE_CONFLICT", err.Code)
	}
	if err.Message != "resource already exists" {
		t.Fatalf("Message = %q, want resource already exists", err.Message)
	}
}

func TestCommonResourceErrors(t *testing.T) {
	if err := validateResourceHeader(apitypes.ResourceAPIVersion("unsupported"), apitypes.ResourceMetadata{Id: "name"}); err == nil {
		t.Fatal("validateResourceHeader unsupported version error = nil, want error")
	}
	if err := validateResourceHeader(apitypes.ResourceAPIVersionGizclawAdminv1alpha1, apitypes.ResourceMetadata{}); err == nil {
		t.Fatal("validateResourceHeader empty name error = nil, want error")
	}
	assertResourceError(t, missingService("credentials"), 500, "RESOURCE_SERVICE_NOT_CONFIGURED")
	assertResourceError(t, notFound(apitypes.ResourceKindCredential, "missing"), 404, "RESOURCE_NOT_FOUND")
	assertResourceError(t, unexpectedResponse("Operation", struct{}{}), 500, "UNEXPECTED_SERVICE_RESPONSE")
	manager := New(Services{})
	_, err := manager.Get(context.Background(), apitypes.ResourceKindResourceList, "bundle")
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_GET")
	_, err = manager.Delete(context.Background(), apitypes.ResourceKindResourceList, "bundle")
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_DELETE")
	_, err = manager.Get(context.Background(), apitypes.ResourceKind("Unknown"), "demo")
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")
	_, err = manager.Delete(context.Background(), apitypes.ResourceKind("Unknown"), "demo")
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")

	err = applyError(400, "INVALID", "invalid resource")
	if err.Error() != "INVALID: invalid resource" {
		t.Fatalf("Error() = %q, want INVALID: invalid resource", err.Error())
	}
	var nilErr *Error
	if nilErr.Error() != "" {
		t.Fatalf("nil Error() = %q, want empty string", nilErr.Error())
	}
	if _, err := marshalResource(func() {}); err == nil {
		t.Fatal("marshalResource unsupported input error = nil, want error")
	}
	if _, err := semanticEqual(func() {}, map[string]any{}); err == nil {
		t.Fatal("semanticEqual unsupported input error = nil, want error")
	}
	fallback := responseError(500, "FALLBACK", "fallback message", struct{}{})
	if fallback.Code != "FALLBACK" {
		t.Fatalf("fallback code = %q, want FALLBACK", fallback.Code)
	}
}

func mustResource(t *testing.T, raw string) apitypes.Resource {
	t.Helper()
	var resource apitypes.Resource
	if err := json.Unmarshal([]byte(raw), &resource); err != nil {
		t.Fatalf("unmarshal resource: %v", err)
	}
	return resource
}

func mustWorkflow(t *testing.T, raw string) apitypes.Workflow {
	t.Helper()
	var doc apitypes.Workflow
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("unmarshal workflow document: %v", err)
	}
	return doc
}

func withResourceID(t *testing.T, resource apitypes.Resource, id string) apitypes.Resource {
	t.Helper()
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("marshal resource: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode resource: %v", err)
	}
	metadata, ok := document["metadata"].(map[string]any)
	if !ok {
		t.Fatal("resource metadata is missing")
	}
	metadata["id"] = id
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal resource with id: %v", err)
	}
	var result apitypes.Resource
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode resource with id: %v", err)
	}
	return result
}

//go:fix inline
func ptr[T any](value T) *T {
	return new(value)
}
