package resourcemanager

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestApplyRegistrationTokenCreatesReadsAndUpdatesOrdinaryResource(t *testing.T) {
	ctx := context.Background()
	profiles := &runtimeprofile.Server{Store: kv.NewMemory(nil)}
	manager := New(Services{
		Firmwares:       &firmware.Server{Store: kv.NewMemory(nil)},
		RuntimeProfiles: profiles,
	})
	if _, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"RuntimeProfile",
		"metadata":{"name":"profile-a"},
		"spec":{
			"workflows":{
				"system":{"friend_chatroom":"chatroom","group_chatroom":"chatroom","pet":"pet-care"},
				"collections":{}
			},
			"resources":{}
		}
	}`)); err != nil {
		t.Fatalf("Apply(RuntimeProfile) error = %v", err)
	}
	if _, err := manager.Apply(ctx, mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"RuntimeProfile",
		"metadata":{"name":"profile-b"},
		"spec":{
			"workflows":{
				"system":{"friend_chatroom":"chatroom","group_chatroom":"chatroom","pet":"pet-care"},
				"collections":{}
			},
			"resources":{}
		}
	}`)); err != nil {
		t.Fatalf("Apply(second RuntimeProfile) error = %v", err)
	}
	profiles.ResolveResource = manager.Get
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
			"spec":{"token":"device-token","runtime_profile_name":"profile-a","firmware_id":"h106"}
		}]}
	}`))
	if err != nil {
		t.Fatalf("Apply(ResourceList) error = %v", err)
	}
	if result.Items == nil || len(*result.Items) != 1 {
		t.Fatalf("Items = %#v, want one item", result.Items)
	}
	created := (*result.Items)[0]
	if created.Action != apitypes.ApplyActionCreated {
		t.Fatalf("created result = %#v", created)
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
	if shownResource.Spec.Token != "device-token" {
		t.Fatalf("shown RegistrationToken token = %q, want device-token", shownResource.Spec.Token)
	}

	unchanged, err := manager.Apply(ctx, shown)
	if err != nil {
		t.Fatalf("Apply(existing RegistrationToken) error = %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged result = %#v", unchanged)
	}

	shownResource.Spec.Token = "replacement-token"
	shownResource.Spec.RuntimeProfileName = "profile-b"
	shownResource.Spec.FirmwareId = nil
	changed, err := marshalResource(shownResource)
	if err != nil {
		t.Fatalf("marshalResource(changed RegistrationToken) error = %v", err)
	}
	updated, err := manager.Apply(ctx, changed)
	if err != nil {
		t.Fatalf("Apply(changed RegistrationToken) error = %v", err)
	}
	if updated.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("updated result = %#v", updated)
	}
	readBack, err := manager.Get(ctx, apitypes.ResourceKindRegistrationToken, "device-a")
	if err != nil {
		t.Fatalf("Get(updated RegistrationToken) error = %v", err)
	}
	readBackResource, err := readBack.AsRegistrationTokenResource()
	if err != nil {
		t.Fatalf("updated AsRegistrationTokenResource() error = %v", err)
	}
	if readBackResource.Spec.Token != "replacement-token" || readBackResource.Spec.RuntimeProfileName != "profile-b" || readBackResource.Spec.FirmwareId != nil {
		t.Fatalf("updated RegistrationToken = %#v", readBackResource)
	}
	if _, err := profiles.ResolveRegistration(ctx, "device-token"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("ResolveRegistration(old token) error = %v, want not found", err)
	}
	registration, err := profiles.ResolveRegistration(ctx, "replacement-token")
	if err != nil {
		t.Fatalf("ResolveRegistration(replacement token) error = %v", err)
	}
	if registration.RuntimeProfile.Name != "profile-b" || registration.FirmwareID != nil {
		t.Fatalf("updated registration = %#v", registration)
	}
}

func TestPutRegistrationTokenRequiresRuntimeProfileService(t *testing.T) {
	resource := mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"RegistrationToken",
		"metadata":{"name":"device-a"},
		"spec":{"token":"device-token","runtime_profile_name":"profile-a"}
	}`)
	if _, err := New(Services{}).Put(context.Background(), resource); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
		t.Fatalf("Put(RegistrationToken) error = %v, want missing service", err)
	}
}
