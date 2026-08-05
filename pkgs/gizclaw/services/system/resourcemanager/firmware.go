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
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	body := firmwareUpsert(item)
	return applyConcreteResource(ctx, item.Metadata, apitypes.ResourceKindFirmware, item.Spec,
		m.getFirmware,
		func(ctx context.Context) (string, error) { return m.createFirmware(ctx, body) },
		func(ctx context.Context, id string) error { return m.putFirmware(ctx, id, body) }, firmwareSpec)
}

func (m *Manager) createFirmware(ctx context.Context, body adminhttp.FirmwareUpsert) (string, error) {
	response, err := m.services.Firmwares.CreateFirmware(ctx, adminhttp.CreateFirmwareRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateFirmware200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateFirmware400JSONResponse:
		return "", responseError(400, "CREATE_FIRMWARE_FAILED", "failed to create firmware", response)
	case adminhttp.CreateFirmware409JSONResponse:
		return "", responseError(409, "CREATE_FIRMWARE_FAILED", "failed to create firmware", response)
	case adminhttp.CreateFirmware500JSONResponse:
		return "", responseError(500, "CREATE_FIRMWARE_FAILED", "failed to create firmware", response)
	default:
		return "", unexpectedResponse("CreateFirmware", response)
	}
}

func (m *Manager) getFirmware(ctx context.Context, name string) (apitypes.Firmware, bool, error) {
	response, err := m.services.Firmwares.GetFirmware(ctx, adminhttp.GetFirmwareRequestObject{Id: name})
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
	response, err := m.services.Firmwares.PutFirmware(ctx, adminhttp.PutFirmwareRequestObject{Id: name, Body: &body})
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
	response, err := m.services.Firmwares.DeleteFirmware(ctx, adminhttp.DeleteFirmwareRequestObject{Id: name})
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
		Name:        item.Name,
		Slots:       firmwareSpecSlots(item.Slots),
	}
}

func firmwareUpsert(resource apitypes.FirmwareResource) adminhttp.FirmwareUpsert {
	return adminhttp.FirmwareUpsert{
		Description: resource.Spec.Description,
		Id:          resource.Metadata.Id,
		Name:        resource.Spec.Name,
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
		Package:     slot.Package,
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
		Package:     slot.Package,
	}
}

func resourceFromFirmware(item apitypes.Firmware) (apitypes.Resource, error) {
	return marshalResource(apitypes.FirmwareResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.FirmwareResourceKind(apitypes.ResourceKindFirmware),
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       firmwareSpec(item),
	})
}
