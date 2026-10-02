package mem0_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mem0 "github.com/GizClaw/gizclaw-go/sdk/go/mem0"
)

func TestGeneratedClientPreservesPolicyScopeAndProviderMetadata(t *testing.T) {
	t.Parallel()
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/memories" || r.Header.Get("X-API-Key") != "fixture" {
			t.Errorf("incorrect route or authentication")
		}
		var body mem0.MemoryCreate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.UserId == nil || *body.UserId != "scope" || body.Prompt == nil {
			t.Error("policy/scope missing")
		}
		prompts = append(prompts, *body.Prompt)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"fact","memory":"fixture","metadata":{"source":"fixture"},"provider_extra":true}]}`))
	}))
	defer server.Close()
	client, err := mem0.NewClientWithResponses(server.URL+"/prefix", mem0.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
		r.Header.Set("X-API-Key", "fixture")
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	user := "scope"
	for _, policy := range []string{"pet policy", "calendar policy"} {
		response, err := client.AddMemoryWithResponse(t.Context(), mem0.MemoryCreate{
			Messages: []mem0.Message{{Role: "user", Content: "fixture"}}, UserId: &user, Prompt: &policy,
		})
		if err != nil {
			t.Fatal(err)
		}
		if response.JSON200 == nil || len(response.JSON200.Results) != 1 || response.JSON200.Results[0].AdditionalProperties["provider_extra"] != true {
			t.Fatalf("provider record metadata lost: %+v", response)
		}
	}
	if prompts[0] != "pet policy" || prompts[1] != "calendar policy" {
		t.Fatalf("policy changed: %v", prompts)
	}
}

func TestGeneratedClientReturnsTypedAuthenticationError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid Mem0 API key"}`))
	}))
	defer server.Close()
	client, err := mem0.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ListMemoriesWithResponse(t.Context(), &mem0.ListMemoriesParams{})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode() != http.StatusUnauthorized || response.JSON401 == nil || response.JSON401.Detail != "Invalid Mem0 API key" {
		t.Fatalf("authentication failure lost: %+v", response)
	}
}

func TestGeneratedClientReturnsTypedProviderRateLimit(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail":"Mem0 model provider rate limit exceeded"}`))
	}))
	defer server.Close()
	client, err := mem0.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.AddMemoryWithResponse(t.Context(), mem0.MemoryCreate{
		Messages: []mem0.Message{{Role: "user", Content: "fixture"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode() != http.StatusTooManyRequests || response.JSON429 == nil || response.JSON429.Detail != "Mem0 model provider rate limit exceeded" {
		t.Fatalf("rate limit response lost: %+v", response)
	}
}
