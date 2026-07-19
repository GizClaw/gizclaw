package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeVolcResolver struct{ calls atomic.Int32 }

func (r *fakeVolcResolver) ResolveMem0APIKey(context.Context, VolcConfig) (string, error) {
	r.calls.Add(1)
	return "resolved-key", nil
}

func TestVolcStoreResolvesKeyAndUsesMem0DataPlane(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token resolved-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"results":[{"id":"fact","memory":"clue"}]}`))
	}))
	t.Cleanup(server.Close)
	resolver := &fakeVolcResolver{}
	store, err := OpenVolcStore(context.Background(), VolcConfig{Mem0: Mem0Config{Endpoint: server.URL, UserID: "user"}, APIKeyID: "key-id"}, resolver, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Recall(context.Background(), Query{Text: "clue", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls.Load() != 1 || len(result.Matches) != 1 {
		t.Fatalf("resolver calls = %d, result = %+v", resolver.calls.Load(), result)
	}
}

func TestVolcStoreSkipsResolverForExplicitMem0Key(t *testing.T) {
	t.Parallel()
	resolver := &fakeVolcResolver{}
	_, err := OpenVolcStore(context.Background(), VolcConfig{Mem0: Mem0Config{Endpoint: "https://example.invalid", APIKey: "explicit", UserID: "user"}}, resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls.Load() != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls.Load())
	}
}

func TestVolcCredentialClientResolvesProjectAPIKey(t *testing.T) {
	t.Parallel()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "" {
			t.Error("control-plane request is not signed")
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Query().Get("Action") {
		case "DescribeMemoryProjectDetail":
			var body map[string]string
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["MemoryProjectId"] != "project" {
				t.Errorf("project body = %v", body)
			}
			_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"APIKeyInfos":[{"APIKeyId":"creating-key","Status":"Creating"},{"APIKeyId":"key-id","Status":"Ready"}]}}`))
		case "DescribeAPIKeyDetail":
			var body map[string]string
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["MemoryProjectId"] != "project" || body["APIKeyId"] != "key-id" {
				t.Errorf("API key body = %v", body)
			}
			_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"APIKeyValue":"resolved-key"}}`))
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
	}))
	t.Cleanup(control.Close)
	client, err := newVolcCredentialClient(VolcConfig{ControlEndpoint: control.URL, AccessKeyID: "ak", AccessKeySecret: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.ResolveMem0APIKey(context.Background(), VolcConfig{MemoryProjectID: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if key != "resolved-key" {
		t.Fatalf("resolved key = %q", key)
	}
}

func TestVolcCredentialClientUsesRegionalControlHost(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		region     string
		wantRegion string
		wantHost   string
	}{
		{name: "default", wantRegion: "cn-beijing", wantHost: "mem0.cn-beijing.volcengineapi.com"},
		{name: "shanghai", region: "cn-shanghai", wantRegion: "cn-shanghai", wantHost: "mem0.cn-shanghai.volcengineapi.com"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := newVolcCredentialClient(VolcConfig{Region: test.region, AccessKeyID: "ak", AccessKeySecret: "sk"})
			if err != nil {
				t.Fatal(err)
			}
			if got := client.client.ServiceInfo.Host; got != test.wantHost {
				t.Fatalf("control host = %q, want %q", got, test.wantHost)
			}
			if got := client.client.ServiceInfo.Credentials.Region; got != test.wantRegion {
				t.Fatalf("credential region = %q, want %q", got, test.wantRegion)
			}
		})
	}
}

func TestVolcCredentialClientRejectsProjectWithoutReadyAPIKey(t *testing.T) {
	t.Parallel()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if action := request.URL.Query().Get("Action"); action != "DescribeMemoryProjectDetail" {
			t.Errorf("unexpected action = %q", action)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"APIKeyInfos":[{"APIKeyId":"creating-key","Status":"Creating"},{"APIKeyId":"released-key","Status":"Released"}]}}`))
	}))
	t.Cleanup(control.Close)
	client, err := newVolcCredentialClient(VolcConfig{ControlEndpoint: control.URL, AccessKeyID: "ak", AccessKeySecret: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ResolveMem0APIKey(context.Background(), VolcConfig{MemoryProjectID: "project"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ResolveMem0APIKey() error = %v, want ErrUnavailable", err)
	}
}

func TestVolcCredentialClientResolvesExplicitAPIKeyWithinProject(t *testing.T) {
	t.Parallel()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if action := request.URL.Query().Get("Action"); action != "DescribeAPIKeyDetail" {
			t.Errorf("Action = %q", action)
		}
		var body map[string]string
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body["MemoryProjectId"] != "project" || body["APIKeyId"] != "key-id" {
			t.Errorf("API key body = %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"APIKeyValue":"resolved-key"}}`))
	}))
	t.Cleanup(control.Close)
	client, err := newVolcCredentialClient(VolcConfig{ControlEndpoint: control.URL, AccessKeyID: "ak", AccessKeySecret: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.ResolveMem0APIKey(context.Background(), VolcConfig{MemoryProjectID: "project", APIKeyID: "key-id"})
	if err != nil {
		t.Fatal(err)
	}
	if key != "resolved-key" {
		t.Fatalf("resolved key = %q", key)
	}
}

func TestVolcValidationAndErrorMapping(t *testing.T) {
	t.Parallel()
	resolver := &fakeVolcResolver{}
	if _, err := OpenVolcStore(context.Background(), VolcConfig{APIKeyID: "key-id"}, resolver, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing data-plane endpoint error = %v", err)
	}
	if resolver.calls.Load() != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls.Load())
	}
	if _, err := newVolcCredentialClient(VolcConfig{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("credentials error = %v", err)
	}
	credentialClient, err := newVolcCredentialClient(VolcConfig{AccessKeyID: "ak", AccessKeySecret: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := credentialClient.ResolveMem0APIKey(context.Background(), VolcConfig{APIKeyID: "key-id"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing memory project error = %v", err)
	}
	if _, err := newVolcCredentialClient(VolcConfig{ControlEndpoint: "https://user:pass@example.test", AccessKeyID: "ak", AccessKeySecret: "sk"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("endpoint error = %v", err)
	}
	if _, err := newVolcCredentialClient(VolcConfig{ControlEndpoint: "ftp://example.test", AccessKeyID: "ak", AccessKeySecret: "sk"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("scheme error = %v", err)
	}
	if _, err := newVolcCredentialClient(VolcConfig{ControlEndpoint: "example.test/path", AccessKeyID: "ak", AccessKeySecret: "sk"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("host-only endpoint error = %v", err)
	}
	if _, err := newVolcCredentialClient(VolcConfig{Region: "cn-beijing/path", AccessKeyID: "ak", AccessKeySecret: "sk"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("region error = %v", err)
	}
	if _, err := OpenVolcStore(context.Background(), VolcConfig{Mem0: Mem0Config{Endpoint: "https://example.test", UserID: "user"}}, fakeVolcResolverError{}, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("resolver error = %v", err)
	}
	if _, err := OpenVolcStore(context.Background(), VolcConfig{Mem0: Mem0Config{Endpoint: "https://example.test", UserID: "user"}}, memoryVolcEmptyResolver{}, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty resolver error = %v", err)
	}
	for status, want := range map[int]error{http.StatusNotFound: ErrNotFound, http.StatusBadRequest: ErrInvalidInput, http.StatusBadGateway: ErrUnavailable} {
		if err := mapVolcControlError("test", status, errors.New("failed")); !errors.Is(err, want) {
			t.Fatalf("status %d error = %v", status, err)
		}
	}
	if err := mapVolcControlError("test", 0, context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	for code, want := range map[string]error{"NotFound": ErrNotFound, "InvalidParameter": ErrInvalidInput, strings.Repeat("Internal", 40): ErrUnavailable} {
		err := (volcResponseMetadata{Error: &struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}{Code: code}}).err()
		if !errors.Is(err, want) {
			t.Fatalf("metadata code %q error = %v", code, err)
		}
	}
}

type fakeVolcResolverError struct{}

func (fakeVolcResolverError) ResolveMem0APIKey(context.Context, VolcConfig) (string, error) {
	return "", fmt.Errorf("%w: resolver", ErrUnavailable)
}

type memoryVolcEmptyResolver struct{}

func (memoryVolcEmptyResolver) ResolveMem0APIKey(context.Context, VolcConfig) (string, error) {
	return " ", nil
}
