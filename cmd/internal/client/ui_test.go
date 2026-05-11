package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestUIAPIProxyReusesHealthyClient(t *testing.T) {
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		connects.Add(1)
		return fake, nil
	}, time.Second)
	defer proxy.Close()

	for i := range 2 {
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("ServeHTTP(%d) status = %d", i, rec.Code)
		}
	}
	if got := connects.Load(); got != 1 {
		t.Fatalf("connects = %d, want 1", got)
	}
	if fake.closed.Load() {
		t.Fatal("healthy client was closed")
	}
}

func TestUIAPIProxyInvalidatesTimedOutClient(t *testing.T) {
	first := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			http.Error(w, "deadline", http.StatusGatewayTimeout)
		}),
	}
	second := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		switch connects.Add(1) {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			t.Fatal("unexpected reconnect")
			return nil, errors.New("unexpected reconnect")
		}
	}, 10*time.Millisecond)
	defer proxy.Close()

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP status = %d", rec.Code)
	}
	if !first.closed.Load() {
		t.Fatal("timed out client was not closed")
	}
	if got := connects.Load(); got != 2 {
		t.Fatalf("connects = %d, want 2", got)
	}
}

func TestUIAPIProxyRetriesBadGatewayClient(t *testing.T) {
	first := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}),
	}
	second := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		switch connects.Add(1) {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			t.Fatal("unexpected reconnect")
			return nil, errors.New("unexpected reconnect")
		}
	}, time.Second)
	defer proxy.Close()

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/minimax-tenants", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP status = %d", rec.Code)
	}
	if !first.closed.Load() {
		t.Fatal("bad gateway client was not closed")
	}
	if got := connects.Load(); got != 2 {
		t.Fatalf("connects = %d, want 2", got)
	}
}

func TestUIAPIProxyInvalidatesCanceledClient(t *testing.T) {
	fake := &fakeUIAPIProxyClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			http.Error(w, "canceled", http.StatusGatewayTimeout)
		}),
	}
	var connects atomic.Int32
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		connects.Add(1)
		return fake, nil
	}, time.Second)
	defer proxy.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req.WithContext(ctx))
	if !fake.closed.Load() {
		t.Fatal("canceled client was not closed")
	}
}

func TestUIAPIProxyConnectErrorReturnsUnavailable(t *testing.T) {
	proxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		return nil, errors.New("dial failed")
	}, time.Second)

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/credentials", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ServeHTTP status = %d", rec.Code)
	}
}

func TestAdminUIRedirectsLegacyWorkspaceTemplateRoutes(t *testing.T) {
	mux := http.NewServeMux()
	registerAdminUIRoutes(mux)

	for _, path := range []string{"/workspace-templates", "/workspace-templates/demo", "/ai/workspace-templates", "/ai/workspace-templates/demo"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusFound {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusFound)
		}
		if got := rec.Header().Get("Location"); got != "/ai/workflows" {
			t.Fatalf("GET %s Location = %q, want /ai/workflows", path, got)
		}
	}
}

type fakeUIAPIProxyClient struct {
	handler http.Handler
	closed  atomic.Bool
}

func (c *fakeUIAPIProxyClient) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *fakeUIAPIProxyClient) ProxyHandler() http.Handler {
	return c.handler
}
