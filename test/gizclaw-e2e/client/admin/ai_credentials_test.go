//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestAdminAPICredentialsListGetPaginationAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	all := collectAdminPages(t, 20, func(cursor *string, limit int32) ([]apitypes.Credential, bool, *string) {
		resp, err := env.api.ListCredentialsWithResponse(env.ctx, &adminservice.ListCredentialsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list credentials: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list credentials missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, all, "openai-catalog-credential", func(item apitypes.Credential) string { return item.Name })
	requirePrefixCount(t, all, "openai-catalog-credential-", 40, func(item apitypes.Credential) string { return item.Name })

	get, err := env.api.GetCredentialWithResponse(env.ctx, "openai-catalog-credential")
	if err != nil {
		t.Fatalf("get credential: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Name != "openai-catalog-credential" || get.JSON200.Provider != "openai" {
		t.Fatalf("get credential = %#v", get.JSON200)
	}

	name := mutationName("credential")
	_, _ = env.api.DeleteCredentialWithResponse(env.ctx, name)
	created, err := env.api.CreateCredentialWithResponse(env.ctx, adminservice.CredentialUpsert{
		Name:        name,
		Provider:    "openai",
		Description: ptr("Admin API mutation credential"),
		Body:        openAICredentialBody(t, "sk-e2e-admin-mut"),
	})
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Name != name {
		t.Fatalf("created credential = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteCredentialWithResponse(env.ctx, name)
	if err != nil {
		t.Fatalf("delete credential: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
