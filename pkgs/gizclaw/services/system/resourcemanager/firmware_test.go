package resourcemanager

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/firmwaretest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
)

func TestFirmwareResourceApplyShowDelete(t *testing.T) {
	ctx := context.Background()
	manager := New(Services{Firmwares: firmwaretest.New(t)})
	resource, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Id: "devkit"},
		Spec: apitypes.FirmwareSpec{
			Slots: testFirmwareSpecSlots("stable firmware"),
		},
	})
	if err != nil {
		t.Fatalf("marshalResource: %v", err)
	}

	result, err := manager.Apply(ctx, resource)
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated || result.Kind != apitypes.ResourceKindFirmware {
		t.Fatalf("Apply result = %+v", result)
	}
	resource = withResourceID(t, resource, *result.Id)
	id := *result.Id

	shown, err := manager.Get(ctx, apitypes.ResourceKindFirmware, id)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	item, err := shown.AsFirmwareResource()
	if err != nil {
		t.Fatalf("AsFirmwareResource: %v", err)
	}
	if item.Metadata.Id != "devkit" || item.Spec.Slots.Stable.Description == nil || *item.Spec.Slots.Stable.Description != "stable firmware" || item.Spec.Slots.Stable.Package == nil || item.Spec.Slots.Stable.Package.Url != "https://firmware.example/stable.tar.zlib" || item.Spec.Slots.Stable.Package.Size != 4096 || item.Spec.Slots.Stable.Package.Version == nil || *item.Spec.Slots.Stable.Package.Version != "1.2.3" {
		t.Fatalf("shown resource = %+v", item)
	}

	unchanged, err := manager.Apply(ctx, resource)
	if err != nil {
		t.Fatalf("Apply unchanged error = %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply unchanged result = %+v", unchanged)
	}

	deleted, err := manager.Delete(ctx, apitypes.ResourceKindFirmware, id)
	if err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	deletedItem, err := deleted.AsFirmwareResource()
	if err != nil {
		t.Fatalf("deleted AsFirmwareResource: %v", err)
	}
	if deletedItem.Metadata.Id != "devkit" {
		t.Fatalf("deleted resource = %+v", deletedItem)
	}
}

func TestFirmwareResourcePutAndErrors(t *testing.T) {
	ctx := context.Background()
	resource, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Id: "devkit"},
		Spec: apitypes.FirmwareSpec{
			Slots: testFirmwareSpecSlots("stable firmware"),
		},
	})
	if err != nil {
		t.Fatalf("marshalResource: %v", err)
	}

	missing := New(Services{})
	updateResource := withResourceID(t, resource, "firmware-id")
	if _, err := missing.Apply(ctx, resource); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
		t.Fatalf("Apply missing service error = %v", err)
	}
	if _, err := missing.Get(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
		t.Fatalf("Get missing service error = %v", err)
	}
	if _, err := missing.Put(ctx, updateResource); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
		t.Fatalf("Put missing service error = %v", err)
	}
	if _, err := missing.Delete(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
		t.Fatalf("Delete missing service error = %v", err)
	}

	misconfigured := New(Services{Firmwares: &firmware.Server{}})
	if _, err := misconfigured.Apply(ctx, resource); !isResourceError(err, 500, "INTERNAL_ERROR") {
		t.Fatalf("Apply misconfigured service error = %v", err)
	}
	if _, err := misconfigured.Get(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "INTERNAL_ERROR") {
		t.Fatalf("Get misconfigured service error = %v", err)
	}
	if _, err := misconfigured.Put(ctx, updateResource); !isResourceError(err, 500, "INTERNAL_ERROR") {
		t.Fatalf("Put misconfigured service error = %v", err)
	}
	if _, err := misconfigured.Delete(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "INTERNAL_ERROR") {
		t.Fatalf("Delete misconfigured service error = %v", err)
	}

	manager := New(Services{Firmwares: firmwaretest.New(t)})
	if _, err := manager.Get(ctx, apitypes.ResourceKindFirmware, "missing"); !isResourceError(err, 404, "RESOURCE_NOT_FOUND") {
		t.Fatalf("Get missing firmware error = %v", err)
	}
	if _, err := manager.Delete(ctx, apitypes.ResourceKindFirmware, "missing"); !isResourceError(err, 404, "RESOURCE_NOT_FOUND") {
		t.Fatalf("Delete missing firmware error = %v", err)
	}
	created, err := manager.Apply(ctx, resource)
	if err != nil {
		t.Fatalf("Apply before Put error = %v", err)
	}
	put, err := manager.Put(ctx, withResourceID(t, resource, *created.Id))
	if err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if item, err := put.AsFirmwareResource(); err != nil || item.Metadata.Id != "devkit" {
		t.Fatalf("Put resource = %+v, err=%v", item, err)
	}

	unexpected := New(Services{Firmwares: unexpectedFirmwareService{}})
	if _, err := unexpected.Apply(ctx, resource); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Apply unexpected service error = %v", err)
	}
	if _, err := unexpected.Get(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Get unexpected service error = %v", err)
	}
	if _, err := unexpected.Put(ctx, updateResource); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Put unexpected service error = %v", err)
	}
	if _, err := unexpected.Delete(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Delete unexpected service error = %v", err)
	}

	badRequest := New(Services{Firmwares: firmwareServiceWithPut400{}})
	if err := badRequest.putFirmware(ctx, "devkit", adminhttp.FirmwareUpsert{Id: "other"}); !isResourceError(err, 400, "INVALID_FIRMWARE") {
		t.Fatalf("putFirmware bad request error = %v", err)
	}

	transportError := New(Services{Firmwares: firmwareServiceWithTransportError{err: errors.New("transport failed")}})
	if _, _, err := transportError.getFirmware(ctx, "devkit"); err == nil || err.Error() != "transport failed" {
		t.Fatalf("getFirmware transport error = %v", err)
	}
	if err := transportError.putFirmware(ctx, "devkit", adminhttp.FirmwareUpsert{Id: "devkit"}); err == nil || err.Error() != "transport failed" {
		t.Fatalf("putFirmware transport error = %v", err)
	}
	if _, _, err := transportError.deleteFirmware(ctx, "devkit"); err == nil || err.Error() != "transport failed" {
		t.Fatalf("deleteFirmware transport error = %v", err)
	}
}

func TestFirmwareResourceApplyUpdatesChangedSpec(t *testing.T) {
	ctx := context.Background()
	manager := New(Services{Firmwares: firmwaretest.New(t)})
	first, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Id: "devkit"},
		Spec:       apitypes.FirmwareSpec{Slots: testFirmwareSpecSlots("stable firmware")},
	})
	if err != nil {
		t.Fatalf("marshal first resource: %v", err)
	}
	second, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Id: "devkit"},
		Spec:       apitypes.FirmwareSpec{Slots: testFirmwareSpecSlots("updated stable firmware")},
	})
	if err != nil {
		t.Fatalf("marshal second resource: %v", err)
	}
	created, err := manager.Apply(ctx, first)
	if err != nil {
		t.Fatalf("Apply first error = %v", err)
	}
	second = withResourceID(t, second, *created.Id)
	result, err := manager.Apply(ctx, second)
	if err != nil {
		t.Fatalf("Apply update error = %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply update result = %+v", result)
	}
}

func isResourceError(err error, status int, code string) bool {
	if err == nil {
		return false
	}
	resourceErr, ok := err.(*Error)
	return ok && resourceErr.StatusCode == status && resourceErr.Code == code
}

type unexpectedFirmwareService struct{}

type firmwareServiceWithPut400 struct {
	unexpectedFirmwareService
}

type firmwareServiceWithTransportError struct {
	unexpectedFirmwareService
	err error
}

func (firmwareServiceWithPut400) PutFirmware(context.Context, adminhttp.PutFirmwareRequestObject) (adminhttp.PutFirmwareResponseObject, error) {
	return adminhttp.PutFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", "invalid firmware")), nil
}

func (s firmwareServiceWithTransportError) DeleteFirmware(context.Context, adminhttp.DeleteFirmwareRequestObject) (adminhttp.DeleteFirmwareResponseObject, error) {
	return nil, s.err
}

func (s firmwareServiceWithTransportError) GetFirmware(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
	return nil, s.err
}

func (s firmwareServiceWithTransportError) PutFirmware(context.Context, adminhttp.PutFirmwareRequestObject) (adminhttp.PutFirmwareResponseObject, error) {
	return nil, s.err
}

func (unexpectedFirmwareService) ListFirmwares(context.Context, adminhttp.ListFirmwaresRequestObject) (adminhttp.ListFirmwaresResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) CreateFirmware(context.Context, adminhttp.CreateFirmwareRequestObject) (adminhttp.CreateFirmwareResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) DeleteFirmware(context.Context, adminhttp.DeleteFirmwareRequestObject) (adminhttp.DeleteFirmwareResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) GetFirmware(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) PutFirmware(context.Context, adminhttp.PutFirmwareRequestObject) (adminhttp.PutFirmwareResponseObject, error) {
	return nil, nil
}

func testFirmwareSpecSlots(stableDescription string) apitypes.FirmwareSpecSlots {
	return apitypes.FirmwareSpecSlots{
		Stable: apitypes.FirmwareSpecSlot{
			Description: new(stableDescription),
			Package: &apitypes.FirmwarePackage{Version: new("1.2.3"),
				Url:    "https://firmware.example/stable.tar.zlib",
				Sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				Size:   4096,
			},
		},
		Beta: apitypes.FirmwareSpecSlot{Description: new("beta firmware")},
		Develop: apitypes.FirmwareSpecSlot{
			Description: new("develop firmware"),
		},
	}
}

//go:fix inline
func stringPtr(value string) *string {
	return new(value)
}

func TestFirmwareResourceRejectsMissingVersion(t *testing.T) {
	manager := New(Services{Firmwares: firmwaretest.New(t)})
	var resource apitypes.Resource
	if err := json.Unmarshal([]byte(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Firmware","metadata":{"id":"invalid-version"},"spec":{"slots":{"stable":{"package":{"url":"https://firmware.example/fw.tar.zlib","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":42}},"beta":{},"develop":{}}}}`), &resource); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Apply(t.Context(), resource); err == nil {
		t.Fatal("apply accepted package without version")
	}
	if _, err := manager.Get(t.Context(), apitypes.ResourceKindFirmware, "invalid-version"); !isResourceError(err, 404, "RESOURCE_NOT_FOUND") {
		t.Fatalf("invalid resource was stored: %v", err)
	}
}

func TestUnversionedFirmwareResourceShowAndApply(t *testing.T) {
	server := firmwaretest.New(t)
	manager := New(Services{Firmwares: server})
	input := adminhttp.FirmwareUpsert{Id: "legacy", Slots: firmwareRuntimeSlots(testFirmwareSpecSlots("release"))}
	created, err := server.CreateFirmware(t.Context(), adminhttp.CreateFirmwareRequestObject{Body: &input})
	if _, ok := created.(adminhttp.CreateFirmware200JSONResponse); err != nil || !ok {
		t.Fatalf("create = %T, %v", created, err)
	}
	input.Slots.Stable.Package.Version = nil
	legacy, err := json.Marshal(input.Slots)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.DB.ExecContext(t.Context(), `UPDATE firmwares SET slots_json=? WHERE id=?`, string(legacy), input.Id); err != nil {
		t.Fatal(err)
	}
	shown, err := manager.Get(t.Context(), apitypes.ResourceKindFirmware, input.Id)
	if err != nil {
		t.Fatal(err)
	}
	item, err := shown.AsFirmwareResource()
	if err != nil || item.Spec.Slots.Stable.Package == nil || item.Spec.Slots.Stable.Package.Version != nil {
		t.Fatalf("shown = %#v, %v", item, err)
	}
	if _, err := manager.Apply(t.Context(), shown); !isResourceError(err, 400, "INVALID_FIRMWARE_RESOURCE") {
		t.Fatalf("unversioned unchanged apply = %v", err)
	}
	item.Spec.Slots.Stable.Package.Version = new("1.2.3")
	versioned, err := marshalResource(item)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Apply(t.Context(), versioned)
	if err != nil || result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("versioned apply = %#v, %v", result, err)
	}
}
