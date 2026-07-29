//go:build gizclaw_e2e

package rpc_test

import (
	"testing"
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
}
