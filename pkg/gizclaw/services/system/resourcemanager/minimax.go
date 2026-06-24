package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func (m *Manager) applyMiniMaxTenant(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ProviderTenants == nil {
		return apitypes.ApplyResult{}, missingService("provider tenants")
	}
	item, err := resource.AsMiniMaxTenantResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_MINIMAX_TENANT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getMiniMaxTenant(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(miniMaxTenantSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindMiniMaxTenant, item.Metadata.Name), nil
		}
	}
	if err := m.putMiniMaxTenant(ctx, name, miniMaxTenantUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindMiniMaxTenant, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindMiniMaxTenant, item.Metadata.Name), nil
}

func (m *Manager) applyVoice(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Voices == nil {
		return apitypes.ApplyResult{}, missingService("voices")
	}
	item, err := resource.AsVoiceResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_VOICE_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	id := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getVoice(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(voiceSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindVoice, item.Metadata.Name), nil
		}
	}
	if err := m.putVoice(ctx, id, voiceUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindVoice, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindVoice, item.Metadata.Name), nil
}

func (m *Manager) applyVolcTenant(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ProviderTenants == nil {
		return apitypes.ApplyResult{}, missingService("provider tenants")
	}
	item, err := resource.AsVolcTenantResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_VOLC_TENANT_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getVolcTenant(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(volcTenantSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindVolcTenant, item.Metadata.Name), nil
		}
	}
	if err := m.putVolcTenant(ctx, name, volcTenantUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindVolcTenant, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindVolcTenant, item.Metadata.Name), nil
}

func (m *Manager) getMiniMaxTenant(ctx context.Context, name string) (apitypes.MiniMaxTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetMiniMaxTenant(ctx, adminservice.GetMiniMaxTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.MiniMaxTenant{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetMiniMaxTenant200JSONResponse:
		return apitypes.MiniMaxTenant(response), true, nil
	case adminservice.GetMiniMaxTenant404JSONResponse:
		return apitypes.MiniMaxTenant{}, false, nil
	case adminservice.GetMiniMaxTenant500JSONResponse:
		return apitypes.MiniMaxTenant{}, false, responseError(500, "GET_MINIMAX_TENANT_FAILED", "failed to get minimax tenant", response)
	default:
		return apitypes.MiniMaxTenant{}, false, unexpectedResponse("GetMiniMaxTenant", response)
	}
}

func (m *Manager) putMiniMaxTenant(ctx context.Context, name string, body adminservice.MiniMaxTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutMiniMaxTenant(ctx, adminservice.PutMiniMaxTenantRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminservice.PutMiniMaxTenant200JSONResponse:
		return nil
	case adminservice.PutMiniMaxTenant400JSONResponse:
		return responseError(400, "PUT_MINIMAX_TENANT_FAILED", "failed to put minimax tenant", response)
	case adminservice.PutMiniMaxTenant500JSONResponse:
		return responseError(500, "PUT_MINIMAX_TENANT_FAILED", "failed to put minimax tenant", response)
	default:
		return unexpectedResponse("PutMiniMaxTenant", response)
	}
}

func (m *Manager) deleteMiniMaxTenant(ctx context.Context, name string) (apitypes.MiniMaxTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteMiniMaxTenant(ctx, adminservice.DeleteMiniMaxTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.MiniMaxTenant{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteMiniMaxTenant200JSONResponse:
		return apitypes.MiniMaxTenant(response), true, nil
	case adminservice.DeleteMiniMaxTenant404JSONResponse:
		return apitypes.MiniMaxTenant{}, false, nil
	case adminservice.DeleteMiniMaxTenant500JSONResponse:
		return apitypes.MiniMaxTenant{}, false, responseError(500, "DELETE_MINIMAX_TENANT_FAILED", "failed to delete minimax tenant", response)
	default:
		return apitypes.MiniMaxTenant{}, false, unexpectedResponse("DeleteMiniMaxTenant", response)
	}
}

func (m *Manager) getVolcTenant(ctx context.Context, name string) (apitypes.VolcTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetVolcTenant(ctx, adminservice.GetVolcTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.VolcTenant{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetVolcTenant200JSONResponse:
		return apitypes.VolcTenant(response), true, nil
	case adminservice.GetVolcTenant404JSONResponse:
		return apitypes.VolcTenant{}, false, nil
	case adminservice.GetVolcTenant500JSONResponse:
		return apitypes.VolcTenant{}, false, responseError(500, "GET_VOLC_TENANT_FAILED", "failed to get volc tenant", response)
	default:
		return apitypes.VolcTenant{}, false, unexpectedResponse("GetVolcTenant", response)
	}
}

func (m *Manager) putVolcTenant(ctx context.Context, name string, body adminservice.VolcTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutVolcTenant(ctx, adminservice.PutVolcTenantRequestObject{Name: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminservice.PutVolcTenant200JSONResponse:
		return nil
	case adminservice.PutVolcTenant400JSONResponse:
		return responseError(400, "PUT_VOLC_TENANT_FAILED", "failed to put volc tenant", response)
	case adminservice.PutVolcTenant500JSONResponse:
		return responseError(500, "PUT_VOLC_TENANT_FAILED", "failed to put volc tenant", response)
	default:
		return unexpectedResponse("PutVolcTenant", response)
	}
}

func (m *Manager) deleteVolcTenant(ctx context.Context, name string) (apitypes.VolcTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteVolcTenant(ctx, adminservice.DeleteVolcTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.VolcTenant{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteVolcTenant200JSONResponse:
		return apitypes.VolcTenant(response), true, nil
	case adminservice.DeleteVolcTenant404JSONResponse:
		return apitypes.VolcTenant{}, false, nil
	case adminservice.DeleteVolcTenant500JSONResponse:
		return apitypes.VolcTenant{}, false, responseError(500, "DELETE_VOLC_TENANT_FAILED", "failed to delete volc tenant", response)
	default:
		return apitypes.VolcTenant{}, false, unexpectedResponse("DeleteVolcTenant", response)
	}
}

func (m *Manager) getVoice(ctx context.Context, id string) (apitypes.Voice, bool, error) {
	response, err := m.services.Voices.GetVoice(ctx, adminservice.GetVoiceRequestObject{Id: id})
	if err != nil {
		return apitypes.Voice{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetVoice200JSONResponse:
		return apitypes.Voice(response), true, nil
	case adminservice.GetVoice404JSONResponse:
		return apitypes.Voice{}, false, nil
	case adminservice.GetVoice500JSONResponse:
		return apitypes.Voice{}, false, responseError(500, "GET_VOICE_FAILED", "failed to get voice", response)
	default:
		return apitypes.Voice{}, false, unexpectedResponse("GetVoice", response)
	}
}

func (m *Manager) putVoice(ctx context.Context, id string, body adminservice.VoiceUpsert) error {
	response, err := m.services.Voices.PutVoice(ctx, adminservice.PutVoiceRequestObject{Id: id, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminservice.PutVoice200JSONResponse:
		return nil
	case adminservice.PutVoice400JSONResponse:
		return responseError(400, "PUT_VOICE_FAILED", "failed to put voice", response)
	case adminservice.PutVoice409JSONResponse:
		return responseError(409, "PUT_VOICE_FAILED", "failed to put voice", response)
	case adminservice.PutVoice500JSONResponse:
		return responseError(500, "PUT_VOICE_FAILED", "failed to put voice", response)
	default:
		return unexpectedResponse("PutVoice", response)
	}
}

func (m *Manager) deleteVoice(ctx context.Context, id string) (apitypes.Voice, bool, error) {
	response, err := m.services.Voices.DeleteVoice(ctx, adminservice.DeleteVoiceRequestObject{Id: id})
	if err != nil {
		return apitypes.Voice{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteVoice200JSONResponse:
		return apitypes.Voice(response), true, nil
	case adminservice.DeleteVoice404JSONResponse:
		return apitypes.Voice{}, false, nil
	case adminservice.DeleteVoice500JSONResponse:
		return apitypes.Voice{}, false, responseError(500, "DELETE_VOICE_FAILED", "failed to delete voice", response)
	default:
		return apitypes.Voice{}, false, unexpectedResponse("DeleteVoice", response)
	}
}

func miniMaxTenantSpec(tenant apitypes.MiniMaxTenant) apitypes.MiniMaxTenantSpec {
	return apitypes.MiniMaxTenantSpec{
		AppId:          tenant.AppId,
		BaseUrl:        tenant.BaseUrl,
		CredentialName: tenant.CredentialName,
		Description:    tenant.Description,
		GroupId:        tenant.GroupId,
	}
}

func miniMaxTenantUpsert(resource apitypes.MiniMaxTenantResource) adminservice.MiniMaxTenantUpsert {
	return adminservice.MiniMaxTenantUpsert{
		AppId:          resource.Spec.AppId,
		BaseUrl:        resource.Spec.BaseUrl,
		CredentialName: resource.Spec.CredentialName,
		Description:    resource.Spec.Description,
		GroupId:        resource.Spec.GroupId,
		Name:           string(resource.Metadata.Name),
	}
}

func volcTenantSpec(tenant apitypes.VolcTenant) apitypes.VolcTenantSpec {
	return apitypes.VolcTenantSpec{
		CredentialName: tenant.CredentialName,
		Description:    tenant.Description,
		Endpoint:       tenant.Endpoint,
		Region:         tenant.Region,
		ResourceIds:    tenant.ResourceIds,
	}
}

func volcTenantUpsert(resource apitypes.VolcTenantResource) adminservice.VolcTenantUpsert {
	return adminservice.VolcTenantUpsert{
		CredentialName: resource.Spec.CredentialName,
		Description:    resource.Spec.Description,
		Endpoint:       resource.Spec.Endpoint,
		Name:           string(resource.Metadata.Name),
		Region:         resource.Spec.Region,
		ResourceIds:    resource.Spec.ResourceIds,
	}
}

func voiceSpec(voice apitypes.Voice) apitypes.VoiceSpec {
	return apitypes.VoiceSpec{
		Description:  voice.Description,
		Name:         voice.Name,
		Provider:     voice.Provider,
		ProviderData: voice.ProviderData,
		Source:       voice.Source,
	}
}

func voiceUpsert(resource apitypes.VoiceResource) adminservice.VoiceUpsert {
	return adminservice.VoiceUpsert{
		Description:  resource.Spec.Description,
		Id:           string(resource.Metadata.Name),
		Name:         resource.Spec.Name,
		Provider:     resource.Spec.Provider,
		ProviderData: resource.Spec.ProviderData,
		Source:       resource.Spec.Source,
	}
}

func resourceFromMiniMaxTenant(item apitypes.MiniMaxTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.MiniMaxTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.MiniMaxTenantResourceKind(apitypes.ResourceKindMiniMaxTenant),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       miniMaxTenantSpec(item),
	})
}

func resourceFromVolcTenant(item apitypes.VolcTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.VolcTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.VolcTenantResourceKind(apitypes.ResourceKindVolcTenant),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Name)},
		Spec:       volcTenantSpec(item),
	})
}

func resourceFromVoice(item apitypes.Voice) (apitypes.Resource, error) {
	return marshalResource(apitypes.VoiceResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.VoiceResourceKind(apitypes.ResourceKindVoice),
		Metadata:   apitypes.ResourceMetadata{Name: string(item.Id)},
		Spec:       voiceSpec(item),
	})
}
