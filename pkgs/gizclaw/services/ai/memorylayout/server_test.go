package memorylayout

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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
	if !ok || createdLayout.Name != layout.Name {
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
	if response, ok := deleted.(adminhttp.DeleteMemoryLayout200JSONResponse); !ok || response.Name != layout.Name {
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
		{"invalid overfetch", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Bbh.SearchOverfetch = new(0)
		}, "search_overfetch"},
		{"invalid analyzer", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Bbh.Bleve.Analyzer = analyzerPtr("unknown")
		}, "analyzer"},
		{"invalid flush interval", func(layout *adminhttp.MemoryLayoutUpsert) {
			layout.Spec.Flowcraft.Bbh.Hnsw.FlushInterval = new("invalid")
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
	layout.Name = "other-memory"
	response, err := server.PutMemoryLayout(t.Context(), adminhttp.PutMemoryLayoutRequestObject{Id: created.Id, Body: &layout})
	if err != nil {
		t.Fatal(err)
	}
	invalid, ok := response.(adminhttp.PutMemoryLayout400JSONResponse)
	if !ok || !strings.Contains(invalid.Error.Message, "must match path name") {
		t.Fatalf("PutMemoryLayout() = %#v", response)
	}
}

func TestServerNormalizesRuntimePolicyStrings(t *testing.T) {
	server := newTestServer(t)
	layout := testLayout(t, "pet-memory")
	layout.Spec.Flowcraft.Extraction.StageTimeout = new(" 30s ")
	layout.Spec.Flowcraft.Bbh.Hnsw.FlushInterval = new(" 1m ")
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
	if got := *created.Spec.Flowcraft.Bbh.Hnsw.FlushInterval; got != "1m" {
		t.Fatalf("flush_interval = %q", got)
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
	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Server{Store: store}
}

func testLayout(t *testing.T, name string) adminhttp.MemoryLayoutUpsert {
	t.Helper()
	raw := `{
		"name":"` + name + `",
		"spec":{
			"flowcraft":{
				"extraction":{"model":"extraction","mode":"two_pass","stage_timeout":"30s"},
				"embedding":{"model":"embedding"},
				"rerank":{"model":"rerank-model"},
				"bbh":{"search_overfetch":20,"bleve":{"analyzer":"standard"},"hnsw":{"flush_interval":"1m"}},
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

func analyzerPtr(value string) *apitypes.FlowcraftMemoryBlevePolicyAnalyzer {
	typed := apitypes.FlowcraftMemoryBlevePolicyAnalyzer(value)
	return &typed
}
