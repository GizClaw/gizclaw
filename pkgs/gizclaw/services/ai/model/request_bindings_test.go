package model

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

func testRequestBinding() apitypes.ModelRequestBinding {
	return apitypes.ModelRequestBinding{
		Target: apitypes.ModelRequestTarget{Kind: apitypes.ModelRequestTargetKindHttpHeader, Name: "X-Device-Metadata"},
		Source: apitypes.ModelRequestSource{Kind: apitypes.ModelRequestSourceKindReportedKv, Key: "identity", Pointer: new("/device/id")},
	}
}

func TestModelRequestBindingsUpgradePersistAndClear(t *testing.T) {
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
	// Existing databases have no binding column. Exercise startup upgrade
	// and resource reads rather than inspecting the resulting SQL layout.
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
	if err != nil || legacy.(adminhttp.GetModel200JSONResponse).RequestBindings != nil {
		t.Fatalf("legacy resource changed: %T, %v", legacy, err)
	}
	body := modelUpsert("bound", "openai-tenant", "main")
	body.RequestBindings = &apitypes.ModelRequestBindings{testRequestBinding()}
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
	if err != nil || !reflect.DeepEqual(got.(adminhttp.GetModel200JSONResponse).RequestBindings, body.RequestBindings) {
		t.Fatalf("bindings did not survive restart: %v", err)
	}
	list, err := srv.ListModels(t.Context(), adminhttp.ListModelsRequestObject{})
	if err != nil || len(requireModelList(t, list).Items) != 2 || requireModelList(t, list).Items[0].RequestBindings == nil {
		t.Fatalf("list omitted persisted bindings: %v", err)
	}
	body.RequestBindings = &apitypes.ModelRequestBindings{}
	put, err := srv.PutModel(t.Context(), adminhttp.PutModelRequestObject{Id: body.Id, Body: &body})
	if err != nil || put.(adminhttp.PutModel200JSONResponse).RequestBindings != nil {
		t.Fatalf("empty bindings were not normalized to disabled: %v", err)
	}
	deleted, err := srv.DeleteModel(t.Context(), adminhttp.DeleteModelRequestObject{Id: body.Id})
	if err != nil || deleted.(adminhttp.DeleteModel200JSONResponse).RequestBindings != nil {
		t.Fatalf("delete returned stale bindings: %v", err)
	}
}

func TestModelRequestBindingsRejectUnsupportedDeclarations(t *testing.T) {
	srv := &Server{DB: newTestDB(t)}
	for _, tc := range []struct {
		name   string
		change func(*adminhttp.ModelUpsert)
	}{
		{"authorization", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Target.Name = "Authorization" }},
		{"credential header", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Target.Name = "X-Api-Key" }},
		{"forwarding", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Target.Name = "X-Forwarded-For" }},
		{"SDK control", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Target.Name = "X-Stainless-Retry-Count" }},
		{"duplicate case", func(b *adminhttp.ModelUpsert) {
			other := testRequestBinding()
			other.Target.Name = "x-device-metadata"
			*b.RequestBindings = append(*b.RequestBindings, other)
		}},
		{"too many", func(b *adminhttp.ModelUpsert) { *b.RequestBindings = make(apitypes.ModelRequestBindings, 17) }},
		{"header token", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Target.Name = "X-Meta\r\nInjected" }},
		{"header length", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Target.Name = "X-" + strings.Repeat("x", 127) }},
		{"query", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Target.Kind = "http_query" }},
		{"credential source", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Source.Kind = "credential" }},
		{"key bytes", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Source.Key = strings.Repeat("中", 22) }},
		{"key empty", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Source.Key = "" }},
		{"pointer escape", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Source.Pointer = new("/bad~2escape") }},
		{"pointer expression", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Source.Pointer = new("$.device.id") }},
		{"pointer depth", func(b *adminhttp.ModelUpsert) { (*b.RequestBindings)[0].Source.Pointer = new(strings.Repeat("/a", 17)) }},
		{"pointer bytes", func(b *adminhttp.ModelUpsert) {
			(*b.RequestBindings)[0].Source.Pointer = new("/" + strings.Repeat("中", 86))
		}},
		{"embedding", func(b *adminhttp.ModelUpsert) { b.Kind = apitypes.ModelKindEmbedding }},
		{"provider", func(b *adminhttp.ModelUpsert) { b.Provider.Kind = apitypes.ModelProviderKindGeminiTenant }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := modelUpsert("invalid", "openai-tenant", "main")
			body.RequestBindings = &apitypes.ModelRequestBindings{testRequestBinding()}
			tc.change(&body)
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
