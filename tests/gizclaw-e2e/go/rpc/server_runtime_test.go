//go:build gizclaw_e2e

package rpc_test

import "testing"

func TestServerRuntimeRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	runtime, err := env.peer.GetServerRuntime(env.ctx, "server.runtime.get")
	if err != nil {
		t.Fatalf("server.runtime.get: %v", err)
	}
	if !runtime.Online {
		t.Fatalf("server.runtime.get online = false: %#v", runtime)
	}
	if runtime.LastSeenAt.IsZero() {
		t.Fatalf("server.runtime.get last_seen_at is zero: %#v", runtime)
	}
}
