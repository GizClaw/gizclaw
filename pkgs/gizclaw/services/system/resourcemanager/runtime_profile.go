package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) getRuntimeProfile(ctx context.Context, name string) (apitypes.RuntimeProfile, bool, error) {
	if m.services.RuntimeProfiles == nil {
		return apitypes.RuntimeProfile{}, false, missingService("runtime profiles")
	}
	response, err := m.services.RuntimeProfiles.GetRuntimeProfile(ctx, adminhttp.GetRuntimeProfileRequestObject{Id: name})
	if err != nil {
		return apitypes.RuntimeProfile{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetRuntimeProfile200JSONResponse:
		return apitypes.RuntimeProfile(response), true, nil
	case adminhttp.GetRuntimeProfile404JSONResponse:
		return apitypes.RuntimeProfile{}, false, nil
	case adminhttp.GetRuntimeProfile500JSONResponse:
		return apitypes.RuntimeProfile{}, false, responseError(500, "GET_RUNTIME_PROFILE_FAILED", "failed to get RuntimeProfile", response)
	default:
		return apitypes.RuntimeProfile{}, false, unexpectedResponse("GetRuntimeProfile", response)
	}
}

func (m *Manager) putRuntimeProfile(ctx context.Context, id, name string, spec apitypes.RuntimeProfileSpec) error {
	body := adminhttp.RuntimeProfileUpsert{Name: name, Spec: spec}
	response, err := m.services.RuntimeProfiles.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{Id: id, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutRuntimeProfile200JSONResponse:
		return nil
	case adminhttp.PutRuntimeProfile400JSONResponse:
		return responseError(400, "PUT_RUNTIME_PROFILE_FAILED", "failed to put RuntimeProfile", response)
	case adminhttp.PutRuntimeProfile409JSONResponse:
		return responseError(409, "PUT_RUNTIME_PROFILE_FAILED", "failed to put RuntimeProfile", response)
	case adminhttp.PutRuntimeProfile500JSONResponse:
		return responseError(500, "PUT_RUNTIME_PROFILE_FAILED", "failed to put RuntimeProfile", response)
	default:
		return unexpectedResponse("PutRuntimeProfile", response)
	}
}

func (m *Manager) deleteRuntimeProfile(ctx context.Context, name string) (apitypes.RuntimeProfile, bool, error) {
	if m.services.RuntimeProfiles == nil {
		return apitypes.RuntimeProfile{}, false, missingService("runtime profiles")
	}
	response, err := m.services.RuntimeProfiles.DeleteRuntimeProfile(ctx, adminhttp.DeleteRuntimeProfileRequestObject{Id: name})
	if err != nil {
		return apitypes.RuntimeProfile{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteRuntimeProfile200JSONResponse:
		return apitypes.RuntimeProfile(response), true, nil
	case adminhttp.DeleteRuntimeProfile404JSONResponse:
		return apitypes.RuntimeProfile{}, false, nil
	case adminhttp.DeleteRuntimeProfile500JSONResponse:
		return apitypes.RuntimeProfile{}, false, responseError(500, "DELETE_RUNTIME_PROFILE_FAILED", "failed to delete RuntimeProfile", response)
	default:
		return apitypes.RuntimeProfile{}, false, unexpectedResponse("DeleteRuntimeProfile", response)
	}
}

func (m *Manager) applyRuntimeProfile(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	item, err := resource.AsRuntimeProfileResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RUNTIME_PROFILE_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id, updating, err := resourceUpdateID(item.Metadata)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !updating {
		body := adminhttp.RuntimeProfileUpsert{Name: item.Metadata.Name, Spec: item.Spec}
		response, err := m.services.RuntimeProfiles.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &body})
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		var createdID string
		switch response := response.(type) {
		case adminhttp.CreateRuntimeProfile200JSONResponse:
			createdID = response.Id
		case adminhttp.CreateRuntimeProfile400JSONResponse:
			return apitypes.ApplyResult{}, responseError(400, "CREATE_RUNTIME_PROFILE_FAILED", "failed to create RuntimeProfile", response)
		case adminhttp.CreateRuntimeProfile409JSONResponse:
			return apitypes.ApplyResult{}, responseError(409, "CREATE_RUNTIME_PROFILE_FAILED", "failed to create RuntimeProfile", response)
		case adminhttp.CreateRuntimeProfile500JSONResponse:
			return apitypes.ApplyResult{}, responseError(500, "CREATE_RUNTIME_PROFILE_FAILED", "failed to create RuntimeProfile", response)
		default:
			return apitypes.ApplyResult{}, unexpectedResponse("CreateRuntimeProfile", response)
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindRuntimeProfile, item.Metadata.Name, createdID), nil
	}
	previous, exists, err := m.getRuntimeProfile(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		return apitypes.ApplyResult{}, notFound(apitypes.ResourceKindRuntimeProfile, id)
	}
	if err := validateImmutableResourceName(apitypes.ResourceKindRuntimeProfile, id, previous.Name, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(previous.Spec, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindRuntimeProfile, item.Metadata.Name, id), nil
		}
	}
	if err := m.putRuntimeProfile(ctx, id, item.Metadata.Name, item.Spec); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindRuntimeProfile, item.Metadata.Name, id), nil
}

func resourceFromRuntimeProfile(item apitypes.RuntimeProfile) (apitypes.Resource, error) {
	return marshalResource(apitypes.RuntimeProfileResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.RuntimeProfileResourceKind(apitypes.ResourceKindRuntimeProfile),
		Metadata:   apitypes.ResourceMetadata{Id: &item.Id, Name: item.Name},
		Spec:       item.Spec,
	})
}
