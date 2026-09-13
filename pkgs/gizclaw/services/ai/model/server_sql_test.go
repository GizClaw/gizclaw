package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestSQLModelFilteringRestrictsReads(t *testing.T) {
	db := newTestDB(t)
	ctx := t.Context()
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 2000 {
		provider, data := "other", "invalid"
		if i >= 1000 && i <= 1002 {
			provider, data = "chosen", `{}`
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO models(id,kind,source,provider_kind,provider_id,provider_data_json,created_at,updated_at) VALUES (?,'llm','manual','openai-tenant',?,?,'2026-09-06T00:00:00Z','2026-09-06T00:00:00Z')`, fmt.Sprintf("model-%04d", i), provider, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	filters := modelFilters{source: new("manual"), providerKind: new("openai-tenant"), providerID: new("chosen")}
	page, more, cursor, err := listModelsPage(ctx, db, filters, "model-0999", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Id != "model-1000" || page[1].Id != "model-1001" || !more || cursor == nil || *cursor != "model-1001" {
		t.Fatalf("page=%+v more=%v cursor=%v", page, more, cursor)
	}
	rows, err := db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT `+modelColumns+` FROM models WHERE id>? AND source=? AND provider_kind=? AND provider_id=? ORDER BY id LIMIT ?`, "model-0999", "manual", "openai-tenant", "chosen", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "SEARCH models USING INDEX models_source_provider_id") || strings.Contains(plan.String(), "USE TEMP B-TREE") {
		t.Fatalf("filtered page query plan: %s", plan.String())
	}
}

func TestSQLModelPutCannotResurrectDeletedRow(t *testing.T) {
	s := &Server{DB: newTestDB(t)}
	ctx := t.Context()
	for i := range 30 {
		id := fmt.Sprintf("race-%03d", i)
		body := modelUpsert(id, "openai-tenant", "main")
		response, err := s.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &body})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := response.(adminhttp.CreateModel200JSONResponse); !ok {
			t.Fatalf("create=%#v", response)
		}
		start := make(chan struct{})
		updated := make(chan adminhttp.PutModelResponseObject, 1)
		failed := make(chan error, 1)
		go func() {
			<-start
			response, err := s.PutModel(ctx, adminhttp.PutModelRequestObject{Id: id, Body: &body})
			updated <- response
			failed <- err
		}()
		close(start)
		removed, err := s.DeleteModel(ctx, adminhttp.DeleteModelRequestObject{Id: id})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := removed.(adminhttp.DeleteModel200JSONResponse); !ok {
			t.Fatalf("delete=%#v", removed)
		}
		result := <-updated
		if err := <-failed; err != nil {
			t.Fatal(err)
		}
		switch result.(type) {
		case adminhttp.PutModel200JSONResponse, adminhttp.PutModel404JSONResponse:
		default:
			t.Fatalf("put=%#v", result)
		}
		got, err := s.GetModel(ctx, adminhttp.GetModelRequestObject{Id: id})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got.(adminhttp.GetModel404JSONResponse); !ok {
			t.Fatalf("deleted model was recreated: %#v", got)
		}
	}
}
