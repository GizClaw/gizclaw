package resourcemanager

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func (m *Manager) applyPetSpecies(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.PetSpecies == nil {
		return apitypes.ApplyResult{}, missingService("pet species")
	}
	item, err := resource.AsPetSpeciesResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_PET_SPECIES_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	existing, exists, err := m.getPetSpecies(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(petSpeciesSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindPetSpecies, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.PetSpecies.Put(ctx, item.Metadata.Name, item.Spec); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindPetSpecies, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindPetSpecies, item.Metadata.Name), nil
}

func (m *Manager) applyBadge(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Badges == nil {
		return apitypes.ApplyResult{}, missingService("badges")
	}
	item, err := resource.AsBadgeResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_BADGE_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	existing, exists, err := m.getBadge(ctx, item.Metadata.Name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(badgeSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindBadge, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.Badges.Put(ctx, item.Metadata.Name, item.Spec); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindBadge, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindBadge, item.Metadata.Name), nil
}

func (m *Manager) getPetSpecies(ctx context.Context, id string) (apitypes.PetSpecies, bool, error) {
	item, err := m.services.PetSpecies.Get(ctx, id)
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.PetSpecies{}, false, nil
	}
	if err != nil {
		return apitypes.PetSpecies{}, false, err
	}
	return item, true, nil
}

func (m *Manager) deletePetSpecies(ctx context.Context, id string) (apitypes.PetSpecies, bool, error) {
	item, err := m.services.PetSpecies.Delete(ctx, id)
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.PetSpecies{}, false, nil
	}
	if err != nil {
		return apitypes.PetSpecies{}, false, err
	}
	return item, true, nil
}

func (m *Manager) getBadge(ctx context.Context, id string) (apitypes.Badge, bool, error) {
	item, err := m.services.Badges.Get(ctx, id)
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.Badge{}, false, nil
	}
	if err != nil {
		return apitypes.Badge{}, false, err
	}
	return item, true, nil
}

func (m *Manager) deleteBadge(ctx context.Context, id string) (apitypes.Badge, bool, error) {
	item, err := m.services.Badges.Delete(ctx, id)
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.Badge{}, false, nil
	}
	if err != nil {
		return apitypes.Badge{}, false, err
	}
	return item, true, nil
}

func petSpeciesSpec(item apitypes.PetSpecies) apitypes.PetSpeciesSpec {
	spec := apitypes.PetSpeciesSpec{Name: item.Name}
	if item.PixaPath != "" {
		spec.PixaPath = &item.PixaPath
	}
	return spec
}

func badgeSpec(item apitypes.Badge) apitypes.BadgeSpec {
	spec := apitypes.BadgeSpec{
		Description: item.Description,
		Name:        item.Name,
	}
	if item.IconPath != "" {
		spec.IconPath = &item.IconPath
	}
	return spec
}

func resourceFromPetSpecies(item apitypes.PetSpecies) (apitypes.Resource, error) {
	return marshalResource(apitypes.PetSpeciesResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.PetSpeciesResourceKind(apitypes.ResourceKindPetSpecies),
		Metadata:   apitypes.ResourceMetadata{Name: item.Id},
		Spec:       petSpeciesSpec(item),
	})
}

func resourceFromBadge(item apitypes.Badge) (apitypes.Resource, error) {
	return marshalResource(apitypes.BadgeResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.BadgeResourceKind(apitypes.ResourceKindBadge),
		Metadata:   apitypes.ResourceMetadata{Name: item.Id},
		Spec:       badgeSpec(item),
	})
}
