package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyCredential(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Credentials == nil {
		return apitypes.ApplyResult{}, missingService("credentials")
	}
	item, err := resource.AsCredentialResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_CREDENTIAL_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id, updating, err := resourceUpdateID(item.Metadata)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !updating {
		createdID, err := m.createCredential(ctx, credentialUpsert(item))
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindCredential, item.Metadata.Name, createdID), nil
	}
	existing, exists, err := m.getCredential(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		return apitypes.ApplyResult{}, notFound(apitypes.ResourceKindCredential, id)
	}
	if err := validateImmutableResourceName(apitypes.ResourceKindCredential, id, existing.Name, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(credentialSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindCredential, item.Metadata.Name, id), nil
		}
	}
	if err := m.putCredential(ctx, id, credentialUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindCredential, item.Metadata.Name, id), nil
}

func (m *Manager) createCredential(ctx context.Context, body adminhttp.CredentialUpsert) (string, error) {
	response, err := m.services.Credentials.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateCredential200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateCredential400JSONResponse:
		return "", responseError(400, "CREATE_CREDENTIAL_FAILED", "failed to create credential", response)
	case adminhttp.CreateCredential409JSONResponse:
		return "", responseError(409, "CREATE_CREDENTIAL_FAILED", "failed to create credential", response)
	case adminhttp.CreateCredential500JSONResponse:
		return "", responseError(500, "CREATE_CREDENTIAL_FAILED", "failed to create credential", response)
	default:
		return "", unexpectedResponse("CreateCredential", response)
	}
}

func (m *Manager) getCredential(ctx context.Context, name string) (apitypes.Credential, bool, error) {
	response, err := m.services.Credentials.GetCredential(ctx, adminhttp.GetCredentialRequestObject{Id: name})
	if err != nil {
		return apitypes.Credential{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetCredential200JSONResponse:
		return apitypes.Credential(response), true, nil
	case adminhttp.GetCredential404JSONResponse:
		return apitypes.Credential{}, false, nil
	case adminhttp.GetCredential500JSONResponse:
		return apitypes.Credential{}, false, responseError(500, "GET_CREDENTIAL_FAILED", "failed to get credential", response)
	default:
		return apitypes.Credential{}, false, unexpectedResponse("GetCredential", response)
	}
}

func (m *Manager) putCredential(ctx context.Context, name string, body adminhttp.CredentialUpsert) error {
	response, err := m.services.Credentials.PutCredential(ctx, adminhttp.PutCredentialRequestObject{Id: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutCredential200JSONResponse:
		return nil
	case adminhttp.PutCredential400JSONResponse:
		return responseError(400, "PUT_CREDENTIAL_FAILED", "failed to put credential", response)
	case adminhttp.PutCredential500JSONResponse:
		return responseError(500, "PUT_CREDENTIAL_FAILED", "failed to put credential", response)
	default:
		return unexpectedResponse("PutCredential", response)
	}
}

func (m *Manager) deleteCredential(ctx context.Context, name string) (apitypes.Credential, bool, error) {
	response, err := m.services.Credentials.DeleteCredential(ctx, adminhttp.DeleteCredentialRequestObject{Id: name})
	if err != nil {
		return apitypes.Credential{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteCredential200JSONResponse:
		return apitypes.Credential(response), true, nil
	case adminhttp.DeleteCredential404JSONResponse:
		return apitypes.Credential{}, false, nil
	case adminhttp.DeleteCredential500JSONResponse:
		return apitypes.Credential{}, false, responseError(500, "DELETE_CREDENTIAL_FAILED", "failed to delete credential", response)
	default:
		return apitypes.Credential{}, false, unexpectedResponse("DeleteCredential", response)
	}
}

func credentialSpec(credential apitypes.Credential) apitypes.CredentialSpec {
	return apitypes.CredentialSpec{
		Body:        credential.Body,
		Description: credential.Description,
		Provider:    credential.Provider,
	}
}

func credentialUpsert(resource apitypes.CredentialResource) adminhttp.CredentialUpsert {
	return adminhttp.CredentialUpsert{
		Body:        resource.Spec.Body,
		Description: resource.Spec.Description,
		Name:        string(resource.Metadata.Name),
		Provider:    resource.Spec.Provider,
	}
}

func resourceFromCredential(item apitypes.Credential) (apitypes.Resource, error) {
	return marshalResource(apitypes.CredentialResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.CredentialResourceKind(apitypes.ResourceKindCredential),
		Metadata:   apitypes.ResourceMetadata{Id: &item.Id, Name: item.Name},
		Spec:       credentialSpec(item),
	})
}
