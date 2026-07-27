package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestServerPutGetListDeleteAndDefensiveCopies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	server := &Server{Store: kv.NewMemory(nil), Now: func() time.Time { return now }}
	tool := testClientTool("volume_set")
	tool.Metadata = json.RawMessage(`{"category":"device"}`)
	created, err := server.PutTool(ctx, tool)
	if err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("timestamps = %s/%s", created.CreatedAt, created.UpdatedAt)
	}
	created.InputSchema.Type = "string"
	created.Metadata[0] = '['
	got, err := server.GetTool(ctx, tool.Name)
	if err != nil {
		t.Fatalf("GetTool(): %v", err)
	}
	if got.InputSchema.Type != "object" || string(got.Metadata) != `{"category":"device"}` {
		t.Fatalf("stored Tool was mutated: %#v", got)
	}
	now = now.Add(time.Minute)
	got.Version = new("2")
	updated, err := server.PutTool(ctx, got)
	if err != nil {
		t.Fatalf("PutTool(update): %v", err)
	}
	if updated.CreatedAt != created.CreatedAt || updated.UpdatedAt != now {
		t.Fatalf("updated timestamps = %s/%s", updated.CreatedAt, updated.UpdatedAt)
	}
	if _, err := server.PutTool(ctx, testHTTPTool("get_weather")); err != nil {
		t.Fatalf("PutTool(second): %v", err)
	}
	items, err := server.ListTools(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListTools() = %d, %v", len(items), err)
	}
	if err := server.DeleteTool(ctx, tool.Name); err != nil {
		t.Fatalf("DeleteTool(): %v", err)
	}
	if _, err := server.GetTool(ctx, tool.Name); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("GetTool(deleted) = %v", err)
	}
}

func TestServerRetainsRotatesAndDropsDirectSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := &Server{Store: kv.NewMemory(nil)}
	tool := testHTTPTool("get_weather")
	tool.HTTP.Auth = HTTPAuth{Method: "bearer", BearerToken: new("first")}
	if _, err := server.PutTool(ctx, tool); err != nil {
		t.Fatalf("PutTool(create): %v", err)
	}
	tool.HTTP.Auth.BearerToken = nil
	retained, err := server.PutTool(ctx, tool)
	if err != nil {
		t.Fatalf("PutTool(retain): %v", err)
	}
	if retained.HTTP.Auth.BearerToken == nil || *retained.HTTP.Auth.BearerToken != "first" {
		t.Fatalf("retained secret = %#v", retained.HTTP.Auth.BearerToken)
	}
	tool.HTTP.Auth.BearerToken = new("second")
	rotated, err := server.PutTool(ctx, tool)
	if err != nil {
		t.Fatalf("PutTool(rotate): %v", err)
	}
	if *rotated.HTTP.Auth.BearerToken != "second" {
		t.Fatalf("rotated secret = %#v", rotated.HTTP.Auth.BearerToken)
	}
	tool.HTTP.Auth = HTTPAuth{Method: "none"}
	changed, err := server.PutTool(ctx, tool)
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

func TestNormalizeToolValidatesTriggersAndStrictLegacyPersistence(t *testing.T) {
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

	server := &Server{Store: kv.NewMemory(nil)}
	legacy := []byte(`{"id":"legacy","source":"builtin","enabled":true,"input_schema":{"type":"object"},"executor":{"kind":"builtin"}}`)
	if err := server.Store.Set(context.Background(), toolKey("legacy"), legacy); err != nil {
		t.Fatalf("raw legacy Set(): %v", err)
	}
	if _, err := server.GetTool(context.Background(), "legacy"); err == nil {
		t.Fatal("legacy persisted Tool was accepted")
	}
}

func TestServerInvalidStateAndConfigErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := (&Server{}).ListTools(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListTools(no store) = %v", err)
	}
	server := &Server{Store: kv.NewMemory(nil)}
	if _, err := server.GetTool(ctx, "bad:name"); !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("GetTool(invalid name) = %v", err)
	}
	if err := server.Store.Set(ctx, toolKey("bad_json"), []byte(`{`)); err != nil {
		t.Fatalf("raw Set(): %v", err)
	}
	if _, err := server.ListTools(ctx); err == nil {
		t.Fatal("ListTools(bad JSON) succeeded")
	}
}
