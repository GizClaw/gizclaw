package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestServerPutGetListDeleteAndDefensiveCopies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	server := &Server{DB: newTestDatabase(t), Now: func() time.Time { return now }}
	tool := testClientTool("volume_set")
	tool.Metadata = json.RawMessage(`{"category":"device"}`)
	created, err := server.CreateTool(ctx, tool)
	if err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("timestamps = %s/%s", created.CreatedAt, created.UpdatedAt)
	}
	created.InputSchema.Type = "string"
	created.Metadata[0] = '['
	got, err := server.GetTool(ctx, tool.InvokeName)
	if err != nil {
		t.Fatalf("GetTool(): %v", err)
	}
	if got.InputSchema.Type != "object" || string(got.Metadata) != `{"category":"device"}` {
		t.Fatalf("stored Tool was mutated: %#v", got)
	}
	now = now.Add(time.Minute)
	got.Version = new("2")
	updated, err := server.PutTool(ctx, got.ID, got)
	if err != nil {
		t.Fatalf("PutTool(update): %v", err)
	}
	if updated.CreatedAt != created.CreatedAt || updated.UpdatedAt != now {
		t.Fatalf("updated timestamps = %s/%s", updated.CreatedAt, updated.UpdatedAt)
	}
	if _, err := server.CreateTool(ctx, testHTTPTool("get_weather")); err != nil {
		t.Fatalf("PutTool(second): %v", err)
	}
	items, err := server.ListTools(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListTools() = %d, %v", len(items), err)
	}
	if err := server.DeleteTool(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTool(): %v", err)
	}
	if _, err := server.GetTool(ctx, tool.InvokeName); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("GetTool(deleted) = %v", err)
	}
}

func TestServerAcceptsOpaqueToolID(t *testing.T) {
	server := &Server{DB: newTestDatabase(t)}
	tool := testClientTool("volume_set")
	tool.ID = "tenant:tool"
	created, err := server.CreateTool(t.Context(), tool)
	if err != nil {
		t.Fatalf("CreateTool() error = %v", err)
	}
	if created.ID != tool.ID {
		t.Fatalf("CreateTool().ID = %q, want %q", created.ID, tool.ID)
	}
}

func TestServerReportsToolIdentityConflicts(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server := &Server{DB: newTestDatabase(t)}
	created, err := server.CreateTool(ctx, testClientTool("volume_set"))
	if err != nil {
		t.Fatalf("CreateTool() error = %v", err)
	}

	duplicateID := testClientTool("volume_get")
	duplicateID.ID = created.ID
	if _, err := server.CreateTool(ctx, duplicateID); !errors.Is(err, ErrToolConflict) {
		t.Fatalf("CreateTool(duplicate ID) error = %v, want %v", err, ErrToolConflict)
	}

	duplicateName := testClientTool(created.InvokeName)
	duplicateName.ID = "different-id"
	if _, err := server.CreateTool(ctx, duplicateName); !errors.Is(err, ErrToolConflict) {
		t.Fatalf("CreateTool(duplicate invoke_name) error = %v, want %v", err, ErrToolConflict)
	}

	renamed := created
	renamed.InvokeName = "volume_get"
	if _, err := server.PutTool(ctx, created.ID, renamed); !errors.Is(err, ErrToolConflict) {
		t.Fatalf("PutTool(renamed) error = %v, want %v", err, ErrToolConflict)
	}
}

func TestCreateToolAtomicallyClaimsIDAndInvokeNameAcrossServers(t *testing.T) {
	t.Parallel()

	db := newTestDatabase(t)
	servers := []*Server{{DB: db}, {DB: db}}
	tools := []Tool{testClientTool("shared_tool"), testClientTool("shared_tool")}
	tools[0].ID = "tool-alpha"
	tools[1].ID = "tool-beta"
	type result struct {
		tool Tool
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, len(tools))
	for i, tool := range tools {
		go func(server *Server, desired Tool) {
			<-start
			created, err := server.CreateTool(t.Context(), desired)
			results <- result{tool: created, err: err}
		}(servers[i], tool)
	}
	close(start)

	var winner Tool
	created := 0
	conflicts := 0
	for range tools {
		result := <-results
		switch {
		case result.err == nil:
			winner = result.tool
			created++
		case errors.Is(result.err, ErrToolConflict):
			conflicts++
		default:
			t.Fatalf("CreateTool() error = %v", result.err)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("create results = %d created, %d conflicts; want 1 each", created, conflicts)
	}
	stored, err := servers[0].GetTool(t.Context(), "shared_tool")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if stored.ID != winner.ID {
		t.Fatalf("GetTool().ID = %q, winner = %q", stored.ID, winner.ID)
	}
	for _, tool := range tools {
		_, err := servers[0].GetToolByID(t.Context(), tool.ID)
		if tool.ID == winner.ID && err != nil {
			t.Fatalf("winner ID %q lookup error = %v", tool.ID, err)
		}
		if tool.ID != winner.ID && !errors.Is(err, ErrToolNotFound) {
			t.Fatalf("loser ID %q lookup error = %v, want not found", tool.ID, err)
		}
	}
}

func TestServerRetainsRotatesAndDropsDirectSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := &Server{DB: newTestDatabase(t)}
	tool := testHTTPTool("get_weather")
	tool.HTTP.Auth = HTTPAuth{Method: "bearer", BearerToken: new("first")}
	created, err := server.CreateTool(ctx, tool)
	if err != nil {
		t.Fatalf("PutTool(create): %v", err)
	}
	tool.HTTP.Auth.BearerToken = nil
	retained, err := server.PutTool(ctx, created.ID, tool)
	if err != nil {
		t.Fatalf("PutTool(retain): %v", err)
	}
	if retained.HTTP.Auth.BearerToken == nil || *retained.HTTP.Auth.BearerToken != "first" {
		t.Fatalf("retained secret = %#v", retained.HTTP.Auth.BearerToken)
	}
	tool.HTTP.Auth.BearerToken = new("second")
	rotated, err := server.PutTool(ctx, created.ID, tool)
	if err != nil {
		t.Fatalf("PutTool(rotate): %v", err)
	}
	if *rotated.HTTP.Auth.BearerToken != "second" {
		t.Fatalf("rotated secret = %#v", rotated.HTTP.Auth.BearerToken)
	}
	tool.HTTP.Auth = HTTPAuth{Method: "none"}
	changed, err := server.PutTool(ctx, created.ID, tool)
	if err != nil {
		t.Fatalf("PutTool(change method): %v", err)
	}
	if changed.HTTP.Auth.BearerToken != nil || changed.HTTP.Auth.APIKey != nil {
		t.Fatalf("stale secret retained: %#v", changed.HTTP.Auth)
	}
}

func TestNormalizeToolRejectsInvalidNamesTypesSchemasAndHTTP(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", " volume", "volume ", "1volume", "音量", "a.b", string(make([]byte, 65))} {
		tool := testClientTool(name)
		if _, err := NormalizeTool(tool); err == nil {
			t.Fatalf("NormalizeTool(name=%q) succeeded", name)
		}
	}
	for _, schema := range []jsonschema.Schema{{}, {Type: "string"}, {Types: []string{"string", "null"}}} {
		tool := testClientTool("volume_set")
		tool.InputSchema = schema
		if _, err := NormalizeTool(tool); err == nil {
			t.Fatalf("NormalizeTool(schema=%#v) succeeded", schema)
		}
	}
	tool := testHTTPTool("get_weather")
	tool.HTTP.URL = "http://127.0.0.1/weather"
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(non-HTTPS) succeeded")
	}
	tool = testHTTPTool("get_weather")
	tool.HTTP.Method = "DELETE"
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(DELETE) succeeded")
	}
	for _, queryKey := range []string{
		"apiKey",
		"apikey",
		"key",
		"password",
		"access-token",
		"access_key_id",
		"client_secret",
	} {
		tool = testHTTPTool("get_weather")
		tool.HTTP.URL = "https://example.com/weather?" + url.QueryEscape(queryKey) + "=fixed-secret"
		if _, err := NormalizeTool(tool); err == nil {
			t.Fatalf("NormalizeTool(fixed query credential %q) succeeded", queryKey)
		}
	}
	for _, headerName := range []string{"X-Api-Key", "X-Access-Token", "X-Client-Secret", "X-Signature"} {
		tool = testHTTPTool("get_weather")
		tool.HTTP.Headers = map[string]string{headerName: "fixed-secret"}
		if _, err := NormalizeTool(tool); err == nil {
			t.Fatalf("NormalizeTool(fixed credential header %q) succeeded", headerName)
		}
	}
}

func TestNormalizeToolValidatesTriggers(t *testing.T) {
	t.Parallel()
	tool := testClientTool("volume_set")
	tool.Triggers = []ToolTrigger{{
		Name:     "set volume",
		Patterns: []string{"set volume to {level}"},
		Examples: []ToolTriggerExample{{Input: "set volume", Args: json.RawMessage(`{"level":8}`)}},
		Metadata: json.RawMessage(`{"intent":"device"}`),
	}}
	if _, err := NormalizeTool(tool); err != nil {
		t.Fatalf("NormalizeTool(): %v", err)
	}
	tool.Triggers[0].Examples[0].Args = json.RawMessage(`{`)
	if _, err := NormalizeTool(tool); err == nil {
		t.Fatal("NormalizeTool(invalid trigger JSON) succeeded")
	}

}

func TestServerInvalidStateAndConfigErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := (&Server{}).ListTools(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListTools(no store) = %v", err)
	}
	server := &Server{DB: newTestDatabase(t)}
	if _, err := server.GetTool(ctx, "bad:name"); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("GetTool(invalid name) = %v", err)
	}
	tool, err := server.CreateTool(ctx, testClientTool("bad_json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.DB.ExecContext(ctx, `UPDATE tools SET input_schema_json='{' WHERE id=?`, tool.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.ListTools(ctx); err == nil {
		t.Fatal("ListTools(bad JSON) succeeded")
	}
}

func newTestDatabase(t testing.TB) *sqlx.DB {
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
	if err := (&Server{DB: db}).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestToolUpdateRetainsConcurrentSecretRotation(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(fmt.Sprintf("recreate=%v", recreate), func(t *testing.T) {
			db := newTestDatabase(t)
			first, second := &Server{DB: db}, &Server{DB: db}
			original := testHTTPTool("get_weather")
			original.HTTP.Auth = HTTPAuth{Method: "bearer", BearerToken: new("old")}
			created, err := first.CreateTool(t.Context(), original)
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var enterOnce, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			first.Now = func() time.Time { enterOnce.Do(func() { close(entered); <-release }); return time.Now() }
			desired := cloneTool(created)
			desired.HTTP.Auth.BearerToken = nil
			done := make(chan error, 1)
			go func() { _, err := first.PutTool(t.Context(), created.ID, desired); done <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("update did not reach the write boundary")
			}
			rotated := cloneTool(created)
			rotated.HTTP.Auth.BearerToken = new("new")
			if recreate {
				if err := second.DeleteTool(t.Context(), created.ID); err != nil {
					t.Fatal(err)
				}
				_, err = second.CreateTool(t.Context(), rotated)
			} else {
				_, err = second.PutTool(t.Context(), created.ID, rotated)
			}
			if err != nil {
				t.Fatal(err)
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("update did not complete")
			}
			got, err := second.GetToolByID(t.Context(), created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.HTTP.Auth.BearerToken == nil || *got.HTTP.Auth.BearerToken != "new" {
				t.Fatal("stale update restored the old secret")
			}
		})
	}
}
