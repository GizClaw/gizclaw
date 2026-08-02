package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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
	body := miniMaxTenantUpsert(item)
	return applyNamedResource(ctx, item.Metadata, apitypes.ResourceKindMiniMaxTenant, item.Spec,
		m.getMiniMaxTenant,
		func(ctx context.Context) (string, error) { return m.createMiniMaxTenant(ctx, body) },
		func(ctx context.Context, id string) error { return m.putMiniMaxTenant(ctx, id, body) },
		func(value apitypes.MiniMaxTenant) string { return value.Name }, miniMaxTenantSpec)
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
	body := voiceUpsert(item)
	return applyNamedResource(ctx, item.Metadata, apitypes.ResourceKindVoice, item.Spec,
		m.getVoice,
		func(ctx context.Context) (string, error) { return m.createVoice(ctx, body) },
		func(ctx context.Context, id string) error { return m.putVoice(ctx, id, body) },
		func(value apitypes.Voice) string { return value.Name }, voiceSpec)
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
	body := volcTenantUpsert(item)
	return applyNamedResource(ctx, item.Metadata, apitypes.ResourceKindVolcTenant, item.Spec,
		m.getVolcTenant,
		func(ctx context.Context) (string, error) { return m.createVolcTenant(ctx, body) },
		func(ctx context.Context, id string) error { return m.putVolcTenant(ctx, id, body) },
		func(value apitypes.VolcTenant) string { return value.Name }, volcTenantSpec)
}

func (m *Manager) createMiniMaxTenant(ctx context.Context, body adminhttp.MiniMaxTenantUpsert) (string, error) {
	response, err := m.services.ProviderTenants.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateMiniMaxTenant200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateMiniMaxTenant400JSONResponse:
		return "", responseError(400, "CREATE_MINIMAX_TENANT_FAILED", "failed to create MiniMax tenant", response)
	case adminhttp.CreateMiniMaxTenant409JSONResponse:
		return "", responseError(409, "CREATE_MINIMAX_TENANT_FAILED", "failed to create MiniMax tenant", response)
	case adminhttp.CreateMiniMaxTenant500JSONResponse:
		return "", responseError(500, "CREATE_MINIMAX_TENANT_FAILED", "failed to create MiniMax tenant", response)
	default:
		return "", unexpectedResponse("CreateMiniMaxTenant", response)
	}
}

func (m *Manager) createVolcTenant(ctx context.Context, body adminhttp.VolcTenantUpsert) (string, error) {
	response, err := m.services.ProviderTenants.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateVolcTenant200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateVolcTenant400JSONResponse:
		return "", responseError(400, "CREATE_VOLC_TENANT_FAILED", "failed to create Volc tenant", response)
	case adminhttp.CreateVolcTenant409JSONResponse:
		return "", responseError(409, "CREATE_VOLC_TENANT_FAILED", "failed to create Volc tenant", response)
	case adminhttp.CreateVolcTenant500JSONResponse:
		return "", responseError(500, "CREATE_VOLC_TENANT_FAILED", "failed to create Volc tenant", response)
	default:
		return "", unexpectedResponse("CreateVolcTenant", response)
	}
}

func (m *Manager) createVoice(ctx context.Context, body adminhttp.VoiceUpsert) (string, error) {
	response, err := m.services.Voices.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{Body: &body})
	if err != nil {
		return "", err
	}
	switch response := response.(type) {
	case adminhttp.CreateVoice200JSONResponse:
		return response.Id, nil
	case adminhttp.CreateVoice400JSONResponse:
		return "", responseError(400, "CREATE_VOICE_FAILED", "failed to create voice", response)
	case adminhttp.CreateVoice409JSONResponse:
		return "", responseError(409, "CREATE_VOICE_FAILED", "failed to create voice", response)
	case adminhttp.CreateVoice500JSONResponse:
		return "", responseError(500, "CREATE_VOICE_FAILED", "failed to create voice", response)
	default:
		return "", unexpectedResponse("CreateVoice", response)
	}
}

func (m *Manager) getMiniMaxTenant(ctx context.Context, name string) (apitypes.MiniMaxTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetMiniMaxTenant(ctx, adminhttp.GetMiniMaxTenantRequestObject{Id: name})
	if err != nil {
		return apitypes.MiniMaxTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetMiniMaxTenant200JSONResponse:
		return apitypes.MiniMaxTenant(response), true, nil
	case adminhttp.GetMiniMaxTenant404JSONResponse:
		return apitypes.MiniMaxTenant{}, false, nil
	case adminhttp.GetMiniMaxTenant500JSONResponse:
		return apitypes.MiniMaxTenant{}, false, responseError(500, "GET_MINIMAX_TENANT_FAILED", "failed to get minimax tenant", response)
	default:
		return apitypes.MiniMaxTenant{}, false, unexpectedResponse("GetMiniMaxTenant", response)
	}
}

func (m *Manager) putMiniMaxTenant(ctx context.Context, name string, body adminhttp.MiniMaxTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutMiniMaxTenant(ctx, adminhttp.PutMiniMaxTenantRequestObject{Id: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutMiniMaxTenant200JSONResponse:
		return nil
	case adminhttp.PutMiniMaxTenant400JSONResponse:
		return responseError(400, "PUT_MINIMAX_TENANT_FAILED", "failed to put minimax tenant", response)
	case adminhttp.PutMiniMaxTenant500JSONResponse:
		return responseError(500, "PUT_MINIMAX_TENANT_FAILED", "failed to put minimax tenant", response)
	default:
		return unexpectedResponse("PutMiniMaxTenant", response)
	}
}

func (m *Manager) deleteMiniMaxTenant(ctx context.Context, name string) (apitypes.MiniMaxTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteMiniMaxTenant(ctx, adminhttp.DeleteMiniMaxTenantRequestObject{Id: name})
	if err != nil {
		return apitypes.MiniMaxTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteMiniMaxTenant200JSONResponse:
		return apitypes.MiniMaxTenant(response), true, nil
	case adminhttp.DeleteMiniMaxTenant404JSONResponse:
		return apitypes.MiniMaxTenant{}, false, nil
	case adminhttp.DeleteMiniMaxTenant500JSONResponse:
		return apitypes.MiniMaxTenant{}, false, responseError(500, "DELETE_MINIMAX_TENANT_FAILED", "failed to delete minimax tenant", response)
	default:
		return apitypes.MiniMaxTenant{}, false, unexpectedResponse("DeleteMiniMaxTenant", response)
	}
}

func (m *Manager) getVolcTenant(ctx context.Context, name string) (apitypes.VolcTenant, bool, error) {
	response, err := m.services.ProviderTenants.GetVolcTenant(ctx, adminhttp.GetVolcTenantRequestObject{Id: name})
	if err != nil {
		return apitypes.VolcTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetVolcTenant200JSONResponse:
		return apitypes.VolcTenant(response), true, nil
	case adminhttp.GetVolcTenant404JSONResponse:
		return apitypes.VolcTenant{}, false, nil
	case adminhttp.GetVolcTenant500JSONResponse:
		return apitypes.VolcTenant{}, false, responseError(500, "GET_VOLC_TENANT_FAILED", "failed to get volc tenant", response)
	default:
		return apitypes.VolcTenant{}, false, unexpectedResponse("GetVolcTenant", response)
	}
}

func (m *Manager) putVolcTenant(ctx context.Context, name string, body adminhttp.VolcTenantUpsert) error {
	response, err := m.services.ProviderTenants.PutVolcTenant(ctx, adminhttp.PutVolcTenantRequestObject{Id: name, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutVolcTenant200JSONResponse:
		return nil
	case adminhttp.PutVolcTenant400JSONResponse:
		return responseError(400, "PUT_VOLC_TENANT_FAILED", "failed to put volc tenant", response)
	case adminhttp.PutVolcTenant500JSONResponse:
		return responseError(500, "PUT_VOLC_TENANT_FAILED", "failed to put volc tenant", response)
	default:
		return unexpectedResponse("PutVolcTenant", response)
	}
}

func (m *Manager) deleteVolcTenant(ctx context.Context, name string) (apitypes.VolcTenant, bool, error) {
	response, err := m.services.ProviderTenants.DeleteVolcTenant(ctx, adminhttp.DeleteVolcTenantRequestObject{Id: name})
	if err != nil {
		return apitypes.VolcTenant{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteVolcTenant200JSONResponse:
		return apitypes.VolcTenant(response), true, nil
	case adminhttp.DeleteVolcTenant404JSONResponse:
		return apitypes.VolcTenant{}, false, nil
	case adminhttp.DeleteVolcTenant500JSONResponse:
		return apitypes.VolcTenant{}, false, responseError(500, "DELETE_VOLC_TENANT_FAILED", "failed to delete volc tenant", response)
	default:
		return apitypes.VolcTenant{}, false, unexpectedResponse("DeleteVolcTenant", response)
	}
}

func (m *Manager) getVoice(ctx context.Context, id string) (apitypes.Voice, bool, error) {
	response, err := m.services.Voices.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: id})
	if err != nil {
		return apitypes.Voice{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetVoice200JSONResponse:
		return apitypes.Voice(response), true, nil
	case adminhttp.GetVoice404JSONResponse:
		return apitypes.Voice{}, false, nil
	case adminhttp.GetVoice500JSONResponse:
		return apitypes.Voice{}, false, responseError(500, "GET_VOICE_FAILED", "failed to get voice", response)
	default:
		return apitypes.Voice{}, false, unexpectedResponse("GetVoice", response)
	}
}

func (m *Manager) putVoice(ctx context.Context, id string, body adminhttp.VoiceUpsert) error {
	response, err := m.services.Voices.PutVoice(ctx, adminhttp.PutVoiceRequestObject{Id: id, Body: &body})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.PutVoice200JSONResponse:
		return nil
	case adminhttp.PutVoice400JSONResponse:
		return responseError(400, "PUT_VOICE_FAILED", "failed to put voice", response)
	case adminhttp.PutVoice409JSONResponse:
		return responseError(409, "PUT_VOICE_FAILED", "failed to put voice", response)
	case adminhttp.PutVoice500JSONResponse:
		return responseError(500, "PUT_VOICE_FAILED", "failed to put voice", response)
	default:
		return unexpectedResponse("PutVoice", response)
	}
}

func (m *Manager) deleteVoice(ctx context.Context, id string) (apitypes.Voice, bool, error) {
	response, err := m.services.Voices.DeleteVoice(ctx, adminhttp.DeleteVoiceRequestObject{Id: id})
	if err != nil {
		return apitypes.Voice{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteVoice200JSONResponse:
		return apitypes.Voice(response), true, nil
	case adminhttp.DeleteVoice404JSONResponse:
		return apitypes.Voice{}, false, nil
	case adminhttp.DeleteVoice500JSONResponse:
		return apitypes.Voice{}, false, responseError(500, "DELETE_VOICE_FAILED", "failed to delete voice", response)
	default:
		return apitypes.Voice{}, false, unexpectedResponse("DeleteVoice", response)
	}
}

func miniMaxTenantSpec(tenant apitypes.MiniMaxTenant) apitypes.MiniMaxTenantSpec {
	return apitypes.MiniMaxTenantSpec{
		AppId:        tenant.AppId,
		BaseUrl:      tenant.BaseUrl,
		CredentialId: tenant.CredentialId,
		Description:  tenant.Description,
		GroupId:      tenant.GroupId,
	}
}

func miniMaxTenantUpsert(resource apitypes.MiniMaxTenantResource) adminhttp.MiniMaxTenantUpsert {
	return adminhttp.MiniMaxTenantUpsert{
		AppId:        resource.Spec.AppId,
		BaseUrl:      resource.Spec.BaseUrl,
		CredentialId: resource.Spec.CredentialId,
		Description:  resource.Spec.Description,
		GroupId:      resource.Spec.GroupId,
		Name:         string(resource.Metadata.Name),
	}
}

func volcTenantSpec(tenant apitypes.VolcTenant) apitypes.VolcTenantSpec {
	return apitypes.VolcTenantSpec{
		CredentialId: tenant.CredentialId,
		Description:  tenant.Description,
		Endpoint:     tenant.Endpoint,
		Region:       tenant.Region,
		ResourceIds:  tenant.ResourceIds,
	}
}

func volcTenantUpsert(resource apitypes.VolcTenantResource) adminhttp.VolcTenantUpsert {
	return adminhttp.VolcTenantUpsert{
		CredentialId: resource.Spec.CredentialId,
		Description:  resource.Spec.Description,
		Endpoint:     resource.Spec.Endpoint,
		Name:         string(resource.Metadata.Name),
		Region:       resource.Spec.Region,
		ResourceIds:  resource.Spec.ResourceIds,
	}
}

func voiceSpec(voice apitypes.Voice) apitypes.VoiceSpec {
	return apitypes.VoiceSpec{
		Description:  voice.Description,
		DisplayName:  voice.DisplayName,
		Provider:     voice.Provider,
		ProviderData: voice.ProviderData,
		Source:       voice.Source,
	}
}

func voiceUpsert(resource apitypes.VoiceResource) adminhttp.VoiceUpsert {
	return adminhttp.VoiceUpsert{
		Description:  resource.Spec.Description,
		Name:         string(resource.Metadata.Name),
		DisplayName:  resource.Spec.DisplayName,
		Provider:     resource.Spec.Provider,
		ProviderData: resource.Spec.ProviderData,
		Source:       resource.Spec.Source,
	}
}

func resourceFromMiniMaxTenant(item apitypes.MiniMaxTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.MiniMaxTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.MiniMaxTenantResourceKind(apitypes.ResourceKindMiniMaxTenant),
		Metadata:   apitypes.ResourceMetadata{Id: &item.Id, Name: item.Name},
		Spec:       miniMaxTenantSpec(item),
	})
}

func resourceFromVolcTenant(item apitypes.VolcTenant) (apitypes.Resource, error) {
	return marshalResource(apitypes.VolcTenantResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.VolcTenantResourceKind(apitypes.ResourceKindVolcTenant),
		Metadata:   apitypes.ResourceMetadata{Id: &item.Id, Name: item.Name},
		Spec:       volcTenantSpec(item),
	})
}

func resourceFromVoice(item apitypes.Voice) (apitypes.Resource, error) {
	return marshalResource(apitypes.VoiceResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.VoiceResourceKind(apitypes.ResourceKindVoice),
		Metadata:   apitypes.ResourceMetadata{Id: &item.Id, Name: item.Name},
		Spec:       voiceSpec(item),
	})
}
