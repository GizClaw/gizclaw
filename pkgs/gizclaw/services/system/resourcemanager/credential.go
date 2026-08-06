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
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if item.Spec.Body == nil {
		existing, exists, err := m.getCredential(ctx, servicePathID(item.Metadata.Id))
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		if !exists {
			return apitypes.ApplyResult{}, applyError(400, "INVALID_CREDENTIAL_RESOURCE", "spec.body is required when creating a credential")
		}
		body := existing.Body
		item.Spec.Body = &body
	}
	body := credentialUpsert(item)
	return applyConcreteResource(ctx, item.Metadata, apitypes.ResourceKindCredential, item.Spec,
		m.getCredential,
		func(ctx context.Context) (string, error) { return m.createCredential(ctx, body) },
		func(ctx context.Context, id string) error { return m.putCredential(ctx, id, body) }, credentialSpec)
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

func (m *Manager) getCredential(ctx context.Context, id string) (apitypes.Credential, bool, error) {
	response, err := m.services.Credentials.GetCredential(ctx, adminhttp.GetCredentialRequestObject{Id: id})
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

func (m *Manager) putCredential(ctx context.Context, id string, body adminhttp.CredentialUpsert) error {
	response, err := m.services.Credentials.PutCredential(ctx, adminhttp.PutCredentialRequestObject{Id: id, Body: &body})
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

func (m *Manager) deleteCredential(ctx context.Context, id string) (apitypes.Credential, bool, error) {
	response, err := m.services.Credentials.DeleteCredential(ctx, adminhttp.DeleteCredentialRequestObject{Id: id})
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
	body := credential.Body
	return apitypes.CredentialSpec{
		Body:        &body,
		Description: credential.Description,
		Provider:    credential.Provider,
	}
}

func credentialUpsert(resource apitypes.CredentialResource) adminhttp.CredentialUpsert {
	var body apitypes.CredentialBody
	if resource.Spec.Body != nil {
		body = *resource.Spec.Body
	}
	return adminhttp.CredentialUpsert{
		Body:        body,
		Description: resource.Spec.Description,
		Id:          resource.Metadata.Id,
		Provider:    resource.Spec.Provider,
	}
}

func resourceFromCredential(item apitypes.Credential) (apitypes.Resource, error) {
	return marshalResource(apitypes.CredentialResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.CredentialResourceKind(apitypes.ResourceKindCredential),
		Metadata:   apitypes.ResourceMetadata{Id: item.Id},
		Spec: apitypes.CredentialSpec{
			Description: item.Description,
			Provider:    item.Provider,
		},
	})
}
