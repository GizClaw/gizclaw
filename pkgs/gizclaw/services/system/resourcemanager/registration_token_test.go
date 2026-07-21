package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestApplyRegistrationTokenReturnsOneTimeToken(t *testing.T) {
	ctx := context.Background()
	profiles := &runtimeprofile.Server{Store: kv.NewMemory(nil)}
	manager := New(Services{
		Firmwares:       &firmware.Server{Store: kv.NewMemory(nil)},
		RuntimeProfiles: profiles,
	})
	profiles.ResolveResource = manager.Get
	if _, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"RuntimeProfile",
		"metadata":{"name":"profile-a"},
		"spec":{"resources":{}}
	}`)); err != nil {
		t.Fatalf("Apply(RuntimeProfile) error = %v", err)
	}
	firmwareResource, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Name: "h106"},
		Spec:       apitypes.FirmwareSpec{Slots: testFirmwareSpecSlots("stable firmware")},
	})
	if err != nil {
		t.Fatalf("marshalResource(Firmware) error = %v", err)
	}
	if _, err := manager.Apply(ctx, firmwareResource); err != nil {
		t.Fatalf("Apply(Firmware) error = %v", err)
	}

	result, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"ResourceList",
		"metadata":{"name":"bootstrap"},
		"spec":{"items":[{
			"apiVersion":"gizclaw.admin/v1alpha1",
			"kind":"RegistrationToken",
			"metadata":{"name":"device-a"},
			"spec":{"runtime_profile_name":"profile-a","firmware_id":"h106"}
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
	if resource.Spec.FirmwareId == nil || *resource.Spec.FirmwareId != "h106" {
		t.Fatalf("created RegistrationToken firmware_id = %#v, want h106", resource.Spec.FirmwareId)
	}

	shown, err := manager.Get(ctx, apitypes.ResourceKindRegistrationToken, "device-a")
	if err != nil {
		t.Fatalf("Get(RegistrationToken) error = %v", err)
	}
	shownResource, err := shown.AsRegistrationTokenResource()
	if err != nil {
		t.Fatalf("shown AsRegistrationTokenResource() error = %v", err)
	}
	if shownResource.Spec.FirmwareId == nil || *shownResource.Spec.FirmwareId != "h106" {
		t.Fatalf("shown RegistrationToken firmware_id = %#v, want h106", shownResource.Spec.FirmwareId)
	}

	unchanged, err := manager.Apply(ctx, *created.Resource)
	if err != nil {
		t.Fatalf("Apply(existing RegistrationToken) error = %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged || unchanged.Resource != nil {
		t.Fatalf("unchanged result = %#v, want no resource/token", unchanged)
	}

	resource.Spec.FirmwareId = nil
	changed, err := marshalResource(resource)
	if err != nil {
		t.Fatalf("marshalResource(changed RegistrationToken) error = %v", err)
	}
	if _, err := manager.Apply(ctx, changed); !isResourceError(err, 409, "REGISTRATION_TOKEN_IMMUTABLE") {
		t.Fatalf("Apply(changed RegistrationToken) error = %v, want immutable conflict", err)
	}
}
