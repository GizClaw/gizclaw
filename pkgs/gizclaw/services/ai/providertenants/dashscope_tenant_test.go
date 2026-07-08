package providertenants

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerDashScopeTenantCRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
	}

	body := dashScopeTenantUpsert("default")
	resp, err := srv.CreateDashScopeTenant(ctx, adminhttp.CreateDashScopeTenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateDashScopeTenant() error = %v", err)
	}
	created, ok := resp.(adminhttp.CreateDashScopeTenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateDashScopeTenant() response = %#v", resp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateDashScopeTenant() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}
	if created.BaseUrl == nil || *created.BaseUrl != "https://dashscope.example.com/default" {
		t.Fatalf("CreateDashScopeTenant() base_url = %#v", created.BaseUrl)
	}

	if resp, err := srv.CreateDashScopeTenant(ctx, adminhttp.CreateDashScopeTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateDashScopeTenant(duplicate) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateDashScopeTenant409JSONResponse); !ok {
		t.Fatalf("CreateDashScopeTenant(duplicate) response = %#v", resp)
	}
	for _, name := range []string{"alpha", "beta"} {
		body := dashScopeTenantUpsert(name)
		if resp, err := srv.CreateDashScopeTenant(ctx, adminhttp.CreateDashScopeTenantRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateDashScopeTenant(%s) error = %v", name, err)
		} else if _, ok := resp.(adminhttp.CreateDashScopeTenant200JSONResponse); !ok {
			t.Fatalf("CreateDashScopeTenant(%s) response = %#v", name, resp)
		}
	}

	limit := int32(2)
	listResp, err := srv.ListDashScopeTenants(ctx, adminhttp.ListDashScopeTenantsRequestObject{
		Params: adminhttp.ListDashScopeTenantsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListDashScopeTenants(first) error = %v", err)
	}
	firstPage := requireDashScopeTenantList(t, listResp)
	if !firstPage.HasNext || firstPage.NextCursor == nil || len(firstPage.Items) != 2 {
		t.Fatalf("ListDashScopeTenants(first) = %#v", firstPage)
	}
	cursor := string(*firstPage.NextCursor)
	listResp, err = srv.ListDashScopeTenants(ctx, adminhttp.ListDashScopeTenantsRequestObject{
		Params: adminhttp.ListDashScopeTenantsParams{Cursor: &cursor, Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListDashScopeTenants(second) error = %v", err)
	}
	secondPage := requireDashScopeTenantList(t, listResp)
	if secondPage.HasNext || secondPage.NextCursor != nil || len(secondPage.Items) != 1 {
		t.Fatalf("ListDashScopeTenants(second) = %#v", secondPage)
	}

	updated := dashScopeTenantUpsert("default")
	description := "updated tenant"
	updated.Description = &description
	now = now.Add(time.Minute)
	putResp, err := srv.PutDashScopeTenant(ctx, adminhttp.PutDashScopeTenantRequestObject{Name: "default", Body: &updated})
	if err != nil {
		t.Fatalf("PutDashScopeTenant() error = %v", err)
	}
	put, ok := putResp.(adminhttp.PutDashScopeTenant200JSONResponse)
	if !ok {
		t.Fatalf("PutDashScopeTenant() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutDashScopeTenant() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	if put.Description == nil || *put.Description != description {
		t.Fatalf("PutDashScopeTenant() description = %#v", put.Description)
	}

	getResp, err := srv.GetDashScopeTenant(ctx, adminhttp.GetDashScopeTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("GetDashScopeTenant() error = %v", err)
	}
	if got, ok := getResp.(adminhttp.GetDashScopeTenant200JSONResponse); !ok || got.Name != "default" {
		t.Fatalf("GetDashScopeTenant() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteDashScopeTenant(ctx, adminhttp.DeleteDashScopeTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("DeleteDashScopeTenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteDashScopeTenant200JSONResponse); !ok {
		t.Fatalf("DeleteDashScopeTenant() response = %#v", deleteResp)
	}
	if resp, err := srv.GetDashScopeTenant(ctx, adminhttp.GetDashScopeTenantRequestObject{Name: "default"}); err != nil {
		t.Fatalf("GetDashScopeTenant(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetDashScopeTenant404JSONResponse); !ok {
		t.Fatalf("GetDashScopeTenant(missing) response = %#v", resp)
	}
}

func TestServerDashScopeTenantValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminhttp.DashScopeTenantUpsert
	}{
		{name: "missing name", body: adminhttp.DashScopeTenantUpsert{CredentialName: "credential"}},
		{name: "missing credential", body: adminhttp.DashScopeTenantUpsert{Name: "tenant"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateDashScopeTenant(ctx, adminhttp.CreateDashScopeTenantRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateDashScopeTenant() error = %v", err)
			}
			if _, ok := resp.(adminhttp.CreateDashScopeTenant400JSONResponse); !ok {
				t.Fatalf("CreateDashScopeTenant() response = %#v", resp)
			}
		})
	}

	body := dashScopeTenantUpsert("tenant")
	if resp, err := srv.PutDashScopeTenant(ctx, adminhttp.PutDashScopeTenantRequestObject{Name: "other", Body: &body}); err != nil {
		t.Fatalf("PutDashScopeTenant(mismatch) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutDashScopeTenant400JSONResponse); !ok {
		t.Fatalf("PutDashScopeTenant(mismatch) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListDashScopeTenants(ctx, adminhttp.ListDashScopeTenantsRequestObject{}); err != nil {
		t.Fatalf("ListDashScopeTenants(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.ListDashScopeTenants500JSONResponse); !ok {
		t.Fatalf("ListDashScopeTenants(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateDashScopeTenant(ctx, adminhttp.CreateDashScopeTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("CreateDashScopeTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetDashScopeTenant(ctx, adminhttp.GetDashScopeTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("GetDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("GetDashScopeTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutDashScopeTenant(ctx, adminhttp.PutDashScopeTenantRequestObject{Name: "tenant", Body: &body}); err != nil {
		t.Fatalf("PutDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("PutDashScopeTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteDashScopeTenant(ctx, adminhttp.DeleteDashScopeTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("DeleteDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.DeleteDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("DeleteDashScopeTenant(nil store) response = %#v", resp)
	}
}

func dashScopeTenantUpsert(name string) adminhttp.DashScopeTenantUpsert {
	baseURL := "https://dashscope.example.com/" + name
	return adminhttp.DashScopeTenantUpsert{
		BaseUrl:        &baseURL,
		CredentialName: string("credential"),
		Name:           string(name),
	}
}

func requireDashScopeTenantList(t *testing.T, resp adminhttp.ListDashScopeTenantsResponseObject) adminhttp.DashScopeTenantList {
	t.Helper()
	list, ok := resp.(adminhttp.ListDashScopeTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListDashScopeTenants() response = %#v", resp)
	}
	return adminhttp.DashScopeTenantList(list)
}
