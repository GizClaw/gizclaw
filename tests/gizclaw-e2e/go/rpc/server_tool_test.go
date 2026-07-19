//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestServerToolPeerCRUD(t *testing.T) {
	env := newServerResourceHarness(t)
	peerID := env.h.ContextPublicKey("peer-a")
	id := "peer." + peerID + ".e2e.echo"
	method := "echo"

	created, err := env.peer.CreateTool(env.ctx, "tool.create", rpcapi.Tool{
		Id:          id,
		Source:      rpcapi.ToolSourceDevice,
		InputSchema: jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{"text": {Type: "string"}}},
		Executor:    rpcapi.ToolExecutor{Kind: rpcapi.ToolExecutorKindDeviceRpc, Method: &method},
	})
	if err != nil {
		t.Fatalf("server.tool.create: %v", err)
	}
	if created.Enabled == nil || !*created.Enabled || created.OwnerPeer == nil || *created.OwnerPeer != peerID || created.OwnerPublicKey == nil || *created.OwnerPublicKey != peerID || created.Executor.PeerId == nil || *created.Executor.PeerId != peerID {
		t.Fatalf("created Tool = %#v", created)
	}
	t.Cleanup(func() {
		_, _ = env.peer.DeleteTool(env.ctx, "tool.cleanup", rpcapi.ToolDeleteRequest{Id: id})
	})

	got, err := env.peer.GetTool(env.ctx, "tool.get", rpcapi.ToolGetRequest{Id: id})
	if err != nil || got.Id != id {
		t.Fatalf("server.tool.get = %#v, %v", got, err)
	}
	listed, err := env.peer.ListTools(env.ctx, "tool.list", rpcapi.ToolListRequest{})
	if err != nil {
		t.Fatalf("server.tool.list: %v", err)
	}
	found := false
	for _, item := range listed.Items {
		found = found || item.Id == id
	}
	if !found {
		t.Fatalf("server.tool.list missing %q: %#v", id, listed.Items)
	}
	denied := env.h.ConnectClientFromContext("peer-denied")
	t.Cleanup(func() { denied.Close() })
	if _, err := denied.GetTool(env.ctx, "tool.get.denied", rpcapi.ToolGetRequest{Id: id}); err == nil {
		t.Fatal("server.tool.get from an unbound peer unexpectedly succeeded")
	}

	description := "updated by rpc e2e"
	created.Description = &description
	updated, err := env.peer.PutTool(env.ctx, "tool.put", rpcapi.ToolPutRequest{Id: id, Body: *created})
	if err != nil || updated.Description == nil || *updated.Description != description {
		t.Fatalf("server.tool.put = %#v, %v", updated, err)
	}
	if _, err := env.peer.DeleteTool(env.ctx, "tool.delete", rpcapi.ToolDeleteRequest{Id: id}); err != nil {
		t.Fatalf("server.tool.delete: %v", err)
	}
}
