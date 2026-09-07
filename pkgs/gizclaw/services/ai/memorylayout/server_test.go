package memorylayout

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestServerMemoryLayoutLifecycle(t *testing.T) {
	server := newTestServer(t)
	ctx := context.Background()
	layout := testLayout(t, "pet-memory")

	created, err := server.CreateMemoryLayout(ctx, adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	createdLayout, ok := created.(adminhttp.CreateMemoryLayout200JSONResponse)
	if !ok || createdLayout.Id != layout.Id {
		t.Fatalf("CreateMemoryLayout() = %#v", created)
	}
	duplicate, err := server.CreateMemoryLayout(ctx, adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := duplicate.(adminhttp.CreateMemoryLayout409JSONResponse); !ok {
		t.Fatalf("duplicate CreateMemoryLayout() = %#v", duplicate)
	}
	got, err := server.GetMemoryLayout(ctx, adminhttp.GetMemoryLayoutRequestObject{Id: createdLayout.Id})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := got.(adminhttp.GetMemoryLayout200JSONResponse); !ok || response.Spec.Flowcraft.Extraction.Model != "extraction" {
		t.Fatalf("GetMemoryLayout() = %#v", got)
	}
	limit := int32(1)
	listed, err := server.ListMemoryLayouts(ctx, adminhttp.ListMemoryLayoutsRequestObject{
		Params: adminhttp.ListMemoryLayoutsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := listed.(adminhttp.ListMemoryLayouts200JSONResponse); !ok || len(response.Items) != 1 {
		t.Fatalf("ListMemoryLayouts() = %#v", listed)
	}

	layout.Spec.Mem0.CustomInstructions = new("updated extraction")
	put, err := server.PutMemoryLayout(ctx, adminhttp.PutMemoryLayoutRequestObject{Id: createdLayout.Id, Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := put.(adminhttp.PutMemoryLayout200JSONResponse); !ok ||
		response.Spec.Mem0.CustomInstructions == nil || *response.Spec.Mem0.CustomInstructions != "updated extraction" {
		t.Fatalf("PutMemoryLayout() = %#v", put)
	}
	deleted, err := server.DeleteMemoryLayout(ctx, adminhttp.DeleteMemoryLayoutRequestObject{Id: createdLayout.Id})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := deleted.(adminhttp.DeleteMemoryLayout200JSONResponse); !ok || response.Id != layout.Id {
		t.Fatalf("DeleteMemoryLayout() = %#v", deleted)
	}
	missing, err := server.GetMemoryLayout(ctx, adminhttp.GetMemoryLayoutRequestObject{Id: createdLayout.Id})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := missing.(adminhttp.GetMemoryLayout404JSONResponse); !ok {
		t.Fatalf("GetMemoryLayout(deleted) = %#v", missing)
	}
}

func TestServerMemoryLayoutPreservesDottedRuntimeAliases(t *testing.T) {
	server := newTestServer(t)
	layout := testLayout(t, "pet-memory")
	layout.Spec.Flowcraft.Extraction.Model = " pet-care.extract "
	layout.Spec.Flowcraft.Embedding.Model = "pet-care.embedding"
	layout.Spec.Flowcraft.Rerank.Model = "pet-care.rerank"

	createResponse, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := createResponse.(adminhttp.CreateMemoryLayout200JSONResponse)
	if !ok {
		t.Fatalf("CreateMemoryLayout() = %#v", createResponse)
	}
	assertMemoryLayoutModelAliases(t, created.Spec, "pet-care.extract", "pet-care.embedding", "pet-care.rerank")

	created.Spec.Flowcraft.Extraction.Model = "story-teller.extract"
	putBody := adminhttp.MemoryLayoutUpsert{Id: created.Id, Spec: created.Spec}
	putResponse, err := server.PutMemoryLayout(t.Context(), adminhttp.PutMemoryLayoutRequestObject{Id: created.Id, Body: &putBody})
	if err != nil {
		t.Fatal(err)
	}
	put, ok := putResponse.(adminhttp.PutMemoryLayout200JSONResponse)
	if !ok {
		t.Fatalf("PutMemoryLayout() = %#v", putResponse)
	}
	assertMemoryLayoutModelAliases(t, put.Spec, "story-teller.extract", "pet-care.embedding", "pet-care.rerank")

	getResponse, err := server.GetMemoryLayout(t.Context(), adminhttp.GetMemoryLayoutRequestObject{Id: created.Id})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := getResponse.(adminhttp.GetMemoryLayout200JSONResponse)
	if !ok {
		t.Fatalf("GetMemoryLayout() = %#v", getResponse)
	}
	assertMemoryLayoutModelAliases(t, got.Spec, "story-teller.extract", "pet-care.embedding", "pet-care.rerank")
}

func assertMemoryLayoutModelAliases(t *testing.T, spec apitypes.MemoryLayoutSpec, extraction, embedding, rerank string) {
	t.Helper()
	if spec.Flowcraft.Extraction.Model != extraction || spec.Flowcraft.Embedding == nil || spec.Flowcraft.Embedding.Model != embedding ||
		spec.Flowcraft.Rerank == nil || spec.Flowcraft.Rerank.Model != rerank {
		t.Fatalf("MemoryLayout model aliases = %#v", spec.Flowcraft)
	}
}

func TestServerMemoryLayoutAcceptsOpaqueIDWithKVSeparator(t *testing.T) {
	server := newTestServer(t)
	layout := testLayout(t, "tenant:memory")
	response, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
	if err != nil {
		t.Fatalf("CreateMemoryLayout() error = %v", err)
	}
	created, ok := response.(adminhttp.CreateMemoryLayout200JSONResponse)
	if !ok || created.Id != layout.Id {
		t.Fatalf("CreateMemoryLayout() = %#v", response)
	}
}

func TestServerConcurrentCreateHasSingleWinner(t *testing.T) {
	server := newTestServer(t)
	layout := testLayout(t, "pet-memory")
	start := make(chan struct{})
	responses := make(chan adminhttp.CreateMemoryLayoutResponseObject, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			response, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
			if err != nil {
				t.Errorf("CreateMemoryLayout() error = %v", err)
				return
			}
			responses <- response
		})
	}
	close(start)
	workers.Wait()
	close(responses)

	var created, conflicts int
	for response := range responses {
		switch response.(type) {
		case adminhttp.CreateMemoryLayout200JSONResponse:
			created++
		case adminhttp.CreateMemoryLayout409JSONResponse:
			conflicts++
		default:
			t.Errorf("CreateMemoryLayout() = %#v", response)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("created = %d, conflicts = %d; want 1 and 1", created, conflicts)
	}
}

func TestConcurrentMemoryLayoutPutCannotRecreateDeletedRow(t *testing.T) {
	server := newTestServer(t)
	second := &Server{DB: server.DB}
	for i := range 30 {
		layout := testLayout(t, fmt.Sprintf("memory-%d", i))
		if _, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout}); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		puts := make(chan adminhttp.PutMemoryLayoutResponseObject, 1)
		deletes := make(chan adminhttp.DeleteMemoryLayoutResponseObject, 1)
		go func() {
			<-start
			response, err := server.PutMemoryLayout(t.Context(), adminhttp.PutMemoryLayoutRequestObject{Id: layout.Id, Body: &layout})
			if err != nil {
				t.Error(err)
			}
			puts <- response
		}()
		go func() {
			<-start
			response, err := second.DeleteMemoryLayout(t.Context(), adminhttp.DeleteMemoryLayoutRequestObject{Id: layout.Id})
			if err != nil {
				t.Error(err)
			}
			deletes <- response
		}()
		close(start)
		switch response := (<-puts).(type) {
		case adminhttp.PutMemoryLayout200JSONResponse, adminhttp.PutMemoryLayout404JSONResponse:
		default:
			t.Fatalf("concurrent put = %T", response)
		}
		if response := <-deletes; response == nil {
			t.Fatal("delete returned no response")
		} else if _, ok := response.(adminhttp.DeleteMemoryLayout200JSONResponse); !ok {
			t.Fatalf("delete = %T", response)
		}
		got, err := server.GetMemoryLayout(t.Context(), adminhttp.GetMemoryLayoutRequestObject{Id: layout.Id})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got.(adminhttp.GetMemoryLayout404JSONResponse); !ok {
			t.Fatalf("concurrent update recreated deleted layout: %T", got)
		}
	}
}

func TestSQLMemoryLayoutPaginationAndIndependentRows(t *testing.T) {
	server := newTestServer(t)
	for _, id := range []string{"first-memory", "second-memory", "third-memory"} {
		layout := testLayout(t, id)
		if _, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout}); err != nil {
			t.Fatal(err)
		}
	}
	update := testLayout(t, "first-memory")
	update.Spec.Mem0.CustomInstructions = new("changed")
	if _, err := server.PutMemoryLayout(t.Context(), adminhttp.PutMemoryLayoutRequestObject{Id: update.Id, Body: &update}); err != nil {
		t.Fatal(err)
	}
	response, err := server.ListMemoryLayouts(t.Context(), adminhttp.ListMemoryLayoutsRequestObject{Params: adminhttp.ListMemoryLayoutsParams{Cursor: new("first-memory"), Limit: new(int32(1))}})
	if err != nil {
		t.Fatal(err)
	}
	page := response.(adminhttp.ListMemoryLayouts200JSONResponse)
	if len(page.Items) != 1 || page.Items[0].Id != "second-memory" || !page.HasNext || page.NextCursor == nil || *page.NextCursor != "second-memory" {
		t.Fatalf("page = %#v", page)
	}
	if *page.Items[0].Spec.Mem0.CustomInstructions == "changed" {
		t.Fatal("update modified another layout")
	}
}

func TestServerRejectsInvalidMemoryLayouts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*adminhttp.MemoryLayoutUpsert)
		want   string
	}{
		{"empty lanes", func(layout *adminhttp.MemoryLayoutUpsert) { layout.Spec.Flowcraft.Lanes = nil }, "lanes must not be empty"},
		{"duplicate lanes", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Lanes = append(layout.Spec.Flowcraft.Lanes, layout.Spec.Flowcraft.Lanes[0])
		}, "duplicate name"},
		{"invalid fact kind", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Lanes[0].Kind = "unknown"
		}, "kind"},
		{"invalid extraction mode", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Extraction.Mode = "unknown"
		}, "extraction.mode"},
		{"invalid extraction timeout", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Extraction.StageTimeout = new("0s")
		}, "stage_timeout"},
		{"invalid dotted extraction alias", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Extraction.Model = "pet-care..extract"
		}, "RuntimeProfile alias"},
		{"invalid BBH overfetch", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Bbh = &apitypes.FlowcraftMemoryBBHPolicy{SearchOverfetch: new(0)}
		}, "search_overfetch"},
		{"invalid BBH flush interval", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Bbh = &apitypes.FlowcraftMemoryBBHPolicy{
				Hnsw: &apitypes.FlowcraftMemoryHNSWPolicy{FlushInterval: new("0s")},
			}
		}, "flush_interval"},
		{"empty mem0 policy", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Mem0 = apitypes.Mem0MemoryLayoutPolicy{}
		}, "spec.mem0 must define"},
		{"duplicate volc strategy", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.VolcMem0.Strategies = append(layout.Spec.VolcMem0.Strategies, layout.Spec.VolcMem0.Strategies[0])
		}, "duplicate name"},
		{"invalid volc strategy", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.VolcMem0.Strategies[0].Type = "unknown"
		}, "strategies[0].type"},
		{"too many volc strategies", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.VolcMem0.Strategies = make([]apitypes.VolcMem0Strategy, 51)
		}, "between 1 and 50"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t)
			layout := testLayout(t, "pet-memory")
			test.mutate(&layout)
			response, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
			if err != nil {
				t.Fatal(err)
			}
			invalid, ok := response.(adminhttp.CreateMemoryLayout400JSONResponse)
			if !ok || !strings.Contains(invalid.Error.Message, test.want) {
				t.Fatalf("CreateMemoryLayout() = %#v, want error containing %q", response, test.want)
			}
		})
	}
}

func TestServerRejectsMemoryLayoutPathMismatch(t *testing.T) {
	server := newTestServer(t)
	layout := testLayout(t, "pet-memory")
	createdResponse, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	created := createdResponse.(adminhttp.CreateMemoryLayout200JSONResponse)
	layout.Id = "other-memory"
	response, err := server.PutMemoryLayout(t.Context(), adminhttp.PutMemoryLayoutRequestObject{Id: created.Id, Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	invalid, ok := response.(adminhttp.PutMemoryLayout400JSONResponse)
	if !ok || !strings.Contains(invalid.Error.Message, "must match path id") {
		t.Fatalf("PutMemoryLayout() = %#v", response)
	}
}

func TestServerNormalizesRuntimePolicyStrings(t *testing.T) {
	server := newTestServer(t)
	layout := testLayout(t, "pet-memory")
	layout.Spec.Flowcraft.Extraction.StageTimeout = new(" 30s ")
	layout.Spec.Flowcraft.Bbh = &apitypes.FlowcraftMemoryBBHPolicy{
		Hnsw: &apitypes.FlowcraftMemoryHNSWPolicy{FlushInterval: new(" 2s ")},
	}
	layout.Spec.Mem0.CustomInstructions = new(" keep durable facts ")
	layout.Spec.VolcMem0.Strategies[0].CustomInstructions = new(" keep pet facts ")

	response, err := server.CreateMemoryLayout(t.Context(), adminhttp.CreateMemoryLayoutRequestObject{Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateMemoryLayout200JSONResponse)
	if !ok {
		t.Fatalf("CreateMemoryLayout() = %#v", response)
	}
	if got := *created.Spec.Flowcraft.Extraction.StageTimeout; got != "30s" {
		t.Fatalf("stage_timeout = %q", got)
	}
	if got := *created.Spec.Flowcraft.Bbh.Hnsw.FlushInterval; got != "2s" {
		t.Fatalf("bbh.flush_interval = %q", got)
	}
	if got := *created.Spec.Mem0.CustomInstructions; got != "keep durable facts" {
		t.Fatalf("mem0 custom_instructions = %q", got)
	}
	if got := *created.Spec.VolcMem0.Strategies[0].CustomInstructions; got != "keep pet facts" {
		t.Fatalf("volc_mem0 custom_instructions = %q", got)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	server := &Server{DB: db}
	if err := server.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return server
}

func testLayout(t *testing.T, name string) adminhttp.MemoryLayoutUpsert {
	t.Helper()
	raw := `{
		"id":"` + name + `",
		"spec":{
			"flowcraft":{
				"extraction":{"model":"extraction","mode":"two_pass","stage_timeout":"30s"},
				"embedding":{"model":"embedding"},
				"rerank":{"model":"rerank-model"},
				"lanes":[{"name":"owner-profile","kind":"preference"}],
				"write":{"mode":"sync","tier":"general"}
			},
			"mem0":{"custom_instructions":"extract durable facts","custom_categories":{"owner-profile":"Owner facts"}},
			"volc_mem0":{"strategies":[{"name":"owner-profile","type":"user_preference"}]}
		}
	}`
	var layout adminhttp.MemoryLayoutUpsert
	if err := json.Unmarshal([]byte(raw), &layout); err != nil {
		t.Fatal(err)
	}
	return layout
}
