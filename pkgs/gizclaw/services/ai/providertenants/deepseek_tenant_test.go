package providertenants

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerDeepSeekTenantCRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
	}

	body := deepSeekTenantUpsert("default")
	resp, err := srv.CreateDeepSeekTenant(ctx, adminhttp.CreateDeepSeekTenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateDeepSeekTenant() error = %v", err)
	}
	created, ok := resp.(adminhttp.CreateDeepSeekTenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateDeepSeekTenant() response = %#v", resp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateDeepSeekTenant() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}
	if created.BaseUrl == nil || *created.BaseUrl != "https://deepseek.example.com/default" {
		t.Fatalf("CreateDeepSeekTenant() base_url = %#v", created.BaseUrl)
	}

	if resp, err := srv.CreateDeepSeekTenant(ctx, adminhttp.CreateDeepSeekTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateDeepSeekTenant(duplicate) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateDeepSeekTenant409JSONResponse); !ok {
		t.Fatalf("CreateDeepSeekTenant(duplicate) response = %#v", resp)
	}
	for _, name := range []string{"alpha", "beta"} {
		body := deepSeekTenantUpsert(name)
		if resp, err := srv.CreateDeepSeekTenant(ctx, adminhttp.CreateDeepSeekTenantRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateDeepSeekTenant(%s) error = %v", name, err)
		} else if _, ok := resp.(adminhttp.CreateDeepSeekTenant200JSONResponse); !ok {
			t.Fatalf("CreateDeepSeekTenant(%s) response = %#v", name, resp)
		}
	}

	limit := int32(2)
	listResp, err := srv.ListDeepSeekTenants(ctx, adminhttp.ListDeepSeekTenantsRequestObject{
		Params: adminhttp.ListDeepSeekTenantsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListDeepSeekTenants(first) error = %v", err)
	}
	firstPage := requireDeepSeekTenantList(t, listResp)
	if !firstPage.HasNext || firstPage.NextCursor == nil || len(firstPage.Items) != 2 {
		t.Fatalf("ListDeepSeekTenants(first) = %#v", firstPage)
	}
	cursor := string(*firstPage.NextCursor)
	listResp, err = srv.ListDeepSeekTenants(ctx, adminhttp.ListDeepSeekTenantsRequestObject{
		Params: adminhttp.ListDeepSeekTenantsParams{Cursor: &cursor, Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListDeepSeekTenants(second) error = %v", err)
	}
	secondPage := requireDeepSeekTenantList(t, listResp)
	if secondPage.HasNext || secondPage.NextCursor != nil || len(secondPage.Items) != 1 {
		t.Fatalf("ListDeepSeekTenants(second) = %#v", secondPage)
	}

	updated := deepSeekTenantUpsert("default")
	description := "updated tenant"
	updated.Description = &description
	now = now.Add(time.Minute)
	putResp, err := srv.PutDeepSeekTenant(ctx, adminhttp.PutDeepSeekTenantRequestObject{Name: "default", Body: &updated})
	if err != nil {
		t.Fatalf("PutDeepSeekTenant() error = %v", err)
	}
	put, ok := putResp.(adminhttp.PutDeepSeekTenant200JSONResponse)
	if !ok {
		t.Fatalf("PutDeepSeekTenant() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutDeepSeekTenant() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	if put.Description == nil || *put.Description != description {
		t.Fatalf("PutDeepSeekTenant() description = %#v", put.Description)
	}

	getResp, err := srv.GetDeepSeekTenant(ctx, adminhttp.GetDeepSeekTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("GetDeepSeekTenant() error = %v", err)
	}
	if got, ok := getResp.(adminhttp.GetDeepSeekTenant200JSONResponse); !ok || got.Name != "default" {
		t.Fatalf("GetDeepSeekTenant() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteDeepSeekTenant(ctx, adminhttp.DeleteDeepSeekTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("DeleteDeepSeekTenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteDeepSeekTenant200JSONResponse); !ok {
		t.Fatalf("DeleteDeepSeekTenant() response = %#v", deleteResp)
	}
	if resp, err := srv.GetDeepSeekTenant(ctx, adminhttp.GetDeepSeekTenantRequestObject{Name: "default"}); err != nil {
		t.Fatalf("GetDeepSeekTenant(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetDeepSeekTenant404JSONResponse); !ok {
		t.Fatalf("GetDeepSeekTenant(missing) response = %#v", resp)
	}
}

func TestServerDeepSeekTenantUsesDedicatedStore(t *testing.T) {
	ctx := context.Background()
	dedicated := kv.NewMemory(nil)
	modelStore := kv.NewMemory(nil)
	srv := &Server{DeepSeekTenantStore: dedicated, ModelStore: modelStore}
	body := deepSeekTenantUpsert("isolated")
	response, err := srv.CreateDeepSeekTenant(ctx, adminhttp.CreateDeepSeekTenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateDeepSeekTenant() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateDeepSeekTenant200JSONResponse); !ok {
		t.Fatalf("CreateDeepSeekTenant() response = %#v", response)
	}
	if _, err := dedicated.Get(ctx, deepSeekTenantKey("isolated")); err != nil {
		t.Fatalf("dedicated store Get() error = %v", err)
	}
	if _, err := modelStore.Get(ctx, deepSeekTenantKey("isolated")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("model store Get() error = %v, want ErrNotFound", err)
	}
}

func TestServerDeepSeekTenantValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminhttp.DeepSeekTenantUpsert
	}{
		{name: "missing name", body: adminhttp.DeepSeekTenantUpsert{CredentialName: "credential"}},
		{name: "missing credential", body: adminhttp.DeepSeekTenantUpsert{Name: "tenant"}},
		{name: "relative base URL", body: adminhttp.DeepSeekTenantUpsert{Name: "tenant", CredentialName: "credential", BaseUrl: stringPtr("/v1")}},
		{name: "non HTTP base URL", body: adminhttp.DeepSeekTenantUpsert{Name: "tenant", CredentialName: "credential", BaseUrl: stringPtr("ftp://deepseek.example.com")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateDeepSeekTenant(ctx, adminhttp.CreateDeepSeekTenantRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateDeepSeekTenant() error = %v", err)
			}
			if _, ok := resp.(adminhttp.CreateDeepSeekTenant400JSONResponse); !ok {
				t.Fatalf("CreateDeepSeekTenant() response = %#v", resp)
			}
		})
	}

	body := deepSeekTenantUpsert("tenant")
	if resp, err := srv.PutDeepSeekTenant(ctx, adminhttp.PutDeepSeekTenantRequestObject{Name: "other", Body: &body}); err != nil {
		t.Fatalf("PutDeepSeekTenant(mismatch) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutDeepSeekTenant400JSONResponse); !ok {
		t.Fatalf("PutDeepSeekTenant(mismatch) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListDeepSeekTenants(ctx, adminhttp.ListDeepSeekTenantsRequestObject{}); err != nil {
		t.Fatalf("ListDeepSeekTenants(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.ListDeepSeekTenants500JSONResponse); !ok {
		t.Fatalf("ListDeepSeekTenants(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateDeepSeekTenant(ctx, adminhttp.CreateDeepSeekTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateDeepSeekTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateDeepSeekTenant500JSONResponse); !ok {
		t.Fatalf("CreateDeepSeekTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetDeepSeekTenant(ctx, adminhttp.GetDeepSeekTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("GetDeepSeekTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetDeepSeekTenant500JSONResponse); !ok {
		t.Fatalf("GetDeepSeekTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutDeepSeekTenant(ctx, adminhttp.PutDeepSeekTenantRequestObject{Name: "tenant", Body: &body}); err != nil {
		t.Fatalf("PutDeepSeekTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutDeepSeekTenant500JSONResponse); !ok {
		t.Fatalf("PutDeepSeekTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteDeepSeekTenant(ctx, adminhttp.DeleteDeepSeekTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("DeleteDeepSeekTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.DeleteDeepSeekTenant500JSONResponse); !ok {
		t.Fatalf("DeleteDeepSeekTenant(nil store) response = %#v", resp)
	}
}

func deepSeekTenantUpsert(name string) adminhttp.DeepSeekTenantUpsert {
	baseURL := "https://deepseek.example.com/" + name
	return adminhttp.DeepSeekTenantUpsert{
		BaseUrl:        &baseURL,
		CredentialName: string("credential"),
		Name:           string(name),
	}
}

func requireDeepSeekTenantList(t *testing.T, resp adminhttp.ListDeepSeekTenantsResponseObject) adminhttp.DeepSeekTenantList {
	t.Helper()
	list, ok := resp.(adminhttp.ListDeepSeekTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListDeepSeekTenants() response = %#v", resp)
	}
	return adminhttp.DeepSeekTenantList(list)
}
