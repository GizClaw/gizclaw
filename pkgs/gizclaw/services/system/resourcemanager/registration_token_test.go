package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestApplyRegistrationTokenReturnsOneTimeToken(t *testing.T) {
	ctx := context.Background()
	manager := New(Services{RuntimeProfiles: &runtimeprofile.Server{Store: kv.NewMemory(nil)}})
	if _, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"RuntimeProfile",
		"metadata":{"name":"profile-a"},
		"spec":{"resources":{}}
	}`)); err != nil {
		t.Fatalf("Apply(RuntimeProfile) error = %v", err)
	}

	result, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"ResourceList",
		"metadata":{"name":"bootstrap"},
		"spec":{"items":[{
			"apiVersion":"gizclaw.admin/v1alpha1",
			"kind":"RegistrationToken",
			"metadata":{"name":"device-a"},
			"spec":{"firmware_name":"firmware-a","runtime_profile_name":"profile-a"}
		}]}
	}`))
	if err != nil {
		t.Fatalf("Apply(ResourceList) error = %v", err)
	}
	if result.Items == nil || len(*result.Items) != 1 {
		t.Fatalf("Items = %#v, want one item", result.Items)
	}
	created := (*result.Items)[0]
	if created.Action != apitypes.ApplyActionCreated || created.Resource == nil {
		t.Fatalf("created result = %#v", created)
	}
	resource, err := created.Resource.AsRegistrationTokenResource()
	if err != nil {
		t.Fatalf("AsRegistrationTokenResource() error = %v", err)
	}
	if resource.Token == nil || *resource.Token == "" {
		t.Fatal("created RegistrationToken did not return its one-time token")
	}

	unchanged, err := manager.Apply(ctx, *created.Resource)
	if err != nil {
		t.Fatalf("Apply(existing RegistrationToken) error = %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged || unchanged.Resource != nil {
		t.Fatalf("unchanged result = %#v, want no resource/token", unchanged)
	}
}
