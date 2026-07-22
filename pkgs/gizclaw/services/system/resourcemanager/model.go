package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyModel(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Models == nil {
		return apitypes.ApplyResult{}, missingService("models")
	}
	item, err := resource.AsModelResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_MODEL_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getModel(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(modelSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindModel, item.Metadata.Name), nil
		}
	}
	if err := m.putModel(ctx, id, modelUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindModel, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindModel, item.Metadata.Name), nil
}

func (m *Manager) getModel(ctx context.Context, id string) (apitypes.Model, bool, error) {
	response, err := m.services.Models.GetModel(ctx, adminhttp.GetModelRequestObject{Id: id})
	if err != nil {
		return apitypes.Model{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetModel200JSONResponse:
		return apitypes.Model(response), true, nil
	case adminhttp.GetModel404JSONResponse:
		return apitypes.Model{}, false, nil
	case adminhttp.GetModel500JSONResponse:
		return apitypes.Model{}, false, responseError(500, "GET_MODEL_FAILED", "failed to get model", response)
	default:
		return apitypes.Model{}, false, unexpectedResponse("GetModel", response)
	}
}

func (m *Manager) putModel(ctx context.Context, id string, body adminhttp.ModelUpsert) error {
	response, err := m.services.Models.PutModel(ctx, adminhttp.PutModelRequestObject{Id: id, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutModel200JSONResponse:
		return nil
	case adminhttp.PutModel400JSONResponse:
		return responseError(400, "PUT_MODEL_FAILED", "failed to put model", response)
	case adminhttp.PutModel409JSONResponse:
		return responseError(409, "PUT_MODEL_FAILED", "failed to put model", response)
	case adminhttp.PutModel500JSONResponse:
		return responseError(500, "PUT_MODEL_FAILED", "failed to put model", response)
	default:
		return unexpectedResponse("PutModel", response)
	}
}

func (m *Manager) deleteModel(ctx context.Context, id string) (apitypes.Model, bool, error) {
	response, err := m.services.Models.DeleteModel(ctx, adminhttp.DeleteModelRequestObject{Id: id})
	if err != nil {
		return apitypes.Model{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteModel200JSONResponse:
		return apitypes.Model(response), true, nil
	case adminhttp.DeleteModel404JSONResponse:
		return apitypes.Model{}, false, nil
	case adminhttp.DeleteModel500JSONResponse:
		return apitypes.Model{}, false, responseError(500, "DELETE_MODEL_FAILED", "failed to delete model", response)
	default:
		return apitypes.Model{}, false, unexpectedResponse("DeleteModel", response)
	}
}

func modelSpec(model apitypes.Model) apitypes.ModelSpec {
	return apitypes.ModelSpec{
		Description:  model.Description,
		Kind:         model.Kind,
		Name:         model.Name,
		Provider:     model.Provider,
		ProviderData: model.ProviderData,
		Source:       model.Source,
	}
}

func modelUpsert(resource apitypes.ModelResource) adminhttp.ModelUpsert {
	return adminhttp.ModelUpsert{
		Description:  resource.Spec.Description,
		Id:           string(resource.Metadata.Name),
		Kind:         resource.Spec.Kind,
		Name:         resource.Spec.Name,
		Provider:     resource.Spec.Provider,
		ProviderData: resource.Spec.ProviderData,
		Source:       resource.Spec.Source,
	}
}

func resourceFromModel(item apitypes.Model) (apitypes.Resource, error) {
	return marshalResource(apitypes.ModelResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ModelResourceKind(apitypes.ResourceKindModel),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Id)},
		Spec:       modelSpec(item),
	})
}
