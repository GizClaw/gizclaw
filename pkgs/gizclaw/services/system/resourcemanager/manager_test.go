package resourcemanager

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestManagerRejectsInvalidInputs(t *testing.T) {
	manager := New(Services{})

	_, err := manager.Get(context.Background(), apitypes.ResourceKind("Unknown"), "example")
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")
	_, err = manager.Get(context.Background(), apitypes.ResourceKindCredential, "")
	assertResourceError(t, err, 400, "INVALID_RESOURCE")
	_, err = manager.Get(context.Background(), apitypes.ResourceKindResourceList, "bundle")
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_GET")
	_, err = manager.Get(context.Background(), apitypes.ResourceKindCredential, "example")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")

	_, err = manager.Put(context.Background(), mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Unknown",
		"metadata":{"name":"example"},
		"spec":{}
	}`))
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKind("Unknown"), "example")
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")
	_, err = manager.Delete(context.Background(), apitypes.ResourceKindResourceList, "bundle")
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_DELETE")
}

func TestManagerRejectsNilReceiver(t *testing.T) {
	var manager *Manager
	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Credential",
		"metadata":{"name":"example"},
		"spec":{"provider":"minimax","body":{"api_key":"secret"}}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_MANAGER_NOT_CONFIGURED")
}

func assertResourceError(t *testing.T, err error, statusCode int, code string) {
	t.Helper()
	var resourceErr *Error
	if !errors.As(err, &resourceErr) {
		t.Fatalf("error = %v, want *Error", err)
	}
	if resourceErr.StatusCode != statusCode || resourceErr.Code != code {
		t.Fatalf("error = %+v, want status=%d code=%s", resourceErr, statusCode, code)
	}
}
