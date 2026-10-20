package infraflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func assertFlowCORS(t *testing.T, header http.Header, origin string) {
	t.Helper()
	if header.Get("Access-Control-Allow-Origin") != origin || header.Get("Access-Control-Allow-Methods") != "GET,OPTIONS" ||
		header.Get("Access-Control-Allow-Headers") != "Authorization,Content-Type" || header.Get("Cache-Control") != "no-store" ||
		header.Get("Access-Control-Allow-Credentials") != "" || header.Get("WWW-Authenticate") != "" {
		t.Fatalf("public read headers: %v", header)
	}
	if origin != "*" && !strings.Contains(header.Get("Vary"), "Origin") {
		t.Fatal("missing Origin variation")
	}
}

func TestPublicReadIgnoresCredentialsAndKeepsEnvironmentAndCursorBounds(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	feed, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().Truncate(Window)
	for i := 3; i >= 0; i-- {
		if err := feed.Append(t.Context(), fixtureFrame(end.Add(-time.Duration(i)*Window), 1000000)); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(Handler(Config{Store: "flows", ReadToken: "ignored-legacy-value"}, feed, nil))
	t.Cleanup(server.Close)
	client, err := infraapi.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, authorization := range []string{"", "Bearer gizclaw_sk_v1_invalid", "Bearer gizclaw_pk_invalid", "Bearer gizclaw_mk_invalid", "malformed"} {
		response, err := client.ReadInfraFlowsWithResponse(t.Context(), &infraapi.ReadInfraFlowsParams{Environment: infraapi.Prod}, func(_ context.Context, request *http.Request) error {
			if authorization != "" {
				request.Header.Set("Authorization", authorization)
			}
			return nil
		})
		if err != nil || response.JSON200 == nil || len(response.JSON200.Frames) != 1 || len(response.JSON200.Frames[0].Flows) != MaxFlows {
			t.Fatalf("public read with %q: %+v %v", authorization, response, err)
		}
		assertFlowCORS(t, response.HTTPResponse.Header, "*")
		wantLimits := infraapi.Limits{WindowMs: 5000, MaxFlows: 64, RetentionFrames: 120, MaxReadFrames: 3}
		if response.JSON200.Limits != wantLimits {
			t.Fatalf("feed bounds changed: %+v", response.JSON200.Limits)
		}
	}
	for _, env := range []infraapi.Environment{infraapi.Dev, infraapi.E2e, infraapi.Trial} {
		response, err := client.ReadInfraFlowsWithResponse(t.Context(), &infraapi.ReadInfraFlowsParams{Environment: env})
		if err != nil || response.JSON200 == nil || len(response.JSON200.Frames) != 0 {
			t.Fatalf("environment isolation: %+v %v", response, err)
		}
	}
	for _, test := range []struct {
		cursor string
		frames int
		reset  bool
	}{
		{strconvIDTest(end.UnixMilli()), 0, false},
		{strconvIDTest(end.Add(-3 * Window).UnixMilli()), 3, false},
		{strconvIDTest(end.Add(-4 * Window).UnixMilli()), 1, true},
		{strconvIDTest(end.Add(Window).UnixMilli()), 1, true},
		{"8640000000000000", 1, true},
	} {
		response, err := client.ReadInfraFlowsWithResponse(t.Context(), &infraapi.ReadInfraFlowsParams{Environment: infraapi.Prod, Cursor: &test.cursor})
		if err != nil || response.JSON200 == nil || len(response.JSON200.Frames) != test.frames || response.JSON200.Reset != test.reset {
			t.Fatalf("cursor %s: %+v %v", test.cursor, response, err)
		}
		if len(response.Body) > MaxReadFrames*MaxFrameBytes+1024 {
			t.Fatalf("oversized page: %d bytes", len(response.Body))
		}
	}
}

func TestReadHTTPDisabledUnavailableAndExactPath(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	feed, _ := New(store)
	unavailable, _ := New(&failingKV{Store: store})
	for _, test := range []struct {
		name string
		cfg  Config
		feed *Feed
		code string
	}{
		{"disabled", Config{}, nil, "FLOW_FEED_DISABLED"},
		{"implicit-store", Config{}, feed, "FLOW_FEED_DISABLED"},
		{"legacy-token-only", Config{ReadToken: "ignored"}, feed, "FLOW_FEED_DISABLED"},
		{"missing-feed", Config{Store: "flows"}, nil, "FLOW_FEED_DISABLED"},
		{"invalid-store-name", Config{Store: " flows "}, feed, "FLOW_FEED_DISABLED"},
		{"unavailable", Config{Store: "flows"}, unavailable, "FLOW_FEED_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := Handler(test.cfg, test.feed, nil)
			request := httptest.NewRequest(http.MethodGet, Path+"?environment=prod", nil)
			request.Header.Set("Origin", "https://globe.example")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != 503 || response.Body.String() != "{\"error\":\""+test.code+"\"}\n" {
				t.Fatalf("feed error: %d %s", response.Code, response.Body.String())
			}
			assertFlowCORS(t, response.Header(), "https://globe.example")
			preflight := httptest.NewRecorder()
			handler.ServeHTTP(preflight, httptest.NewRequest(http.MethodOptions, Path, nil))
			if preflight.Code != 204 || preflight.Body.Len() != 0 {
				t.Fatalf("disabled preflight: %d %s", preflight.Code, preflight.Body.String())
			}
			assertFlowCORS(t, preflight.Header(), "*")
		})
	}
	handler := Handler(Config{Store: "flows"}, feed, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	for _, path := range []string{Path + "/", Path + "/other", "/monitor/api/node", "/gizclaw/v1/device"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusTeapot || response.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("feed intercepted adjacent route %s: %d", path, response.Code)
		}
	}
}

func TestPublicReadHTTPRetainsTwoSecondDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &firstReaderDeadlineKV{Store: kv.NewMemory(nil)}
		t.Cleanup(func() { _ = store.Close() })
		feed, _ := New(store)
		handler := Handler(Config{Store: "flows"}, feed, nil)
		response := httptest.NewRecorder()
		started := time.Now()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, Path+"?environment=prod", nil))
		if time.Since(started) != 2*time.Second || response.Code != 503 || !strings.Contains(response.Body.String(), "FLOW_FEED_UNAVAILABLE") {
			t.Fatalf("read deadline: %v %d %s", time.Since(started), response.Code, response.Body.String())
		}
	})
}
