//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerModelRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	modelList, err := env.peer.ListModels(env.ctx, "model.list.shared", rpcapi.ModelListRequest{})
	if err != nil {
		t.Fatalf("model.list shared: %v", err)
	}
	if len(modelList.Items) == 0 {
		t.Fatalf("model.list returned no items")
	}
	sharedModelObject, err := env.peer.GetModel(env.ctx, "model.get.shared", rpcapi.ModelGetRequest{Alias: "generate-model"})
	if err != nil {
		t.Fatalf("model.get shared: %v", err)
	}
	if sharedModelObject.Value.Alias != "generate-model" || sharedModelObject.Value.Kind != rpcapi.ModelKindLlm {
		t.Fatalf("model.get shared = %#v", sharedModelObject)
	}
	assertModelPagination(t, env.ctx, env.peer, "generate-model", "reward-claim")
}
