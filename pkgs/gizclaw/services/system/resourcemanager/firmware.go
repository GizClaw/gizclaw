package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyFirmware(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Firmwares == nil {
		return apitypes.ApplyResult{}, missingService("firmwares")
	}
	item, err := resource.AsFirmwareResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_FIRMWARE_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getFirmware(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindFirmware, item.Metadata.Name, item.Metadata, exists); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(firmwareSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			ownerChanged, err := m.ensureOwnedResourceOwnerFromMetadata(ctx, apitypes.ACLResourceKindFirmware, item.Metadata.Name, item.Metadata)
			if err != nil {
				return apitypes.ApplyResult{}, err
			}
			if ownerChanged {
				return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFirmware, item.Metadata.Name), nil
			}
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindFirmware, item.Metadata.Name), nil
		}
	}
	ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindFirmware, item.Metadata.Name, item.Metadata)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if err := m.putFirmware(ctx, name, firmwareUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindFirmware, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindFirmware, item.Metadata.Name), nil
}

func (m *Manager) getFirmware(ctx context.Context, name string) (apitypes.Firmware, bool, error) {
	response, err := m.services.Firmwares.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Name: name})
	if err != nil {
		return apitypes.Firmware{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetFirmware200JSONResponse:
		return apitypes.Firmware(response), true, nil
	case adminhttp.GetFirmware404JSONResponse:
		return apitypes.Firmware{}, false, nil
	case adminhttp.GetFirmware500JSONResponse:
		return apitypes.Firmware{}, false, responseError(500, "GET_FIRMWARE_FAILED", "failed to get firmware", response)
	default:
		return apitypes.Firmware{}, false, unexpectedResponse("GetFirmware", response)
	}
}

func (m *Manager) putFirmware(ctx context.Context, name string, body adminhttp.FirmwareUpsert) error {
	response, err := m.services.Firmwares.PutFirmware(ctx, adminhttp.PutFirmwareRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutFirmware200JSONResponse:
		return nil
	case adminhttp.PutFirmware400JSONResponse:
		return responseError(400, "PUT_FIRMWARE_FAILED", "failed to put firmware", response)
	case adminhttp.PutFirmware500JSONResponse:
		return responseError(500, "PUT_FIRMWARE_FAILED", "failed to put firmware", response)
	default:
		return unexpectedResponse("PutFirmware", response)
	}
}

func (m *Manager) deleteFirmware(ctx context.Context, name string) (apitypes.Firmware, bool, error) {
	response, err := m.services.Firmwares.DeleteFirmware(ctx, adminhttp.DeleteFirmwareRequestObject{Name: name})
	if err != nil {
		return apitypes.Firmware{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteFirmware200JSONResponse:
		return apitypes.Firmware(response), true, nil
	case adminhttp.DeleteFirmware404JSONResponse:
		return apitypes.Firmware{}, false, nil
	case adminhttp.DeleteFirmware500JSONResponse:
		return apitypes.Firmware{}, false, responseError(500, "DELETE_FIRMWARE_FAILED", "failed to delete firmware", response)
	default:
		return apitypes.Firmware{}, false, unexpectedResponse("DeleteFirmware", response)
	}
}

func firmwareSpec(item apitypes.Firmware) apitypes.FirmwareSpec {
	return apitypes.FirmwareSpec{
		Description: item.Description,
		Slots:       firmwareSpecSlots(item.Slots),
	}
}

func firmwareUpsert(resource apitypes.FirmwareResource) adminhttp.FirmwareUpsert {
	return adminhttp.FirmwareUpsert{
		Description: resource.Spec.Description,
		Name:        string(resource.Metadata.Name),
		Slots:       firmwareRuntimeSlots(resource.Spec.Slots),
	}
}

func firmwareSpecSlots(slots apitypes.FirmwareSlots) apitypes.FirmwareSpecSlots {
	return apitypes.FirmwareSpecSlots{
		Stable:  firmwareSpecSlot(slots.Stable),
		Beta:    firmwareSpecSlot(slots.Beta),
		Develop: firmwareSpecSlot(slots.Develop),
		Pending: firmwareSpecSlot(slots.Pending),
	}
}

func firmwareSpecSlot(slot apitypes.FirmwareSlot) apitypes.FirmwareSpecSlot {
	return apitypes.FirmwareSpecSlot{
		Description: slot.Description,
	}
}

func firmwareRuntimeSlots(slots apitypes.FirmwareSpecSlots) apitypes.FirmwareSlots {
	return apitypes.FirmwareSlots{
		Stable:  firmwareRuntimeSlot(slots.Stable),
		Beta:    firmwareRuntimeSlot(slots.Beta),
		Develop: firmwareRuntimeSlot(slots.Develop),
		Pending: firmwareRuntimeSlot(slots.Pending),
	}
}

func firmwareRuntimeSlot(slot apitypes.FirmwareSpecSlot) apitypes.FirmwareSlot {
	return apitypes.FirmwareSlot{
		Description: slot.Description,
	}
}

func resourceFromFirmware(item apitypes.Firmware) (apitypes.Resource, error) {
	return marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Name: item.Name},
		Spec:       firmwareSpec(item),
	})
}
