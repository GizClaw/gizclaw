package gizclaw

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

func TestDeviceMonitorLogsUseAuthenticatedOwner(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	logger, closeLogger, err := gizlog.NewLogger(gizlog.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer closeLogger()
	logger.InfoContext(gizlog.WithPeerPublicKey(context.Background(), f.owner.String()), "monitor-owner-record", slog.String("error", "visible-error-detail"))
	logger.InfoContext(gizlog.WithPeerPublicKey(context.Background(), "another-peer"), "another-monitor-peer")
	// A caller-supplied identity attribute must not grant ownership of the record.
	logger.Info("forged-monitor-peer", slog.String("peer_public_key", f.owner.String()))
	if err := f.manager.PeerRun.SetDebugMode(context.Background(), f.owner, "readonly"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/gizclaw/v1/device/logs?public_key=another-peer", nil)
	req.Header.Set("Authorization", "Bearer gizclaw_pk_"+f.owner.String())
	res := httptest.NewRecorder()
	f.handler.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("status=%d %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "monitor-owner-record") || !strings.Contains(body, "visible-error-detail") || strings.Contains(body, "another-monitor-peer") || strings.Contains(body, "forged-monitor-peer") {
		t.Fatalf("unexpected logs: %s", body)
	}
	if err := f.manager.PeerRun.SetDebugMode(context.Background(), f.owner, "off"); err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	f.handler.ServeHTTP(res, req)
	if res.Code != 403 {
		t.Fatal("off mode allowed logs")
	}
}
