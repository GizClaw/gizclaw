package monitorweb

import (
	"io/fs"
	"net/http/httptest"
	"testing"
)

func TestMonitorRoutes(t *testing.T) {
	handler := Handler()
	_, buildErr := fs.Stat(assets, "dist/index.html")
	for _, path := range []string{"/monitor/node", "/monitor/peer"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		want := 200
		if buildErr != nil {
			want = 503
		}
		if response.Code != want {
			t.Fatalf("%s: status %d, want %d", path, response.Code, want)
		}
		if response.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
	for _, path := range []string{"/monitor/README.md", "/monitor/assets/", "/monitor/unknown"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 404 {
			t.Fatalf("%s: status %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/monitor/node", nil))
	if response.Code != 405 {
		t.Fatalf("POST: status %d", response.Code)
	}
}
