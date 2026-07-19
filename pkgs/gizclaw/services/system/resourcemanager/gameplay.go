package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

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
		same, err := semanticEqual(
			struct {
				Spec apitypes.GameDefSpec `json:"spec"`
			}{Spec: existing.Spec},
			struct {
				Spec apitypes.GameDefSpec `json:"spec"`
			}{Spec: item.Spec},
		)
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

func (m *Manager) getPetDef(ctx context.Context, id string) (apitypes.PetDef, bool, error) {
	if m.services.GameplayCatalog == nil {
		return apitypes.PetDef{}, false, missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.GetPetDef(ctx, adminhttp.GetPetDefRequestObject{Id: id})
	if err != nil {
		return apitypes.PetDef{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetPetDef200JSONResponse:
		return apitypes.PetDef(response), true, nil
	case adminhttp.GetPetDef404JSONResponse:
		return apitypes.PetDef{}, false, nil
	case adminhttp.GetPetDef500JSONResponse:
		return apitypes.PetDef{}, false, responseError(500, "GET_PET_DEF_FAILED", "failed to get pet def", response)
	default:
		return apitypes.PetDef{}, false, unexpectedResponse("GetPetDef", response)
	}
}

func (m *Manager) putPetDef(ctx context.Context, id string, body adminhttp.PetDefUpsert) error {
	if m.services.GameplayCatalog == nil {
		return missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.PutPetDef(ctx, adminhttp.PutPetDefRequestObject{Id: id, Body: &body})
	return putGameplayResponse("PutPetDef", response, err)
}

func (m *Manager) deletePetDef(ctx context.Context, id string) (apitypes.PetDef, bool, error) {
	response, err := m.services.GameplayCatalog.DeletePetDef(ctx, adminhttp.DeletePetDefRequestObject{Id: id})
	if err != nil {
		return apitypes.PetDef{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeletePetDef200JSONResponse:
		return apitypes.PetDef(response), true, nil
	case adminhttp.DeletePetDef404JSONResponse:
		return apitypes.PetDef{}, false, nil
	case adminhttp.DeletePetDef500JSONResponse:
		return apitypes.PetDef{}, false, responseError(500, "DELETE_PET_DEF_FAILED", "failed to delete pet def", response)
	default:
		return apitypes.PetDef{}, false, unexpectedResponse("DeletePetDef", response)
	}
}

func (m *Manager) getBadgeDef(ctx context.Context, id string) (apitypes.BadgeDef, bool, error) {
	if m.services.GameplayCatalog == nil {
		return apitypes.BadgeDef{}, false, missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.GetBadgeDef(ctx, adminhttp.GetBadgeDefRequestObject{Id: id})
	if err != nil {
		return apitypes.BadgeDef{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetBadgeDef200JSONResponse:
		return apitypes.BadgeDef(response), true, nil
	case adminhttp.GetBadgeDef404JSONResponse:
		return apitypes.BadgeDef{}, false, nil
	case adminhttp.GetBadgeDef500JSONResponse:
		return apitypes.BadgeDef{}, false, responseError(500, "GET_BADGE_DEF_FAILED", "failed to get badge def", response)
	default:
		return apitypes.BadgeDef{}, false, unexpectedResponse("GetBadgeDef", response)
	}
}

func (m *Manager) putBadgeDef(ctx context.Context, id string, body adminhttp.BadgeDefUpsert) error {
	if m.services.GameplayCatalog == nil {
		return missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.PutBadgeDef(ctx, adminhttp.PutBadgeDefRequestObject{Id: id, Body: &body})
	return putGameplayResponse("PutBadgeDef", response, err)
}

func (m *Manager) deleteBadgeDef(ctx context.Context, id string) (apitypes.BadgeDef, bool, error) {
	response, err := m.services.GameplayCatalog.DeleteBadgeDef(ctx, adminhttp.DeleteBadgeDefRequestObject{Id: id})
	if err != nil {
		return apitypes.BadgeDef{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteBadgeDef200JSONResponse:
		return apitypes.BadgeDef(response), true, nil
	case adminhttp.DeleteBadgeDef404JSONResponse:
		return apitypes.BadgeDef{}, false, nil
	case adminhttp.DeleteBadgeDef500JSONResponse:
		return apitypes.BadgeDef{}, false, responseError(500, "DELETE_BADGE_DEF_FAILED", "failed to delete badge def", response)
	default:
		return apitypes.BadgeDef{}, false, unexpectedResponse("DeleteBadgeDef", response)
	}
}

func (m *Manager) getGameDef(ctx context.Context, id string) (apitypes.GameDef, bool, error) {
	if m.services.GameplayCatalog == nil {
		return apitypes.GameDef{}, false, missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.GetGameDef(ctx, adminhttp.GetGameDefRequestObject{Id: id})
	if err != nil {
		return apitypes.GameDef{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.GetGameDef200JSONResponse:
		return apitypes.GameDef(response), true, nil
	case adminhttp.GetGameDef404JSONResponse:
		return apitypes.GameDef{}, false, nil
	case adminhttp.GetGameDef500JSONResponse:
		return apitypes.GameDef{}, false, responseError(500, "GET_GAME_DEF_FAILED", "failed to get game def", response)
	default:
		return apitypes.GameDef{}, false, unexpectedResponse("GetGameDef", response)
	}
}

func (m *Manager) putGameDef(ctx context.Context, id string, body adminhttp.GameDefUpsert) error {
	if m.services.GameplayCatalog == nil {
		return missingService("gameplay catalog")
	}
	response, err := m.services.GameplayCatalog.PutGameDef(ctx, adminhttp.PutGameDefRequestObject{Id: id, Body: &body})
	return putGameplayResponse("PutGameDef", response, err)
}

func (m *Manager) deleteGameDef(ctx context.Context, id string) (apitypes.GameDef, bool, error) {
	response, err := m.services.GameplayCatalog.DeleteGameDef(ctx, adminhttp.DeleteGameDefRequestObject{Id: id})
	if err != nil {
		return apitypes.GameDef{}, false, err
	}
	switch response := response.(type) {
	case adminhttp.DeleteGameDef200JSONResponse:
		return apitypes.GameDef(response), true, nil
	case adminhttp.DeleteGameDef404JSONResponse:
		return apitypes.GameDef{}, false, nil
	case adminhttp.DeleteGameDef500JSONResponse:
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
	case adminhttp.PutPetDef200JSONResponse,
		adminhttp.PutBadgeDef200JSONResponse,
		adminhttp.PutGameDef200JSONResponse:
		return nil
	case adminhttp.PutPetDef400JSONResponse:
		return responseError(400, "PUT_PET_DEF_FAILED", "failed to put pet def", response)
	case adminhttp.PutBadgeDef400JSONResponse:
		return responseError(400, "PUT_BADGE_DEF_FAILED", "failed to put badge def", response)
	case adminhttp.PutGameDef400JSONResponse:
		return responseError(400, "PUT_GAME_DEF_FAILED", "failed to put game def", response)
	case adminhttp.PutPetDef409JSONResponse:
		return responseError(409, "PUT_PET_DEF_FAILED", "failed to put pet def", response)
	case adminhttp.PutBadgeDef409JSONResponse:
		return responseError(409, "PUT_BADGE_DEF_FAILED", "failed to put badge def", response)
	case adminhttp.PutGameDef409JSONResponse:
		return responseError(409, "PUT_GAME_DEF_FAILED", "failed to put game def", response)
	case adminhttp.PutPetDef500JSONResponse:
		return responseError(500, "PUT_PET_DEF_FAILED", "failed to put pet def", response)
	case adminhttp.PutBadgeDef500JSONResponse:
		return responseError(500, "PUT_BADGE_DEF_FAILED", "failed to put badge def", response)
	case adminhttp.PutGameDef500JSONResponse:
		return responseError(500, "PUT_GAME_DEF_FAILED", "failed to put game def", response)
	default:
		return unexpectedResponse(operation, response)
	}
}

func petDefUpsert(resource apitypes.PetDefResource) adminhttp.PetDefUpsert {
	return adminhttp.PetDefUpsert{Id: resource.Metadata.Name, Spec: resource.Spec, I18n: resource.I18n}
}

func badgeDefUpsert(resource apitypes.BadgeDefResource) adminhttp.BadgeDefUpsert {
	return adminhttp.BadgeDefUpsert{Id: resource.Metadata.Name, Spec: resource.Spec}
}

func gameDefUpsert(resource apitypes.GameDefResource) adminhttp.GameDefUpsert {
	return adminhttp.GameDefUpsert{Id: resource.Metadata.Name, Spec: resource.Spec}
}

func resourceFromPetDef(item apitypes.PetDef) (apitypes.Resource, error) {
	i18n := item.I18n
	return marshalResource(apitypes.PetDefResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.PetDefResourceKind(apitypes.ResourceKindPetDef),
		Metadata:   apitypes.ResourceMetadata{Name: item.Id},
		Spec:       item.Spec,
		I18n:       &i18n,
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
		Icon:       item.Icon,
		Spec:       item.Spec,
	})
}
