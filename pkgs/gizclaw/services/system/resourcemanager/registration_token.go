package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyRegistrationToken(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	item, err := resource.AsRegistrationTokenResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_REGISTRATION_TOKEN_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	previous, exists, err := m.getRegistrationToken(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		if !registrationTokenMappingMatches(previous, item.Spec.RuntimeProfileName, item.Spec.FirmwareId) {
			return apitypes.ApplyResult{}, applyError(409, "REGISTRATION_TOKEN_IMMUTABLE", "RegistrationToken mappings are immutable")
		}
		return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindRegistrationToken, item.Metadata.Name), nil
	}
	created, err := m.putRegistrationToken(ctx, item)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	result := applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindRegistrationToken, item.Metadata.Name)
	result.Resource = &created
	return result, nil
}

func (m *Manager) getRegistrationToken(ctx context.Context, name string) (apitypes.RegistrationToken, bool, error) {
	if m.services.RuntimeProfiles == nil {
		return apitypes.RegistrationToken{}, false, missingService("registration tokens")
	}
	response, err := m.services.RuntimeProfiles.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Name: name})
	if err != nil {
		return apitypes.RegistrationToken{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetRegistrationToken200JSONResponse:
		return apitypes.RegistrationToken(response), true, nil
	case adminhttp.GetRegistrationToken404JSONResponse:
		return apitypes.RegistrationToken{}, false, nil
	case adminhttp.GetRegistrationToken500JSONResponse:
		return apitypes.RegistrationToken{}, false, responseError(500, "GET_REGISTRATION_TOKEN_FAILED", "failed to get RegistrationToken", response)
	default:
		return apitypes.RegistrationToken{}, false, unexpectedResponse("GetRegistrationToken", response)
	}
}

func (m *Manager) putRegistrationToken(ctx context.Context, item apitypes.RegistrationTokenResource) (apitypes.Resource, error) {
	previous, exists, err := m.getRegistrationToken(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.Resource{}, err
	}
	if exists {
		if !registrationTokenMappingMatches(previous, item.Spec.RuntimeProfileName, item.Spec.FirmwareId) {
			return apitypes.Resource{}, applyError(409, "REGISTRATION_TOKEN_IMMUTABLE", "RegistrationToken mappings are immutable")
		}
		return resourceFromRegistrationToken(previous, nil)
	}
	body := adminhttp.RegistrationTokenUpsert{
		Name:               item.Metadata.Name,
		RuntimeProfileName: item.Spec.RuntimeProfileName,
		FirmwareId:         item.Spec.FirmwareId,
	}
	response, err := m.services.RuntimeProfiles.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &body})
	if err != nil {
		return apitypes.Resource{}, err
	}
	switch response := response.(type) {
	case adminhttp.CreateRegistrationToken200JSONResponse:
		stored := apitypes.RegistrationToken{
			Name:               response.Name,
			RuntimeProfileName: response.RuntimeProfileName,
			FirmwareId:         response.FirmwareId,
			CreatedAt:          response.CreatedAt,
		}
		token := response.Token
		return resourceFromRegistrationToken(stored, &token)
	case adminhttp.CreateRegistrationToken400JSONResponse:
		return apitypes.Resource{}, responseError(400, "CREATE_REGISTRATION_TOKEN_FAILED", "failed to create RegistrationToken", response)
	case adminhttp.CreateRegistrationToken409JSONResponse:
		return apitypes.Resource{}, responseError(409, "CREATE_REGISTRATION_TOKEN_FAILED", "failed to create RegistrationToken", response)
	case adminhttp.CreateRegistrationToken500JSONResponse:
		return apitypes.Resource{}, responseError(500, "CREATE_REGISTRATION_TOKEN_FAILED", "failed to create RegistrationToken", response)
	default:
		return apitypes.Resource{}, unexpectedResponse("CreateRegistrationToken", response)
	}
}

func (m *Manager) deleteRegistrationToken(ctx context.Context, name string) (apitypes.RegistrationToken, bool, error) {
	if m.services.RuntimeProfiles == nil {
		return apitypes.RegistrationToken{}, false, missingService("registration tokens")
	}
	response, err := m.services.RuntimeProfiles.DeleteRegistrationToken(ctx, adminhttp.DeleteRegistrationTokenRequestObject{Name: name})
	if err != nil {
		return apitypes.RegistrationToken{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteRegistrationToken200JSONResponse:
		return apitypes.RegistrationToken(response), true, nil
	case adminhttp.DeleteRegistrationToken404JSONResponse:
		return apitypes.RegistrationToken{}, false, nil
	case adminhttp.DeleteRegistrationToken500JSONResponse:
		return apitypes.RegistrationToken{}, false, responseError(500, "DELETE_REGISTRATION_TOKEN_FAILED", "failed to delete RegistrationToken", response)
	default:
		return apitypes.RegistrationToken{}, false, unexpectedResponse("DeleteRegistrationToken", response)
	}
}

func resourceFromRegistrationToken(item apitypes.RegistrationToken, token *string) (apitypes.Resource, error) {
	resource := apitypes.RegistrationTokenResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.RegistrationTokenResourceKind(apitypes.ResourceKindRegistrationToken),
		Metadata:   apitypes.ResourceMetadata{Name: item.Name},
		Token:      token,
	}
	resource.Spec.RuntimeProfileName = item.RuntimeProfileName
	resource.Spec.FirmwareId = item.FirmwareId
	return marshalResource(resource)
}

func registrationTokenMappingMatches(item apitypes.RegistrationToken, runtimeProfileName string, firmwareID *string) bool {
	if item.RuntimeProfileName != runtimeProfileName {
		return false
	}
	if item.FirmwareId == nil || firmwareID == nil {
		return item.FirmwareId == nil && firmwareID == nil
	}
	return *item.FirmwareId == *firmwareID
}
