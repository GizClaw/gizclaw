//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIModelsListGetPaginationAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 20, func(cursor *string, limit int32) ([]apitypes.Model, bool, *string) {
		resp, err := env.api.ListModelsWithResponse(env.ctx, &adminhttp.ListModelsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list models: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list models missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	seed := requireName(t, all, "fake-openai-chat-000", func(item apitypes.Model) string { return item.Id })
	requirePrefixCount(t, all, "fake-openai-chat-", 70, func(item apitypes.Model) string { return item.Id })

	get, err := env.api.GetModelWithResponse(env.ctx, seed.Id)
	if err != nil {
		t.Fatalf("get model: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != seed.Id || get.JSON200.Provider.Id == "" {
		t.Fatalf("get model = %#v", get.JSON200)
	}

	name := mutationName("model")
	created, err := env.api.CreateModelWithResponse(env.ctx, adminhttp.ModelUpsert{
		Id:          name,
		Kind:        apitypes.ModelKindLlm,
		DisplayName: ptr("Admin API mutation model"),
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKindOpenaiTenant,
			Id:   seed.Provider.Id,
		},
		ProviderData: openAIModelProviderData(t, "e2e-admin-mut-upstream"),
		Source:       apitypes.ModelSourceManual,
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Id != name {
		t.Fatalf("created model = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteModelWithResponse(env.ctx, created.JSON200.Id)
	if err != nil {
		t.Fatalf("delete model: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
