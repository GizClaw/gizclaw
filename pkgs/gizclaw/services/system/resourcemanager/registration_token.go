package resourcemanager

import (
	"context"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyRegistrationToken(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	item, err := resource.AsRegistrationTokenResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_REGISTRATION_TOKEN_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := item.Metadata.Id
	transportID := servicePathID(id)
	previous, exists, err := m.getRegistrationToken(ctx, transportID)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		createdID, err := m.createRegistrationToken(ctx, item)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindRegistrationToken, createdID), nil
	}
	if registrationTokenMatches(previous, item.Spec.Token, item.Spec.RuntimeProfileId, item.Spec.FirmwareId) {
		return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindRegistrationToken, id), nil
	}
	_, err = m.putRegistrationToken(ctx, transportID, item)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindRegistrationToken, id), nil
}

func (m *Manager) getRegistrationToken(ctx context.Context, name string) (apitypes.RegistrationToken, bool, error) {
	if m.services.RuntimeProfiles == nil {
		return apitypes.RegistrationToken{}, false, missingService("registration tokens")
	}
	response, err := m.services.RuntimeProfiles.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Id: name})
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

func (m *Manager) putRegistrationToken(ctx context.Context, id string, item apitypes.RegistrationTokenResource) (apitypes.Resource, error) {
	if m.services.RuntimeProfiles == nil {
		return apitypes.Resource{}, missingService("registration tokens")
	}
	body := adminhttp.RegistrationTokenUpsert{
		Id:               item.Metadata.Id,
		Token:            item.Spec.Token,
		RuntimeProfileId: item.Spec.RuntimeProfileId,
		FirmwareId:       item.Spec.FirmwareId,
	}
	response, err := m.services.RuntimeProfiles.PutRegistrationToken(ctx, adminhttp.PutRegistrationTokenRequestObject{Id: id, Body: &body})
	if err != nil {
		return apitypes.Resource{}, err
	}
	switch response := response.(type) {
	case adminhttp.PutRegistrationToken200JSONResponse:
		return resourceFromRegistrationToken(apitypes.RegistrationToken(response))
	case adminhttp.PutRegistrationToken400JSONResponse:
		return apitypes.Resource{}, responseError(400, "PUT_REGISTRATION_TOKEN_FAILED", "failed to put RegistrationToken", response)
	case adminhttp.PutRegistrationToken409JSONResponse:
		return apitypes.Resource{}, responseError(409, "PUT_REGISTRATION_TOKEN_FAILED", "failed to put RegistrationToken", response)
	case adminhttp.PutRegistrationToken500JSONResponse:
		return apitypes.Resource{}, responseError(500, "PUT_REGISTRATION_TOKEN_FAILED", "failed to put RegistrationToken", response)
	default:
		return apitypes.Resource{}, unexpectedResponse("PutRegistrationToken", response)
	}
}

func (m *Manager) createRegistrationToken(ctx context.Context, item apitypes.RegistrationTokenResource) (string, error) {
	body := adminhttp.RegistrationTokenUpsert{Id: item.Metadata.Id, Token: item.Spec.Token, RuntimeProfileId: item.Spec.RuntimeProfileId, FirmwareId: item.Spec.FirmwareId}
	response, err := m.services.RuntimeProfiles.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateRegistrationToken200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateRegistrationToken400JSONResponse:
		return "", responseError(400, "CREATE_REGISTRATION_TOKEN_FAILED", "failed to create RegistrationToken", response)
	case adminhttp.CreateRegistrationToken409JSONResponse:
		return "", responseError(409, "CREATE_REGISTRATION_TOKEN_FAILED", "failed to create RegistrationToken", response)
	case adminhttp.CreateRegistrationToken500JSONResponse:
		return "", responseError(500, "CREATE_REGISTRATION_TOKEN_FAILED", "failed to create RegistrationToken", response)
	default:
		return "", unexpectedResponse("CreateRegistrationToken", response)
	}
}

func (m *Manager) deleteRegistrationToken(ctx context.Context, name string) (apitypes.RegistrationToken, bool, error) {
	if m.services.RuntimeProfiles == nil {
		return apitypes.RegistrationToken{}, false, missingService("registration tokens")
	}
	response, err := m.services.RuntimeProfiles.DeleteRegistrationToken(ctx, adminhttp.DeleteRegistrationTokenRequestObject{Id: name})
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

func resourceFromRegistrationToken(item apitypes.RegistrationToken) (apitypes.Resource, error) {
	resource := apitypes.RegistrationTokenResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.RegistrationTokenResourceKind(apitypes.ResourceKindRegistrationToken),
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
	}
	resource.Spec.Token = item.Token
	resource.Spec.RuntimeProfileId = item.RuntimeProfileId
	resource.Spec.FirmwareId = item.FirmwareId
	return marshalResource(resource)
}

func registrationTokenMatches(item apitypes.RegistrationToken, token, runtimeProfileName string, firmwareID *string) bool {
	if item.Token != strings.TrimSpace(token) || item.RuntimeProfileId != runtimeProfileName {
		return false
	}
	if item.FirmwareId == nil || firmwareID == nil {
		return item.FirmwareId == nil && firmwareID == nil
	}
	return *item.FirmwareId == *firmwareID
}
