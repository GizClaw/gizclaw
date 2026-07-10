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
	desired := apitypes.ACLPolicy{
		Subject:  acl.PublicKeySubject(owner),
		Resource: apitypes.ACLResource{Kind: kind, Id: id},
		Role:     resourceOwnerRole,
	}
	existing, exists, err := m.ownedResourceOwnerBinding(ctx, kind, id)
	if err != nil {
		return false, err
	}
	roleChanged, err := m.ensureResourceOwnerRole(ctx)
	if err != nil {
		return false, err
	}
	if exists && ownerPolicyEqual(existing.Policy, desired) {
		return roleChanged, nil
	}
	_, err = m.services.ACL.PutPolicyBinding(ctx, resourceOwnerPolicyBindingID(kind, id), 0, desired)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) ensureResourceOwnerRole(ctx context.Context) (bool, error) {
	role, err := m.services.ACL.GetRole(ctx, resourceOwnerRole)
	if err == nil && permissionListsEqual(role.Permissions, resourceOwnerPermissions) {
		return false, nil
	}
	if err != nil && !errors.Is(err, acl.ErrRoleNotFound) {
		return false, err
	}
	if _, err := m.services.ACL.PutRole(ctx, resourceOwnerRole, resourceOwnerPermissions); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) ownedResourceOwner(ctx context.Context, kind apitypes.ACLResourceKind, id string) (*string, error) {
	binding, exists, err := m.ownedResourceOwnerBinding(ctx, kind, id)
	if err != nil || !exists {
		return nil, err
	}
	if binding.Policy.Subject.Kind == acl.SubjectKindPublicKey && strings.TrimSpace(binding.Policy.Subject.Id) != "" {
		owner := binding.Policy.Subject.Id
		return &owner, nil
	}
	return nil, nil
}

func (m *Manager) ownedResourceOwnerBinding(ctx context.Context, kind apitypes.ACLResourceKind, id string) (apitypes.ACLPolicyBinding, bool, error) {
	if m.services.ACL == nil {
		return apitypes.ACLPolicyBinding{}, false, nil
	}
	binding, err := m.services.ACL.GetPolicyBinding(ctx, resourceOwnerPolicyBindingID(kind, id))
	if errors.Is(err, acl.ErrPolicyBindingNotFound) {
		return apitypes.ACLPolicyBinding{}, false, nil
	}
	if err != nil {
		return apitypes.ACLPolicyBinding{}, false, err
	}
	return binding, true, nil
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
	ids := append([]string{}, extraBindingIDs...)
	ids = append(ids, resourceOwnerPolicyBindingID(kind, id))
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

func ownerPolicyEqual(left, right apitypes.ACLPolicy) bool {
	return left.Subject == right.Subject && left.Resource == right.Resource && left.Role == right.Role
}

func permissionListsEqual(left, right apitypes.ACLPermissionList) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[apitypes.ACLPermission]int, len(left))
	for _, permission := range left {
		seen[permission]++
	}
	for _, permission := range right {
		if seen[permission] == 0 {
			return false
		}
		seen[permission]--
	}
	return true
}

type ownedResourceOwnerRollback struct {
	kind     apitypes.ACLResourceKind
	id       string
	previous *string
	changed  bool
}

func (m *Manager) ensureOwnedResourceOwnerBeforeWrite(ctx context.Context, kind apitypes.ACLResourceKind, id string, metadata apitypes.ResourceMetadata) (*ownedResourceOwnerRollback, error) {
	owner, hasOwner, err := resourceOwnerHint(metadata)
	if err != nil || !hasOwner {
		return nil, err
	}
	if m.services.ACL == nil {
		return nil, missingService("acl")
	}
	previous, err := m.ownedResourceOwner(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	changed, err := m.putOwnedResourceOwner(ctx, kind, id, owner)
	if err != nil {
		return nil, err
	}
	return &ownedResourceOwnerRollback{kind: kind, id: id, previous: previous, changed: changed}, nil
}

func (m *Manager) removeOwnedResourceOwnerBeforeDelete(ctx context.Context, kind apitypes.ACLResourceKind, id string, extraBindingIDs ...string) (*ownedResourceOwnerRollback, error) {
	if m.services.ACL == nil {
		return nil, nil
	}
	previous, err := m.ownedResourceOwner(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if err := m.removeOwnedResourceOwner(context.WithoutCancel(ctx), kind, id, extraBindingIDs...); err != nil {
		return nil, err
	}
	return &ownedResourceOwnerRollback{kind: kind, id: id, previous: previous, changed: previous != nil}, nil
}

func (m *Manager) rollbackOwnedResourceOwner(ctx context.Context, rollback *ownedResourceOwnerRollback, cause error) error {
	if rollback == nil || !rollback.changed {
		return cause
	}
	var err error
	if rollback.previous == nil {
		err = m.removeOwnedResourceOwner(context.WithoutCancel(ctx), rollback.kind, rollback.id)
	} else {
		_, err = m.putOwnedResourceOwner(context.WithoutCancel(ctx), rollback.kind, rollback.id, *rollback.previous)
	}
	if err != nil {
		return applyError(500, "RESOURCE_OWNER_ACL_ROLLBACK_FAILED", fmt.Sprintf("%v; rollback failed: %v", cause, err))
	}
	return cause
}
