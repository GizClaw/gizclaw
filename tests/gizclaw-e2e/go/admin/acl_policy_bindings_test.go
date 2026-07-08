//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIACLPolicyBindingsListGetAndMutation(t *testing.T) {
	env := newAdminAPIHarness(t)

	list, err := env.api.ListACLPolicyBindingsWithResponse(env.ctx, &adminhttp.ListACLPolicyBindingsParams{Limit: ptr[int32](200)})
	if err != nil {
		t.Fatalf("list ACL policy bindings: %v", err)
	}
	requireStatusOK(t, list, list.Body)
	if list.JSON200 == nil {
		t.Fatalf("list ACL policy bindings missing JSON200")
	}
	if len(list.JSON200.Items) == 0 {
		t.Fatalf("list ACL policy bindings returned no items")
	}

	fixture, err := env.api.GetACLPolicyBindingWithResponse(env.ctx, "view-default-client-workflow-collection")
	if err != nil {
		t.Fatalf("get fixture ACL policy binding: %v", err)
	}
	requireStatusOK(t, fixture, fixture.Body)
	if fixture.JSON200 == nil || fixture.JSON200.Policy.Resource.Kind != apitypes.ACLResourceKindWorkflow {
		t.Fatalf("fixture ACL policy binding = %#v", fixture.JSON200)
	}

	bindingID := mutationName("acl-binding")
	_, _ = env.api.DeleteACLPolicyBindingWithResponse(env.ctx, bindingID)
	created, err := env.api.CreateACLPolicyBindingWithResponse(env.ctx, adminhttp.ACLPolicyBindingUpsert{
		Id: ptr(bindingID),
		Policy: apitypes.ACLPolicy{
			Subject:  apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindPk, Id: env.peerKey},
			Resource: apitypes.ACLResource{Kind: apitypes.ACLResourceKindWorkflow, Id: "flowcraft-support"},
			Role:     "default-client",
		},
	})
	if err != nil {
		t.Fatalf("create ACL policy binding: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.Id != bindingID {
		t.Fatalf("created ACL policy binding = %#v", created.JSON200)
	}

	get, err := env.api.GetACLPolicyBindingWithResponse(env.ctx, bindingID)
	if err != nil {
		t.Fatalf("get ACL policy binding: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Policy.Subject.Id != env.peerKey {
		t.Fatalf("get ACL policy binding = %#v", get.JSON200)
	}

	deleted, err := env.api.DeleteACLPolicyBindingWithResponse(env.ctx, bindingID)
	if err != nil {
		t.Fatalf("delete ACL policy binding: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
}
