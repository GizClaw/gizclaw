package observability

import (
	"context"
	"log/slog"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

func TestCompletionIdentitySurvivesProductionLogger(t *testing.T) {
	logger, closeLogger, err := gizlog.NewLogger(gizlog.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closeLogger(); err != nil {
			t.Error(err)
		}
	})
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })
	before := len(gizlog.ReadMonitorLogs("monitor-completion-owner"))
	outcome := NewOutcome(TransportHTTP, SurfacePeerHTTP, "getDevice")
	outcome.SetPeer("monitor-completion-owner", "client")
	Log(gizlog.WithPeerPublicKey(context.Background(), "edge-transport-peer"), outcome)
	entries := gizlog.ReadMonitorLogs("monitor-completion-owner")
	if len(entries) != before+1 || entries[0].Message != CompletionMessage {
		t.Fatalf("authenticated completion missing from peer logs: %+v", entries)
	}
	if entries := gizlog.ReadMonitorLogs("edge-transport-peer"); len(entries) != 0 {
		t.Fatal("completion retained transport peer instead of authenticated owner")
	}
}
