package resourcemanager

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestFirmwareResourceApplyShowDelete(t *testing.T) {
	ctx := context.Background()
	manager := New(Services{Firmwares: &firmware.Server{Store: kv.NewMemory(nil)}})
	resource, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Name: "devkit"},
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

	shown, err := manager.Get(ctx, apitypes.ResourceKindFirmware, "devkit")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	item, err := shown.AsFirmwareResource()
	if err != nil {
		t.Fatalf("AsFirmwareResource: %v", err)
	}
	if item.Metadata.Name != "devkit" || item.Spec.Slots.Stable.Description == nil || *item.Spec.Slots.Stable.Description != "stable firmware" {
		t.Fatalf("shown resource = %+v", item)
	}

	unchanged, err := manager.Apply(ctx, resource)
	if err != nil {
		t.Fatalf("Apply unchanged error = %v", err)
	}
	if unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply unchanged result = %+v", unchanged)
	}

	deleted, err := manager.Delete(ctx, apitypes.ResourceKindFirmware, "devkit")
	if err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	deletedItem, err := deleted.AsFirmwareResource()
	if err != nil {
		t.Fatalf("deleted AsFirmwareResource: %v", err)
	}
	if deletedItem.Metadata.Name != "devkit" {
		t.Fatalf("deleted resource = %+v", deletedItem)
	}
}

func TestFirmwareResourcePutAndErrors(t *testing.T) {
	ctx := context.Background()
	resource, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Name: "devkit"},
		Spec: apitypes.FirmwareSpec{
			Slots: testFirmwareSpecSlots("stable firmware"),
		},
	})
	if err != nil {
		t.Fatalf("marshalResource: %v", err)
	}

	missing := New(Services{})
	if _, err := missing.Apply(ctx, resource); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
		t.Fatalf("Apply missing service error = %v", err)
	}
	if _, err := missing.Get(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
		t.Fatalf("Get missing service error = %v", err)
	}
	if _, err := missing.Put(ctx, resource); !isResourceError(err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED") {
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
	if _, err := misconfigured.Put(ctx, resource); !isResourceError(err, 500, "INTERNAL_ERROR") {
		t.Fatalf("Put misconfigured service error = %v", err)
	}
	if _, err := misconfigured.Delete(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "INTERNAL_ERROR") {
		t.Fatalf("Delete misconfigured service error = %v", err)
	}

	manager := New(Services{Firmwares: &firmware.Server{Store: kv.NewMemory(nil)}})
	if _, err := manager.Get(ctx, apitypes.ResourceKindFirmware, "missing"); !isResourceError(err, 404, "RESOURCE_NOT_FOUND") {
		t.Fatalf("Get missing firmware error = %v", err)
	}
	if _, err := manager.Delete(ctx, apitypes.ResourceKindFirmware, "missing"); !isResourceError(err, 404, "RESOURCE_NOT_FOUND") {
		t.Fatalf("Delete missing firmware error = %v", err)
	}
	put, err := manager.Put(ctx, resource)
	if err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if item, err := put.AsFirmwareResource(); err != nil || item.Metadata.Name != "devkit" {
		t.Fatalf("Put resource = %+v, err=%v", item, err)
	}

	unexpected := New(Services{Firmwares: unexpectedFirmwareService{}})
	if _, err := unexpected.Apply(ctx, resource); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Apply unexpected service error = %v", err)
	}
	if _, err := unexpected.Get(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Get unexpected service error = %v", err)
	}
	if _, err := unexpected.Put(ctx, resource); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Put unexpected service error = %v", err)
	}
	if _, err := unexpected.Delete(ctx, apitypes.ResourceKindFirmware, "devkit"); !isResourceError(err, 500, "UNEXPECTED_SERVICE_RESPONSE") {
		t.Fatalf("Delete unexpected service error = %v", err)
	}

	badRequest := New(Services{Firmwares: firmwareServiceWithPut400{}})
	if err := badRequest.putFirmware(ctx, "devkit", adminhttp.FirmwareUpsert{Name: "other"}); !isResourceError(err, 400, "INVALID_FIRMWARE") {
		t.Fatalf("putFirmware bad request error = %v", err)
	}

	transportError := New(Services{Firmwares: firmwareServiceWithTransportError{err: errors.New("transport failed")}})
	if _, _, err := transportError.getFirmware(ctx, "devkit"); err == nil || err.Error() != "transport failed" {
		t.Fatalf("getFirmware transport error = %v", err)
	}
	if err := transportError.putFirmware(ctx, "devkit", adminhttp.FirmwareUpsert{Name: "devkit"}); err == nil || err.Error() != "transport failed" {
		t.Fatalf("putFirmware transport error = %v", err)
	}
	if _, _, err := transportError.deleteFirmware(ctx, "devkit"); err == nil || err.Error() != "transport failed" {
		t.Fatalf("deleteFirmware transport error = %v", err)
	}
}

func TestFirmwareResourceApplyUpdatesChangedSpec(t *testing.T) {
	ctx := context.Background()
	manager := New(Services{Firmwares: &firmware.Server{Store: kv.NewMemory(nil)}})
	first, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Name: "devkit"},
		Spec:       apitypes.FirmwareSpec{Slots: testFirmwareSpecSlots("stable firmware")},
	})
	if err != nil {
		t.Fatalf("marshal first resource: %v", err)
	}
	second, err := marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Name: "devkit"},
		Spec:       apitypes.FirmwareSpec{Slots: testFirmwareSpecSlots("updated stable firmware")},
	})
	if err != nil {
		t.Fatalf("marshal second resource: %v", err)
	}
	if _, err := manager.Apply(ctx, first); err != nil {
		t.Fatalf("Apply first error = %v", err)
	}
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

func (unexpectedFirmwareService) ReleaseFirmware(context.Context, adminhttp.ReleaseFirmwareRequestObject) (adminhttp.ReleaseFirmwareResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) RollbackFirmware(context.Context, adminhttp.RollbackFirmwareRequestObject) (adminhttp.RollbackFirmwareResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) DownloadFirmwareArtifact(context.Context, adminhttp.DownloadFirmwareArtifactRequestObject) (adminhttp.DownloadFirmwareArtifactResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) UploadFirmwareArtifact(context.Context, adminhttp.UploadFirmwareArtifactRequestObject) (adminhttp.UploadFirmwareArtifactResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) DeleteFirmwareArtifact(context.Context, adminhttp.DeleteFirmwareArtifactRequestObject) (adminhttp.DeleteFirmwareArtifactResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) ListFirmwareArtifactEntries(context.Context, adminhttp.ListFirmwareArtifactEntriesRequestObject) (adminhttp.ListFirmwareArtifactEntriesResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) TreeFirmwareArtifactEntries(context.Context, adminhttp.TreeFirmwareArtifactEntriesRequestObject) (adminhttp.TreeFirmwareArtifactEntriesResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) StatFirmwareArtifactEntry(context.Context, adminhttp.StatFirmwareArtifactEntryRequestObject) (adminhttp.StatFirmwareArtifactEntryResponseObject, error) {
	return nil, nil
}

func (unexpectedFirmwareService) DownloadFirmwareArtifactEntry(context.Context, adminhttp.DownloadFirmwareArtifactEntryRequestObject) (adminhttp.DownloadFirmwareArtifactEntryResponseObject, error) {
	return nil, nil
}

func testFirmwareSpecSlots(stableDescription string) apitypes.FirmwareSpecSlots {
	return apitypes.FirmwareSpecSlots{
		Stable: apitypes.FirmwareSpecSlot{Description: stringPtr(stableDescription)},
	}
}

func stringPtr(value string) *string {
	return &value
}
