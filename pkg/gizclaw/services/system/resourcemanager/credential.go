package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
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
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getCredential(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(credentialSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindCredential, item.Metadata.Name), nil
		}
	}
	if err := m.putCredential(ctx, name, credentialUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindCredential, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindCredential, item.Metadata.Name), nil
}

func (m *Manager) getCredential(ctx context.Context, name string) (apitypes.Credential, bool, error) {
	response, err := m.services.Credentials.GetCredential(ctx, adminservice.GetCredentialRequestObject{Name: name})
	if err != nil {
		return apitypes.Credential{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetCredential200JSONResponse:
		return apitypes.Credential(response), true, nil
	case adminservice.GetCredential404JSONResponse:
		return apitypes.Credential{}, false, nil
	case adminservice.GetCredential500JSONResponse:
		return apitypes.Credential{}, false, responseError(500, "GET_CREDENTIAL_FAILED", "failed to get credential", response)
	default:
		return apitypes.Credential{}, false, unexpectedResponse("GetCredential", response)
	}
}

func (m *Manager) putCredential(ctx context.Context, name string, body adminservice.CredentialUpsert) error {
	response, err := m.services.Credentials.PutCredential(ctx, adminservice.PutCredentialRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminservice.PutCredential200JSONResponse:
		return nil
	case adminservice.PutCredential400JSONResponse:
		return responseError(400, "PUT_CREDENTIAL_FAILED", "failed to put credential", response)
	case adminservice.PutCredential500JSONResponse:
		return responseError(500, "PUT_CREDENTIAL_FAILED", "failed to put credential", response)
	default:
		return unexpectedResponse("PutCredential", response)
	}
}

func (m *Manager) deleteCredential(ctx context.Context, name string) (apitypes.Credential, bool, error) {
	response, err := m.services.Credentials.DeleteCredential(ctx, adminservice.DeleteCredentialRequestObject{Name: name})
	if err != nil {
		return apitypes.Credential{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteCredential200JSONResponse:
		return apitypes.Credential(response), true, nil
	case adminservice.DeleteCredential404JSONResponse:
		return apitypes.Credential{}, false, nil
	case adminservice.DeleteCredential500JSONResponse:
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

func credentialUpsert(resource apitypes.CredentialResource) adminservice.CredentialUpsert {
	return adminservice.CredentialUpsert{
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
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       credentialSpec(item),
	})
}
