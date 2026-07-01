//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIModelsListGetPaginationAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 20, func(cursor *string, limit int32) ([]apitypes.Model, bool, *string) {
		resp, err := env.api.ListModelsWithResponse(env.ctx, &adminservice.ListModelsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list models: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list models missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, all, "fake-openai-chat-000", func(item apitypes.Model) string { return item.Id })
	requirePrefixCount(t, all, "fake-openai-chat-", 70, func(item apitypes.Model) string { return item.Id })

	get, err := env.api.GetModelWithResponse(env.ctx, "fake-openai-chat-000")
	if err != nil {
		t.Fatalf("get model: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != "fake-openai-chat-000" || get.JSON200.Provider.Name != "fake-openai" {
		t.Fatalf("get model = %#v", get.JSON200)
	}

	id := mutationName("model")
	_, _ = env.api.DeleteModelWithResponse(env.ctx, id)
	created, err := env.api.CreateModelWithResponse(env.ctx, adminservice.ModelUpsert{
		Id:   id,
		Kind: apitypes.ModelKindLlm,
		Name: ptr("Admin API mutation model"),
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKindOpenaiTenant,
			Name: "fake-openai",
		},
		ProviderData: openAIModelProviderData(t, "e2e-admin-mut-upstream"),
		Source:       apitypes.ModelSourceManual,
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Id != id {
		t.Fatalf("created model = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteModelWithResponse(env.ctx, id)
	if err != nil {
		t.Fatalf("delete model: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
