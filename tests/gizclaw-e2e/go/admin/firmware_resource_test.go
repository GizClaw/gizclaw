//go:build gizclaw_e2e

package admin_test

import (
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIFirmwareResourceLifecycle(t *testing.T) {
	env := newAdminAPIHarness(t)
	name := mutationName("firmware-resource")
	var id string
	t.Cleanup(func() {
		if id != "" {
			_, _ = env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFirmware, id)
		}
	})

	description := "Firmware resource E2E"
	firmware := apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKindFirmware,
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec: apitypes.FirmwareSpec{
			Description: &description,
			Slots: apitypes.FirmwareSpecSlots{
				Stable:  firmwareResourceSlot("stable", "https://downloads.example.com/resource/stable.tar.zlib", firmwarePackageSHA256, 4096),
				Beta:    firmwareResourceSlot("beta", "https://downloads.example.com/resource/beta.tar.zlib", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", 8192),
				Develop: firmwareResourceSlot("", "https://downloads.example.com/resource/develop.tar.zlib", "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0", 12288),
				Pending: firmwareResourceSlot("pending", "https://downloads.example.com/resource/pending.tar.zlib", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", 16384),
			},
		},
	}
	var resource apitypes.Resource
	if err := resource.FromFirmwareResource(firmware); err != nil {
		t.Fatalf("build Firmware resource: %v", err)
	}

	applied, err := env.api.ApplyResourceWithResponse(env.ctx, resource)
	if err != nil {
		t.Fatalf("apply Firmware resource: %v", err)
	}
	requireStatusOK(t, applied, applied.Body)
	if applied.JSON200 == nil || applied.JSON200.Id == nil || applied.JSON200.Kind != apitypes.ResourceKindFirmware {
		t.Fatalf("apply Firmware resource = %#v", applied.JSON200)
	}
	id = *applied.JSON200.Id

	got, err := env.api.GetResourceWithResponse(env.ctx, apitypes.ResourceKindFirmware, id)
	if err != nil {
		t.Fatalf("get Firmware resource: %v", err)
	}
	requireStatusOK(t, got, got.Body)
	if got.JSON200 == nil {
		t.Fatalf("get Firmware resource returned no body: %s", got.Body)
	}
	decoded, err := got.JSON200.AsFirmwareResource()
	if err != nil {
		t.Fatalf("decode Firmware resource: %v", err)
	}
	if !reflect.DeepEqual(decoded.Spec.Slots, firmware.Spec.Slots) {
		t.Fatalf("Firmware resource slots = %#v, want %#v", decoded.Spec.Slots, firmware.Spec.Slots)
	}

	updatedDescription := "updated stable"
	decoded.Spec.Slots.Stable.Description = &updatedDescription
	decoded.Spec.Slots.Stable.Package.Size = 5000
	wantUpdatedSlots := decoded.Spec.Slots
	if err := resource.FromFirmwareResource(decoded); err != nil {
		t.Fatalf("build updated Firmware resource: %v", err)
	}
	updated, err := env.api.PutResourceWithResponse(env.ctx, apitypes.ResourceKindFirmware, id, resource)
	if err != nil {
		t.Fatalf("put Firmware resource: %v", err)
	}
	requireStatusOK(t, updated, updated.Body)
	if updated.JSON200 == nil {
		t.Fatalf("put Firmware resource returned no body: %s", updated.Body)
	}
	updatedFirmware, err := updated.JSON200.AsFirmwareResource()
	if err != nil {
		t.Fatalf("decode updated Firmware resource: %v", err)
	}
	if !reflect.DeepEqual(updatedFirmware.Spec.Slots, wantUpdatedSlots) {
		t.Fatalf("put Firmware resource slots = %#v, want %#v", updatedFirmware.Spec.Slots, wantUpdatedSlots)
	}

	deleted, err := env.api.DeleteResourceWithResponse(env.ctx, apitypes.ResourceKindFirmware, id)
	if err != nil {
		t.Fatalf("delete Firmware resource: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
	id = ""
}

func firmwareResourceSlot(description, url, sha256 string, size int64) apitypes.FirmwareSpecSlot {
	slot := apitypes.FirmwareSpecSlot{
		Package: &apitypes.FirmwarePackage{Url: url, Sha256: sha256, Size: size},
	}
	if description != "" {
		slot.Description = &description
	}
	return slot
}
