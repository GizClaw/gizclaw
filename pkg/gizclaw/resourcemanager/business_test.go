package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/badge"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/petspecies"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestApplyPetSpeciesCreatesUpdatesAndDeletesResource(t *testing.T) {
	manager := newBusinessResourceManager(t)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "PetSpecies",
		"metadata": {"name": "rabbit"},
		"spec": {"name": "Rabbit", "pixa_path": "rabbit.pixa"}
	}`))
	if err != nil {
		t.Fatalf("Apply(create) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create action = %q, want created", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "PetSpecies",
		"metadata": {"name": "rabbit"},
		"spec": {"name": "Rabbit", "pixa_path": "rabbit.pixa"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "PetSpecies",
		"metadata": {"name": "rabbit"},
		"spec": {"name": "Lucky Rabbit", "pixa_path": "rabbit-v2.pixa"}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	species, err := resource.AsPetSpeciesResource()
	if err != nil {
		t.Fatalf("AsPetSpeciesResource returned error: %v", err)
	}
	if species.Spec.Name != "Lucky Rabbit" || species.Spec.PixaPath == nil || *species.Spec.PixaPath != "rabbit-v2.pixa" {
		t.Fatalf("species spec = %+v", species.Spec)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindPetSpecies, "rabbit")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedSpecies, err := deleted.AsPetSpeciesResource()
	if err != nil {
		t.Fatalf("deleted AsPetSpeciesResource returned error: %v", err)
	}
	if deletedSpecies.Metadata.Name != "rabbit" {
		t.Fatalf("deleted metadata.name = %q, want rabbit", deletedSpecies.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindPetSpecies, "rabbit")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func TestApplyBadgeCreatesUpdatesAndDeletesResource(t *testing.T) {
	manager := newBusinessResourceManager(t)

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Badge",
		"metadata": {"name": "founder"},
		"spec": {"name": "Founder", "description": "early user", "icon_path": "founder/icon"}
	}`))
	if err != nil {
		t.Fatalf("Apply(create) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("create action = %q, want created", result.Action)
	}

	result, err = manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Badge",
		"metadata": {"name": "founder"},
		"spec": {"name": "Founder", "description": "early user", "icon_path": "founder/icon"}
	}`))
	if err != nil {
		t.Fatalf("Apply(unchanged) returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("unchanged action = %q, want unchanged", result.Action)
	}

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Badge",
		"metadata": {"name": "founder"},
		"spec": {"name": "Founder+", "description": "updated", "icon_path": "founder/v2"}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	badgeResource, err := resource.AsBadgeResource()
	if err != nil {
		t.Fatalf("AsBadgeResource returned error: %v", err)
	}
	if badgeResource.Spec.Name != "Founder+" || badgeResource.Spec.IconPath == nil || *badgeResource.Spec.IconPath != "founder/v2" {
		t.Fatalf("badge spec = %+v", badgeResource.Spec)
	}

	deleted, err := manager.Delete(context.Background(), apitypes.ResourceKindBadge, "founder")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	deletedBadge, err := deleted.AsBadgeResource()
	if err != nil {
		t.Fatalf("deleted AsBadgeResource returned error: %v", err)
	}
	if deletedBadge.Metadata.Name != "founder" {
		t.Fatalf("deleted metadata.name = %q, want founder", deletedBadge.Metadata.Name)
	}
	_, err = manager.Get(context.Background(), apitypes.ResourceKindBadge, "founder")
	assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
}

func newBusinessResourceManager(t *testing.T) *Manager {
	t.Helper()

	return New(Services{
		Badges: &badge.Server{
			Store:  kv.NewMemory(nil),
			Assets: objectstore.Dir(t.TempDir()),
		},
		PetSpecies: &petspecies.Server{
			Store:  kv.NewMemory(nil),
			Assets: objectstore.Dir(t.TempDir()),
		},
	})
}
