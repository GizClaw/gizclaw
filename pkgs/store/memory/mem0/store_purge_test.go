package mem0

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type purgeRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

func newPurgeServer(t *testing.T, residual bool) (*httptest.Server, func() []purgeRequest) {
	t.Helper()
	var (
		mu       sync.Mutex
		requests []purgeRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		request := purgeRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &request.Body)
		}
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			_, _ = io.WriteString(w, `{"message":"Delete in progress.","event_id":"event"}`)
		case residual:
			_, _ = io.WriteString(w, `{"results":[{"id":"left","memory":"still here"}]}`)
		default:
			_, _ = io.WriteString(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(server.Close)
	return server, func() []purgeRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]purgeRequest(nil), requests...)
	}
}

func TestStorePurgeScopeUsesExactEntitySelection(t *testing.T) {
	t.Parallel()
	workspace := Scope{AppID: "workspace-a"}
	for _, test := range []struct {
		name   string
		flavor Flavor
		want   []purgeRequest
	}{
		{
			name: "platform", flavor: Platform,
			want: []purgeRequest{
				{Method: http.MethodDelete, Path: "/v1/memories/", Query: url.Values{"app_id": {"workspace-a"}}},
				{Method: http.MethodPost, Path: "/v3/memories/", Query: url.Values{}, Body: map[string]any{
					"filters": map[string]any{"app_id": "workspace-a"}, "page": float64(1), "page_size": float64(1),
				}},
			},
		},
		{
			name: "self-hosted", flavor: SelfHosted,
			want: []purgeRequest{
				{Method: http.MethodDelete, Path: "/memories", Query: url.Values{"user_id": {encodeSelfHostedScope(workspace)}}},
				{Method: http.MethodGet, Path: "/memories", Query: url.Values{"user_id": {encodeSelfHostedScope(workspace)}}},
			},
		},
		{
			name: "volc", flavor: VolcPlatform,
			want: []purgeRequest{
				{Method: http.MethodDelete, Path: "/v1/memories/", Query: url.Values{"user_id": {volcScopeUserID(workspace)}}},
				{Method: http.MethodGet, Path: "/v1/memories/", Query: url.Values{
					"app_id": {"workspace-a"}, "user_id": {volcScopeUserID(workspace)},
				}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server, requests := newPurgeServer(t, false)
			store, err := New(Config{Endpoint: server.URL, APIKey: "key", Flavor: test.flavor, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			bound, err := memorystore.BindApp(store, workspace.AppID)
			if err != nil {
				t.Fatal(err)
			}
			if err := memorystore.PurgeScope(t.Context(), bound, Scope{}); err != nil {
				t.Fatalf("PurgeScope() error = %v", err)
			}
			empty, err := memorystore.ScopeEmpty(t.Context(), bound, Scope{})
			if err != nil || !empty {
				t.Fatalf("ScopeEmpty() = %v, %v; want true", empty, err)
			}
			if got := requests(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("requests = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestStoreScopeEmptyReportsResidualMemories(t *testing.T) {
	t.Parallel()
	for _, flavor := range []Flavor{Platform, SelfHosted, VolcPlatform} {
		server, _ := newPurgeServer(t, true)
		store, err := New(Config{Endpoint: server.URL, APIKey: "key", Flavor: flavor, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		empty, err := store.ScopeEmpty(context.Background(), Scope{AppID: "workspace-a"})
		if err != nil || empty {
			t.Fatalf("%s ScopeEmpty() = %v, %v; want false", flavor, empty, err)
		}
	}
}

func TestStorePurgeScopeRejectsWideningSelections(t *testing.T) {
	t.Parallel()
	server, requests := newPurgeServer(t, false)
	for _, test := range []struct {
		flavor Flavor
		scope  Scope
		want   error
	}{
		{Platform, Scope{AppID: "*"}, memorystore.ErrInvalidInput},
		{Platform, Scope{}, memorystore.ErrInvalidInput},
		{VolcPlatform, Scope{AppID: "workspace-a", UserID: "user"}, memorystore.ErrUnsupported},
		{VolcPlatform, Scope{UserID: "user"}, memorystore.ErrUnsupported},
	} {
		store, err := New(Config{Endpoint: server.URL, APIKey: "key", Flavor: test.flavor, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PurgeScope(context.Background(), test.scope); !errors.Is(err, test.want) {
			t.Fatalf("%s PurgeScope(%+v) error = %v; want %v", test.flavor, test.scope, err, test.want)
		}
		if _, err := store.ScopeEmpty(context.Background(), test.scope); !errors.Is(err, test.want) {
			t.Fatalf("%s ScopeEmpty(%+v) error = %v; want %v", test.flavor, test.scope, err, test.want)
		}
	}
	if got := requests(); len(got) != 0 {
		t.Fatalf("rejected purges sent requests: %#v", got)
	}
}

func TestStorePurgeScopePropagatesProviderFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	store, err := New(Config{Endpoint: server.URL, APIKey: "key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PurgeScope(context.Background(), Scope{AppID: "workspace-a"}); !errors.Is(err, memorystore.ErrUnavailable) {
		t.Fatalf("PurgeScope() error = %v; want ErrUnavailable", err)
	}
}
