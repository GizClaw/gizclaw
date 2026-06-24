package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestApplyResourceListRecurses(t *testing.T) {
	credentials := newFakeCredentials()
	credentials.items["existing"] = apitypes.Credential{
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
					"metadata": {"name": "existing"},
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

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ResourceList",
		"metadata": {"name": "bundle"},
		"spec": {
			"items": [
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
		t.Fatalf("Put returned error: %v", err)
	}
	list, err := resource.AsResourceListResource()
	if err != nil {
		t.Fatalf("AsResourceListResource returned error: %v", err)
	}
	if list.Metadata.Name != "bundle" {
		t.Fatalf("metadata.name = %q, want bundle", list.Metadata.Name)
	}
	if len(list.Spec.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(list.Spec.Items))
	}
	if credentials.putCount != 1 {
		t.Fatalf("putCount = %d, want 1", credentials.putCount)
	}
}
