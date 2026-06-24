package providertenants

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerDashScopeTenantCRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
	}

	body := dashScopeTenantUpsert("default")
	resp, err := srv.CreateDashScopeTenant(ctx, adminservice.CreateDashScopeTenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateDashScopeTenant() error = %v", err)
	}
	created, ok := resp.(adminservice.CreateDashScopeTenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateDashScopeTenant() response = %#v", resp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateDashScopeTenant() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}
	if created.BaseUrl == nil || *created.BaseUrl != "https://dashscope.example.com/default" {
		t.Fatalf("CreateDashScopeTenant() base_url = %#v", created.BaseUrl)
	}

	if resp, err := srv.CreateDashScopeTenant(ctx, adminservice.CreateDashScopeTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateDashScopeTenant(duplicate) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateDashScopeTenant409JSONResponse); !ok {
		t.Fatalf("CreateDashScopeTenant(duplicate) response = %#v", resp)
	}
	for _, name := range []string{"alpha", "beta"} {
		body := dashScopeTenantUpsert(name)
		if resp, err := srv.CreateDashScopeTenant(ctx, adminservice.CreateDashScopeTenantRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateDashScopeTenant(%s) error = %v", name, err)
		} else if _, ok := resp.(adminservice.CreateDashScopeTenant200JSONResponse); !ok {
			t.Fatalf("CreateDashScopeTenant(%s) response = %#v", name, resp)
		}
	}

	limit := int32(2)
	listResp, err := srv.ListDashScopeTenants(ctx, adminservice.ListDashScopeTenantsRequestObject{
		Params: adminservice.ListDashScopeTenantsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListDashScopeTenants(first) error = %v", err)
	}
	firstPage := requireDashScopeTenantList(t, listResp)
	if !firstPage.HasNext || firstPage.NextCursor == nil || len(firstPage.Items) != 2 {
		t.Fatalf("ListDashScopeTenants(first) = %#v", firstPage)
	}
	cursor := string(*firstPage.NextCursor)
	listResp, err = srv.ListDashScopeTenants(ctx, adminservice.ListDashScopeTenantsRequestObject{
		Params: adminservice.ListDashScopeTenantsParams{Cursor: &cursor, Limit: &limit},
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
	putResp, err := srv.PutDashScopeTenant(ctx, adminservice.PutDashScopeTenantRequestObject{Name: "default", Body: &updated})
	if err != nil {
		t.Fatalf("PutDashScopeTenant() error = %v", err)
	}
	put, ok := putResp.(adminservice.PutDashScopeTenant200JSONResponse)
	if !ok {
		t.Fatalf("PutDashScopeTenant() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutDashScopeTenant() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	if put.Description == nil || *put.Description != description {
		t.Fatalf("PutDashScopeTenant() description = %#v", put.Description)
	}

	getResp, err := srv.GetDashScopeTenant(ctx, adminservice.GetDashScopeTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("GetDashScopeTenant() error = %v", err)
	}
	if got, ok := getResp.(adminservice.GetDashScopeTenant200JSONResponse); !ok || got.Name != "default" {
		t.Fatalf("GetDashScopeTenant() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteDashScopeTenant(ctx, adminservice.DeleteDashScopeTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("DeleteDashScopeTenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteDashScopeTenant200JSONResponse); !ok {
		t.Fatalf("DeleteDashScopeTenant() response = %#v", deleteResp)
	}
	if resp, err := srv.GetDashScopeTenant(ctx, adminservice.GetDashScopeTenantRequestObject{Name: "default"}); err != nil {
		t.Fatalf("GetDashScopeTenant(missing) error = %v", err)
	} else if _, ok := resp.(adminservice.GetDashScopeTenant404JSONResponse); !ok {
		t.Fatalf("GetDashScopeTenant(missing) response = %#v", resp)
	}
}

func TestServerDashScopeTenantValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminservice.DashScopeTenantUpsert
	}{
		{name: "missing name", body: adminservice.DashScopeTenantUpsert{CredentialName: "credential"}},
		{name: "missing credential", body: adminservice.DashScopeTenantUpsert{Name: "tenant"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateDashScopeTenant(ctx, adminservice.CreateDashScopeTenantRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateDashScopeTenant() error = %v", err)
			}
			if _, ok := resp.(adminservice.CreateDashScopeTenant400JSONResponse); !ok {
				t.Fatalf("CreateDashScopeTenant() response = %#v", resp)
			}
		})
	}

	body := dashScopeTenantUpsert("tenant")
	if resp, err := srv.PutDashScopeTenant(ctx, adminservice.PutDashScopeTenantRequestObject{Name: "other", Body: &body}); err != nil {
		t.Fatalf("PutDashScopeTenant(mismatch) error = %v", err)
	} else if _, ok := resp.(adminservice.PutDashScopeTenant400JSONResponse); !ok {
		t.Fatalf("PutDashScopeTenant(mismatch) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListDashScopeTenants(ctx, adminservice.ListDashScopeTenantsRequestObject{}); err != nil {
		t.Fatalf("ListDashScopeTenants(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.ListDashScopeTenants500JSONResponse); !ok {
		t.Fatalf("ListDashScopeTenants(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateDashScopeTenant(ctx, adminservice.CreateDashScopeTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("CreateDashScopeTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetDashScopeTenant(ctx, adminservice.GetDashScopeTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("GetDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.GetDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("GetDashScopeTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutDashScopeTenant(ctx, adminservice.PutDashScopeTenantRequestObject{Name: "tenant", Body: &body}); err != nil {
		t.Fatalf("PutDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.PutDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("PutDashScopeTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteDashScopeTenant(ctx, adminservice.DeleteDashScopeTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("DeleteDashScopeTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.DeleteDashScopeTenant500JSONResponse); !ok {
		t.Fatalf("DeleteDashScopeTenant(nil store) response = %#v", resp)
	}
}

func dashScopeTenantUpsert(name string) adminservice.DashScopeTenantUpsert {
	baseURL := "https://dashscope.example.com/" + name
	return adminservice.DashScopeTenantUpsert{
		BaseUrl:        &baseURL,
		CredentialName: string("credential"),
		Name:           string(name),
	}
}

func requireDashScopeTenantList(t *testing.T, resp adminservice.ListDashScopeTenantsResponseObject) adminservice.DashScopeTenantList {
	t.Helper()
	list, ok := resp.(adminservice.ListDashScopeTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListDashScopeTenants() response = %#v", resp)
	}
	return adminservice.DashScopeTenantList(list)
}
