package adminresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestResponseErrorPrefersStructuredError(t *testing.T) {
	resp := apitypes.NewErrorResponse("NOPE", "not implemented")
	err := responseError(501, nil, &resp)
	if err == nil || !strings.Contains(err.Error(), "NOPE: not implemented") {
		t.Fatalf("responseError() = %v", err)
	}
}

func TestClientApplyAndGet(t *testing.T) {
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`)
	api := &fakeAdminResourceAPI{
		applyResp: &adminhttp.ApplyResourceResponse{
			JSON200: &apitypes.ApplyResult{
				Action:     apitypes.ApplyActionUpdated,
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.ResourceKindCredential,
				Id:         new("minimax-main"),
			},
		},
		deleteResp: &adminhttp.DeleteResourceResponse{
			JSON200: &resource,
		},
		getResp: &adminhttp.GetResourceResponse{
			JSON200: &resource,
		},
	}
	closed := false
	bridge := NewClient(api, func() error {
		closed = true
		return nil
	})

	result, err := bridge.ApplyResource(context.Background(), resource)
	if err != nil {
		t.Fatalf("ApplyResource error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated || result.Id == nil || *result.Id != "minimax-main" {
		t.Fatalf("ApplyResource result = %+v", result)
	}
	got, err := bridge.GetResource(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err != nil {
		t.Fatalf("GetResource error: %v", err)
	}
	if kind, name, err := resourceKindAndName(got); err != nil || kind != apitypes.ResourceKindCredential || name != "minimax-main" {
		t.Fatalf("GetResource = %s/%s, %v", kind, name, err)
	}
	deleted, err := bridge.DeleteResource(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err != nil {
		t.Fatalf("DeleteResource error: %v", err)
	}
	if kind, name, err := resourceKindAndName(deleted); err != nil || kind != apitypes.ResourceKindCredential || name != "minimax-main" {
		t.Fatalf("DeleteResource = %s/%s, %v", kind, name, err)
	}
	if err := bridge.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !closed {
		t.Fatal("Close did not call close hook")
	}
}

func TestClientStructuredErrors(t *testing.T) {
	errResp := apitypes.NewErrorResponse("APPLY_NOT_IMPLEMENTED", "admin apply is not implemented yet")
	bridge := NewClient(&fakeAdminResourceAPI{
		applyResp:  &adminhttp.ApplyResourceResponse{JSON501: &errResp},
		deleteResp: &adminhttp.DeleteResourceResponse{JSON500: &errResp},
		getResp:    &adminhttp.GetResourceResponse{JSON501: &errResp},
	}, nil)

	_, err := bridge.ApplyResource(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`))
	if err == nil || !strings.Contains(err.Error(), "APPLY_NOT_IMPLEMENTED") {
		t.Fatalf("ApplyResource error = %v", err)
	}
	_, err = bridge.GetResource(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err == nil || !strings.Contains(err.Error(), "APPLY_NOT_IMPLEMENTED") {
		t.Fatalf("GetResource error = %v", err)
	}
	_, err = bridge.DeleteResource(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err == nil || !strings.Contains(err.Error(), "APPLY_NOT_IMPLEMENTED") {
		t.Fatalf("DeleteResource error = %v", err)
	}
}

func TestClientPropagatesAPIErrors(t *testing.T) {
	want := errors.New("api failed")
	bridge := NewClient(&fakeAdminResourceAPI{
		applyErr:  want,
		deleteErr: want,
		getErr:    want,
	}, nil)
	_, err := bridge.ApplyResource(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`))
	if !errors.Is(err, want) {
		t.Fatalf("ApplyResource error = %v, want %v", err, want)
	}
	_, err = bridge.GetResource(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if !errors.Is(err, want) {
		t.Fatalf("GetResource error = %v, want %v", err, want)
	}
	_, err = bridge.DeleteResource(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if !errors.Is(err, want) {
		t.Fatalf("DeleteResource error = %v, want %v", err, want)
	}
	if err := NewClient(nil, nil).Close(); err != nil {
		t.Fatalf("nil Close error = %v", err)
	}
}

func TestResponseErrorFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{
			name: "body",
			err:  responseError(500, []byte("plain failure")),
			want: "unexpected status 500: plain failure",
		},
		{
			name: "status",
			err:  responseError(404, nil),
			want: "unexpected status 404",
		},
		{
			name: "empty",
			err:  responseError(0, nil),
			want: "unexpected empty response",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil || tc.err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", tc.err, tc.want)
			}
		})
	}
}

type fakeAdminResourceAPI struct {
	applyResp  *adminhttp.ApplyResourceResponse
	applyErr   error
	deleteResp *adminhttp.DeleteResourceResponse
	deleteErr  error
	getResp    *adminhttp.GetResourceResponse
	getErr     error
}

func (f *fakeAdminResourceAPI) ApplyResourceWithResponse(context.Context, adminhttp.ApplyResourceJSONRequestBody, ...adminhttp.RequestEditorFn) (*adminhttp.ApplyResourceResponse, error) {
	return f.applyResp, f.applyErr
}

func (f *fakeAdminResourceAPI) DeleteResourceWithResponse(context.Context, adminhttp.ResourceKind, string, ...adminhttp.RequestEditorFn) (*adminhttp.DeleteResourceResponse, error) {
	return f.deleteResp, f.deleteErr
}

func (f *fakeAdminResourceAPI) GetResourceWithResponse(context.Context, adminhttp.ResourceKind, string, ...adminhttp.RequestEditorFn) (*adminhttp.GetResourceResponse, error) {
	return f.getResp, f.getErr
}

func mustResource(t *testing.T, raw string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := json.Unmarshal([]byte(raw), &resource); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return resource
}

func resourceKindAndName(resource apitypes.Resource) (apitypes.ResourceKind, string, error) {
	var header struct {
		Kind     apitypes.ResourceKind `json:"kind"`
		Metadata struct {
			ID string `json:"id"`
		} `json:"metadata"`
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return "", "", err
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return "", "", err
	}
	return header.Kind, header.Metadata.ID, nil
}

func TestIsNotFound(t *testing.T) {
	notFound := apitypes.NewErrorResponse("RESOURCE_NOT_FOUND", "missing")
	if !IsNotFound(fmt.Errorf("wrapped: %w", responseError(404, nil, &notFound))) {
		t.Fatal("structured RESOURCE_NOT_FOUND was not recognized")
	}
	for _, code := range []string{"RESOURCE_NOT_FOUND_EXTRA", "resource_not_found", "NOT_FOUND", "INVALID_RESOURCE"} {
		resp := apitypes.NewErrorResponse(code, "x")
		if IsNotFound(responseError(404, nil, &resp)) {
			t.Fatalf("code %q was treated as not found", code)
		}
	}
	for _, err := range []error{
		responseError(404, []byte("route not found")),
		errors.New("RESOURCE_NOT_FOUND: missing"),
		nil,
	} {
		if IsNotFound(err) {
			t.Fatalf("%v was treated as not found", err)
		}
	}
}
