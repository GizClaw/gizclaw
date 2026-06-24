package providertenants

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerGeminiTenantCRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
	}

	body := geminiTenantUpsert("default")
	resp, err := srv.CreateGeminiTenant(ctx, adminservice.CreateGeminiTenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateGeminiTenant() error = %v", err)
	}
	created, ok := resp.(adminservice.CreateGeminiTenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateGeminiTenant() response = %#v", resp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateGeminiTenant() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}
	if created.ProjectId == nil || *created.ProjectId != "project-default" {
		t.Fatalf("CreateGeminiTenant() project_id = %#v", created.ProjectId)
	}

	if resp, err := srv.CreateGeminiTenant(ctx, adminservice.CreateGeminiTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateGeminiTenant(duplicate) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateGeminiTenant409JSONResponse); !ok {
		t.Fatalf("CreateGeminiTenant(duplicate) response = %#v", resp)
	}
	for _, name := range []string{"alpha", "beta"} {
		body := geminiTenantUpsert(name)
		if resp, err := srv.CreateGeminiTenant(ctx, adminservice.CreateGeminiTenantRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateGeminiTenant(%s) error = %v", name, err)
		} else if _, ok := resp.(adminservice.CreateGeminiTenant200JSONResponse); !ok {
			t.Fatalf("CreateGeminiTenant(%s) response = %#v", name, resp)
		}
	}

	limit := int32(2)
	listResp, err := srv.ListGeminiTenants(ctx, adminservice.ListGeminiTenantsRequestObject{
		Params: adminservice.ListGeminiTenantsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListGeminiTenants(first) error = %v", err)
	}
	firstPage := requireGeminiTenantList(t, listResp)
	if !firstPage.HasNext || firstPage.NextCursor == nil || len(firstPage.Items) != 2 {
		t.Fatalf("ListGeminiTenants(first) = %#v", firstPage)
	}
	cursor := string(*firstPage.NextCursor)
	listResp, err = srv.ListGeminiTenants(ctx, adminservice.ListGeminiTenantsRequestObject{
		Params: adminservice.ListGeminiTenantsParams{Cursor: &cursor, Limit: &limit},
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
	putResp, err := srv.PutGeminiTenant(ctx, adminservice.PutGeminiTenantRequestObject{Name: "default", Body: &updated})
	if err != nil {
		t.Fatalf("PutGeminiTenant() error = %v", err)
	}
	put, ok := putResp.(adminservice.PutGeminiTenant200JSONResponse)
	if !ok {
		t.Fatalf("PutGeminiTenant() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutGeminiTenant() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	if put.Description == nil || *put.Description != description {
		t.Fatalf("PutGeminiTenant() description = %#v", put.Description)
	}

	getResp, err := srv.GetGeminiTenant(ctx, adminservice.GetGeminiTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("GetGeminiTenant() error = %v", err)
	}
	if got, ok := getResp.(adminservice.GetGeminiTenant200JSONResponse); !ok || got.Name != "default" {
		t.Fatalf("GetGeminiTenant() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteGeminiTenant(ctx, adminservice.DeleteGeminiTenantRequestObject{Name: "default"})
	if err != nil {
		t.Fatalf("DeleteGeminiTenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteGeminiTenant200JSONResponse); !ok {
		t.Fatalf("DeleteGeminiTenant() response = %#v", deleteResp)
	}
	if resp, err := srv.GetGeminiTenant(ctx, adminservice.GetGeminiTenantRequestObject{Name: "default"}); err != nil {
		t.Fatalf("GetGeminiTenant(missing) error = %v", err)
	} else if _, ok := resp.(adminservice.GetGeminiTenant404JSONResponse); !ok {
		t.Fatalf("GetGeminiTenant(missing) response = %#v", resp)
	}
}

func TestServerGeminiTenantValidationAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminservice.GeminiTenantUpsert
	}{
		{name: "missing name", body: adminservice.GeminiTenantUpsert{CredentialName: "credential"}},
		{name: "missing credential", body: adminservice.GeminiTenantUpsert{Name: "tenant"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateGeminiTenant(ctx, adminservice.CreateGeminiTenantRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateGeminiTenant() error = %v", err)
			}
			if _, ok := resp.(adminservice.CreateGeminiTenant400JSONResponse); !ok {
				t.Fatalf("CreateGeminiTenant() response = %#v", resp)
			}
		})
	}

	body := geminiTenantUpsert("tenant")
	if resp, err := srv.PutGeminiTenant(ctx, adminservice.PutGeminiTenantRequestObject{Name: "other", Body: &body}); err != nil {
		t.Fatalf("PutGeminiTenant(mismatch) error = %v", err)
	} else if _, ok := resp.(adminservice.PutGeminiTenant400JSONResponse); !ok {
		t.Fatalf("PutGeminiTenant(mismatch) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListGeminiTenants(ctx, adminservice.ListGeminiTenantsRequestObject{}); err != nil {
		t.Fatalf("ListGeminiTenants(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.ListGeminiTenants500JSONResponse); !ok {
		t.Fatalf("ListGeminiTenants(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateGeminiTenant(ctx, adminservice.CreateGeminiTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateGeminiTenant500JSONResponse); !ok {
		t.Fatalf("CreateGeminiTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetGeminiTenant(ctx, adminservice.GetGeminiTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("GetGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.GetGeminiTenant500JSONResponse); !ok {
		t.Fatalf("GetGeminiTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutGeminiTenant(ctx, adminservice.PutGeminiTenantRequestObject{Name: "tenant", Body: &body}); err != nil {
		t.Fatalf("PutGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.PutGeminiTenant500JSONResponse); !ok {
		t.Fatalf("PutGeminiTenant(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteGeminiTenant(ctx, adminservice.DeleteGeminiTenantRequestObject{Name: "tenant"}); err != nil {
		t.Fatalf("DeleteGeminiTenant(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.DeleteGeminiTenant500JSONResponse); !ok {
		t.Fatalf("DeleteGeminiTenant(nil store) response = %#v", resp)
	}
}

func geminiTenantUpsert(name string) adminservice.GeminiTenantUpsert {
	projectID := "project-" + name
	location := "global"
	return adminservice.GeminiTenantUpsert{
		CredentialName: string("credential"),
		Location:       &location,
		Name:           string(name),
		ProjectId:      &projectID,
	}
}

func requireGeminiTenantList(t *testing.T, resp adminservice.ListGeminiTenantsResponseObject) adminservice.GeminiTenantList {
	t.Helper()
	list, ok := resp.(adminservice.ListGeminiTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListGeminiTenants() response = %#v", resp)
	}
	return adminservice.GeminiTenantList(list)
}
