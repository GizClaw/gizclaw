package credential

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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
	createResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateCredential200JSONResponse)
	if !ok {
		t.Fatalf("CreateCredential() response = %#v", createResp)
	}
	if created.Name != "openai-primary" || created.Provider != "openai" {
		t.Fatalf("CreateCredential() credential = %#v", created)
	}
	if testCredentialBodyString(created.Body, "api_key") != "sk-test" {
		t.Fatalf("CreateCredential() body = %#v", created.Body)
	}

	getResp, err := srv.GetCredential(ctx, adminhttp.GetCredentialRequestObject{Name: "openai-primary"})
	if err != nil {
		t.Fatalf("GetCredential() error = %v", err)
	}
	got, ok := getResp.(adminhttp.GetCredential200JSONResponse)
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
			"body": {"api_key": "volc-api-key"}
	}`)
	putResp, err := srv.PutCredential(ctx, adminhttp.PutCredentialRequestObject{
		Name: "openai-primary",
		Body: &updateBody,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	updated, ok := putResp.(adminhttp.PutCredential200JSONResponse)
	if !ok {
		t.Fatalf("PutCredential() response = %#v", putResp)
	}
	if updated.Provider != "volc" {
		t.Fatalf("PutCredential() credential = %#v", updated)
	}
	if testCredentialBodyString(updated.Body, "api_key") != "volc-api-key" {
		t.Fatalf("PutCredential() body = %#v", updated.Body)
	}

	oldProvider := string("openai")
	oldListResp, err := srv.ListCredentials(ctx, adminhttp.ListCredentialsRequestObject{
		Params: adminhttp.ListCredentialsParams{Provider: &oldProvider},
	})
	if err != nil {
		t.Fatalf("ListCredentials(old provider) error = %v", err)
	}
	oldList, ok := oldListResp.(adminhttp.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(old provider) response = %#v", oldListResp)
	}
	if len(oldList.Items) != 0 {
		t.Fatalf("ListCredentials(old provider) = %#v", oldList)
	}

	newProvider := string("volc")
	newListResp, err := srv.ListCredentials(ctx, adminhttp.ListCredentialsRequestObject{
		Params: adminhttp.ListCredentialsParams{Provider: &newProvider},
	})
	if err != nil {
		t.Fatalf("ListCredentials(new provider) error = %v", err)
	}
	newList, ok := newListResp.(adminhttp.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(new provider) response = %#v", newListResp)
	}
	if len(newList.Items) != 1 || newList.Items[0].Name != "openai-primary" {
		t.Fatalf("ListCredentials(new provider) = %#v", newList)
	}
	if testCredentialBodyString(newList.Items[0].Body, "api_key") != "volc-api-key" {
		t.Fatalf("ListCredentials(new provider) body = %#v", newList.Items[0].Body)
	}

	deleteResp, err := srv.DeleteCredential(ctx, adminhttp.DeleteCredentialRequestObject{Name: "openai-primary"})
	if err != nil {
		t.Fatalf("DeleteCredential() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteCredential200JSONResponse); !ok {
		t.Fatalf("DeleteCredential() response = %#v", deleteResp)
	}

	getAfterDelete, err := srv.GetCredential(ctx, adminhttp.GetCredentialRequestObject{Name: "openai-primary"})
	if err != nil {
		t.Fatalf("GetCredential() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminhttp.GetCredential404JSONResponse); !ok {
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
		if _, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateCredential(%s) error = %v", raw, err)
		}
	}

	limit := int32(1)
	provider := string("openai")
	firstResp, err := srv.ListCredentials(ctx, adminhttp.ListCredentialsRequestObject{
		Params: adminhttp.ListCredentialsParams{
			Provider: &provider,
			Limit:    &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListCredentials(first page) error = %v", err)
	}
	first, ok := firstResp.(adminhttp.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListCredentials(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListCredentials(ctx, adminhttp.ListCredentialsRequestObject{
		Params: adminhttp.ListCredentialsParams{
			Provider: &provider,
			Cursor:   &cursor,
			Limit:    &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListCredentials(second page) error = %v", err)
	}
	second, ok := secondResp.(adminhttp.ListCredentials200JSONResponse)
	if !ok {
		t.Fatalf("ListCredentials(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 || second.Items[0].Name == first.Items[0].Name || second.HasNext {
		t.Fatalf("ListCredentials(second page) = %#v", second)
	}
	if testCredentialBodyString(second.Items[0].Body, "api_key") == "" {
		t.Fatalf("ListCredentials(second page) body = %#v", second.Items[0].Body)
	}

	allResp, err := srv.ListCredentials(ctx, adminhttp.ListCredentialsRequestObject{
		Params: adminhttp.ListCredentialsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListCredentials(all first page) error = %v", err)
	}
	allFirst, ok := allResp.(adminhttp.ListCredentials200JSONResponse)
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
	createResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}
	if _, ok := createResp.(adminhttp.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential() response = %#v", createResp)
	}

	putBody := mustCredentialUpsert(t, `{
		"name": "beta",
		"provider": "openai"
	}`)
	putResp, err := srv.PutCredential(ctx, adminhttp.PutCredentialRequestObject{
		Name: "beta",
		Body: &putBody,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	if _, ok := putResp.(adminhttp.PutCredential400JSONResponse); !ok {
		t.Fatalf("PutCredential() response = %#v", putResp)
	}

	nilCreateResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{})
	if err != nil {
		t.Fatalf("CreateCredential(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminhttp.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(nil body) response = %#v", nilCreateResp)
	}

	nilPutResp, err := srv.PutCredential(ctx, adminhttp.PutCredentialRequestObject{Name: "beta"})
	if err != nil {
		t.Fatalf("PutCredential(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminhttp.PutCredential400JSONResponse); !ok {
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
	tokenResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &tokenOnly})
	if err != nil {
		t.Fatalf("CreateCredential(token body) error = %v", err)
	}
	if _, ok := tokenResp.(adminhttp.CreateCredential200JSONResponse); !ok {
		t.Fatalf("CreateCredential(token body) response = %#v", tokenResp)
	}

	wrongBody := mustCredentialUpsert(t, `{
		"name": "wrong-body",
		"provider": "openai",
		"body": {"openapi_access_key_id": "ak-test"}
	}`)
	wrongResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &wrongBody})
	if err != nil {
		t.Fatalf("CreateCredential(wrong body) error = %v", err)
	}
	if _, ok := wrongResp.(adminhttp.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(wrong body) response = %#v", wrongResp)
	}

	emptyObject := mustCredentialUpsert(t, `{
		"name": "empty-body",
		"provider": "volc",
		"body": {}
	}`)
	emptyResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &emptyObject})
	if err != nil {
		t.Fatalf("CreateCredential(empty body) error = %v", err)
	}
	if _, ok := emptyResp.(adminhttp.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(empty body) response = %#v", emptyResp)
	}

	unknownProvider := mustCredentialUpsert(t, `{
		"name": "unknown-provider",
		"provider": "custom",
		"body": {"api_key": "sk-test"}
	}`)
	unknownResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &unknownProvider})
	if err != nil {
		t.Fatalf("CreateCredential(unknown provider) error = %v", err)
	}
	if _, ok := unknownResp.(adminhttp.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(unknown provider) response = %#v", unknownResp)
	}
}

func TestValidateCredentialBodyProviderShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		body     string
		wantErr  bool
	}{
		{name: "openai api key", provider: "openai", body: `{"api_key":"sk-openai"}`},
		{name: "gemini api key", provider: "gemini", body: `{"api_key":"sk-gemini"}`},
		{name: "dashscope api key", provider: "dashscope", body: `{"api_key":"sk-dashscope"}`},
		{name: "minimax api key", provider: "minimax", body: `{"api_key":"sk-minimax"}`},
		{name: "volc speech api key", provider: "volc", body: `{"api_key":"sk-volc"}`},
		{name: "volc openapi key", provider: "volc", body: `{"openapi_access_key_id":"ak","openapi_access_key":"sk"}`},
		{name: "volcengine alias", provider: "volcengine", body: `{"api_key":"sk-volc"}`},
		{name: "unsupported provider", provider: "unknown", body: `{"api_key":"sk-test"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upsert := mustCredentialUpsert(t, `{"name":"shape","provider":"`+tt.provider+`","body":`+tt.body+`}`)
			err := validateCredentialBody(tt.provider, upsert.Body)
			if tt.wantErr && err == nil {
				t.Fatal("validateCredentialBody() error = nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateCredentialBody() error = %v", err)
			}
		})
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
	if _, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &createBody}); err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}

	putBody := mustCredentialUpsert(t, `{
		"name": "alpha",
		"provider": "openai",
		"description": "second"
	}`)
	putResp, err := srv.PutCredential(ctx, adminhttp.PutCredentialRequestObject{
		Name: "alpha",
		Body: &putBody,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	if _, ok := putResp.(adminhttp.PutCredential200JSONResponse); !ok {
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
	resp, err := srv.PutCredential(ctx, adminhttp.PutCredentialRequestObject{
		Name: "expected",
		Body: &body,
	})
	if err != nil {
		t.Fatalf("PutCredential() error = %v", err)
	}
	if _, ok := resp.(adminhttp.PutCredential400JSONResponse); !ok {
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
	if _, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &duplicate}); err != nil {
		t.Fatalf("CreateCredential(seed) error = %v", err)
	}
	dupResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &duplicate})
	if err != nil {
		t.Fatalf("CreateCredential(duplicate) error = %v", err)
	}
	if _, ok := dupResp.(adminhttp.CreateCredential409JSONResponse); !ok {
		t.Fatalf("CreateCredential(duplicate) response = %#v", dupResp)
	}

	missingProvider := mustCredentialUpsert(t, `{
		"name": "bad",
		"body": {"api_key": "sk-test"}
	}`)
	badResp, err := srv.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &missingProvider})
	if err != nil {
		t.Fatalf("CreateCredential(missing provider) error = %v", err)
	}
	if _, ok := badResp.(adminhttp.CreateCredential400JSONResponse); !ok {
		t.Fatalf("CreateCredential(missing provider) response = %#v", badResp)
	}

	missingDelete, err := srv.DeleteCredential(ctx, adminhttp.DeleteCredentialRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteCredential(missing) error = %v", err)
	}
	if _, ok := missingDelete.(adminhttp.DeleteCredential404JSONResponse); !ok {
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

func mustCredentialUpsert(t *testing.T, raw string) adminhttp.CredentialUpsert {
	t.Helper()

	var upsert adminhttp.CredentialUpsert
	if err := json.Unmarshal([]byte(raw), &upsert); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return upsert
}
