package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyGameRuleset(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	item, err := resource.AsGameRulesetResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_GAME_RULESET_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	existing, exists, err := m.getGameRuleset(ctx, string(pathParam(item.Metadata.Name)))
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(existing.Spec, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindGameRuleset, item.Metadata.Name), nil
		}
	}
	if err := m.putGameRuleset(ctx, string(pathParam(item.Metadata.Name)), gameRulesetUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindGameRuleset, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindGameRuleset, item.Metadata.Name), nil
}

func (m *Manager) applyPetDef(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	item, err := resource.AsPetDefResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_PET_DEF_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	existing, exists, err := m.getPetDef(ctx, string(pathParam(item.Metadata.Name)))
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(existing.Spec, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindPetDef, item.Metadata.Name), nil
		}
	}
	if err := m.putPetDef(ctx, string(pathParam(item.Metadata.Name)), petDefUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindPetDef, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindPetDef, item.Metadata.Name), nil
}

func (m *Manager) applyBadgeDef(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	item, err := resource.AsBadgeDefResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_BADGE_DEF_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	existing, exists, err := m.getBadgeDef(ctx, string(pathParam(item.Metadata.Name)))
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(existing.Spec, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindBadgeDef, item.Metadata.Name), nil
		}
	}
	if err := m.putBadgeDef(ctx, string(pathParam(item.Metadata.Name)), badgeDefUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindBadgeDef, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindBadgeDef, item.Metadata.Name), nil
}

func (m *Manager) applyGameDef(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	item, err := resource.AsGameDefResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_GAME_DEF_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	existing, exists, err := m.getGameDef(ctx, string(pathParam(item.Metadata.Name)))
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(existing.Spec, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindGameDef, item.Metadata.Name), nil
		}
	}
	if err := m.putGameDef(ctx, string(pathParam(item.Metadata.Name)), gameDefUpsert(item)); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindGameDef, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindGameDef, item.Metadata.Name), nil
}

func (m *Manager) getGameRuleset(ctx context.Context, name string) (apitypes.GameRuleset, bool, error) {
	if m.services.GameplayCatalog == nil {
		return apitypes.GameRuleset{}, false, missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.GetGameRuleset(ctx, adminservice.GetGameRulesetRequestObject{Name: name})
	if err != nil {
		return apitypes.GameRuleset{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetGameRuleset200JSONResponse:
		return apitypes.GameRuleset(response), true, nil
	case adminservice.GetGameRuleset404JSONResponse:
		return apitypes.GameRuleset{}, false, nil
	case adminservice.GetGameRuleset500JSONResponse:
		return apitypes.GameRuleset{}, false, responseError(500, "GET_GAME_RULESET_FAILED", "failed to get game ruleset", response)
	default:
		return apitypes.GameRuleset{}, false, unexpectedResponse("GetGameRuleset", response)
	}
}

func (m *Manager) putGameRuleset(ctx context.Context, name string, body adminservice.GameRulesetUpsert) error {
	if m.services.GameplayCatalog == nil {
		return missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.PutGameRuleset(ctx, adminservice.PutGameRulesetRequestObject{Name: name, Body: &body})
	return putGameplayResponse("PutGameRuleset", response, err)
}

func (m *Manager) deleteGameRuleset(ctx context.Context, name string) (apitypes.GameRuleset, bool, error) {
	response, err := m.services.GameplayCatalog.DeleteGameRuleset(ctx, adminservice.DeleteGameRulesetRequestObject{Name: name})
	if err != nil {
		return apitypes.GameRuleset{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteGameRuleset200JSONResponse:
		return apitypes.GameRuleset(response), true, nil
	case adminservice.DeleteGameRuleset404JSONResponse:
		return apitypes.GameRuleset{}, false, nil
	case adminservice.DeleteGameRuleset500JSONResponse:
		return apitypes.GameRuleset{}, false, responseError(500, "DELETE_GAME_RULESET_FAILED", "failed to delete game ruleset", response)
	default:
		return apitypes.GameRuleset{}, false, unexpectedResponse("DeleteGameRuleset", response)
	}
}

func (m *Manager) getPetDef(ctx context.Context, id string) (apitypes.PetDef, bool, error) {
	if m.services.GameplayCatalog == nil {
		return apitypes.PetDef{}, false, missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.GetPetDef(ctx, adminservice.GetPetDefRequestObject{Id: id})
	if err != nil {
		return apitypes.PetDef{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetPetDef200JSONResponse:
		return apitypes.PetDef(response), true, nil
	case adminservice.GetPetDef404JSONResponse:
		return apitypes.PetDef{}, false, nil
	case adminservice.GetPetDef500JSONResponse:
		return apitypes.PetDef{}, false, responseError(500, "GET_PET_DEF_FAILED", "failed to get pet def", response)
	default:
		return apitypes.PetDef{}, false, unexpectedResponse("GetPetDef", response)
	}
}

func (m *Manager) putPetDef(ctx context.Context, id string, body adminservice.PetDefUpsert) error {
	if m.services.GameplayCatalog == nil {
		return missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.PutPetDef(ctx, adminservice.PutPetDefRequestObject{Id: id, Body: &body})
	return putGameplayResponse("PutPetDef", response, err)
}

func (m *Manager) deletePetDef(ctx context.Context, id string) (apitypes.PetDef, bool, error) {
	response, err := m.services.GameplayCatalog.DeletePetDef(ctx, adminservice.DeletePetDefRequestObject{Id: id})
	if err != nil {
		return apitypes.PetDef{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeletePetDef200JSONResponse:
		return apitypes.PetDef(response), true, nil
	case adminservice.DeletePetDef404JSONResponse:
		return apitypes.PetDef{}, false, nil
	case adminservice.DeletePetDef500JSONResponse:
		return apitypes.PetDef{}, false, responseError(500, "DELETE_PET_DEF_FAILED", "failed to delete pet def", response)
	default:
		return apitypes.PetDef{}, false, unexpectedResponse("DeletePetDef", response)
	}
}

func (m *Manager) getBadgeDef(ctx context.Context, id string) (apitypes.BadgeDef, bool, error) {
	if m.services.GameplayCatalog == nil {
		return apitypes.BadgeDef{}, false, missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.GetBadgeDef(ctx, adminservice.GetBadgeDefRequestObject{Id: id})
	if err != nil {
		return apitypes.BadgeDef{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetBadgeDef200JSONResponse:
		return apitypes.BadgeDef(response), true, nil
	case adminservice.GetBadgeDef404JSONResponse:
		return apitypes.BadgeDef{}, false, nil
	case adminservice.GetBadgeDef500JSONResponse:
		return apitypes.BadgeDef{}, false, responseError(500, "GET_BADGE_DEF_FAILED", "failed to get badge def", response)
	default:
		return apitypes.BadgeDef{}, false, unexpectedResponse("GetBadgeDef", response)
	}
}

func (m *Manager) putBadgeDef(ctx context.Context, id string, body adminservice.BadgeDefUpsert) error {
	if m.services.GameplayCatalog == nil {
		return missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.PutBadgeDef(ctx, adminservice.PutBadgeDefRequestObject{Id: id, Body: &body})
	return putGameplayResponse("PutBadgeDef", response, err)
}

func (m *Manager) deleteBadgeDef(ctx context.Context, id string) (apitypes.BadgeDef, bool, error) {
	response, err := m.services.GameplayCatalog.DeleteBadgeDef(ctx, adminservice.DeleteBadgeDefRequestObject{Id: id})
	if err != nil {
		return apitypes.BadgeDef{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteBadgeDef200JSONResponse:
		return apitypes.BadgeDef(response), true, nil
	case adminservice.DeleteBadgeDef404JSONResponse:
		return apitypes.BadgeDef{}, false, nil
	case adminservice.DeleteBadgeDef500JSONResponse:
		return apitypes.BadgeDef{}, false, responseError(500, "DELETE_BADGE_DEF_FAILED", "failed to delete badge def", response)
	default:
		return apitypes.BadgeDef{}, false, unexpectedResponse("DeleteBadgeDef", response)
	}
}

func (m *Manager) getGameDef(ctx context.Context, id string) (apitypes.GameDef, bool, error) {
	if m.services.GameplayCatalog == nil {
		return apitypes.GameDef{}, false, missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.GetGameDef(ctx, adminservice.GetGameDefRequestObject{Id: id})
	if err != nil {
		return apitypes.GameDef{}, false, err
	}
	switch response := response.(type) {
	case adminservice.GetGameDef200JSONResponse:
		return apitypes.GameDef(response), true, nil
	case adminservice.GetGameDef404JSONResponse:
		return apitypes.GameDef{}, false, nil
	case adminservice.GetGameDef500JSONResponse:
		return apitypes.GameDef{}, false, responseError(500, "GET_GAME_DEF_FAILED", "failed to get game def", response)
	default:
		return apitypes.GameDef{}, false, unexpectedResponse("GetGameDef", response)
	}
}

func (m *Manager) putGameDef(ctx context.Context, id string, body adminservice.GameDefUpsert) error {
	if m.services.GameplayCatalog == nil {
		return missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.PutGameDef(ctx, adminservice.PutGameDefRequestObject{Id: id, Body: &body})
	return putGameplayResponse("PutGameDef", response, err)
}

func (m *Manager) deleteGameDef(ctx context.Context, id string) (apitypes.GameDef, bool, error) {
	response, err := m.services.GameplayCatalog.DeleteGameDef(ctx, adminservice.DeleteGameDefRequestObject{Id: id})
	if err != nil {
		return apitypes.GameDef{}, false, err
	}
	switch response := response.(type) {
	case adminservice.DeleteGameDef200JSONResponse:
		return apitypes.GameDef(response), true, nil
	case adminservice.DeleteGameDef404JSONResponse:
		return apitypes.GameDef{}, false, nil
	case adminservice.DeleteGameDef500JSONResponse:
		return apitypes.GameDef{}, false, responseError(500, "DELETE_GAME_DEF_FAILED", "failed to delete game def", response)
	default:
		return apitypes.GameDef{}, false, unexpectedResponse("DeleteGameDef", response)
	}
}

func putGameplayResponse(operation string, response any, err error) error {
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminservice.PutGameRuleset200JSONResponse,
		adminservice.PutPetDef200JSONResponse,
		adminservice.PutBadgeDef200JSONResponse,
		adminservice.PutGameDef200JSONResponse:
		return nil
	case adminservice.PutGameRuleset400JSONResponse:
		return responseError(400, "PUT_GAME_RULESET_FAILED", "failed to put game ruleset", response)
	case adminservice.PutPetDef400JSONResponse:
		return responseError(400, "PUT_PET_DEF_FAILED", "failed to put pet def", response)
	case adminservice.PutBadgeDef400JSONResponse:
		return responseError(400, "PUT_BADGE_DEF_FAILED", "failed to put badge def", response)
	case adminservice.PutGameDef400JSONResponse:
		return responseError(400, "PUT_GAME_DEF_FAILED", "failed to put game def", response)
	case adminservice.PutGameRuleset409JSONResponse:
		return responseError(409, "PUT_GAME_RULESET_FAILED", "failed to put game ruleset", response)
	case adminservice.PutPetDef409JSONResponse:
		return responseError(409, "PUT_PET_DEF_FAILED", "failed to put pet def", response)
	case adminservice.PutBadgeDef409JSONResponse:
		return responseError(409, "PUT_BADGE_DEF_FAILED", "failed to put badge def", response)
	case adminservice.PutGameDef409JSONResponse:
		return responseError(409, "PUT_GAME_DEF_FAILED", "failed to put game def", response)
	case adminservice.PutGameRuleset500JSONResponse:
		return responseError(500, "PUT_GAME_RULESET_FAILED", "failed to put game ruleset", response)
	case adminservice.PutPetDef500JSONResponse:
		return responseError(500, "PUT_PET_DEF_FAILED", "failed to put pet def", response)
	case adminservice.PutBadgeDef500JSONResponse:
		return responseError(500, "PUT_BADGE_DEF_FAILED", "failed to put badge def", response)
	case adminservice.PutGameDef500JSONResponse:
		return responseError(500, "PUT_GAME_DEF_FAILED", "failed to put game def", response)
	default:
		return unexpectedResponse(operation, response)
	}
}

func gameRulesetUpsert(resource apitypes.GameRulesetResource) adminservice.GameRulesetUpsert {
	return adminservice.GameRulesetUpsert{Name: resource.Metadata.Name, Spec: resource.Spec}
}

func petDefUpsert(resource apitypes.PetDefResource) adminservice.PetDefUpsert {
	return adminservice.PetDefUpsert{Id: resource.Metadata.Name, Spec: resource.Spec}
}

func badgeDefUpsert(resource apitypes.BadgeDefResource) adminservice.BadgeDefUpsert {
	return adminservice.BadgeDefUpsert{Id: resource.Metadata.Name, Spec: resource.Spec}
}

func gameDefUpsert(resource apitypes.GameDefResource) adminservice.GameDefUpsert {
	return adminservice.GameDefUpsert{Id: resource.Metadata.Name, Spec: resource.Spec}
}

func resourceFromGameRuleset(item apitypes.GameRuleset) (apitypes.Resource, error) {
	return marshalResource(apitypes.GameRulesetResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.GameRulesetResourceKind(apitypes.ResourceKindGameRuleset),
		Metadata:   apitypes.ResourceMetadata{Name: item.Name},
		Spec:       item.Spec,
	})
}

func resourceFromPetDef(item apitypes.PetDef) (apitypes.Resource, error) {
	return marshalResource(apitypes.PetDefResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.PetDefResourceKind(apitypes.ResourceKindPetDef),
		Metadata:   apitypes.ResourceMetadata{Name: item.Id},
		Spec:       item.Spec,
	})
}

func resourceFromBadgeDef(item apitypes.BadgeDef) (apitypes.Resource, error) {
	return marshalResource(apitypes.BadgeDefResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.BadgeDefResourceKind(apitypes.ResourceKindBadgeDef),
		Metadata:   apitypes.ResourceMetadata{Name: item.Id},
		Spec:       item.Spec,
	})
}

func resourceFromGameDef(item apitypes.GameDef) (apitypes.Resource, error) {
	return marshalResource(apitypes.GameDefResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.GameDefResourceKind(apitypes.ResourceKindGameDef),
		Metadata:   apitypes.ResourceMetadata{Name: item.Id},
		Spec:       item.Spec,
	})
}
