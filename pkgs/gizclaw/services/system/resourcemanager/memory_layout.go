package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyMemoryLayout(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.MemoryLayouts == nil {
		return apitypes.ApplyResult{}, missingService("memory layouts")
	}
	item, err := resource.AsMemoryLayoutResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_MEMORY_LAYOUT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getMemoryLayout(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(existing.Spec, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindMemoryLayout, item.Metadata.Name), nil
		}
	}
	if err := m.putMemoryLayout(ctx, name, memoryLayoutFromResource(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindMemoryLayout, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindMemoryLayout, item.Metadata.Name), nil
}

func (m *Manager) getMemoryLayout(ctx context.Context, name string) (apitypes.MemoryLayout, bool, error) {
	response, err := m.services.MemoryLayouts.GetMemoryLayout(ctx, adminhttp.GetMemoryLayoutRequestObject{Name: name})
	if err != nil {
		return apitypes.MemoryLayout{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetMemoryLayout200JSONResponse:
		return apitypes.MemoryLayout(response), true, nil
	case adminhttp.GetMemoryLayout404JSONResponse:
		return apitypes.MemoryLayout{}, false, nil
	case adminhttp.GetMemoryLayout500JSONResponse:
		return apitypes.MemoryLayout{}, false, responseError(500, "GET_MEMORY_LAYOUT_FAILED", "failed to get memory layout", response)
	default:
		return apitypes.MemoryLayout{}, false, unexpectedResponse("GetMemoryLayout", response)
	}
}

func (m *Manager) putMemoryLayout(ctx context.Context, name string, body apitypes.MemoryLayout) error {
	response, err := m.services.MemoryLayouts.PutMemoryLayout(ctx, adminhttp.PutMemoryLayoutRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutMemoryLayout200JSONResponse:
		return nil
	case adminhttp.PutMemoryLayout400JSONResponse:
		return responseError(400, "PUT_MEMORY_LAYOUT_FAILED", "failed to put memory layout", response)
	case adminhttp.PutMemoryLayout500JSONResponse:
		return responseError(500, "PUT_MEMORY_LAYOUT_FAILED", "failed to put memory layout", response)
	default:
		return unexpectedResponse("PutMemoryLayout", response)
	}
}

func (m *Manager) deleteMemoryLayout(ctx context.Context, name string) (apitypes.MemoryLayout, bool, error) {
	response, err := m.services.MemoryLayouts.DeleteMemoryLayout(ctx, adminhttp.DeleteMemoryLayoutRequestObject{Name: name})
	if err != nil {
		return apitypes.MemoryLayout{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteMemoryLayout200JSONResponse:
		return apitypes.MemoryLayout(response), true, nil
	case adminhttp.DeleteMemoryLayout404JSONResponse:
		return apitypes.MemoryLayout{}, false, nil
	case adminhttp.DeleteMemoryLayout500JSONResponse:
		return apitypes.MemoryLayout{}, false, responseError(500, "DELETE_MEMORY_LAYOUT_FAILED", "failed to delete memory layout", response)
	default:
		return apitypes.MemoryLayout{}, false, unexpectedResponse("DeleteMemoryLayout", response)
	}
}

func resourceFromMemoryLayout(item apitypes.MemoryLayout) (apitypes.Resource, error) {
	return marshalResource(apitypes.MemoryLayoutResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.MemoryLayoutResourceKindMemoryLayout,
		Metadata:   apitypes.ResourceMetadata{Name: item.Name},
		Spec:       item.Spec,
	})
}

func memoryLayoutFromResource(item apitypes.MemoryLayoutResource) apitypes.MemoryLayout {
	return apitypes.MemoryLayout{Name: item.Metadata.Name, Spec: item.Spec}
}
