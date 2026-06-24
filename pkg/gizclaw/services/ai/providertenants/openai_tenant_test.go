package providertenants

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerOpenAITenantCRUDDefaultsAndPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
	}

	body := openAITenantUpsert("minimax")
	resp, err := srv.CreateOpenAITenant(ctx, adminservice.CreateOpenAITenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateOpenAITenant() error = %v", err)
	}
	created, ok := resp.(adminservice.CreateOpenAITenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateOpenAITenant() response = %#v", resp)
	}
	if created.Kind != apitypes.OpenAITenantKindCompatible {
		t.Fatalf("CreateOpenAITenant() kind = %s", created.Kind)
	}
	if created.ApiMode != apitypes.OpenAITenantAPIModeChatCompletions {
		t.Fatalf("CreateOpenAITenant() api_mode = %s", created.ApiMode)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateOpenAITenant() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}

	if resp, err := srv.CreateOpenAITenant(ctx, adminservice.CreateOpenAITenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateOpenAITenant(duplicate) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateOpenAITenant409JSONResponse); !ok {
		t.Fatalf("CreateOpenAITenant(duplicate) response = %#v", resp)
	}
	for _, name := range []string{"openai", "azure"} {
		body := openAITenantUpsert(name)
		if resp, err := srv.CreateOpenAITenant(ctx, adminservice.CreateOpenAITenantRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateOpenAITenant(%s) error = %v", name, err)
		} else if _, ok := resp.(adminservice.CreateOpenAITenant200JSONResponse); !ok {
			t.Fatalf("CreateOpenAITenant(%s) response = %#v", name, resp)
		}
	}

	limit := int32(2)
	listResp, err := srv.ListOpenAITenants(ctx, adminservice.ListOpenAITenantsRequestObject{
		Params: adminservice.ListOpenAITenantsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListOpenAITenants(first) error = %v", err)
	}
	firstPage := requireOpenAITenantList(t, listResp)
	if !firstPage.HasNext || firstPage.NextCursor == nil || len(firstPage.Items) != 2 {
		t.Fatalf("ListOpenAITenants(first) = %#v", firstPage)
	}
	cursor := string(*firstPage.NextCursor)
	listResp, err = srv.ListOpenAITenants(ctx, adminservice.ListOpenAITenantsRequestObject{
		Params: adminservice.ListOpenAITenantsParams{Cursor: &cursor, Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListOpenAITenants(second) error = %v", err)
	}
	secondPage := requireOpenAITenantList(t, listResp)
	if secondPage.HasNext || secondPage.NextCursor != nil || len(secondPage.Items) != 1 {
		t.Fatalf("ListOpenAITenants(second) = %#v", secondPage)
	}

	updated := openAITenantUpsert("minimax")
	description := "updated tenant"
	updated.Description = &description
	now = now.Add(time.Minute)
	putResp, err := srv.PutOpenAITenant(ctx, adminservice.PutOpenAITenantRequestObject{Name: "minimax", Body: &updated})
	if err != nil {
		t.Fatalf("PutOpenAITenant() error = %v", err)
	}
	put, ok := putResp.(adminservice.PutOpenAITenant200JSONResponse)
	if !ok {
		t.Fatalf("PutOpenAITenant() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutOpenAITenant() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	if put.Description == nil || *put.Description != description {
		t.Fatalf("PutOpenAITenant() description = %#v", put.Description)
	}

	getResp, err := srv.GetOpenAITenant(ctx, adminservice.GetOpenAITenantRequestObject{Name: "minimax"})
	if err != nil {
		t.Fatalf("GetOpenAITenant() error = %v", err)
	}
	if got, ok := getResp.(adminservice.GetOpenAITenant200JSONResponse); !ok || got.Name != "minimax" {
		t.Fatalf("GetOpenAITenant() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteOpenAITenant(ctx, adminservice.DeleteOpenAITenantRequestObject{Name: "minimax"})
	if err != nil {
		t.Fatalf("DeleteOpenAITenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteOpenAITenant200JSONResponse); !ok {
		t.Fatalf("DeleteOpenAITenant() response = %#v", deleteResp)
	}
	if resp, err := srv.GetOpenAITenant(ctx, adminservice.GetOpenAITenantRequestObject{Name: "minimax"}); err != nil {
		t.Fatalf("GetOpenAITenant(missing) error = %v", err)
	} else if _, ok := resp.(adminservice.GetOpenAITenant404JSONResponse); !ok {
		t.Fatalf("GetOpenAITenant(missing) response = %#v", resp)
	}
}

func TestServerOpenAITenantValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminservice.OpenAITenantUpsert
	}{
		{name: "missing name", body: adminservice.OpenAITenantUpsert{CredentialName: "credential"}},
		{name: "missing credential", body: adminservice.OpenAITenantUpsert{Name: "tenant"}},
		{name: "bad kind", body: openAITenantUpsertWith("tenant", stringPtr("bad-kind"), nil)},
		{name: "bad api mode", body: openAITenantUpsertWith("tenant", nil, stringPtr("responses"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateOpenAITenant(ctx, adminservice.CreateOpenAITenantRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateOpenAITenant() error = %v", err)
			}
			if _, ok := resp.(adminservice.CreateOpenAITenant400JSONResponse); !ok {
				t.Fatalf("CreateOpenAITenant() response = %#v", resp)
			}
		})
	}

	body := openAITenantUpsert("tenant")
	if resp, err := srv.PutOpenAITenant(ctx, adminservice.PutOpenAITenantRequestObject{Name: "other", Body: &body}); err != nil {
		t.Fatalf("PutOpenAITenant(mismatch) error = %v", err)
	} else if _, ok := resp.(adminservice.PutOpenAITenant400JSONResponse); !ok {
		t.Fatalf("PutOpenAITenant(mismatch) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListOpenAITenants(ctx, adminservice.ListOpenAITenantsRequestObject{}); err != nil {
		t.Fatalf("ListOpenAITenants(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.ListOpenAITenants500JSONResponse); !ok {
		t.Fatalf("ListOpenAITenants(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateOpenAITenant(ctx, adminservice.CreateOpenAITenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateOpenAITenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateOpenAITenant500JSONResponse); !ok {
		t.Fatalf("CreateOpenAITenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetOpenAITenant(ctx, adminservice.GetOpenAITenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("GetOpenAITenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.GetOpenAITenant500JSONResponse); !ok {
		t.Fatalf("GetOpenAITenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutOpenAITenant(ctx, adminservice.PutOpenAITenantRequestObject{Name: "tenant", Body: &body}); err != nil {
		t.Fatalf("PutOpenAITenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.PutOpenAITenant500JSONResponse); !ok {
		t.Fatalf("PutOpenAITenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteOpenAITenant(ctx, adminservice.DeleteOpenAITenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("DeleteOpenAITenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.DeleteOpenAITenant500JSONResponse); !ok {
		t.Fatalf("DeleteOpenAITenant(nil store) response = %#v", resp)
	}
}

func openAITenantUpsert(name string) adminservice.OpenAITenantUpsert {
	return openAITenantUpsertWith(name, nil, nil)
}

func openAITenantUpsertWith(name string, kind, apiMode *string) adminservice.OpenAITenantUpsert {
	out := adminservice.OpenAITenantUpsert{
		Name:           string(name),
		CredentialName: string("credential"),
	}
	if kind != nil {
		value := apitypes.OpenAITenantKind(*kind)
		out.Kind = &value
	}
	if apiMode != nil {
		value := apitypes.OpenAITenantAPIMode(*apiMode)
		out.ApiMode = &value
	}
	return out
}

func requireOpenAITenantList(t *testing.T, resp adminservice.ListOpenAITenantsResponseObject) adminservice.OpenAITenantList {
	t.Helper()
	list, ok := resp.(adminservice.ListOpenAITenants200JSONResponse)
	if !ok {
		t.Fatalf("ListOpenAITenants() response = %#v", resp)
	}
	return adminservice.OpenAITenantList(list)
}
