//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIACLViewsListGetAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	list, err := env.api.ListACLViewsWithResponse(env.ctx, &adminhttp.ListACLViewsParams{Limit: ptr[int32](100)})
	if err != nil {
		t.Fatalf("list ACL views: %v", err)
	}
	requireStatusOK(t, list, list.Body)
	if list.JSON200 == nil {
		t.Fatalf("list ACL views missing JSON200")
	}
	requireName(t, list.JSON200.Items, "default-client", func(item apitypes.ACLView) string { return item.Name })

	get, err := env.api.GetACLViewWithResponse(env.ctx, "default-client")
	if err != nil {
		t.Fatalf("get ACL view: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Name != "default-client" {
		t.Fatalf("get ACL view = %#v", get.JSON200)
	}

	name := mutationName("acl-view")
	_, _ = env.api.DeleteACLViewWithResponse(env.ctx, name)
	created, err := env.api.CreateACLViewWithResponse(env.ctx, adminhttp.ACLViewUpsert{
		Name:        name,
		Description: ptr("Admin API mutation ACL view"),
	})
	if err != nil {
		t.Fatalf("create ACL view: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Name != name {
		t.Fatalf("created ACL view = %#v", created.JSON200)
	}
	deleted, err := env.api.DeleteACLViewWithResponse(env.ctx, name)
	if err != nil {
		t.Fatalf("delete ACL view: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
