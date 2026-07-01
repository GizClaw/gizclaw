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
	sharedModelObject, err := env.peer.GetModel(env.ctx, "model.get.shared", rpcapi.ModelGetRequest{Id: sharedModel})
	if err != nil {
		t.Fatalf("model.get shared: %v", err)
	}
	if sharedModelObject.Id != sharedModel {
		t.Fatalf("model.get shared id = %q", sharedModelObject.Id)
	}
	_, _ = env.peer.DeleteModel(env.ctx, "model.delete.preclean", rpcapi.ModelDeleteRequest{Id: mutationModel})
	model, err := env.peer.CreateModel(env.ctx, "model.create", rpcModel(mutationModel, "openai-main"))
	if err != nil {
		t.Fatalf("model.create: %v", err)
	}
	if model.Id != mutationModel {
		t.Fatalf("model.create id = %q", model.Id)
	}
	modelName := "peer model updated"
	model, err = env.peer.PutModel(env.ctx, "model.put", rpcapi.ModelPutRequest{
		Id: mutationModel,
		Body: func() rpcapi.Model {
			body := rpcModel(mutationModel, "openai-main")
			body.Name = &modelName
			return body
		}(),
	})
	if err != nil {
		t.Fatalf("model.put: %v", err)
	}
	if model.Name == nil || *model.Name != modelName {
		t.Fatalf("model.put name = %#v", model.Name)
	}
	model, err = env.peer.GetModel(env.ctx, "model.get.updated", rpcapi.ModelGetRequest{Id: mutationModel})
	if err != nil {
		t.Fatalf("model.get updated: %v", err)
	}
	if model.Name == nil || *model.Name != modelName {
		t.Fatalf("model.get updated name = %#v", model.Name)
	}
	assertModelPagination(t, env.ctx, env.peer, sharedModel, mutationModel)
	if _, err := env.peer.DeleteModel(env.ctx, "model.delete", rpcapi.ModelDeleteRequest{Id: mutationModel}); err != nil {
		t.Fatalf("model.delete: %v", err)
	}
}
