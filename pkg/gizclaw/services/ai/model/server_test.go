package model

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestServerModelCRUDListFiltersAndIndexes(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
	}
	first := modelUpsert("qwen-flash", "openai-tenant", "dashscope")
	first.Name = stringPtr("Qwen Flash")
	levels := []string{"low", "medium"}
	first.Capabilities = &apitypes.ModelCapabilities{
		Thinking: &apitypes.ModelThinkingCapability{
			Supported: true,
			Levels:    &levels,
		},
	}
	first.ProviderData = openAIProviderData("https://dashscope.aliyuncs.com/compatible-mode/v1")
	second := modelUpsert("speech", "openai-tenant", "global")

	resp, err := srv.CreateModel(ctx, adminservice.CreateModelRequestObject{Body: &first})
	if err != nil {
		t.Fatalf("CreateModel() error = %v", err)
	}
	created, ok := resp.(adminservice.CreateModel200JSONResponse)
	if !ok {
		t.Fatalf("CreateModel() response = %#v", resp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateModel() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}
	if created.Name == nil || *created.Name != "Qwen Flash" {
		t.Fatalf("CreateModel() name = %#v", created.Name)
	}
	if resp, err := srv.CreateModel(ctx, adminservice.CreateModelRequestObject{Body: &first}); err != nil {
		t.Fatalf("CreateModel(duplicate) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateModel409JSONResponse); !ok {
		t.Fatalf("CreateModel(duplicate) response = %#v", resp)
	}
	if resp, err := srv.CreateModel(ctx, adminservice.CreateModelRequestObject{Body: &second}); err != nil {
		t.Fatalf("CreateModel(second) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateModel200JSONResponse); !ok {
		t.Fatalf("CreateModel(second) response = %#v", resp)
	}

	listResp, err := srv.ListModels(ctx, adminservice.ListModelsRequestObject{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	listed := requireModelList(t, listResp)
	if len(listed.Items) != 2 {
		t.Fatalf("ListModels() items = %#v", listed.Items)
	}

	providerKind := adminservice.ModelProviderKind("openai-tenant")
	providerName := string("global")
	providerResp, err := srv.ListModels(ctx, adminservice.ListModelsRequestObject{
		Params: adminservice.ListModelsParams{ProviderKind: &providerKind, ProviderName: &providerName},
	})
	if err != nil {
		t.Fatalf("ListModels(provider) error = %v", err)
	}
	providerListed := requireModelList(t, providerResp)
	if len(providerListed.Items) != 1 || providerListed.Items[0].Id != "speech" {
		t.Fatalf("ListModels(provider) items = %#v", providerListed.Items)
	}

	updated := first
	updated.Provider = apitypes.ModelProvider{Kind: "openai-tenant", Name: "global"}
	now = now.Add(time.Minute)
	putResp, err := srv.PutModel(ctx, adminservice.PutModelRequestObject{Id: "qwen-flash", Body: &updated})
	if err != nil {
		t.Fatalf("PutModel() error = %v", err)
	}
	put, ok := putResp.(adminservice.PutModel200JSONResponse)
	if !ok {
		t.Fatalf("PutModel() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutModel() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	getResp, err := srv.GetModel(ctx, adminservice.GetModelRequestObject{Id: "qwen-flash"})
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if got, ok := getResp.(adminservice.GetModel200JSONResponse); !ok || got.Provider.Name != "global" {
		t.Fatalf("GetModel() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteModel(ctx, adminservice.DeleteModelRequestObject{Id: "qwen-flash"})
	if err != nil {
		t.Fatalf("DeleteModel() error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteModel200JSONResponse); !ok {
		t.Fatalf("DeleteModel() response = %#v", deleteResp)
	}
	missingResp, err := srv.GetModel(ctx, adminservice.GetModelRequestObject{Id: "qwen-flash"})
	if err != nil {
		t.Fatalf("GetModel(missing) error = %v", err)
	}
	if _, ok := missingResp.(adminservice.GetModel404JSONResponse); !ok {
		t.Fatalf("GetModel(missing) response = %#v", missingResp)
	}
}

func TestServerListModelsPagination(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, id := range []string{"a", "b", "c"} {
		upsert := modelUpsert(id, "openai-tenant", "main")
		if resp, err := srv.CreateModel(ctx, adminservice.CreateModelRequestObject{Body: &upsert}); err != nil {
			t.Fatalf("CreateModel(%s) error = %v", id, err)
		} else if _, ok := resp.(adminservice.CreateModel200JSONResponse); !ok {
			t.Fatalf("CreateModel(%s) response = %#v", id, resp)
		}
	}
	limit := int32(2)
	firstResp, err := srv.ListModels(ctx, adminservice.ListModelsRequestObject{
		Params: adminservice.ListModelsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListModels(first) error = %v", err)
	}
	first := requireModelList(t, firstResp)
	if !first.HasNext || first.NextCursor == nil || len(first.Items) != 2 {
		t.Fatalf("ListModels(first) = %#v", first)
	}
	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListModels(ctx, adminservice.ListModelsRequestObject{
		Params: adminservice.ListModelsParams{Cursor: &cursor, Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListModels(second) error = %v", err)
	}
	second := requireModelList(t, secondResp)
	if second.HasNext || second.NextCursor != nil || len(second.Items) != 1 || second.Items[0].Id != "c" {
		t.Fatalf("ListModels(second) = %#v", second)
	}
}

func TestServerListModelsEmptyReturnsEmptyItems(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}

	resp, err := srv.ListModels(ctx, adminservice.ListModelsRequestObject{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	listed := requireModelList(t, resp)
	if listed.Items == nil {
		t.Fatal("ListModels() items is nil, want empty slice")
	}
	if len(listed.Items) != 0 {
		t.Fatalf("ListModels() items = %#v, want empty", listed.Items)
	}
}

func TestServerRejectsInvalidAndSyncModelWrites(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	if resp, err := srv.CreateModel(ctx, adminservice.CreateModelRequestObject{}); err != nil {
		t.Fatalf("CreateModel(nil) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateModel400JSONResponse); !ok {
		t.Fatalf("CreateModel(nil) response = %#v", resp)
	}
	syncModel := apitypes.Model{
		Id:        "synced",
		Source:    apitypes.ModelSourceSync,
		Kind:      apitypes.ModelKindLlm,
		Provider:  apitypes.ModelProvider{Kind: "sync-provider", Name: "main"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := writeModel(ctx, srv.Store, syncModel, nil); err != nil {
		t.Fatalf("writeModel(sync) error = %v", err)
	}
	manual := modelUpsert("synced", "openai-tenant", "main")
	if resp, err := srv.PutModel(ctx, adminservice.PutModelRequestObject{Id: "synced", Body: &manual}); err != nil {
		t.Fatalf("PutModel(sync) error = %v", err)
	} else if _, ok := resp.(adminservice.PutModel409JSONResponse); !ok {
		t.Fatalf("PutModel(sync) response = %#v", resp)
	}
}

func TestServerModelValidationAndErrorResponses(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminservice.ModelUpsert
	}{
		{name: "missing id", body: adminservice.ModelUpsert{Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "openai-tenant", Name: "main"}}},
		{name: "missing kind", body: adminservice.ModelUpsert{Id: "kind", Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "openai-tenant", Name: "main"}}},
		{name: "sync source", body: adminservice.ModelUpsert{Id: "sync", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceSync, Provider: apitypes.ModelProvider{Kind: "openai-tenant", Name: "main"}}},
		{name: "missing provider kind", body: adminservice.ModelUpsert{Id: "provider", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Name: "main"}}},
		{name: "missing provider name", body: adminservice.ModelUpsert{Id: "provider", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "openai-tenant"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateModel(ctx, adminservice.CreateModelRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateModel() error = %v", err)
			}
			if _, ok := resp.(adminservice.CreateModel400JSONResponse); !ok {
				t.Fatalf("CreateModel() response = %#v", resp)
			}
		})
	}

	base := modelUpsert("manual", "openai-tenant", "main")
	if resp, err := srv.PutModel(ctx, adminservice.PutModelRequestObject{Id: "other", Body: &base}); err != nil {
		t.Fatalf("PutModel(id mismatch) error = %v", err)
	} else if _, ok := resp.(adminservice.PutModel400JSONResponse); !ok {
		t.Fatalf("PutModel(id mismatch) response = %#v", resp)
	}
	if resp, err := srv.DeleteModel(ctx, adminservice.DeleteModelRequestObject{Id: "missing"}); err != nil {
		t.Fatalf("DeleteModel(missing) error = %v", err)
	} else if _, ok := resp.(adminservice.DeleteModel404JSONResponse); !ok {
		t.Fatalf("DeleteModel(missing) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListModels(ctx, adminservice.ListModelsRequestObject{}); err != nil {
		t.Fatalf("ListModels(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.ListModels500JSONResponse); !ok {
		t.Fatalf("ListModels(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateModel(ctx, adminservice.CreateModelRequestObject{Body: &base}); err != nil {
		t.Fatalf("CreateModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.CreateModel500JSONResponse); !ok {
		t.Fatalf("CreateModel(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetModel(ctx, adminservice.GetModelRequestObject{Id: "x"}); err != nil {
		t.Fatalf("GetModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.GetModel500JSONResponse); !ok {
		t.Fatalf("GetModel(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutModel(ctx, adminservice.PutModelRequestObject{Id: "x", Body: &base}); err != nil {
		t.Fatalf("PutModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.PutModel500JSONResponse); !ok {
		t.Fatalf("PutModel(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteModel(ctx, adminservice.DeleteModelRequestObject{Id: "x"}); err != nil {
		t.Fatalf("DeleteModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminservice.DeleteModel500JSONResponse); !ok {
		t.Fatalf("DeleteModel(nil store) response = %#v", resp)
	}
}

func TestServerListModelsSourceFilterAndSyncedTimePreserved(t *testing.T) {
	ctx := context.Background()
	syncedAt := time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC)
	srv := &Server{Store: kv.NewMemory(nil)}
	previous := apitypes.Model{
		Id:       "sync-preserved",
		Kind:     apitypes.ModelKindLlm,
		Provider: apitypes.ModelProvider{Kind: "openai-tenant", Name: "main"},
		Source:   apitypes.ModelSourceManual,
		SyncedAt: &syncedAt,
	}
	if err := writeModel(ctx, srv.Store, previous, nil); err != nil {
		t.Fatalf("writeModel() error = %v", err)
	}
	update := modelUpsert("sync-preserved", "openai-tenant", "main")
	resp, err := srv.PutModel(ctx, adminservice.PutModelRequestObject{Id: "sync-preserved", Body: &update})
	if err != nil {
		t.Fatalf("PutModel() error = %v", err)
	}
	stored, ok := resp.(adminservice.PutModel200JSONResponse)
	if !ok {
		t.Fatalf("PutModel() response = %#v", resp)
	}
	if stored.SyncedAt == nil || !stored.SyncedAt.Equal(syncedAt) {
		t.Fatalf("PutModel() synced_at = %#v", stored.SyncedAt)
	}

	source := adminservice.ModelSource(apitypes.ModelSourceManual)
	sourceResp, err := srv.ListModels(ctx, adminservice.ListModelsRequestObject{
		Params: adminservice.ListModelsParams{Source: &source},
	})
	if err != nil {
		t.Fatalf("ListModels(source) error = %v", err)
	}
	sourceList := requireModelList(t, sourceResp)
	if len(sourceList.Items) != 1 || sourceList.Items[0].Id != "sync-preserved" {
		t.Fatalf("ListModels(source) = %#v", sourceList.Items)
	}
}

func modelUpsert(id string, providerKind, providerName string) adminservice.ModelUpsert {
	return adminservice.ModelUpsert{
		Id:     string(id),
		Kind:   apitypes.ModelKindLlm,
		Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKind(providerKind),
			Name: string(providerName),
		},
	}
}

func openAIProviderData(baseURL string) *apitypes.ModelProviderData {
	_ = baseURL
	out := apitypes.ModelProviderData{}
	if err := out.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{}); err != nil {
		panic(err)
	}
	return &out
}

func requireModelList(t *testing.T, resp adminservice.ListModelsResponseObject) adminservice.ModelList {
	t.Helper()
	list, ok := resp.(adminservice.ListModels200JSONResponse)
	if !ok {
		t.Fatalf("ListModels() response = %#v", resp)
	}
	return adminservice.ModelList(list)
}

func stringPtr(value string) *string {
	return &value
}
