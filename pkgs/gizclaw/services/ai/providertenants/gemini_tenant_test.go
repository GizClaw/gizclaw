package providertenants

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerGeminiTenantCRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
	}

	body := geminiTenantUpsert("default")
	resp, err := srv.CreateGeminiTenant(ctx, adminhttp.CreateGeminiTenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateGeminiTenant() error = %v", err)
	}
	created, ok := resp.(adminhttp.CreateGeminiTenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateGeminiTenant() response = %#v", resp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateGeminiTenant() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}
	if created.ProjectId == nil || *created.ProjectId != "project-default" {
		t.Fatalf("CreateGeminiTenant() project_id = %#v", created.ProjectId)
	}

	if resp, err := srv.CreateGeminiTenant(ctx, adminhttp.CreateGeminiTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateGeminiTenant(duplicate) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateGeminiTenant409JSONResponse); !ok {
		t.Fatalf("CreateGeminiTenant(duplicate) response = %#v", resp)
	}
	for _, name := range []string{"alpha", "beta"} {
		body := geminiTenantUpsert(name)
		if resp, err := srv.CreateGeminiTenant(ctx, adminhttp.CreateGeminiTenantRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateGeminiTenant(%s) error = %v", name, err)
		} else if _, ok := resp.(adminhttp.CreateGeminiTenant200JSONResponse); !ok {
			t.Fatalf("CreateGeminiTenant(%s) response = %#v", name, resp)
		}
	}

	limit := int32(2)
	listResp, err := srv.ListGeminiTenants(ctx, adminhttp.ListGeminiTenantsRequestObject{
		Params: adminhttp.ListGeminiTenantsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListGeminiTenants(first) error = %v", err)
	}
	firstPage := requireGeminiTenantList(t, listResp)
	if !firstPage.HasNext || firstPage.NextCursor == nil || len(firstPage.Items) != 2 {
		t.Fatalf("ListGeminiTenants(first) = %#v", firstPage)
	}
	cursor := string(*firstPage.NextCursor)
	listResp, err = srv.ListGeminiTenants(ctx, adminhttp.ListGeminiTenantsRequestObject{
		Params: adminhttp.ListGeminiTenantsParams{Cursor: &cursor, Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListGeminiTenants(second) error = %v", err)
	}
	secondPage := requireGeminiTenantList(t, listResp)
	if secondPage.HasNext || secondPage.NextCursor != nil || len(secondPage.Items) != 1 {
		t.Fatalf("ListGeminiTenants(second) = %#v", secondPage)
	}

	updated := geminiTenantUpsert("default")
	description := "updated tenant"
	updated.Description = &description
	now = now.Add(time.Minute)
	putResp, err := srv.PutGeminiTenant(ctx, adminhttp.PutGeminiTenantRequestObject{Name: "default", Body: &updated})
	if err != nil {
		t.Fatalf("PutGeminiTenant() error = %v", err)
	}
	put, ok := putResp.(adminhttp.PutGeminiTenant200JSONResponse)
	if !ok {
		t.Fatalf("PutGeminiTenant() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutGeminiTenant() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	if put.Description == nil || *put.Description != description {
		t.Fatalf("PutGeminiTenant() description = %#v", put.Description)
	}

	getResp, err := srv.GetGeminiTenant(ctx, adminhttp.GetGeminiTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("GetGeminiTenant() error = %v", err)
	}
	if got, ok := getResp.(adminhttp.GetGeminiTenant200JSONResponse); !ok || got.Name != "default" {
		t.Fatalf("GetGeminiTenant() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteGeminiTenant(ctx, adminhttp.DeleteGeminiTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("DeleteGeminiTenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteGeminiTenant200JSONResponse); !ok {
		t.Fatalf("DeleteGeminiTenant() response = %#v", deleteResp)
	}
	if resp, err := srv.GetGeminiTenant(ctx, adminhttp.GetGeminiTenantRequestObject{Name: "default"}); err != nil {
		t.Fatalf("GetGeminiTenant(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetGeminiTenant404JSONResponse); !ok {
		t.Fatalf("GetGeminiTenant(missing) response = %#v", resp)
	}
}

func TestServerGeminiTenantValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminhttp.GeminiTenantUpsert
	}{
		{name: "missing name", body: adminhttp.GeminiTenantUpsert{CredentialName: "credential"}},
		{name: "missing credential", body: adminhttp.GeminiTenantUpsert{Name: "tenant"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateGeminiTenant(ctx, adminhttp.CreateGeminiTenantRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateGeminiTenant() error = %v", err)
			}
			if _, ok := resp.(adminhttp.CreateGeminiTenant400JSONResponse); !ok {
				t.Fatalf("CreateGeminiTenant() response = %#v", resp)
			}
		})
	}

	body := geminiTenantUpsert("tenant")
	if resp, err := srv.PutGeminiTenant(ctx, adminhttp.PutGeminiTenantRequestObject{Name: "other", Body: &body}); err != nil {
		t.Fatalf("PutGeminiTenant(mismatch) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutGeminiTenant400JSONResponse); !ok {
		t.Fatalf("PutGeminiTenant(mismatch) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListGeminiTenants(ctx, adminhttp.ListGeminiTenantsRequestObject{}); err != nil {
		t.Fatalf("ListGeminiTenants(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.ListGeminiTenants500JSONResponse); !ok {
		t.Fatalf("ListGeminiTenants(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateGeminiTenant(ctx, adminhttp.CreateGeminiTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateGeminiTenant500JSONResponse); !ok {
		t.Fatalf("CreateGeminiTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetGeminiTenant(ctx, adminhttp.GetGeminiTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("GetGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetGeminiTenant500JSONResponse); !ok {
		t.Fatalf("GetGeminiTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutGeminiTenant(ctx, adminhttp.PutGeminiTenantRequestObject{Name: "tenant", Body: &body}); err != nil {
		t.Fatalf("PutGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutGeminiTenant500JSONResponse); !ok {
		t.Fatalf("PutGeminiTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteGeminiTenant(ctx, adminhttp.DeleteGeminiTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("DeleteGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.DeleteGeminiTenant500JSONResponse); !ok {
		t.Fatalf("DeleteGeminiTenant(nil store) response = %#v", resp)
	}
}

func geminiTenantUpsert(name string) adminhttp.GeminiTenantUpsert {
	projectID := "project-" + name
	location := "global"
	return adminhttp.GeminiTenantUpsert{
		CredentialName: string("credential"),
		Location:       &location,
		Name:           string(name),
		ProjectId:      &projectID,
	}
}

func requireGeminiTenantList(t *testing.T, resp adminhttp.ListGeminiTenantsResponseObject) adminhttp.GeminiTenantList {
	t.Helper()
	list, ok := resp.(adminhttp.ListGeminiTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListGeminiTenants() response = %#v", resp)
	}
	return adminhttp.GeminiTenantList(list)
}
