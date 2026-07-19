package volc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
)

type resolverFunc func(context.Context, Config) (string, error)

func (f resolverFunc) ResolveMem0APIKey(ctx context.Context, config Config) (string, error) {
	return f(ctx, config)
}

func TestOpenUsesInjectedResolverAndMem0Client(t *testing.T) {
	t.Parallel()
	var (
		gotAuthorization string
		gotUserIDs       []string
		resolverCalls    int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotUserID, _ := body["user_id"].(string)
		if filters, ok := body["filters"].(map[string]any); ok {
			gotUserID, _ = filters["user_id"].(string)
		}
		gotUserIDs = append(gotUserIDs, gotUserID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()
	store, err := Open(context.Background(), Config{
		Mem0: mem0.Config{Endpoint: server.URL, HTTPClient: server.Client()}, APIKeyID: "key-id",
		Resolver: resolverFunc(func(context.Context, Config) (string, error) {
			resolverCalls++
			return "resolved-key", nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []memorystore.Scope{"conversation", "other-conversation"} {
		if _, err := store.Observe(context.Background(), memorystore.Observation{Scope: scope, Text: "remember"}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Recall(context.Background(), memorystore.Query{Scope: scope, Text: "remember", Limit: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if resolverCalls != 1 || gotAuthorization != "Token resolved-key" || !reflect.DeepEqual(gotUserIDs, []string{"conversation", "conversation", "other-conversation", "other-conversation"}) {
		t.Fatalf("calls=%d auth=%q user_ids=%q", resolverCalls, gotAuthorization, gotUserIDs)
	}
}

func TestOpenExplicitKeySkipsResolver(t *testing.T) {
	t.Parallel()
	called := false
	store, err := Open(context.Background(), Config{
		Mem0: mem0.Config{Endpoint: "https://example.test", APIKey: "explicit"},
		Resolver: resolverFunc(func(context.Context, Config) (string, error) {
			called = true
			return "", errors.New("unexpected")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if called || store == nil {
		t.Fatalf("resolver called=%v store=%v", called, store)
	}
}

func TestOpenValidation(t *testing.T) {
	t.Parallel()
	for _, config := range []Config{
		{},
		{Mem0: mem0.Config{Endpoint: "https://example.test"}, Resolver: resolverFunc(func(context.Context, Config) (string, error) { return "", nil })},
		{Mem0: mem0.Config{Endpoint: "https://example.test"}, AccessKeyID: "id"},
		{Mem0: mem0.Config{Endpoint: "https://example.test"}, AccessKeyID: "id", AccessKeySecret: "secret", Region: "INVALID"},
	} {
		if _, err := Open(context.Background(), config); !errors.Is(err, memorystore.ErrInvalidInput) && !errors.Is(err, memorystore.ErrUnavailable) {
			t.Fatalf("Open(%+v) error = %v", config, err)
		}
	}
}

func TestConfigHasNoSerializationTags(t *testing.T) {
	t.Parallel()
	typeOfConfig := reflect.TypeFor[Config]()
	for field := range typeOfConfig.Fields() {
		if field.Tag.Get("yaml") != "" || field.Tag.Get("json") != "" {
			t.Fatalf("Config.%s contains serialization tags", field.Name)
		}
	}
}

func TestVolcControlAddress(t *testing.T) {
	t.Parallel()
	scheme, host, err := volcControlAddress("", "cn-shanghai")
	if err != nil || scheme != "https" || host != "mem0.cn-shanghai.volcengineapi.com" {
		t.Fatalf("default address = %q %q, %v", scheme, host, err)
	}
	for _, endpoint := range []string{"ftp://example.test", "https://user@example.test", "https://example.test/path"} {
		if _, _, err := volcControlAddress(endpoint, "cn-beijing"); !errors.Is(err, memorystore.ErrInvalidInput) {
			t.Fatalf("endpoint %q error = %v", endpoint, err)
		}
	}
}
