package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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
	first.ProviderData = openAIProviderData("qwen-flash")
	second := modelUpsert("speech", "openai-tenant", "global")

	resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &first})
	if err != nil {
		t.Fatalf("CreateModel() error = %v", err)
	}
	created, ok := resp.(adminhttp.CreateModel200JSONResponse)
	if !ok {
		t.Fatalf("CreateModel() response = %#v", resp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("CreateModel() timestamps = %s %s", created.CreatedAt, created.UpdatedAt)
	}
	if created.Name == nil || *created.Name != "Qwen Flash" {
		t.Fatalf("CreateModel() name = %#v", created.Name)
	}
	if resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &first}); err != nil {
		t.Fatalf("CreateModel(duplicate) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateModel409JSONResponse); !ok {
		t.Fatalf("CreateModel(duplicate) response = %#v", resp)
	}
	if resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &second}); err != nil {
		t.Fatalf("CreateModel(second) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateModel200JSONResponse); !ok {
		t.Fatalf("CreateModel(second) response = %#v", resp)
	}

	listResp, err := srv.ListModels(ctx, adminhttp.ListModelsRequestObject{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	listed := requireModelList(t, listResp)
	if len(listed.Items) != 2 {
		t.Fatalf("ListModels() items = %#v", listed.Items)
	}

	providerKind := adminhttp.ModelProviderKind("openai-tenant")
	providerName := string("global")
	providerResp, err := srv.ListModels(ctx, adminhttp.ListModelsRequestObject{
		Params: adminhttp.ListModelsParams{ProviderKind: &providerKind, ProviderName: &providerName},
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
	putResp, err := srv.PutModel(ctx, adminhttp.PutModelRequestObject{Id: "qwen-flash", Body: &updated})
	if err != nil {
		t.Fatalf("PutModel() error = %v", err)
	}
	put, ok := putResp.(adminhttp.PutModel200JSONResponse)
	if !ok {
		t.Fatalf("PutModel() response = %#v", putResp)
	}
	if put.CreatedAt != created.CreatedAt || put.UpdatedAt != now {
		t.Fatalf("PutModel() timestamps = %s %s", put.CreatedAt, put.UpdatedAt)
	}
	getResp, err := srv.GetModel(ctx, adminhttp.GetModelRequestObject{Id: "qwen-flash"})
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if got, ok := getResp.(adminhttp.GetModel200JSONResponse); !ok || got.Provider.Name != "global" {
		t.Fatalf("GetModel() response = %#v", getResp)
	}
	deleteResp, err := srv.DeleteModel(ctx, adminhttp.DeleteModelRequestObject{Id: "qwen-flash"})
	if err != nil {
		t.Fatalf("DeleteModel() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteModel200JSONResponse); !ok {
		t.Fatalf("DeleteModel() response = %#v", deleteResp)
	}
	missingResp, err := srv.GetModel(ctx, adminhttp.GetModelRequestObject{Id: "qwen-flash"})
	if err != nil {
		t.Fatalf("GetModel(missing) error = %v", err)
	}
	if _, ok := missingResp.(adminhttp.GetModel404JSONResponse); !ok {
		t.Fatalf("GetModel(missing) response = %#v", missingResp)
	}
}

func TestServerListModelsPagination(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, id := range []string{"a", "b", "c"} {
		upsert := modelUpsert(id, "openai-tenant", "main")
		if resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &upsert}); err != nil {
			t.Fatalf("CreateModel(%s) error = %v", id, err)
		} else if _, ok := resp.(adminhttp.CreateModel200JSONResponse); !ok {
			t.Fatalf("CreateModel(%s) response = %#v", id, resp)
		}
	}
	limit := int32(2)
	firstResp, err := srv.ListModels(ctx, adminhttp.ListModelsRequestObject{
		Params: adminhttp.ListModelsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListModels(first) error = %v", err)
	}
	first := requireModelList(t, firstResp)
	if !first.HasNext || first.NextCursor == nil || len(first.Items) != 2 {
		t.Fatalf("ListModels(first) = %#v", first)
	}
	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListModels(ctx, adminhttp.ListModelsRequestObject{
		Params: adminhttp.ListModelsParams{Cursor: &cursor, Limit: &limit},
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

	resp, err := srv.ListModels(ctx, adminhttp.ListModelsRequestObject{})
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
	if resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{}); err != nil {
		t.Fatalf("CreateModel(nil) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateModel400JSONResponse); !ok {
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
	if resp, err := srv.PutModel(ctx, adminhttp.PutModelRequestObject{Id: "synced", Body: &manual}); err != nil {
		t.Fatalf("PutModel(sync) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutModel409JSONResponse); !ok {
		t.Fatalf("PutModel(sync) response = %#v", resp)
	}
}

func TestServerModelValidationAndErrorResponses(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}
	for _, tc := range []struct {
		name string
		body adminhttp.ModelUpsert
	}{
		{name: "missing id", body: adminhttp.ModelUpsert{Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "openai-tenant", Name: "main"}}},
		{name: "missing kind", body: adminhttp.ModelUpsert{Id: "kind", Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "openai-tenant", Name: "main"}}},
		{name: "sync source", body: adminhttp.ModelUpsert{Id: "sync", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceSync, Provider: apitypes.ModelProvider{Kind: "openai-tenant", Name: "main"}}},
		{name: "missing provider kind", body: adminhttp.ModelUpsert{Id: "provider", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Name: "main"}}},
		{name: "missing provider name", body: adminhttp.ModelUpsert{Id: "provider", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: "openai-tenant"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &tc.body})
			if err != nil {
				t.Fatalf("CreateModel() error = %v", err)
			}
			if _, ok := resp.(adminhttp.CreateModel400JSONResponse); !ok {
				t.Fatalf("CreateModel() response = %#v", resp)
			}
		})
	}

	base := modelUpsert("manual", "openai-tenant", "main")
	if resp, err := srv.PutModel(ctx, adminhttp.PutModelRequestObject{Id: "other", Body: &base}); err != nil {
		t.Fatalf("PutModel(id mismatch) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutModel400JSONResponse); !ok {
		t.Fatalf("PutModel(id mismatch) response = %#v", resp)
	}
	if resp, err := srv.DeleteModel(ctx, adminhttp.DeleteModelRequestObject{Id: "missing"}); err != nil {
		t.Fatalf("DeleteModel(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.DeleteModel404JSONResponse); !ok {
		t.Fatalf("DeleteModel(missing) response = %#v", resp)
	}

	badStore := &Server{}
	if resp, err := badStore.ListModels(ctx, adminhttp.ListModelsRequestObject{}); err != nil {
		t.Fatalf("ListModels(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.ListModels500JSONResponse); !ok {
		t.Fatalf("ListModels(nil store) response = %#v", resp)
	}
	if resp, err := badStore.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &base}); err != nil {
		t.Fatalf("CreateModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateModel500JSONResponse); !ok {
		t.Fatalf("CreateModel(nil store) response = %#v", resp)
	}
	if resp, err := badStore.GetModel(ctx, adminhttp.GetModelRequestObject{Id: "x"}); err != nil {
		t.Fatalf("GetModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetModel500JSONResponse); !ok {
		t.Fatalf("GetModel(nil store) response = %#v", resp)
	}
	if resp, err := badStore.PutModel(ctx, adminhttp.PutModelRequestObject{Id: "x", Body: &base}); err != nil {
		t.Fatalf("PutModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutModel500JSONResponse); !ok {
		t.Fatalf("PutModel(nil store) response = %#v", resp)
	}
	if resp, err := badStore.DeleteModel(ctx, adminhttp.DeleteModelRequestObject{Id: "x"}); err != nil {
		t.Fatalf("DeleteModel(nil store) error = %v", err)
	} else if _, ok := resp.(adminhttp.DeleteModel500JSONResponse); !ok {
		t.Fatalf("DeleteModel(nil store) response = %#v", resp)
	}
}

func TestServerValidatesProviderKindAgainstProviderData(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil)}

	dashScopeMode := apitypes.DashScopeTenantModelProviderDataApiModeChatCompletions
	volcMode := apitypes.VolcTenantModelProviderDataApiModeChatCompletions
	falseValue := false
	valid := []adminhttp.ModelUpsert{
		modelUpsert("openai-chat", string(apitypes.ModelProviderKindOpenaiTenant), "openai-main"),
		modelUpsertWithProviderData("gemini-chat", apitypes.ModelProviderKindGeminiTenant, modelProviderData(t, apitypes.GeminiTenantModelProviderData{UpstreamModel: stringPtr("gemini-pro"), SupportJsonOutput: &falseValue, SupportToolCalls: &falseValue, SupportTextOnly: &falseValue, UseSystemRole: &falseValue, SupportTemperature: &falseValue, SupportThinking: &falseValue})),
		modelUpsertWithProviderData("qwen-chat", apitypes.ModelProviderKindDashscopeTenant, modelProviderData(t, apitypes.DashScopeTenantModelProviderData{ApiMode: &dashScopeMode, UpstreamModel: stringPtr("qwen-max"), SupportJsonOutput: &falseValue, SupportToolCalls: &falseValue, SupportTextOnly: &falseValue, UseSystemRole: &falseValue, SupportTemperature: &falseValue, SupportThinking: &falseValue})),
		modelUpsertWithProviderData("volc-chat", apitypes.ModelProviderKindVolcTenant, modelProviderData(t, apitypes.VolcTenantModelProviderData{ApiMode: &volcMode, UpstreamModel: stringPtr("doubao-pro"), SupportJsonOutput: &falseValue, SupportToolCalls: &falseValue, SupportTextOnly: &falseValue, UseSystemRole: &falseValue, SupportTemperature: &falseValue, SupportThinking: &falseValue})),
		modelUpsertWithProviderData("minimax-m2", apitypes.ModelProviderKindMinimaxTenant, miniMaxProviderData("MiniMax-M2")),
		modelUpsertWithProviderData("deepseek-chat", apitypes.ModelProviderKindDeepseekTenant, deepSeekProviderData("deepseek-chat")),
	}
	for _, body := range valid {
		resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &body})
		if err != nil {
			t.Fatalf("CreateModel(%s) error = %v", body.Id, err)
		}
		if _, ok := resp.(adminhttp.CreateModel200JSONResponse); !ok {
			t.Fatalf("CreateModel(%s) response = %#v", body.Id, resp)
		}
		description := "updated"
		body.Description = &description
		put, err := srv.PutModel(ctx, adminhttp.PutModelRequestObject{Id: body.Id, Body: &body})
		if err != nil {
			t.Fatalf("PutModel(%s) error = %v", body.Id, err)
		}
		if _, ok := put.(adminhttp.PutModel200JSONResponse); !ok {
			t.Fatalf("PutModel(%s) response = %#v", body.Id, put)
		}
	}

	deepSeek := valid[len(valid)-1]
	wrongKind := deepSeek
	wrongKind.Id = "wrong-kind"
	wrongKind.Provider = apitypes.ModelProvider{Kind: apitypes.ModelProviderKindOpenaiTenant, Name: "openai-main"}
	unknownField := modelUpsert("unknown-field", string(apitypes.ModelProviderKindOpenaiTenant), "openai-main")
	if err := json.Unmarshal([]byte(`{"upstream_model":"gpt-test","vendor_option":true}`), &unknownField.ProviderData); err != nil {
		t.Fatalf("json.Unmarshal(provider_data) error = %v", err)
	}
	wrongModelKind := deepSeek
	wrongModelKind.Id = "deepseek-embedding"
	wrongModelKind.Kind = apitypes.ModelKindEmbedding
	defaultBehavior := modelUpsert("default-behavior", string(apitypes.ModelProviderKindOpenaiTenant), "openai-main")
	if err := defaultBehavior.ProviderData.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{UpstreamModel: stringPtr("gpt-test")}); err != nil {
		t.Fatalf("FromOpenAITenantModelProviderData() error = %v", err)
	}
	resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &defaultBehavior})
	if err != nil {
		t.Fatalf("CreateModel(default-behavior) error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateModel200JSONResponse); !ok {
		t.Fatalf("CreateModel(default-behavior) response = %#v, want 200", resp)
	}

	for _, body := range []adminhttp.ModelUpsert{wrongKind, unknownField, wrongModelKind} {
		resp, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &body})
		if err != nil {
			t.Fatalf("CreateModel(%s) error = %v", body.Id, err)
		}
		if _, ok := resp.(adminhttp.CreateModel400JSONResponse); !ok {
			t.Fatalf("CreateModel(%s) response = %#v, want 400", body.Id, resp)
		}
	}
}

func modelUpsertWithProviderData(id string, kind apitypes.ModelProviderKind, data apitypes.ModelProviderData) adminhttp.ModelUpsert {
	out := modelUpsert(id, string(kind), string(kind)+"-main")
	out.ProviderData = data
	return out
}

func modelProviderData(t *testing.T, value any) apitypes.ModelProviderData {
	t.Helper()
	var out apitypes.ModelProviderData
	var err error
	switch typed := value.(type) {
	case apitypes.GeminiTenantModelProviderData:
		err = out.FromGeminiTenantModelProviderData(typed)
	case apitypes.DashScopeTenantModelProviderData:
		err = out.FromDashScopeTenantModelProviderData(typed)
	case apitypes.VolcTenantModelProviderData:
		err = out.FromVolcTenantModelProviderData(typed)
	default:
		t.Fatalf("unsupported provider data %T", value)
	}
	if err != nil {
		t.Fatalf("encode provider data %T: %v", value, err)
	}
	return out
}

func TestModelProviderKindAndDataSchemasStayExhaustive(t *testing.T) {
	expected := map[string]string{
		"openai-tenant":    "OpenAITenantModelProviderData",
		"gemini-tenant":    "GeminiTenantModelProviderData",
		"dashscope-tenant": "DashScopeTenantModelProviderData",
		"volc-tenant":      "VolcTenantModelProviderData",
		"minimax-tenant":   "MiniMaxTenantModelProviderData",
		"deepseek-tenant":  "DeepSeekTenantModelProviderData",
	}
	var kindSchema struct {
		Components struct {
			Schemas struct {
				ModelProviderKind struct {
					Enum []string `json:"enum"`
				} `json:"ModelProviderKind"`
			} `json:"schemas"`
		} `json:"components"`
	}
	readModelSchema(t, "api/http/shared/model_provider_kind.json", &kindSchema)
	if len(kindSchema.Components.Schemas.ModelProviderKind.Enum) != len(expected) {
		t.Fatalf("ModelProviderKind enum = %#v, want %d exhaustive entries", kindSchema.Components.Schemas.ModelProviderKind.Enum, len(expected))
	}
	for _, kind := range kindSchema.Components.Schemas.ModelProviderKind.Enum {
		if expected[kind] == "" || !apitypes.ModelProviderKind(kind).Valid() {
			t.Fatalf("ModelProviderKind %q has no validated provider-data mapping", kind)
		}
	}

	var dataSchema struct {
		Components struct {
			Schemas struct {
				ModelProviderData struct {
					AnyOf []struct {
						Ref string `json:"$ref"`
					} `json:"anyOf"`
				} `json:"ModelProviderData"`
			} `json:"schemas"`
		} `json:"components"`
	}
	readModelSchema(t, "api/http/shared/model_provider_data.json", &dataSchema)
	variants := map[string]bool{}
	for _, item := range dataSchema.Components.Schemas.ModelProviderData.AnyOf {
		variants[item.Ref[strings.LastIndex(item.Ref, "/")+1:]] = true
	}
	if len(variants) != len(expected) {
		t.Fatalf("ModelProviderData variants = %#v, want %d exhaustive entries", variants, len(expected))
	}
	for kind, variant := range expected {
		if !variants[variant] {
			t.Fatalf("provider kind %q is missing provider-data variant %q", kind, variant)
		}
	}
}

func TestVolcProviderDataRequiresTheAPIModeForEachModelRole(t *testing.T) {
	upstream := "volc-upstream"
	tests := []struct {
		kind apitypes.ModelKind
		mode apitypes.VolcTenantModelProviderDataApiMode
	}{
		{kind: apitypes.ModelKindLlm, mode: apitypes.VolcTenantModelProviderDataApiModeChatCompletions},
		{kind: apitypes.ModelKindTts, mode: apitypes.VolcTenantModelProviderDataApiModeTts},
		{kind: apitypes.ModelKindAsr, mode: apitypes.VolcTenantModelProviderDataApiModeAsr},
		{kind: apitypes.ModelKindRealtime, mode: apitypes.VolcTenantModelProviderDataApiModeRealtime},
		{kind: apitypes.ModelKindTranslation, mode: apitypes.VolcTenantModelProviderDataApiModeTranslation},
		{kind: apitypes.ModelKindEmbedding, mode: apitypes.VolcTenantModelProviderDataApiModeEmbedding},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			data := apitypes.ModelProviderData{}
			value := apitypes.VolcTenantModelProviderData{ApiMode: &tt.mode}
			if tt.kind == apitypes.ModelKindLlm {
				falseValue := false
				value.SupportJsonOutput = &falseValue
				value.SupportToolCalls = &falseValue
				value.SupportTextOnly = &falseValue
				value.UseSystemRole = &falseValue
				value.SupportTemperature = &falseValue
				value.SupportThinking = &falseValue
			}
			if tt.kind == apitypes.ModelKindLlm || tt.kind == apitypes.ModelKindEmbedding {
				value.UpstreamModel = &upstream
			}
			if err := data.FromVolcTenantModelProviderData(value); err != nil {
				t.Fatalf("FromVolcTenantModelProviderData() error = %v", err)
			}
			if err := ValidateProviderData(tt.kind, apitypes.ModelProviderKindVolcTenant, data); err != nil {
				t.Fatalf("ValidateProviderData() error = %v", err)
			}

			wrongMode := apitypes.VolcTenantModelProviderDataApiModeRealtime
			if wrongMode == tt.mode {
				wrongMode = apitypes.VolcTenantModelProviderDataApiModeAsr
			}
			value.ApiMode = &wrongMode
			if err := data.FromVolcTenantModelProviderData(value); err != nil {
				t.Fatalf("FromVolcTenantModelProviderData(wrong mode) error = %v", err)
			}
			if err := ValidateProviderData(tt.kind, apitypes.ModelProviderKindVolcTenant, data); err == nil {
				t.Fatalf("ValidateProviderData(%s) accepted api_mode %q", tt.kind, wrongMode)
			}
		})
	}

	if err := ValidateProviderData(apitypes.ModelKindTts, apitypes.ModelProviderKindOpenaiTenant, openAIProviderData("tts")); err == nil {
		t.Fatal("ValidateProviderData() accepted unsupported OpenAI TTS model")
	}
}

func readModelSchema(t *testing.T, relativePath string, out any) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../../.."))
	data, err := os.ReadFile(filepath.Join(repoRoot, relativePath))
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", relativePath, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", relativePath, err)
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
	resp, err := srv.PutModel(ctx, adminhttp.PutModelRequestObject{Id: "sync-preserved", Body: &update})
	if err != nil {
		t.Fatalf("PutModel() error = %v", err)
	}
	stored, ok := resp.(adminhttp.PutModel200JSONResponse)
	if !ok {
		t.Fatalf("PutModel() response = %#v", resp)
	}
	if stored.SyncedAt == nil || !stored.SyncedAt.Equal(syncedAt) {
		t.Fatalf("PutModel() synced_at = %#v", stored.SyncedAt)
	}

	source := adminhttp.ModelSource(apitypes.ModelSourceManual)
	sourceResp, err := srv.ListModels(ctx, adminhttp.ListModelsRequestObject{
		Params: adminhttp.ListModelsParams{Source: &source},
	})
	if err != nil {
		t.Fatalf("ListModels(source) error = %v", err)
	}
	sourceList := requireModelList(t, sourceResp)
	if len(sourceList.Items) != 1 || sourceList.Items[0].Id != "sync-preserved" {
		t.Fatalf("ListModels(source) = %#v", sourceList.Items)
	}
}

func modelUpsert(id string, providerKind, providerName string) adminhttp.ModelUpsert {
	return adminhttp.ModelUpsert{
		Id:     string(id),
		Kind:   apitypes.ModelKindLlm,
		Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKind(providerKind),
			Name: string(providerName),
		},
		ProviderData: openAIProviderData(id),
	}
}

func openAIProviderData(upstreamModel string) apitypes.ModelProviderData {
	out := apitypes.ModelProviderData{}
	falseValue := false
	if err := out.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{UpstreamModel: &upstreamModel, SupportJsonOutput: &falseValue, SupportToolCalls: &falseValue, SupportTextOnly: &falseValue, UseSystemRole: &falseValue, SupportTemperature: &falseValue, SupportThinking: &falseValue}); err != nil {
		panic(err)
	}
	return out
}

func deepSeekProviderData(upstreamModel string) apitypes.ModelProviderData {
	out := apitypes.ModelProviderData{}
	falseValue := false
	if err := out.FromDeepSeekTenantModelProviderData(apitypes.DeepSeekTenantModelProviderData{
		ApiMode:            apitypes.DeepSeekTenantModelProviderDataApiModeChatCompletions,
		UpstreamModel:      upstreamModel,
		SupportJsonOutput:  &falseValue,
		SupportToolCalls:   &falseValue,
		SupportTextOnly:    &falseValue,
		UseSystemRole:      &falseValue,
		SupportTemperature: &falseValue,
		SupportThinking:    &falseValue,
	}); err != nil {
		panic(err)
	}
	return out
}

func miniMaxProviderData(upstreamModel string) apitypes.ModelProviderData {
	out := apitypes.ModelProviderData{}
	falseValue := false
	if err := out.FromMiniMaxTenantModelProviderData(apitypes.MiniMaxTenantModelProviderData{
		ApiMode:            apitypes.MiniMaxTenantModelProviderDataApiModeChatCompletions,
		UpstreamModel:      upstreamModel,
		SupportJsonOutput:  &falseValue,
		SupportToolCalls:   &falseValue,
		SupportTextOnly:    &falseValue,
		UseSystemRole:      &falseValue,
		SupportTemperature: &falseValue,
		SupportThinking:    &falseValue,
	}); err != nil {
		panic(err)
	}
	return out
}

func requireModelList(t *testing.T, resp adminhttp.ListModelsResponseObject) adminhttp.ModelList {
	t.Helper()
	list, ok := resp.(adminhttp.ListModels200JSONResponse)
	if !ok {
		t.Fatalf("ListModels() response = %#v", resp)
	}
	return adminhttp.ModelList(list)
}

func stringPtr(value string) *string {
	return &value
}
