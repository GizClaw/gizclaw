package model

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

func testRequestBinding(t *testing.T) apitypes.ModelRequestBinding {
	t.Helper()
	var source apitypes.ModelRequestSource
	if err := source.FromModelRequestPeerRuntimeKVSource(apitypes.ModelRequestPeerRuntimeKVSource{PeerRuntimeKv: apitypes.ModelRequestPeerRuntimeKVSelector{Key: "identity", Pointer: new("/device/id")}}); err != nil {
		t.Fatal(err)
	}
	return apitypes.ModelRequestBinding{
		Target: apitypes.ModelRequestTarget{Kind: apitypes.ModelRequestTargetKindHttpHeader, Name: "X-Device-Metadata"},
		Source: source,
	}
}

func modelBindings(t *testing.T, data apitypes.ModelProviderData) *apitypes.ModelRequestBindings {
	t.Helper()
	value, err := data.AsOpenAITenantModelProviderData()
	if err != nil {
		t.Fatal(err)
	}
	return value.RequestBindings
}

func setModelBindings(t *testing.T, body *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings) {
	t.Helper()
	value, err := body.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil {
		t.Fatal(err)
	}
	value.RequestBindings = bindings
	if err := body.ProviderData.FromOpenAITenantModelProviderData(value); err != nil {
		t.Fatal(err)
	}
}

func TestModelRequestBindingsPersistAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.sqlite")
	open := func() *sqlx.DB {
		db, err := sqlx.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	db := open()
	// Existing provider_data JSON stores both older models and optional bindings.
	// Exercise persistence and resource reads against the unchanged SQL schema.
	if _, err := db.Exec(`CREATE TABLE models(id TEXT PRIMARY KEY,kind TEXT NOT NULL,source TEXT NOT NULL,provider_kind TEXT NOT NULL,provider_id TEXT NOT NULL,provider_data_json TEXT NOT NULL,display_name TEXT,description TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,synced_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO models(id,kind,source,provider_kind,provider_id,provider_data_json,created_at,updated_at) VALUES ('legacy','llm','manual','openai-tenant','main','{"upstream_model":"chat"}','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	srv := &Server{DB: db}
	for range 2 {
		if err := srv.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := srv.GetModel(t.Context(), adminhttp.GetModelRequestObject{Id: "legacy"})
	if err != nil || modelBindings(t, legacy.(adminhttp.GetModel200JSONResponse).ProviderData) != nil {
		t.Fatalf("legacy resource changed: %T, %v", legacy, err)
	}
	body := modelUpsert("bound", "openai-tenant", "main")
	constant := testRequestBinding(t)
	constant.Target.Name = "X-Fixed-Target"
	if err := constant.Source.UnmarshalJSON([]byte(`{"constant":"vendor.example:30100"}`)); err != nil {
		t.Fatal(err)
	}
	setModelBindings(t, &body, &apitypes.ModelRequestBindings{testRequestBinding(t), constant})
	response, err := srv.CreateModel(t.Context(), adminhttp.CreateModelRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateModel200JSONResponse); !ok {
		t.Fatalf("create: %T", response)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	srv = &Server{DB: open()}
	if err := srv.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := srv.GetModel(t.Context(), adminhttp.GetModelRequestObject{Id: body.Id})
	if err != nil || !reflect.DeepEqual(modelBindings(t, got.(adminhttp.GetModel200JSONResponse).ProviderData), modelBindings(t, body.ProviderData)) {
		t.Fatalf("bindings did not survive restart: %v", err)
	}
	list, err := srv.ListModels(t.Context(), adminhttp.ListModelsRequestObject{})
	if err != nil || len(requireModelList(t, list).Items) != 2 || modelBindings(t, requireModelList(t, list).Items[0].ProviderData) == nil {
		t.Fatalf("list omitted persisted bindings: %v", err)
	}
	setModelBindings(t, &body, &apitypes.ModelRequestBindings{})
	empty, err := srv.PutModel(t.Context(), adminhttp.PutModelRequestObject{Id: body.Id, Body: &body})
	if err != nil || len(*modelBindings(t, empty.(adminhttp.PutModel200JSONResponse).ProviderData)) != 0 {
		t.Fatalf("empty bindings were not preserved: %v", err)
	}
	setModelBindings(t, &body, nil)
	put, err := srv.PutModel(t.Context(), adminhttp.PutModelRequestObject{Id: body.Id, Body: &body})
	if err != nil || modelBindings(t, put.(adminhttp.PutModel200JSONResponse).ProviderData) != nil {
		t.Fatalf("removed bindings were retained: %v", err)
	}
	deleted, err := srv.DeleteModel(t.Context(), adminhttp.DeleteModelRequestObject{Id: body.Id})
	if err != nil || modelBindings(t, deleted.(adminhttp.DeleteModel200JSONResponse).ProviderData) != nil {
		t.Fatalf("delete returned stale bindings: %v", err)
	}
}

func TestModelRequestBindingsRejectUnsupportedDeclarations(t *testing.T) {
	srv := &Server{DB: newTestDB(t)}
	for _, tc := range []struct {
		name   string
		change func(*adminhttp.ModelUpsert, *apitypes.ModelRequestBindings, *apitypes.ModelRequestPeerRuntimeKVSelector)
	}{
		{"authorization", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			(*bindings)[0].Target.Name = "Authorization"
		}},
		{"credential header", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			(*bindings)[0].Target.Name = "X-Api-Key"
		}},
		{"forwarding", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			(*bindings)[0].Target.Name = "X-Forwarded-For"
		}},
		{"SDK control", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			(*bindings)[0].Target.Name = "X-Stainless-Retry-Count"
		}},
		{"duplicate case", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			other := testRequestBinding(t)
			other.Target.Name = "x-device-metadata"
			*bindings = append(*bindings, other)
		}},
		{"too many", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			*bindings = make(apitypes.ModelRequestBindings, 17)
		}},
		{"header token", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			(*bindings)[0].Target.Name = "X-Meta\r\nInjected"
		}},
		{"header length", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			(*bindings)[0].Target.Name = "X-" + strings.Repeat("x", 127)
		}},
		{"query", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			(*bindings)[0].Target.Kind = "http_query"
		}},
		{"key bytes", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			source.Key = strings.Repeat("中", 22)
		}},
		{"key empty", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			source.Key = ""
		}},
		{"pointer escape", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			source.Pointer = new("/bad~2escape")
		}},
		{"pointer expression", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			source.Pointer = new("$.device.id")
		}},
		{"pointer depth", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			source.Pointer = new(strings.Repeat("/a", 17))
		}},
		{"pointer bytes", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			source.Pointer = new("/" + strings.Repeat("中", 86))
		}},
		{"embedding", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			b.Kind = apitypes.ModelKindEmbedding
		}},
		{"provider", func(b *adminhttp.ModelUpsert, bindings *apitypes.ModelRequestBindings, source *apitypes.ModelRequestPeerRuntimeKVSelector) {
			b.Provider.Kind = apitypes.ModelProviderKindGeminiTenant
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := modelUpsert("invalid", "openai-tenant", "main")
			bindings := apitypes.ModelRequestBindings{testRequestBinding(t)}
			source, err := bindings[0].Source.AsModelRequestPeerRuntimeKVSource()
			if err != nil {
				t.Fatal(err)
			}
			tc.change(&body, &bindings, &source.PeerRuntimeKv)
			raw, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := bindings[0].Source.UnmarshalJSON(raw); err != nil {
				t.Fatal(err)
			}
			setModelBindings(t, &body, &bindings)
			response, err := srv.CreateModel(t.Context(), adminhttp.CreateModelRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := response.(adminhttp.CreateModel400JSONResponse); !ok {
				t.Fatalf("invalid declaration was persisted: %T", response)
			}
		})
	}
	if list, err := srv.ListModels(t.Context(), adminhttp.ListModelsRequestObject{}); err != nil || len(requireModelList(t, list).Items) != 0 {
		t.Fatal("rejected declaration left a resource")
	}
}

func TestModelRequestSourceOneOfRejectsInvalidShapes(t *testing.T) {
	srv := &Server{DB: newTestDB(t)}
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"null source", `null`},
		{"array source", `[]`},
		{"missing branch", `{}`},
		{"unknown branch", `{"runtime_profile":{"key":"identity"}}`},
		{"credential branch", `{"credential":{"id":"provider-key"}}`},
		{"multiple branches", `{"peer_runtime_kv":{"key":"identity"},"runtime_profile":{"key":"identity"}}`},
		{"old flat source", `{"kind":"peer_runtime_kv","key":"identity"}`},
		{"null selector", `{"peer_runtime_kv":null}`},
		{"missing key", `{"peer_runtime_kv":{}}`},
		{"mixed branch fields", `{"peer_runtime_kv":{"key":"identity","profile_id":"other"}}`},
		{"unexpected value", `{"peer_runtime_kv":{"key":"identity","value":"other"}}`},
		{"wrong key type", `{"peer_runtime_kv":{"key":42}}`},
		{"wrong pointer type", `{"peer_runtime_kv":{"key":"identity","pointer":[]}}`},
		{"constant and KV", `{"constant":"fixed","peer_runtime_kv":{"key":"identity"}}`},
		{"null constant", `{"constant":null}`},
		{"number constant", `{"constant":42}`},
		{"boolean constant", `{"constant":false}`},
		{"object constant", `{"constant":{"value":"fixed"}}`},
		{"empty constant", `{"constant":""}`},
		{"constant fields", `{"constant":"fixed","key":"identity"}`},
		{"constant newline", `{"constant":"fixed\r\nInjected: true"}`},
		{"constant whitespace", `{"constant":" padded"}`},
		{"constant Unicode", `{"constant":"设备"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := modelUpsert("invalid-source", "openai-tenant", "main")
			binding := testRequestBinding(t)
			if err := json.Unmarshal([]byte(tc.raw), &binding.Source); err != nil {
				t.Fatal(err)
			}
			setModelBindings(t, &body, &apitypes.ModelRequestBindings{binding})
			response, err := srv.CreateModel(t.Context(), adminhttp.CreateModelRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := response.(adminhttp.CreateModel400JSONResponse); !ok {
				t.Fatalf("invalid source union was persisted: %T", response)
			}
		})
	}
	if list, err := srv.ListModels(t.Context(), adminhttp.ListModelsRequestObject{}); err != nil || len(requireModelList(t, list).Items) != 0 {
		t.Fatal("rejected source left a resource")
	}
}
