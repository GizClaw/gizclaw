package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestApplyResourceListRecurses(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["existing"] = apitypes.Credential{
		Id:        "existing",
		Body:      testOpenAICredentialBody("old"),
		CreatedAt: time.Now().UTC(),
		Name:      "existing",
		Provider:  "minimax",
		UpdatedAt: time.Now().UTC(),
	}
	manager := New(Services{Credentials: credentials})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ResourceList",
		"metadata": {"name": "bundle"},
		"spec": {
			"items": [
				{
					"apiVersion": "gizclaw.admin/v1alpha1",
					"kind": "Credential",
					"metadata": {"id": "existing", "name": "existing"},
					"spec": {
						"provider": "minimax",
						"body": {"api_key": "old"}
					}
				},
				{
					"apiVersion": "gizclaw.admin/v1alpha1",
					"kind": "Credential",
					"metadata": {"name": "created"},
					"spec": {
						"provider": "minimax",
						"body": {"api_key": "new"}
					}
				}
			]
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionApplied {
		t.Fatalf("action = %q, want %q", result.Action, apitypes.ApplyActionApplied)
	}
	if result.Items == nil || len(*result.Items) != 2 {
		t.Fatalf("items = %#v, want two child results", result.Items)
	}
	if (*result.Items)[0].Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("first child action = %q, want unchanged", (*result.Items)[0].Action)
	}
	if (*result.Items)[1].Action != apitypes.ApplyActionCreated {
		t.Fatalf("second child action = %q, want created", (*result.Items)[1].Action)
	}
}

func TestPutResourceListRecurses(t *testing.T) {
	credentials := newFakeCredentials()
	manager := New(Services{Credentials: credentials})

	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ResourceList",
		"metadata": {"id": "bundle", "name": "bundle"},
		"spec": {
			"items": [
				{
					"apiVersion": "gizclaw.admin/v1alpha1",
					"kind": "Credential",
					"metadata": {"id": "created", "name": "created"},
					"spec": {
						"provider": "minimax",
						"body": {"api_key": "new"}
					}
				}
			]
		}
	}`))
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_PUT")
}
