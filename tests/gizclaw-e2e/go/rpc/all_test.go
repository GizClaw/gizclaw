//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestAllRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	ping, err := env.peer.Ping(env.ctx, "all.ping")
	if err != nil {
		t.Fatalf("all.ping: %v", err)
	}
	if ping == nil || ping.ServerTime == 0 {
		t.Fatalf("all.ping = %#v, want server time", ping)
	}

	speed, err := env.peer.SpeedTest(env.ctx, "all.speed_test.run", rpcapi.SpeedTestRequest{
		UpContentLength:   1024,
		DownContentLength: 1024,
	})
	if err != nil {
		t.Fatalf("all.speed_test.run: %v", err)
	}
	if speed.UpBytes != 1024 || speed.DownBytes != 1024 {
		t.Fatalf("speed test bytes = %+v, want 1024/1024", speed)
	}
}
