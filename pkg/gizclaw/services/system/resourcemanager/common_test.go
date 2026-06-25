package resourcemanager

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestSemanticEqualNormalizesJSONValues(t *testing.T) {
	left := map[string]interface{}{
		"nested": map[string]interface{}{
			"enabled": true,
		},
		"items": []string{"a", "b"},
	}
	right := struct {
		Items  []string               `json:"items"`
		Nested map[string]interface{} `json:"nested"`
	}{
		Items: []string{"a", "b"},
		Nested: map[string]interface{}{
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
	if err := validateResourceHeader(apitypes.ResourceAPIVersion("unsupported"), "name"); err == nil {
		t.Fatal("validateResourceHeader unsupported version error = nil, want error")
	}
	if err := validateResourceHeader(apitypes.ResourceAPIVersionGizclawAdminv1alpha1, ""); err == nil {
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
	if _, err := semanticEqual(func() {}, map[string]interface{}{}); err == nil {
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

func mustWorkflowDocument(t *testing.T, raw string) apitypes.WorkflowDocument {
	t.Helper()
	var doc apitypes.WorkflowDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("unmarshal workflow document: %v", err)
	}
	return doc
}

func ptr[T any](value T) *T {
	return &value
}

func mustUnescapePathParam(value string) string {
	unescaped, err := url.PathUnescape(value)
	if err != nil {
		panic(err)
	}
	return unescaped
}
