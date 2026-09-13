package gizclaw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeviceRecentLogsRouteIsRemoved(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	if err := f.manager.PeerRun.SetDebugMode(context.Background(), f.owner, "readonly"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/gizclaw/v1/device/logs", nil)
	req.Header.Set("Authorization", "Bearer gizclaw_pk_"+f.owner.String())
	res := httptest.NewRecorder()
	f.handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404: %s", res.Code, res.Body.String())
	}
}
