package resourcemanager

import (
	"context"
	"errors"
	"fmt"
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
		"metadata": {"id": "minimax-main"},
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

func TestApplyCredentialRequiresBodyWhenCreating(t *testing.T) {
	manager := New(Services{Credentials: newFakeCredentials()})
	_, err := manager.Apply(t.Context(), mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Credential",
		"metadata":{"id":"missing-body"},
		"spec":{"provider":"openai"}
	}`))
	assertResourceError(t, err, 400, "INVALID_CREDENTIAL_RESOURCE")
}

func TestApplyCredentialReconcilesAcceptedRequestAfterLostResponse(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.createErrAfterStore = errors.New("injected lost response")
	manager := New(Services{Credentials: credentials})
	resource := mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Credential",
		"metadata":{"id":"retry-stable"},
		"spec":{"provider":"openai","body":{"api_key":"secret"}}
	}`)

	if _, err := manager.Apply(t.Context(), resource); !errors.Is(err, credentials.createErrAfterStore) {
		t.Fatalf("Apply(lost response) error = %v", err)
	}
	credentials.createErrAfterStore = nil
	reconciled, err := manager.Apply(t.Context(), resource)
	if err != nil || reconciled.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(reconcile) = %#v, %v", reconciled, err)
	}
	if credentials.putCount != 1 || len(credentials.items) != 1 || credentials.items["retry-stable"].Id != "retry-stable" {
		t.Fatalf("reconciled store = %#v, writes = %d", credentials.items, credentials.putCount)
	}
}

func TestApplyCredentialPreservesOpaqueIDAcrossHTTPShapedCalls(t *testing.T) {
	for _, id := range []string{"mini/max", "literal%2Fsegment", "literal%segment"} {
		t.Run(id, func(t *testing.T) {
			credentials := newFakeCredentials()
			manager := New(Services{Credentials: credentials})
			resource := mustResource(t, fmt.Sprintf(`{
				"apiVersion":"gizclaw.admin/v1alpha1",
				"kind":"Credential",
				"metadata":{"id":%q},
				"spec":{"provider":"minimax","body":{"api_key":"secret"}}
			}`, id))

			first, err := manager.Apply(context.Background(), resource)
			if err != nil {
				t.Fatalf("first Apply() error = %v", err)
			}
			second, err := manager.Apply(context.Background(), resource)
			if err != nil {
				t.Fatalf("second Apply() error = %v", err)
			}
			if first.Action != apitypes.ApplyActionCreated || second.Action != apitypes.ApplyActionUnchanged {
				t.Fatalf("Apply() actions = %q, %q", first.Action, second.Action)
			}
			if got := credentials.items[id].Id; got != id {
				t.Fatalf("stored id = %q, want %q", got, id)
			}
		})
	}
}

func TestApplyCredentialUnchangedSkipsPut(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["minimax-main"] = apitypes.Credential{
		Body:        testOpenAICredentialBody("secret"),
		CreatedAt:   time.Now().UTC(),
		Description: new("primary key"),
		Id:          "minimax-main",
		Provider:    "minimax",
		UpdatedAt:   time.Now().UTC(),
	}
	manager := New(Services{Credentials: credentials})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
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
		Id:        "minimax-main",
		Provider:  "minimax",
		UpdatedAt: time.Now().UTC(),
	}
	manager := New(Services{Credentials: credentials})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
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
		Id:        "minimax-main",
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
	if metadataID(t, credential.Metadata) != "minimax-main" {
		t.Fatalf("metadata.id = %q, want minimax-main", metadataID(t, credential.Metadata))
	}
	if credential.Spec.Body != nil {
		t.Fatal("Credential Resource read exposed write-only body")
	}
}

func TestApplyCredentialReadbackRetainsWriteOnlyBody(t *testing.T) {
	credentials := newFakeCredentials()
	manager := New(Services{Credentials: credentials})
	desired := mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Credential",
		"metadata":{"id":"openai-main"},
		"spec":{"provider":"openai","body":{"api_key":"secret"}}
	}`)
	if _, err := manager.Apply(t.Context(), desired); err != nil {
		t.Fatal(err)
	}
	readback, err := manager.Get(t.Context(), apitypes.ResourceKindCredential, "openai-main")
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := manager.Apply(t.Context(), readback)
	if err != nil || unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(redacted readback) = %#v, %v", unchanged, err)
	}
	if got := testCredentialBodyString(credentials.items["openai-main"].Body, "api_key"); got != "secret" {
		t.Fatalf("stored api_key = %q, want retained secret", got)
	}
}

func TestPutCredentialWritesAndReturnsResource(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["minimax-main"] = apitypes.Credential{Id: "minimax-main"}
	manager := New(Services{Credentials: credentials})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
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
	if metadataID(t, credential.Metadata) != "minimax-main" {
		t.Fatalf("metadata.id = %q, want minimax-main", metadataID(t, credential.Metadata))
	}
	if credential.Spec.Provider != "minimax" {
		t.Fatalf("provider = %q, want minimax", credential.Spec.Provider)
	}
}

func TestPutCredentialCreatesAbsentResource(t *testing.T) {
	credentials := newFakeCredentials()
	manager := New(Services{Credentials: credentials})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Credential",
		"metadata":{"id":"caller-supplied"},
		"spec":{"provider":"minimax","body":{"api_key":"secret"}}
	}`))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	credential, err := resource.AsCredentialResource()
	if err != nil {
		t.Fatal(err)
	}
	if credential.Metadata.Id != "caller-supplied" || credentials.items["caller-supplied"].Id != "caller-supplied" {
		t.Fatalf("Put() did not create caller-supplied ID: %#v", credential.Metadata)
	}
}

func TestPutCredentialEscapesServicePathName(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["mini/max%main"] = apitypes.Credential{Id: "mini/max%main"}
	manager := New(Services{Credentials: credentials})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "mini/max%main"},
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
	if metadataID(t, credential.Metadata) != "mini/max%main" {
		t.Fatalf("metadata.id = %q, want mini/max%%main", metadataID(t, credential.Metadata))
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
	items               map[string]apitypes.Credential
	createErrAfterStore error
	putCount            int
	getStatus           int
	putStatus           int
}

func newFakeCredentials() *fakeCredentials {
	return &fakeCredentials{items: map[string]apitypes.Credential{}}
}

func (f *fakeCredentials) ListCredentials(context.Context, adminhttp.ListCredentialsRequestObject) (adminhttp.ListCredentialsResponseObject, error) {
	return nil, nil
}

func (f *fakeCredentials) CreateCredential(_ context.Context, request adminhttp.CreateCredentialRequestObject) (adminhttp.CreateCredentialResponseObject, error) {
	f.putCount++
	body := *request.Body
	now := time.Now().UTC()
	item := apitypes.Credential{
		Id: body.Id, Body: body.Body, CreatedAt: now,
		Description: body.Description, Provider: body.Provider, UpdatedAt: now,
	}
	f.items[item.Id] = item
	if f.createErrAfterStore != nil {
		return nil, f.createErrAfterStore
	}
	return adminhttp.CreateCredential200JSONResponse(item), nil
}

func (f *fakeCredentials) DeleteCredential(_ context.Context, request adminhttp.DeleteCredentialRequestObject) (adminhttp.DeleteCredentialResponseObject, error) {
	name := string(request.Id)
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
	name := string(request.Id)
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
	name := string(request.Id)
	body := *request.Body
	now := time.Now().UTC()
	item := apitypes.Credential{
		Id:          name,
		Body:        body.Body,
		CreatedAt:   now,
		Description: body.Description,
		Provider:    body.Provider,
		UpdatedAt:   now,
	}
	f.items[name] = item
	return adminhttp.PutCredential200JSONResponse(item), nil
}
