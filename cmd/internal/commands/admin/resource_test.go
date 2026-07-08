package admincmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminResourceCLIUserStoryApplyThenShow(t *testing.T) {
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`)
	resourceFile := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(resourceFile, mustJSON(t, resource), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}

	fake := &fakeResourceClient{
		applyResult: apitypes.ApplyResult{
			Action:     apitypes.ApplyActionCreated,
			ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
			Kind:       apitypes.ResourceKindCredential,
			Name:       "minimax-main",
		},
		getResource: resource,
	}
	restore := stubResourceClient(fake)
	defer restore()

	applyCmd := NewCmd()
	var applyOut bytes.Buffer
	applyCmd.SetOut(&applyOut)
	applyCmd.SetArgs([]string{"apply", "-f", resourceFile})
	if err := applyCmd.Execute(); err != nil {
		t.Fatalf("admin apply error: %v", err)
	}
	if fake.appliedKind != apitypes.ResourceKindCredential || fake.appliedName != "minimax-main" {
		t.Fatalf("applied resource = %s/%s", fake.appliedKind, fake.appliedName)
	}
	var applyResult apitypes.ApplyResult
	if err := json.Unmarshal(applyOut.Bytes(), &applyResult); err != nil {
		t.Fatalf("decode apply output: %v", err)
	}
	if applyResult.Action != apitypes.ApplyActionCreated || applyResult.Name != "minimax-main" {
		t.Fatalf("apply output = %+v", applyResult)
	}

	showCmd := NewCmd()
	var showOut bytes.Buffer
	showCmd.SetOut(&showOut)
	showCmd.SetArgs([]string{"show", "Credential", "minimax-main"})
	if err := showCmd.Execute(); err != nil {
		t.Fatalf("admin show error: %v", err)
	}
	if fake.gotKind != apitypes.ResourceKindCredential || fake.gotName != "minimax-main" {
		t.Fatalf("got resource = %s/%s", fake.gotKind, fake.gotName)
	}
	var shown map[string]interface{}
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("decode show output: %v", err)
	}
	if shown["kind"] != "Credential" {
		t.Fatalf("show output kind = %v", shown["kind"])
	}

	deleteCmd := NewCmd()
	var deleteOut bytes.Buffer
	deleteCmd.SetOut(&deleteOut)
	deleteCmd.SetArgs([]string{"delete", "Credential", "minimax-main"})
	if err := deleteCmd.Execute(); err != nil {
		t.Fatalf("admin delete error: %v", err)
	}
	if fake.deletedKind != apitypes.ResourceKindCredential || fake.deletedName != "minimax-main" {
		t.Fatalf("deleted resource = %s/%s", fake.deletedKind, fake.deletedName)
	}
	var deleted map[string]interface{}
	if err := json.Unmarshal(deleteOut.Bytes(), &deleted); err != nil {
		t.Fatalf("decode delete output: %v", err)
	}
	if deleted["kind"] != "Credential" {
		t.Fatalf("delete output kind = %v", deleted["kind"])
	}
}

func TestAdminResourceShowRejectsResourceList(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"show", "ResourceList", "bundle"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("admin show ResourceList should fail")
	}
}

func TestAdminResourceShowRejectsUnknownKind(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"show", "UnknownKind", "demo"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("admin show unknown kind should fail")
	}
}

func TestAdminResourceApplyReadsStdin(t *testing.T) {
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Workspace",
		"metadata": {"name": "demo-workspace"},
		"spec": {
			"workflow_name": "demo-workflow",
			"parameters": {"mode": "test"}
		}
	}`)
	fake := &fakeResourceClient{
		applyResult: apitypes.ApplyResult{
			Action:     apitypes.ApplyActionUnchanged,
			ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
			Kind:       apitypes.ResourceKindWorkspace,
			Name:       "demo-workspace",
		},
	}
	restore := stubResourceClient(fake)
	defer restore()

	cmd := NewCmd()
	cmd.SetIn(bytes.NewReader(mustJSON(t, resource)))
	cmd.SetArgs([]string{"apply", "-f", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin apply stdin error: %v", err)
	}
	if fake.appliedKind != apitypes.ResourceKindWorkspace || fake.appliedName != "demo-workspace" {
		t.Fatalf("applied resource = %s/%s", fake.appliedKind, fake.appliedName)
	}
}

func TestAdminResourceApplyExpandsProcessEnv(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_CREDENTIAL", "env-credential")
	t.Setenv("GIZCLAW_TEST_SECRET", "env-\"secret\"\\line\nnext")
	resourceFile := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(resourceFile, []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "${GIZCLAW_TEST_CREDENTIAL:-fallback-credential}"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "${GIZCLAW_TEST_SECRET}"}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}
	fake := &fakeResourceClient{applyResult: apitypes.ApplyResult{Name: "env-credential"}}
	restore := stubResourceClient(fake)
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"apply", "-f", resourceFile})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin apply error: %v", err)
	}
	if fake.appliedName != "env-credential" {
		t.Fatalf("applied name = %q", fake.appliedName)
	}
	data := string(mustJSON(t, fake.appliedResource))
	if !strings.Contains(data, `"api_key":"env-\"secret\"\\line\nnext"`) {
		t.Fatalf("applied resource did not contain expanded secret: %s", data)
	}
	credential, err := fake.appliedResource.AsCredentialResource()
	if err != nil {
		t.Fatalf("applied resource is not credential: %v", err)
	}
	if got := testCredentialBodyString(credential.Spec.Body, "api_key"); got != "env-\"secret\"\\line\nnext" {
		t.Fatalf("expanded secret = %#v", got)
	}
}

func TestAdminResourceApplyReadsYAMLFile(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_YAML_SECRET", "yaml-\"secret\"\\line\nnext")
	resourceFile := filepath.Join(t.TempDir(), "credential.yaml")
	if err := os.WriteFile(resourceFile, []byte(`
apiVersion: gizclaw.admin/v1alpha1
kind: Credential
metadata:
  name: yaml-credential
spec:
  provider: minimax
  body:
    api_key: ${GIZCLAW_TEST_YAML_SECRET}
`), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}
	fake := &fakeResourceClient{applyResult: apitypes.ApplyResult{Name: "yaml-credential"}}
	restore := stubResourceClient(fake)
	defer restore()

	cmd := NewCmd()
	cmd.SetArgs([]string{"apply", "-f", resourceFile})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin apply yaml error: %v", err)
	}
	if fake.appliedName != "yaml-credential" {
		t.Fatalf("applied name = %q", fake.appliedName)
	}
	credential, err := fake.appliedResource.AsCredentialResource()
	if err != nil {
		t.Fatalf("applied resource is not credential: %v", err)
	}
	if got := testCredentialBodyString(credential.Spec.Body, "api_key"); got != "yaml-\"secret\"\\line\nnext" {
		t.Fatalf("expanded YAML secret = %#v", got)
	}
}

func TestAdminResourceYAMLExpansionHandlesNestedValues(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_YAML_NESTED_SECRET", "nested-secret")
	resource, err := decodeResourceData("resources.yaml", []byte(`
apiVersion: gizclaw.admin/v1alpha1
kind: ResourceList
metadata:
  name: nested-resources
spec:
  items:
    - apiVersion: gizclaw.admin/v1alpha1
      kind: Credential
      metadata:
        name: nested-credential
      spec:
        provider: openai
        body:
          api_key: ${GIZCLAW_TEST_YAML_NESTED_SECRET}
`))
	if err != nil {
		t.Fatalf("decodeResourceData YAML: %v", err)
	}
	list, err := resource.AsResourceListResource()
	if err != nil {
		t.Fatalf("AsResourceListResource: %v", err)
	}
	if len(list.Spec.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(list.Spec.Items))
	}
	credential, err := list.Spec.Items[0].AsCredentialResource()
	if err != nil {
		t.Fatalf("nested AsCredentialResource: %v", err)
	}
	if got := testCredentialBodyString(credential.Spec.Body, "api_key"); got != "nested-secret" {
		t.Fatalf("nested api_key = %q, want nested-secret", got)
	}

	if _, err := expandResourceYAMLValue(map[any]any{"key": "${GIZCLAW_TEST_YAML_NESTED_SECRET}"}); err != nil {
		t.Fatalf("expandResourceYAMLValue string-key map: %v", err)
	}
	if _, err := expandResourceYAMLValue(map[any]any{1: "value"}); err == nil {
		t.Fatal("expandResourceYAMLValue non-string key error = nil")
	}
}

func TestAdminResourceApplyRejectsUnsupportedResourceFileExtension(t *testing.T) {
	resourceFile := filepath.Join(t.TempDir(), "credential.txt")
	if err := os.WriteFile(resourceFile, []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "txt-credential"},
		"spec": {"provider": "minimax", "body": {"api_key": "secret"}}
	}`), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}

	cmd := NewCmd()
	cmd.SetArgs([]string{"apply", "-f", resourceFile})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), ".json, .yaml, or .yml") {
		t.Fatalf("admin apply error = %v, want unsupported extension", err)
	}
}

func TestAdminResourceApplyRejectsUnknownResourceKind(t *testing.T) {
	_, err := decodeResourceData("unknown.json", []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Unknown",
		"metadata": {"name": "unknown"},
		"spec": {}
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown resource kind "Unknown"`) {
		t.Fatalf("decodeResourceData unknown kind error = %v", err)
	}
}

func TestAdminResourceApplyAcceptsGeneratedResourceKindAlias(t *testing.T) {
	_, err := decodeResourceData("credential.json", []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "CredentialResource",
		"metadata": {"name": "credential-alias"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`))
	if err != nil {
		t.Fatalf("decodeResourceData generated alias error = %v", err)
	}
}

func TestAdminResourceApplyRejectsMissingEnv(t *testing.T) {
	resourceFile := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(resourceFile, []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
			"metadata": {"name": "env-credential"},
			"spec": {
				"provider": "minimax",
				"body": {"api_key": "${GIZCLAW_TEST_MISSING_SECRET}"}
			}
		}`), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}

	cmd := NewCmd()
	cmd.SetArgs([]string{"apply", "-f", resourceFile})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "GIZCLAW_TEST_MISSING_SECRET") {
		t.Fatalf("admin apply error = %v, want missing env", err)
	}
}

func TestExpandResourceEnvSupportsJSONDefaults(t *testing.T) {
	data, err := expandResourceEnv([]byte(`{"resource_ids": ${GIZCLAW_TEST_IDS_JSON:-["a", "b"]}}`))
	if err != nil {
		t.Fatalf("expandResourceEnv() error = %v", err)
	}
	var decoded struct {
		ResourceIDs []string `json:"resource_ids"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("expanded JSON did not decode: %v; data=%s", err, data)
	}
	if len(decoded.ResourceIDs) != 2 || decoded.ResourceIDs[0] != "a" || decoded.ResourceIDs[1] != "b" {
		t.Fatalf("resource_ids = %#v", decoded.ResourceIDs)
	}
}

func TestAdminResourceApplyRequiresFile(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"apply"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("admin apply without --file should fail")
	}
}

func TestAdminResourcePropagatesOpenClientError(t *testing.T) {
	want := errors.New("connect failed")
	original := openResourceClient
	openResourceClient = func(string) (resourceClient, error) {
		return nil, want
	}
	defer func() { openResourceClient = original }()

	cmd := NewCmd()
	cmd.SetArgs([]string{"show", "Credential", "minimax-main"})
	if err := cmd.Execute(); !errors.Is(err, want) {
		t.Fatalf("admin show error = %v, want %v", err, want)
	}
}

func TestResourceResponseErrorPrefersStructuredError(t *testing.T) {
	resp := apitypes.NewErrorResponse("NOPE", "not implemented")
	err := resourceResponseError(501, nil, &resp)
	if err == nil || !strings.Contains(err.Error(), "NOPE: not implemented") {
		t.Fatalf("resourceResponseError() = %v", err)
	}
}

func TestResourceClientBridgeApplyAndGet(t *testing.T) {
	resource := mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
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
				Name:       "minimax-main",
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
	bridge := &resourceClientBridge{
		api: api,
		close: func() error {
			closed = true
			return nil
		},
	}

	result, err := bridge.ApplyResource(context.Background(), resource)
	if err != nil {
		t.Fatalf("ApplyResource error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated || result.Name != "minimax-main" {
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

func TestResourceClientBridgeStructuredErrors(t *testing.T) {
	errResp := apitypes.NewErrorResponse("APPLY_NOT_IMPLEMENTED", "admin apply is not implemented yet")
	bridge := &resourceClientBridge{
		api: &fakeAdminResourceAPI{
			applyResp:  &adminhttp.ApplyResourceResponse{JSON501: &errResp},
			deleteResp: &adminhttp.DeleteResourceResponse{JSON500: &errResp},
			getResp:    &adminhttp.GetResourceResponse{JSON501: &errResp},
		},
	}

	_, err := bridge.ApplyResource(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
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

func TestResourceClientBridgePropagatesAPIErrors(t *testing.T) {
	want := errors.New("api failed")
	bridge := &resourceClientBridge{
		api: &fakeAdminResourceAPI{
			applyErr:  want,
			deleteErr: want,
			getErr:    want,
		},
	}
	_, err := bridge.ApplyResource(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
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
	if err := (&resourceClientBridge{}).Close(); err != nil {
		t.Fatalf("nil Close error = %v", err)
	}
}

func TestResourceResponseErrorFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{
			name: "body",
			err:  resourceResponseError(500, []byte("plain failure")),
			want: "unexpected status 500: plain failure",
		},
		{
			name: "status",
			err:  resourceResponseError(404, nil),
			want: "unexpected status 404",
		},
		{
			name: "empty",
			err:  resourceResponseError(0, nil),
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

func stubResourceClient(fake *fakeResourceClient) func() {
	original := openResourceClient
	openResourceClient = func(string) (resourceClient, error) {
		return fake, nil
	}
	return func() { openResourceClient = original }
}

type fakeResourceClient struct {
	applyResult apitypes.ApplyResult
	getResource apitypes.Resource

	appliedResource apitypes.Resource
	appliedKind     apitypes.ResourceKind
	appliedName     string
	deletedKind     apitypes.ResourceKind
	deletedName     string
	gotKind         apitypes.ResourceKind
	gotName         string
}

func (f *fakeResourceClient) ApplyResource(_ context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	kind, name, err := resourceKindAndName(resource)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	f.appliedResource = resource
	f.appliedKind = kind
	f.appliedName = name
	return f.applyResult, nil
}

func (f *fakeResourceClient) DeleteResource(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	f.deletedKind = kind
	f.deletedName = name
	return f.getResource, nil
}

func (f *fakeResourceClient) GetResource(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	f.gotKind = kind
	f.gotName = name
	return f.getResource, nil
}

func (f *fakeResourceClient) Close() error { return nil }

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

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}

func resourceKindAndName(resource apitypes.Resource) (apitypes.ResourceKind, string, error) {
	var header struct {
		Kind     apitypes.ResourceKind `json:"kind"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return "", "", err
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return "", "", err
	}
	return header.Kind, header.Metadata.Name, nil
}
