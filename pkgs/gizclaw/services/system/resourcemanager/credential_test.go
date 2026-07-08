package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestApplyCredentialCreatesResource(t *testing.T) {
	credentials := newFakeCredentials()
	manager := New(Services{Credentials: credentials})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"},
			"description": "primary key"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("action = %q, want %q", result.Action, apitypes.ApplyActionCreated)
	}
	if credentials.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", credentials.putCount)
	}
	if credentials.items["minimax-main"].Provider != "minimax" {
		t.Fatalf("stored provider = %q, want minimax", credentials.items["minimax-main"].Provider)
	}
}

func TestApplyCredentialUnchangedSkipsPut(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["minimax-main"] = apitypes.Credential{
		Body:        testOpenAICredentialBody("secret"),
		CreatedAt:   time.Now().UTC(),
		Description: ptr("primary key"),
		Name:        "minimax-main",
		Provider:    "minimax",
		UpdatedAt:   time.Now().UTC(),
	}
	manager := New(Services{Credentials: credentials})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"},
			"description": "primary key"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want %q", result.Action, apitypes.ApplyActionUnchanged)
	}
	if credentials.putCount != 0 {
		t.Fatalf("putCount = %d, want 0", credentials.putCount)
	}
}

func TestApplyCredentialUpdatesResource(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["minimax-main"] = apitypes.Credential{
		Body:      testOpenAICredentialBody("old"),
		CreatedAt: time.Now().UTC(),
		Name:      "minimax-main",
		Provider:  "minimax",
		UpdatedAt: time.Now().UTC(),
	}
	manager := New(Services{Credentials: credentials})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "new"}
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("action = %q, want updated", result.Action)
	}
	if credentials.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", credentials.putCount)
	}
}

func TestGetCredentialReturnsResource(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["minimax-main"] = apitypes.Credential{
		Body:      testOpenAICredentialBody("secret"),
		CreatedAt: time.Now().UTC(),
		Name:      "minimax-main",
		Provider:  "minimax",
		UpdatedAt: time.Now().UTC(),
	}
	manager := New(Services{Credentials: credentials})

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	credential, err := resource.AsCredentialResource()
	if err != nil {
		t.Fatalf("AsCredentialResource returned error: %v", err)
	}
	if credential.Kind != apitypes.CredentialResourceKind(apitypes.ResourceKindCredential) {
		t.Fatalf("kind = %q, want Credential", credential.Kind)
	}
	if credential.Metadata.Name != "minimax-main" {
		t.Fatalf("metadata.name = %q, want minimax-main", credential.Metadata.Name)
	}
	if got := testCredentialBodyString(credential.Spec.Body, "api_key"); got != "secret" {
		t.Fatalf("api_key = %q, want secret", got)
	}
}

func TestPutCredentialWritesAndReturnsResource(t *testing.T) {
	credentials := newFakeCredentials()
	manager := New(Services{Credentials: credentials})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if credentials.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", credentials.putCount)
	}
	credential, err := resource.AsCredentialResource()
	if err != nil {
		t.Fatalf("AsCredentialResource returned error: %v", err)
	}
	if credential.Metadata.Name != "minimax-main" {
		t.Fatalf("metadata.name = %q, want minimax-main", credential.Metadata.Name)
	}
	if credential.Spec.Provider != "minimax" {
		t.Fatalf("provider = %q, want minimax", credential.Spec.Provider)
	}
}

func TestPutCredentialEscapesServicePathName(t *testing.T) {
	credentials := newFakeCredentials()
	manager := New(Services{Credentials: credentials})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "mini/max%main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	credential, err := resource.AsCredentialResource()
	if err != nil {
		t.Fatalf("AsCredentialResource returned error: %v", err)
	}
	if credential.Metadata.Name != "mini/max%main" {
		t.Fatalf("metadata.name = %q, want mini/max%%main", credential.Metadata.Name)
	}
	if _, ok := credentials.items["mini/max%main"]; !ok {
		t.Fatal("credential was not stored under unescaped logical name")
	}
}

func TestCredentialServiceErrorResponses(t *testing.T) {
	credentials := newFakeCredentials()
	manager := New(Services{Credentials: credentials})

	credentials.getStatus = 500
	_, _, err := manager.getCredential(context.Background(), "missing")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	credentials.getStatus = 0
	credentials.putStatus = 400
	err = manager.putCredential(context.Background(), "bad", adminhttp.CredentialUpsert{})
	assertResourceError(t, err, 400, "INVALID_CREDENTIAL")

	credentials.putStatus = 500
	err = manager.putCredential(context.Background(), "bad", adminhttp.CredentialUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")
}

type fakeCredentials struct {
	items     map[string]apitypes.Credential
	putCount  int
	getStatus int
	putStatus int
}

func newFakeCredentials() *fakeCredentials {
	return &fakeCredentials{items: map[string]apitypes.Credential{}}
}

func (f *fakeCredentials) ListCredentials(context.Context, adminhttp.ListCredentialsRequestObject) (adminhttp.ListCredentialsResponseObject, error) {
	return nil, nil
}

func (f *fakeCredentials) CreateCredential(context.Context, adminhttp.CreateCredentialRequestObject) (adminhttp.CreateCredentialResponseObject, error) {
	return nil, nil
}

func (f *fakeCredentials) DeleteCredential(_ context.Context, request adminhttp.DeleteCredentialRequestObject) (adminhttp.DeleteCredentialResponseObject, error) {
	name := mustUnescapePathParam(string(request.Name))
	item, ok := f.items[name]
	if !ok {
		return adminhttp.DeleteCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", "not found")), nil
	}
	delete(f.items, name)
	return adminhttp.DeleteCredential200JSONResponse(item), nil
}

func (f *fakeCredentials) GetCredential(_ context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	if f.getStatus == 500 {
		return adminhttp.GetCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	name := mustUnescapePathParam(string(request.Name))
	item, ok := f.items[name]
	if !ok {
		return adminhttp.GetCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetCredential200JSONResponse(item), nil
}

func (f *fakeCredentials) PutCredential(_ context.Context, request adminhttp.PutCredentialRequestObject) (adminhttp.PutCredentialResponseObject, error) {
	switch f.putStatus {
	case 400:
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", "invalid")), nil
	case 500:
		return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	f.putCount++
	name := mustUnescapePathParam(string(request.Name))
	body := *request.Body
	now := time.Now().UTC()
	item := apitypes.Credential{
		Body:        body.Body,
		CreatedAt:   now,
		Description: body.Description,
		Name:        body.Name,
		Provider:    body.Provider,
		UpdatedAt:   now,
	}
	f.items[name] = item
	return adminhttp.PutCredential200JSONResponse(item), nil
}
