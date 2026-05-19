package gizrun

import (
	"context"
	"testing"
)

func TestTagHTTP(t *testing.T) {
	ctx := tagHTTP(context.Background(), httpMethod, "POST", httpPath, "/v1/chat")
	ctx = tagHTTP(ctx, httpStatusCode, "200")

	ns, ok := httpLabels(ctx)
	if !ok {
		t.Fatal("httpLabels ok = false, want true")
	}
	for key, want := range map[string]string{
		httpMethod:     "POST",
		httpPath:       "/v1/chat",
		httpStatusCode: "200",
	} {
		if got, ok := ns.Value(key); !ok || got != want {
			t.Fatalf("HTTP namespace value %q = (%q, %v), want (%q, true)", key, got, ok, want)
		}
	}
}

func TestTagGenx(t *testing.T) {
	ctx := tagGenx(context.Background(), genxProvider, "openai", genxModel, "gpt-test")
	ctx = tagGenx(ctx, genxTokenType, tokenPrompt)

	ns, ok := genxLabels(ctx)
	if !ok {
		t.Fatal("genxLabels ok = false, want true")
	}
	for key, want := range map[string]string{
		genxProvider:  "openai",
		genxModel:     "gpt-test",
		genxTokenType: tokenPrompt,
	} {
		if got, ok := ns.Value(key); !ok || got != want {
			t.Fatalf("Genx namespace value %q = (%q, %v), want (%q, true)", key, got, ok, want)
		}
	}
}

func TestTagLogSink(t *testing.T) {
	ctx := tagLogSink(context.Background(), "status", "ok")

	ns, ok := logSinkLabels(ctx)
	if !ok {
		t.Fatal("logSinkLabels ok = false, want true")
	}
	if got, ok := ns.Value("status"); !ok || got != "ok" {
		t.Fatalf("logsink namespace status = (%q, %v), want (%q, true)", got, ok, "ok")
	}
}

func TestTag(t *testing.T) {
	ctx := tag(context.Background(), "custom", "key", "value")

	ns, ok := labels(ctx, "custom")
	if !ok {
		t.Fatal("labels(custom) ok = false, want true")
	}
	if got, ok := ns.Value("key"); !ok || got != "value" {
		t.Fatalf("custom namespace key = (%q, %v), want (%q, true)", got, ok, "value")
	}
}
