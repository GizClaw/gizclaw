//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerInfoRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	info, err := env.peer.GetServerInfo(env.ctx, "server.info.get.initial")
	if err != nil {
		t.Fatalf("server.info.get initial: %v", err)
	}
	if info.Sn == nil || *info.Sn != "peer-a-sn" {
		t.Fatalf("server.info.get initial = %#v, want peer-a sn", info)
	}

	name := "RPC Peer A"
	put, err := env.peer.PutServerInfo(env.ctx, "server.info.put", rpcapi.ServerPutInfoRequest{
		Sn:   testStringPtr("peer-a-sn-updated"),
		Name: &name,
	})
	if err != nil {
		t.Fatalf("server.info.put: %v", err)
	}
	if put.Name == nil || *put.Name != name {
		t.Fatalf("server.info.put name = %#v, want %q", put.Name, name)
	}
	got, err := env.peer.GetServerInfo(env.ctx, "server.info.get.updated")
	if err != nil {
		t.Fatalf("server.info.get updated: %v", err)
	}
	if got.Sn == nil || *got.Sn != "peer-a-sn-updated" || got.Name == nil || *got.Name != name {
		t.Fatalf("server.info.get updated = %#v", got)
	}
}
