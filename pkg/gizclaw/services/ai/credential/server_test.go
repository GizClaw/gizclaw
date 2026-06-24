package credential

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestMigrationNoop(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	legacy := []byte(`{
		"name":"legacy-volc",
		"provider":"volc",
		"method":"api_key",
		"description":"legacy credential",
		"body":{"method":"api_key","api_key":"ak","token":"tok"},
		"created_at":"2026-01-01T00:00:00Z",
		"updated_at":"2026-01-01T00:00:00Z"
	}`)
	if err := srv.Store.BatchSet(ctx, []kv.Entry{
		{Key: credentialKey("legacy-volc"), Value: legacy},
	}); err != nil {
		t.Fatalf("seed legacy credential: %v", err)
	}

	for range 2 {
		if err := srv.Migration(ctx); err != nil {
			t.Fatalf("Migration() error = %v", err)
		}
	}

	data, err := srv.Store.Get(ctx, credentialKey("legacy-volc"))
	if err != nil {
		t.Fatalf("get credential after migration: %v", err)
	}
	if string(data) != string(legacy) {
		t.Fatalf("Migration changed credential: %s", data)
	}
}

func TestServerCredentialsCRUD(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	createBody := mustCredentialUpsert(t, `{
		"name": "openai-primary",
		"provider": "openai",
		"description": "primary openai credential",
		"body": {"api_key": "sk-test"}
	}`)
	createResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}
	created, ok := createResp.(adminservice.CreateCredential200JSONResponse)
	if !ok {
		t.Fatalf("CreateCredential() response = %#v", createResp)
	}
	if created.Name != "openai-primary" || created.Provider != "openai" {
		t.Fatalf("CreateCredential() credential = %#v", created)
	}
	if testCredentialBodyString(created.Body, "api_key") != "sk-test" {
		t.Fatalf("CreateCredential() body = %#v", created.Body)
	}

	getResp, err := srv.GetCredential(ctx, adminservice.GetCredentialRequestObject{Name: "openai-primary"})
	if err != nil {
		t.Fatalf("GetCredential() error = %v", err)
	}
	got, ok := getResp.(adminservice.GetCredential200JSONResponse)
	if !ok {
		t.Fatalf("GetCredential() response = %#v", getResp)
	}
	if got.Description == nil || *got.Description != "primary openai credential" {
		t.Fatalf("GetCredential() description = %#v", got.Description)
	}
	if testCredentialBodyString(got.Body, "api_key") != "sk-test" {
		t.Fatalf("GetCredential() body = %#v", got.Body)
	}

	updateBody := mustCredentialUpsert(t, `{
		"name": "openai-primary",
		"provider": "volc",
		"description": "migrated credential",
		"body": {"app_id": "app-123", "speech_token": "tok-123"}
	}`)
	putResp, err := srv.PutCredential(ctx, adminservice.PutCredentialRequestObject{
		Name: "openai-primary",
		Body: &updateBody,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	updated, ok := putResp.(adminservice.PutCredential200JSONResponse)
	if !ok {
		t.Fatalf("PutCredential() response = %#v", putResp)
	}
	if updated.Provider != "volc" {
		t.Fatalf("PutCredential() credential = %#v", updated)
	}
	if testCredentialBodyString(updated.Body, "app_id") != "app-123" || testCredentialBodyString(updated.Body, "speech_token") != "tok-123" {
		t.Fatalf("PutCredential() body = %#v", updated.Body)
	}

	oldProvider := string("openai")
	oldListResp, err := srv.ListCredentials(ctx, adminservice.ListCredentialsRequestObject{
		Params: adminservice.ListCredentialsParams{Provider: &oldProvider},
	})
	if err != nil {
		t.Fatalf("ListCredentials(old provider) error = %v", err)
	}
	oldList, ok := oldListResp.(adminservice.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(old provider) response = %#v", oldListResp)
	}
	if len(oldList.Items) != 0 {
		t.Fatalf("ListCredentials(old provider) = %#v", oldList)
	}

	newProvider := string("volc")
	newListResp, err := srv.ListCredentials(ctx, adminservice.ListCredentialsRequestObject{
		Params: adminservice.ListCredentialsParams{Provider: &newProvider},
	})
	if err != nil {
		t.Fatalf("ListCredentials(new provider) error = %v", err)
	}
	newList, ok := newListResp.(adminservice.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(new provider) response = %#v", newListResp)
	}
	if len(newList.Items) != 1 || newList.Items[0].Name != "openai-primary" {
		t.Fatalf("ListCredentials(new provider) = %#v", newList)
	}
	if testCredentialBodyString(newList.Items[0].Body, "app_id") != "app-123" {
		t.Fatalf("ListCredentials(new provider) body = %#v", newList.Items[0].Body)
	}

	deleteResp, err := srv.DeleteCredential(ctx, adminservice.DeleteCredentialRequestObject{Name: "openai-primary"})
	if err != nil {
		t.Fatalf("DeleteCredential() error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteCredential200JSONResponse); !ok {
		t.Fatalf("DeleteCredential() response = %#v", deleteResp)
	}

	getAfterDelete, err := srv.GetCredential(ctx, adminservice.GetCredentialRequestObject{Name: "openai-primary"})
	if err != nil {
		t.Fatalf("GetCredential() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminservice.GetCredential404JSONResponse); !ok {
		t.Fatalf("GetCredential() after delete response = %#v", getAfterDelete)
	}
}

func TestServerListCredentialsPaginationAndFilter(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	for _, raw := range []string{
		`{"name":"alpha","provider":"openai","body":{"api_key":"a"}}`,
		`{"name":"beta","provider":"openai","body":{"api_key":"b"}}`,
		`{"name":"gamma","provider":"minimax","body":{"api_key":"c"}}`,
	} {
		body := mustCredentialUpsert(t, raw)
		if _, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateCredential(%s) error = %v", raw, err)
		}
	}

	limit := int32(1)
	provider := string("openai")
	firstResp, err := srv.ListCredentials(ctx, adminservice.ListCredentialsRequestObject{
		Params: adminservice.ListCredentialsParams{
			Provider: &provider,
			Limit:    &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListCredentials(first page) error = %v", err)
	}
	first, ok := firstResp.(adminservice.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListCredentials(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListCredentials(ctx, adminservice.ListCredentialsRequestObject{
		Params: adminservice.ListCredentialsParams{
			Provider: &provider,
			Cursor:   &cursor,
			Limit:    &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListCredentials(second page) error = %v", err)
	}
	second, ok := secondResp.(adminservice.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 || second.Items[0].Name == first.Items[0].Name || second.HasNext {
		t.Fatalf("ListCredentials(second page) = %#v", second)
	}
	if testCredentialBodyString(second.Items[0].Body, "api_key") == "" {
		t.Fatalf("ListCredentials(second page) body = %#v", second.Items[0].Body)
	}

	allResp, err := srv.ListCredentials(ctx, adminservice.ListCredentialsRequestObject{
		Params: adminservice.ListCredentialsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListCredentials(all first page) error = %v", err)
	}
	allFirst, ok := allResp.(adminservice.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(all first page) response = %#v", allResp)
	}
	if len(allFirst.Items) != 1 || !allFirst.HasNext || allFirst.NextCursor == nil {
		t.Fatalf("ListCredentials(all first page) = %#v", allFirst)
	}
}

func TestServerRejectsMissingBodyOnCreateAndNewPut(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	createBody := mustCredentialUpsert(t, `{
		"name": "alpha",
		"provider": "openai"
	}`)
	createResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}
	if _, ok := createResp.(adminservice.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential() response = %#v", createResp)
	}

	putBody := mustCredentialUpsert(t, `{
		"name": "beta",
		"provider": "openai"
	}`)
	putResp, err := srv.PutCredential(ctx, adminservice.PutCredentialRequestObject{
		Name: "beta",
		Body: &putBody,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	if _, ok := putResp.(adminservice.PutCredential400JSONResponse); !ok {
		t.Fatalf("PutCredential() response = %#v", putResp)
	}

	nilCreateResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{})
	if err != nil {
		t.Fatalf("CreateCredential(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminservice.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(nil body) response = %#v", nilCreateResp)
	}

	nilPutResp, err := srv.PutCredential(ctx, adminservice.PutCredentialRequestObject{Name: "beta"})
	if err != nil {
		t.Fatalf("PutCredential(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminservice.PutCredential400JSONResponse); !ok {
		t.Fatalf("PutCredential(nil body) response = %#v", nilPutResp)
	}
}

func TestServerValidatesBodyForProvider(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	tokenOnly := mustCredentialUpsert(t, `{
		"name": "token-only",
		"provider": "openai",
		"body": {"token": "tok-test"}
	}`)
	tokenResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &tokenOnly})
	if err != nil {
		t.Fatalf("CreateCredential(token body) error = %v", err)
	}
	if _, ok := tokenResp.(adminservice.CreateCredential200JSONResponse); !ok {
		t.Fatalf("CreateCredential(token body) response = %#v", tokenResp)
	}

	wrongBody := mustCredentialUpsert(t, `{
		"name": "wrong-body",
		"provider": "openai",
		"body": {"app_id": "app-test", "token": "tok-test"}
	}`)
	wrongResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &wrongBody})
	if err != nil {
		t.Fatalf("CreateCredential(wrong body) error = %v", err)
	}
	if _, ok := wrongResp.(adminservice.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(wrong body) response = %#v", wrongResp)
	}

	emptyObject := mustCredentialUpsert(t, `{
		"name": "empty-body",
		"provider": "volc",
		"body": {}
	}`)
	emptyResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &emptyObject})
	if err != nil {
		t.Fatalf("CreateCredential(empty body) error = %v", err)
	}
	if _, ok := emptyResp.(adminservice.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(empty body) response = %#v", emptyResp)
	}

	unknownProvider := mustCredentialUpsert(t, `{
		"name": "unknown-provider",
		"provider": "custom",
		"body": {"api_key": "sk-test"}
	}`)
	unknownResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &unknownProvider})
	if err != nil {
		t.Fatalf("CreateCredential(unknown provider) error = %v", err)
	}
	if _, ok := unknownResp.(adminservice.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(unknown provider) response = %#v", unknownResp)
	}
}

func TestServerPutRetainsExistingSecretForSameMethod(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	createBody := mustCredentialUpsert(t, `{
		"name": "alpha",
		"provider": "openai",
		"description": "first",
		"body": {"api_key": "sk-test"}
	}`)
	if _, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &createBody}); err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}

	putBody := mustCredentialUpsert(t, `{
		"name": "alpha",
		"provider": "openai",
		"description": "second"
	}`)
	putResp, err := srv.PutCredential(ctx, adminservice.PutCredentialRequestObject{
		Name: "alpha",
		Body: &putBody,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	if _, ok := putResp.(adminservice.PutCredential200JSONResponse); !ok {
		t.Fatalf("PutCredential() response = %#v", putResp)
	}

	record, err := getCredentialRecord(ctx, srv.Store, "alpha")
	if err != nil {
		t.Fatalf("getCredentialRecord() error = %v", err)
	}
	if testCredentialBodyString(record.Body, "api_key") != "sk-test" {
		t.Fatalf("stored credential = %#v", record)
	}
	if record.Description == nil || *record.Description != "second" {
		t.Fatalf("stored description = %#v", record.Description)
	}
}

func TestServerPutRejectsPathNameMismatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	body := mustCredentialUpsert(t, `{
		"name": "other",
		"provider": "openai",
		"body": {"api_key": "sk-test"}
	}`)
	resp, err := srv.PutCredential(ctx, adminservice.PutCredentialRequestObject{
		Name: "expected",
		Body: &body,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	if _, ok := resp.(adminservice.PutCredential400JSONResponse); !ok {
		t.Fatalf("PutCredential() response = %#v", resp)
	}
}

func TestServerCredentialValidationAndMissingPaths(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	duplicate := mustCredentialUpsert(t, `{
		"name": "alpha",
		"provider": "openai",
		"body": {"api_key": "sk-test"}
	}`)
	if _, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &duplicate}); err != nil {
		t.Fatalf("CreateCredential(seed) error = %v", err)
	}
	dupResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &duplicate})
	if err != nil {
		t.Fatalf("CreateCredential(duplicate) error = %v", err)
	}
	if _, ok := dupResp.(adminservice.CreateCredential409JSONResponse); !ok {
		t.Fatalf("CreateCredential(duplicate) response = %#v", dupResp)
	}

	missingProvider := mustCredentialUpsert(t, `{
		"name": "bad",
		"body": {"api_key": "sk-test"}
	}`)
	badResp, err := srv.CreateCredential(ctx, adminservice.CreateCredentialRequestObject{Body: &missingProvider})
	if err != nil {
		t.Fatalf("CreateCredential(missing provider) error = %v", err)
	}
	if _, ok := badResp.(adminservice.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(missing provider) response = %#v", badResp)
	}

	missingDelete, err := srv.DeleteCredential(ctx, adminservice.DeleteCredentialRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteCredential(missing) error = %v", err)
	}
	if _, ok := missingDelete.(adminservice.DeleteCredential404JSONResponse); !ok {
		t.Fatalf("DeleteCredential(missing) response = %#v", missingDelete)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Server{Store: store}
}

func mustCredentialUpsert(t *testing.T, raw string) adminservice.CredentialUpsert {
	t.Helper()

	var upsert adminservice.CredentialUpsert
	if err := json.Unmarshal([]byte(raw), &upsert); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return upsert
}
