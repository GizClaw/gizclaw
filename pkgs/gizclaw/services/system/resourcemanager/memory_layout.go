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
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	body := memoryLayoutFromResource(item)
	return applyConcreteResource(ctx, item.Metadata, apitypes.ResourceKindMemoryLayout, item.Spec,
		m.getMemoryLayout,
		func(ctx context.Context) (string, error) { return m.createMemoryLayout(ctx, body) },
		func(ctx context.Context, id string) error { return m.putMemoryLayout(ctx, id, body) },
		func(value apitypes.MemoryLayout) apitypes.MemoryLayoutSpec { return value.Spec })
}

func (m *Manager) createMemoryLayout(ctx context.Context, body adminhttp.MemoryLayoutUpsert) (string, error) {
	response, err := m.services.MemoryLayouts.CreateMemoryLayout(ctx, adminhttp.CreateMemoryLayoutRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateMemoryLayout200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateMemoryLayout400JSONResponse:
		return "", responseError(400, "CREATE_MEMORY_LAYOUT_FAILED", "failed to create memory layout", response)
	case adminhttp.CreateMemoryLayout409JSONResponse:
		return "", responseError(409, "CREATE_MEMORY_LAYOUT_FAILED", "failed to create memory layout", response)
	case adminhttp.CreateMemoryLayout500JSONResponse:
		return "", responseError(500, "CREATE_MEMORY_LAYOUT_FAILED", "failed to create memory layout", response)
	default:
		return "", unexpectedResponse("CreateMemoryLayout", response)
	}
}

func (m *Manager) getMemoryLayout(ctx context.Context, name string) (apitypes.MemoryLayout, bool, error) {
	response, err := m.services.MemoryLayouts.GetMemoryLayout(ctx, adminhttp.GetMemoryLayoutRequestObject{Id: name})
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

func (m *Manager) putMemoryLayout(ctx context.Context, name string, body adminhttp.MemoryLayoutUpsert) error {
	response, err := m.services.MemoryLayouts.PutMemoryLayout(ctx, adminhttp.PutMemoryLayoutRequestObject{Id: name, Body: &body})
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
	response, err := m.services.MemoryLayouts.DeleteMemoryLayout(ctx, adminhttp.DeleteMemoryLayoutRequestObject{Id: name})
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
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec:       item.Spec,
	})
}

func memoryLayoutFromResource(item apitypes.MemoryLayoutResource) adminhttp.MemoryLayoutUpsert {
	return adminhttp.MemoryLayoutUpsert{Id: item.Metadata.Id, Spec: item.Spec}
}
