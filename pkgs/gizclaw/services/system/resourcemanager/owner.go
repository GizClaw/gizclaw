package resourcemanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

const resourceOwnerRole = "resource-owner"

var resourceOwnerPermissions = apitypes.ACLPermissionList{
	apitypes.ACLPermissionRead,
	apitypes.ACLPermissionUse,
	apitypes.ACLPermissionAdmin,
}

func resourceOwnerPolicyBindingID(kind apitypes.ACLResourceKind, id string) string {
	return "resource-owner:" + url.PathEscape(string(kind)) + ":" + url.PathEscape(id)
}

func resourceOwnerHint(metadata apitypes.ResourceMetadata) (string, bool, error) {
	if metadata.OwnerPublicKey == nil {
		return "", false, nil
	}
	owner := strings.TrimSpace(*metadata.OwnerPublicKey)
	if owner == "" {
		return "", false, applyError(400, "INVALID_RESOURCE_OWNER", "metadata.owner_public_key must not be empty")
	}
	if strings.Contains(owner, ":") {
		return "", false, applyError(400, "INVALID_RESOURCE_OWNER", "metadata.owner_public_key must not contain ':'")
	}
	return owner, true, nil
}

func (m *Manager) validateOwnedResourceOwner(kind apitypes.ACLResourceKind, id string, metadata apitypes.ResourceMetadata, exists bool) error {
	_, hasOwner, err := resourceOwnerHint(metadata)
	if err != nil {
		return err
	}
	if m.services.ACL == nil {
		if hasOwner {
			return missingService("acl")
		}
		return nil
	}
	if !exists && !hasOwner {
		return applyError(400, "RESOURCE_OWNER_REQUIRED", fmt.Sprintf("metadata.owner_public_key is required for new %s resource %q", kind, id))
	}
	return nil
}

func (m *Manager) ensureOwnedResourceOwnerFromMetadata(ctx context.Context, kind apitypes.ACLResourceKind, id string, metadata apitypes.ResourceMetadata) (bool, error) {
	owner, hasOwner, err := resourceOwnerHint(metadata)
	if err != nil || !hasOwner {
		return false, err
	}
	if m.services.ACL == nil {
		return false, missingService("acl")
	}
	return m.putOwnedResourceOwner(ctx, kind, id, owner)
}

func (m *Manager) putOwnedResourceOwner(ctx context.Context, kind apitypes.ACLResourceKind, id, owner string) (bool, error) {
	existing, err := m.ownedResourceOwner(ctx, kind, id)
	if err != nil {
		return false, err
	}
	if existing != nil && *existing == owner {
		return false, nil
	}
	if err := m.ensureResourceOwnerRole(ctx); err != nil {
		return false, err
	}
	_, err = m.services.ACL.PutPolicyBinding(ctx, resourceOwnerPolicyBindingID(kind, id), 0, apitypes.ACLPolicy{
		Subject:  acl.PublicKeySubject(owner),
		Resource: apitypes.ACLResource{Kind: kind, Id: id},
		Role:     resourceOwnerRole,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) ensureResourceOwnerRole(ctx context.Context) error {
	if _, err := m.services.ACL.PutRole(ctx, resourceOwnerRole, resourceOwnerPermissions); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ownedResourceOwner(ctx context.Context, kind apitypes.ACLResourceKind, id string) (*string, error) {
	if m.services.ACL == nil {
		return nil, nil
	}
	bindings, _, _, err := m.services.ACL.ListPolicyBindings(ctx, acl.ListPolicyBindingsRequest{
		ResourceKind: kind,
		ResourceID:   id,
		Role:         resourceOwnerRole,
	})
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if binding.Policy.Subject.Kind == acl.SubjectKindPublicKey && strings.TrimSpace(binding.Policy.Subject.Id) != "" {
			owner := binding.Policy.Subject.Id
			return &owner, nil
		}
	}
	return nil, nil
}

func (m *Manager) withOwnedResourceOwner(ctx context.Context, kind apitypes.ACLResourceKind, id string, resource apitypes.Resource) (apitypes.Resource, error) {
	owner, err := m.ownedResourceOwner(ctx, kind, id)
	if err != nil || owner == nil {
		return resource, err
	}
	return withOwnerMetadata(resource, *owner)
}

func withOwnerMetadata(resource apitypes.Resource, owner string) (apitypes.Resource, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return apitypes.Resource{}, err
	}
	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		return apitypes.Resource{}, err
	}
	metadata, _ := body["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["owner_public_key"] = owner
	body["metadata"] = metadata
	data, err = json.Marshal(body)
	if err != nil {
		return apitypes.Resource{}, err
	}
	var out apitypes.Resource
	if err := json.Unmarshal(data, &out); err != nil {
		return apitypes.Resource{}, err
	}
	return out, nil
}

func (m *Manager) removeOwnedResourceOwner(ctx context.Context, kind apitypes.ACLResourceKind, id string, extraBindingIDs ...string) error {
	if m.services.ACL == nil {
		return nil
	}
	ids := append([]string{resourceOwnerPolicyBindingID(kind, id)}, extraBindingIDs...)
	for _, bindingID := range ids {
		if strings.TrimSpace(bindingID) == "" {
			continue
		}
		_, err := m.services.ACL.DeletePolicyBinding(ctx, bindingID)
		if err == nil || errors.Is(err, acl.ErrPolicyBindingNotFound) {
			continue
		}
		return err
	}
	return nil
}
